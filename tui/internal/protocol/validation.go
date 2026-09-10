package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type Contract string

const (
	ContractEnvelope          Contract = "message"
	ContractEmpty             Contract = "empty"
	ContractProtocolInfo      Contract = "protocol-info"
	ContractInspectionRequest Contract = "inspection-request"
	ContractInspectionResult  Contract = "inspection-result"
	ContractProposal          Contract = "proposal"
	ContractSetting           Contract = "setting"
	ContractRevisionRequest   Contract = "revision-request"
	ContractRevisionResult    Contract = "revision-result"
	ContractValidationResult  Contract = "validation-result"
	ContractFinalRequest      Contract = "final-request"
	ContractFinalResult       Contract = "final-result"
	ContractApplyRequest      Contract = "apply-request"
	ContractApplyResult       Contract = "apply-result"
	ContractAuditEvent        Contract = "audit-event"
	ContractValidatorRequest  Contract = "validator-request"
	ContractValidatorResult   Contract = "validator-result"
)

var (
	digestPattern     = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	errorCodePattern  = regexp.MustCompile(`^[a-z]+(\.[a-z0-9_]+)+$`)
	dottedCodePattern = regexp.MustCompile(`^[a-z]+(\.[a-z0-9_]+)*$`)
)

// ValidateContract applies the semantic checks documented by the named v1
// process contract. expectedDigest is used to bind external-validator results.
func ValidateContract(contract Contract, data []byte, expectedDigest string) error {
	switch contract {
	case ContractEnvelope:
		return validateEnvelope(data)
	case ContractEmpty:
		var value struct{}
		return decodeStrict(bytes.NewReader(data), MaxMessageBytes, &value)
	case ContractProtocolInfo:
		var value ProtocolInfo
		if err := decodeStrict(bytes.NewReader(data), MaxMessageBytes, &value); err != nil {
			return err
		}
		if len(value.ProtocolVersions) == 0 || len(value.Operations) == 0 || len(value.Schemas) == 0 || value.Limits.MessageBytes < 1 {
			return errors.New("invalid protocol capabilities")
		}
		return nil
	case ContractInspectionRequest:
		var value InspectSource
		if err := decodeStrict(bytes.NewReader(data), MaxMessageBytes, &value); err != nil {
			return err
		}
		if value.SourcePath == "" {
			return errors.New("missing source path")
		}
		return nil
	case ContractInspectionResult:
		return validateInspection(data)
	case ContractProposal:
		return validateProposal(data)
	case ContractSetting:
		return validateSetting(data)
	case ContractRevisionRequest:
		return validateRevision(data, true)
	case ContractRevisionResult:
		return validateRevision(data, false)
	case ContractValidationResult:
		return validateChecksDocument(data)
	case ContractFinalRequest:
		return validateFinalRequest(data)
	case ContractFinalResult:
		return validateFinalResult(data)
	case ContractApplyRequest:
		return validateApplyRequest(data)
	case ContractApplyResult:
		return validateApplyResult(data)
	case ContractAuditEvent:
		return validateAudit(data)
	case ContractValidatorRequest:
		var value ExternalValidatorRequest
		if err := decodeStrict(bytes.NewReader(data), MaxMessageBytes, &value); err != nil {
			return err
		}
		if value.CandidatePath == "" || !validDigest(value.CandidateDigest) || !validDigest(value.SourceDigest) {
			return errors.New("invalid validator request")
		}
		return nil
	case ContractValidatorResult:
		return validateValidatorResult(data, expectedDigest)
	default:
		return fmt.Errorf("unknown contract %q", contract)
	}
}

func validateEnvelope(data []byte) error {
	var shape map[string]json.RawMessage
	if err := decodeStrict(bytes.NewReader(data), MaxMessageBytes, &shape); err != nil {
		return err
	}
	if _, ok := shape["operation"]; ok {
		_, err := DecodeRequest(bytes.NewReader(data), MaxMessageBytes)
		return err
	}
	var response Response
	if err := decodeStrict(bytes.NewReader(data), MaxMessageBytes, &response); err != nil {
		return err
	}
	if response.RequestID == "" {
		return errors.New("missing request id")
	}
	if _, err := DecodeResponse(bytes.NewReader(data), MaxMessageBytes, response.RequestID); err != nil {
		return err
	}
	if !response.OK && !errorCodePattern.MatchString(response.Error.Code) {
		return errors.New("invalid error code")
	}
	return nil
}

