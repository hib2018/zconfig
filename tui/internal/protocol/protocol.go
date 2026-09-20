package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	Version         = "1.0"
	MaxMessageBytes = 16 << 20
)

type Operation string

const (
	OperationProtocolInfo     Operation = "protocol_info"
	OperationInspectSource    Operation = "inspect_source"
	OperationValidateProposal Operation = "validate_proposal"
	OperationValidateRevision Operation = "validate_revision"
	OperationAssembleFinal    Operation = "assemble_final"
	OperationApplyFinal       Operation = "apply_final"
	OperationReviseProposal   Operation = "revise_proposal"
)

type Request struct {
	ProtocolVersion string          `json:"protocol_version"`
	RequestID       string          `json:"request_id"`
	Operation       Operation       `json:"operation"`
	PayloadSchema   string          `json:"payload_schema"`
	Payload         json.RawMessage `json:"payload"`
}

type Response struct {
	ProtocolVersion string          `json:"protocol_version"`
	RequestID       string          `json:"request_id"`
	OK              bool            `json:"ok"`
	ResultSchema    string          `json:"result_schema,omitempty"`
	Result          json.RawMessage `json:"result,omitempty"`
	Error           *Error          `json:"error,omitempty"`
}

type Error struct {
	Code      string        `json:"code"`
	Message   string        `json:"message"`
	Retryable bool          `json:"retryable"`
	Details   []ErrorDetail `json:"details,omitempty"`
}
type ErrorDetail struct {
	Pointer string `json:"pointer,omitempty"`
	Reason  string `json:"reason"`
}