type proposalDocument struct {
	ProposalID   string         `json:"proposal_id"`
	Revision     int            `json:"revision"`
	SourceDigest string         `json:"source_digest"`
	CreatedAt    string         `json:"created_at"`
	Producer     string         `json:"producer,omitempty"`
	Items        []proposalItem `json:"items"`
}
type proposalItem struct {
	ChangeID      string          `json:"change_id"`
	Path          string          `json:"path"`
	Operation     string          `json:"operation"`
	ExpectedOld   json.RawMessage `json:"expected_old,omitempty"`
	ProposedValue json.RawMessage `json:"proposed_value,omitempty"`
	Explanation   string          `json:"explanation"`
}

func validateProposal(data []byte) error {
	var value proposalDocument
	if err := decodeStrict(bytes.NewReader(data), MaxMessageBytes, &value); err != nil {
		return err
	}
	return value.validate()
}
func (p proposalDocument) validate() error {
	if p.ProposalID == "" || p.Revision < 1 || !validDigest(p.SourceDigest) || len(p.Items) < 1 || len(p.Items) > 1000 {
		return errors.New("invalid proposal")
	}
	if _, err := time.Parse(time.RFC3339, p.CreatedAt); err != nil {
		return errors.New("invalid proposal timestamp")
	}
	ids := map[string]bool{}
	paths := map[string]bool{}
	for _, item := range p.Items {
		if item.ChangeID == "" || ids[item.ChangeID] || !validPointer(item.Path) || item.Explanation == "" {
			return errors.New("invalid change item")
		}
		ids[item.ChangeID] = true
		for path := range paths {
			if path == item.Path || isAncestor(path, item.Path) || isAncestor(item.Path, path) {
				return errors.New("overlapping change targets")
			}
		}
		paths[item.Path] = true
		old := item.ExpectedOld != nil
		proposed := item.ProposedValue != nil
		switch item.Operation {
		case "add":
			if old || !proposed {
				return errors.New("invalid add")
			}
		case "replace":
			if !old || !proposed {
				return errors.New("invalid replace")
			}
		case "remove":
			if !old || proposed {
				return errors.New("invalid remove")
			}
		default:
			return errors.New("invalid operation")
		}
	}
	return nil
}
func validPointer(value string) bool {
	if value == "" {
		return true
	}
	if !strings.HasPrefix(value, "/") {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] == '~' && (i+1 >= len(value) || (value[i+1] != '0' && value[i+1] != '1')) {
			return false
		}
	}
	return true
}
func isAncestor(parent, child string) bool {
	return parent != "" && strings.HasPrefix(child, parent+"/")
}

type settingDocument struct {
	Path        string          `json:"path"`
	Value       json.RawMessage `json:"value"`
	Sensitivity string          `json:"sensitivity"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
}
type visibleValue struct {
	Visibility  string          `json:"visibility"`
	Value       json.RawMessage `json:"value,omitempty"`
	SecretRef   string          `json:"secret_ref,omitempty"`
	ValueType   string          `json:"value_type,omitempty"`
	Fingerprint string          `json:"fingerprint,omitempty"`
}

func validateSetting(data []byte) error {
	var value settingDocument
	if err := decodeStrict(bytes.NewReader(data), MaxMessageBytes, &value); err != nil {
		return err
	}
	if !validPointer(value.Path) || len(value.Value) == 0 {
		return errors.New("invalid setting")
	}
	if value.Sensitivity != "normal" && value.Sensitivity != "schema_sensitive" && value.Sensitivity != "suspected_sensitive" {
		return errors.New("invalid sensitivity")
	}
	var visible visibleValue
	if err := decodeStrict(bytes.NewReader(value.Value), MaxMessageBytes, &visible); err != nil {
		return err
	}
	switch visible.Visibility {
	case "plain":
		if visible.Value == nil || visible.SecretRef != "" || visible.ValueType != "" || visible.Fingerprint != "" {
			return errors.New("invalid plain value")
		}
	case "redacted":
		r := RedactedValue{Visibility: VisibilityRedacted, SecretRef: visible.SecretRef, ValueType: ValueType(visible.ValueType), Fingerprint: visible.Fingerprint}
		if visible.Value != nil {
			return errors.New("redacted value contains raw value")
		}
		return r.Validate()
	default:
		return errors.New("invalid visibility")
	}
	return nil
}

func validateInspection(data []byte) error {
	var value InspectionResult
	if err := decodeStrict(bytes.NewReader(data), MaxMessageBytes, &value); err != nil {
		return err
	}
	if !validDigest(value.SourceDigest) || value.ByteLength < 0 || value.ByteLength > 10<<20 || value.NodeCount < 1 || value.NodeCount > 100000 {
		return errors.New("invalid inspection")
	}
	for _, setting := range value.Settings {
		encoded, _ := json.Marshal(setting)
		if err := validateSetting(encoded); err != nil {
			return err
		}
	}
	return nil
}

type revisionRequest struct {
	ProposalID        string            `json:"proposal_id"`
	BaseRevision      int               `json:"base_revision"`
	SourceDigest      string            `json:"source_digest"`
	Allowed           []string          `json:"allowed_change_item_ids"`
	Comments          []revisionComment `json:"comments"`
	Proposal          proposalDocument  `json:"proposal"`
	SourceContext     json.RawMessage   `json:"source_context,omitempty"`
	AuthorizedSecrets json.RawMessage   `json:"authorized_secrets,omitempty"`
}
type revisionResult struct {
	ProposalID   string           `json:"proposal_id"`
	BaseRevision int              `json:"base_revision"`
	SourceDigest string           `json:"source_digest"`
	Proposal     proposalDocument `json:"proposal"`
	Resolved     []string         `json:"resolved_comment_ids"`
}
type revisionComment struct {
	CommentID string `json:"comment_id"`
	ChangeID  string `json:"change_id"`
	Body      string `json:"body"`
}

func validateRevision(data []byte, request bool) error {
	if request {
		var v revisionRequest
		if err := decodeStrict(bytes.NewReader(data), MaxMessageBytes, &v); err != nil {
			return err
		}
		if v.ProposalID == "" || v.BaseRevision < 1 || !validDigest(v.SourceDigest) || len(v.Allowed) < 1 || len(v.Comments) < 1 {
			return errors.New("invalid revision request")
		}
		if duplicate(v.Allowed) {
			return errors.New("duplicate allowed id")
		}
		for _, c := range v.Comments {
			if c.CommentID == "" || c.ChangeID == "" || c.Body == "" || len(c.Body) > 65536 {
				return errors.New("invalid comment")
			}
		}
		return v.Proposal.validate()
	}
	var v revisionResult
	if err := decodeStrict(bytes.NewReader(data), MaxMessageBytes, &v); err != nil {
		return err
	}
	if v.ProposalID == "" || v.BaseRevision < 1 || !validDigest(v.SourceDigest) || duplicate(v.Resolved) {
		return errors.New("invalid revision result")
	}
	return v.Proposal.validate()
}

func validateChecksDocument(data []byte) error {
	var value ValidationResult
	if err := decodeStrict(bytes.NewReader(data), MaxMessageBytes, &value); err != nil {
		return err
	}
	if !validDigest(value.SubjectDigest) {
		return errors.New("invalid subject digest")
	}
	return validateChecks(value.Checks)
}
func validateChecks(checks []CheckResult) error {
	for _, check := range checks {
		if check.Kind == "" || check.Code == "" || len(check.Message) > 4096 {
			return errors.New("invalid check")
		}
		if check.Status != "passed" && check.Status != "failed" && check.Status != "unverified" {
			return errors.New("invalid check status")
		}
	}
	return nil
}

type finalRequest struct {
	SourcePath     string             `json:"source_path"`
	SourceDigest   string             `json:"source_digest"`
	Proposal       proposalDocument   `json:"proposal"`
	Decisions      map[string]string  `json:"decisions"`
	Comments       []json.RawMessage  `json:"comments"`
	ExternalChecks []ValidationResult `json:"external_checks"`
}
type finalResult struct {
	FinalChangeDigest string        `json:"final_change_digest"`
	SourceDigest      string        `json:"source_digest"`
	Approved          []string      `json:"approved_change_item_ids"`
	Diff              string        `json:"diff"`
	Checks            []CheckResult `json:"checks"`
	Nonce             string        `json:"confirmation_nonce"`
}

func validateFinalRequest(data []byte) error {
	var v finalRequest
	if err := decodeStrict(bytes.NewReader(data), MaxMessageBytes, &v); err != nil {
		return err
	}
	if v.SourcePath == "" || !validDigest(v.SourceDigest) {
		return errors.New("invalid final request")
	}
	if err := v.Proposal.validate(); err != nil {
		return err
	}
	for _, decision := range v.Decisions {
		if decision != "pending" && decision != "approved" && decision != "rejected" {
			return errors.New("invalid decision")
		}
	}
	for _, check := range v.ExternalChecks {
		if !validDigest(check.SubjectDigest) || validateChecks(check.Checks) != nil {
			return errors.New("invalid external check")
		}
	}
	return nil
}
func validateFinalResult(data []byte) error {
	var v finalResult
	if err := decodeStrict(bytes.NewReader(data), MaxMessageBytes, &v); err != nil {
		return err
	}
	if !validDigest(v.FinalChangeDigest) || !validDigest(v.SourceDigest) || len(v.Nonce) < 32 || duplicate(v.Approved) {
		return errors.New("invalid final result")
	}
	return validateChecks(v.Checks)
}

type applyResult struct {
	Before   string `json:"source_digest_before"`
	After    string `json:"source_digest_after"`
	Outcome  string `json:"outcome"`
	Recovery *struct {
		Available bool   `json:"available"`
		Path      string `json:"path,omitempty"`
	} `json:"recovery"`
}

func validateApplyRequest(data []byte) error {
	var v ApplyFinal
	if err := decodeStrict(bytes.NewReader(data), MaxMessageBytes, &v); err != nil {
		return err
	}
	if v.SourcePath == "" || !validDigest(v.SourceDigest) || !validDigest(v.FinalChangeDigest) || len(v.ConfirmationNonce) < 32 {
		return errors.New("invalid apply request")
	}
	return nil
}
func validateApplyResult(data []byte) error {
	var v applyResult
	if err := decodeStrict(bytes.NewReader(data), MaxMessageBytes, &v); err != nil {
		return err
	}
	if !validDigest(v.Before) || !validDigest(v.After) || v.Recovery == nil || (v.Outcome != "applied" && v.Outcome != "recovered" && v.Outcome != "unchanged") {
		return errors.New("invalid apply result")
	}
	return nil
}

type auditDocument struct {
	AuditSchema    string   `json:"audit_schema"`
	EventID        string   `json:"event_id"`
	SessionID      string   `json:"session_id"`
	Sequence       int      `json:"sequence"`
	OccurredAt     string   `json:"occurred_at"`
	ActorType      string   `json:"actor_type"`
	Action         string   `json:"action"`
	ChangeIDs      []string `json:"change_ids"`
	CommentIDs     []string `json:"comment_ids,omitempty"`
	SourceDigest   string   `json:"source_digest"`
	ProposalDigest string   `json:"proposal_digest,omitempty"`
	OutcomeCode    string   `json:"outcome_code,omitempty"`
	Previous       *string  `json:"previous_event_digest"`
	EventDigest    string   `json:"event_digest"`
}

func validateAudit(data []byte) error {
	var v auditDocument
	if err := decodeStrict(bytes.NewReader(data), MaxMessageBytes, &v); err != nil {
		return err
	}
	if v.AuditSchema != "zconfig.audit-event/1" || v.EventID == "" || v.SessionID == "" || v.Sequence < 1 || v.Sequence > 5000 || !validDigest(v.SourceDigest) || !validDigest(v.EventDigest) || duplicate(v.ChangeIDs) || duplicate(v.CommentIDs) {
		return errors.New("invalid audit event")
	}
	if _, err := time.Parse(time.RFC3339, v.OccurredAt); err != nil {
		return err
	}
	if v.Previous != nil && !validDigest(*v.Previous) {
		return errors.New("invalid previous digest")
	}
	if v.ProposalDigest != "" && !validDigest(v.ProposalDigest) {
		return errors.New("invalid proposal digest")
	}
	if v.OutcomeCode != "" && !dottedCodePattern.MatchString(v.OutcomeCode) {
		return errors.New("invalid outcome code")
	}
	return nil
}

func validateValidatorResult(data []byte, expected string) error {
	var v ExternalValidatorResult
	if err := decodeStrict(bytes.NewReader(data), MaxMessageBytes, &v); err != nil {
		return err
	}
	if !validDigest(v.CandidateDigest) || (expected != "" && v.CandidateDigest != expected) || v.Code == "" || len(v.Message) > 4096 {
		return errors.New("invalid validator result")
	}
	if v.Status != "passed" && v.Status != "failed" && v.Status != "unverified" {
		return errors.New("invalid validator status")
	}
	return nil
}
func validDigest(value string) bool { return digestPattern.MatchString(value) }
func duplicate(values []string) bool {
	seen := map[string]bool{}
	for _, v := range values {
		if seen[v] {
			return true
		}
		seen[v] = true
	}
	return false
}