type ProtocolInfo struct {
	ProtocolVersions []string       `json:"protocol_versions"`
	Operations       []string       `json:"operations"`
	Schemas          []string       `json:"schemas"`
	Limits           ProtocolLimits `json:"limits"`
}
type ProtocolLimits struct {
	MessageBytes int `json:"message_bytes"`
}
type InspectSource struct {
	SourcePath string `json:"source_path"`
	SchemaPath string `json:"schema_path,omitempty"`
}
type ApplyFinal struct {
	SourcePath        string             `json:"source_path"`
	SchemaPath        string             `json:"schema_path,omitempty"`
	SourceDigest      string             `json:"source_digest"`
	FinalChangeDigest string             `json:"final_change_digest"`
	ConfirmationNonce string             `json:"confirmation_nonce"`
	Proposal          json.RawMessage    `json:"proposal"`
	Decisions         map[string]string  `json:"decisions"`
	Comments          []FinalComment     `json:"comments"`
	ExternalChecks    []ValidationResult `json:"external_checks"`
}
type FinalComment struct {
	CommentID string `json:"comment_id"`
	ChangeID  string `json:"change_id"`
	Status    string `json:"status"`
}
type InspectionResult struct {
	SourceDigest string    `json:"source_digest"`
	ByteLength   int64     `json:"byte_length"`
	NodeCount    int       `json:"node_count"`
	Settings     []Setting `json:"settings"`
}
type Setting struct {
	Path        string          `json:"path"`
	Value       json.RawMessage `json:"value"`
	Sensitivity string          `json:"sensitivity"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
}
type ValidationResult struct {
	SubjectDigest string        `json:"subject_digest"`
	Checks        []CheckResult `json:"checks"`
}
type CheckResult struct {
	Kind           string `json:"kind"`
	Status         string `json:"status"`
	Code           string `json:"code"`
	Message        string `json:"message"`
	Pointer        string `json:"pointer,omitempty"`
	EvidenceDigest string `json:"evidence_digest,omitempty"`
}
type ExternalValidatorRequest struct {
	CandidatePath   string `json:"candidate_path"`
	CandidateDigest string `json:"candidate_digest"`
	SourceDigest    string `json:"source_digest"`
}
type ExternalValidatorResult struct {
	CandidateDigest string          `json:"candidate_digest"`
	Status          string          `json:"status"`
	Code            string          `json:"code"`
	Message         string          `json:"message"`
	Details         json.RawMessage `json:"details,omitempty"`
}

type Visibility string

const (
	VisibilityPlain    Visibility = "plain"
	VisibilityRedacted Visibility = "redacted"
)

type ValueType string

const (
	ValueTypeString  ValueType = "string"
	ValueTypeNumber  ValueType = "number"
	ValueTypeInteger ValueType = "integer"
	ValueTypeBoolean ValueType = "boolean"
	ValueTypeNull    ValueType = "null"
	ValueTypeArray   ValueType = "array"
	ValueTypeObject  ValueType = "object"
)

type RedactedValue struct {
	Visibility  Visibility `json:"visibility"`
	SecretRef   string     `json:"secret_ref"`
	ValueType   ValueType  `json:"value_type"`
	Fingerprint string     `json:"fingerprint"`
}

func (v RedactedValue) Validate() error {
	if v.Visibility != VisibilityRedacted || v.SecretRef == "" {
		return errors.New("invalid redacted value")
	}
	if !regexp.MustCompile(`^hmac-sha256:[0-9a-f]{64}$`).MatchString(v.Fingerprint) {
		return errors.New("invalid redaction fingerprint")
	}
	switch v.ValueType {
	case ValueTypeString, ValueTypeNumber, ValueTypeInteger, ValueTypeBoolean, ValueTypeNull, ValueTypeArray, ValueTypeObject:
		return nil
	}
	return errors.New("invalid redacted value type")
}

func Sanitize(data []byte, secrets []string) []byte {
	out := string(data)
	ordered := append([]string(nil), secrets...)
	sort.SliceStable(ordered, func(i, j int) bool { return len(ordered[i]) > len(ordered[j]) })
	for _, secret := range ordered {
		if secret != "" {
			out = strings.ReplaceAll(out, secret, "[REDACTED]")
		}
	}
	return []byte(out)
}

func EncodeRequest(w io.Writer, request Request) error { return json.NewEncoder(w).Encode(request) }
func DecodeRequest(r io.Reader, limit int64) (Request, error) {
	var request Request
	if err := decodeStrict(r, limit, &request); err != nil {
		return request, err
	}
	if request.ProtocolVersion != Version || request.RequestID == "" || len(request.RequestID) > 128 || !validOperation(request.Operation) || !schemaName(request.PayloadSchema) || len(request.Payload) == 0 {
		return request, errors.New("invalid request envelope")
	}
	return request, nil
}
func DecodeResponse(r io.Reader, limit int64, requestID string) (Response, error) {
	var response Response
	if err := decodeStrict(r, limit, &response); err != nil {
		return response, err
	}
	if response.ProtocolVersion != Version || response.RequestID != requestID {
		return response, errors.New("invalid response envelope")
	}
	if response.OK {
		if response.ResultSchema == "" || len(response.Result) == 0 || response.Error != nil {
			return response, errors.New("invalid success response")
		}
	} else if response.Error == nil || response.ResultSchema != "" || len(response.Result) != 0 {
		return response, errors.New("invalid error response")
	}
	return response, nil
}
func DecodePayload(raw json.RawMessage, dst any) error {
	return decodeStrict(bytes.NewReader(raw), MaxMessageBytes, dst)
}

func decodeStrict(r io.Reader, limit int64, dst any) error {
	if limit <= 0 {
		return errors.New("invalid size limit")
	}
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > limit {
		return errors.New("message exceeds size limit")
	}
	if !utf8.Valid(data) {
		return errors.New("invalid UTF-8")
	}
	if err := rejectDuplicateNames(data); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("extra JSON value")
		}
		return err
	}
	return nil
}
func rejectDuplicateNames(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var walk func() error
	walk = func() error {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		d, ok := tok.(json.Delim)
		if !ok {
			return nil
		}
		switch d {
		case '{':
			seen := map[string]bool{}
			for dec.More() {
				k, err := dec.Token()
				if err != nil {
					return err
				}
				name, ok := k.(string)
				if !ok {
					return errors.New("object name is not a string")
				}
				if seen[name] {
					return fmt.Errorf("duplicate object name %q", name)
				}
				seen[name] = true
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = dec.Token()
			return err
		case '[':
			for dec.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = dec.Token()
			return err
		}
		return errors.New("invalid delimiter")
	}
	if err := walk(); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("extra JSON value")
	}
	return nil
}
func validOperation(op Operation) bool {
	switch op {
	case OperationProtocolInfo, OperationInspectSource, OperationValidateProposal, OperationValidateRevision, OperationAssembleFinal, OperationApplyFinal, OperationReviseProposal:
		return true
	}
	return false
}

var schemaRE = regexp.MustCompile(`^zconfig\.[a-z0-9-]+/[1-9][0-9]*$`)

func schemaName(s string) bool { return schemaRE.MatchString(s) }
