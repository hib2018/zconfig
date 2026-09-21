package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/hib2018/zconfig/tui/internal/config"
	"github.com/hib2018/zconfig/tui/internal/protocol"
	"github.com/hib2018/zconfig/tui/internal/review"
	"github.com/hib2018/zconfig/tui/internal/runner"
	"github.com/hib2018/zconfig/tui/internal/session"
	"github.com/hib2018/zconfig/tui/internal/ui"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(exitCode(err))
	}
}
func run(args []string) error {
	if len(args) > 0 && args[0] == "review" {
		return runReview(args[1:])
	}
	if len(args) > 0 && args[0] == "apply" {
		return runApply(args[1:])
	}
	fs := flag.NewFlagSet("zconfig", flag.ContinueOnError)
	core := fs.String("core", "", "path to zconfig-core")
	agent := fs.String("agent", "", "registered agent name")
	configPath := fs.String("config", "", "command registration file")
	handshake := fs.Bool("handshake", false, "verify core and selected agent protocols")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !*handshake {
		return errors.New("foundation build supports --handshake only")
	}
	project, err := os.Getwd()
	if err != nil {
		return err
	}
	if *configPath == "" {
		*configPath, err = config.DefaultPath()
		if err != nil {
			return err
		}
	}
	if *core == "" {
		*core = findCore()
		if *core == "" {
			return errors.New("zconfig core not found; pass --core")
		}
	}
	if err := checkProtocol(context.Background(), runner.Command{Executable: *core, Timeout: 10 * time.Second}, "core"); err != nil {
		return err
	}
	if *agent != "" {
		cfg, err := config.Load(*configPath, project)
		if err != nil {
			return err
		}
		registration, ok := findRegistration(cfg.Agents, *agent)
		if !ok {
			return fmt.Errorf("agent %q is not registered", *agent)
		}
		command := runner.Command{Executable: registration.Executable, Args: registration.Args, WorkingDirectory: registration.WorkingDirectory, EnvironmentList: registration.Environment(), Timeout: registration.Timeout}
		if err := checkProtocol(context.Background(), command, "agent"); err != nil {
			return err
		}
	}
	fmt.Println("protocol handshake succeeded")
	return nil
}

var (
	errConfirmationRequired = errors.New("final confirmation required; inspect the diff and repeat with --confirm-token and the displayed token")
	errValidationBlocked    = errors.New("validation blocked application")
)

func exitCode(err error) int {
	switch {
	case errors.Is(err, errConfirmationRequired):
		return 2
	case errors.Is(err, errValidationBlocked):
		return 3
	default:
		return 1
	}
}

type stringList []string

func (v *stringList) String() string { return strings.Join(*v, ",") }
func (v *stringList) Set(value string) error {
	if value == "" {
		return errors.New("empty value")
	}
	*v = append(*v, value)
	return nil
}

type applyOptions struct {
	proposal, source, schema, corePath, configPath string
	project, sessionID, confirmToken               string
	approved, rejected, validators                 []string
}

func runApply(args []string) error {
	options, err := parseApplyArgs(args)
	if err != nil {
		return err
	}
	return executeApply(options)
}

func parseApplyArgs(args []string) (applyOptions, error) {
	fs := flag.NewFlagSet("zconfig apply", flag.ContinueOnError)
	var approved, rejected, validators stringList
	source := fs.String("source", "", "JSON source configuration")
	schema := fs.String("schema", "", "optional JSON Schema")
	corePath := fs.String("core", "", "path to zconfig-core")
	configPath := fs.String("config", "", "command registration file")
	project := fs.String("project", "", "project directory containing review and audit state")
	sessionID := fs.String("session", "", "review session ID")
	confirmToken := fs.String("confirm-token", "", "token emitted by a previous final preview")
	fs.Var(&approved, "approve", "approved change ID (repeatable)")
	fs.Var(&rejected, "reject", "rejected change ID (repeatable)")
	fs.Var(&validators, "validator", "registered validator name (repeatable)")
	proposalPath := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		proposalPath, args = args[0], args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return applyOptions{}, err
	}
	if proposalPath == "" && fs.NArg() == 1 {
		proposalPath = fs.Arg(0)
	}
	if proposalPath == "" || *source == "" || fs.NArg() > 1 {
		return applyOptions{}, errors.New("usage: zconfig apply <proposal> --source <file> --session <id> (--approve <id>|--reject <id>)... [--validator <name>] [--confirm-token <token>]")
	}
	if *sessionID == "" {
		return applyOptions{}, errors.New("apply requires --session so decisions, comments, and audit events share one review identity")
	}
	return applyOptions{proposal: proposalPath, source: *source, schema: *schema, corePath: *corePath, configPath: *configPath, project: *project, sessionID: *sessionID, confirmToken: *confirmToken, approved: approved, rejected: rejected, validators: validators}, nil
}

func executeApply(options applyOptions) error {
	ctx := context.Background()
	if err := canonicalizeApplyPaths(&options); err != nil {
		return err
	}
	if options.corePath == "" {
		options.corePath = findCore()
	}
	if options.corePath == "" {
		return errors.New("zconfig core not found; pass --core")
	}
	coreCommand := runner.Command{Executable: options.corePath, Timeout: 10 * time.Second}
	if err := checkProtocol(ctx, coreCommand, "core"); err != nil {
		return err
	}
	proposalBytes, err := os.ReadFile(options.proposal)
	if err != nil {
		return err
	}
	if err := protocol.ValidateContract(protocol.ContractProposal, proposalBytes, ""); err != nil {
		return fmt.Errorf("invalid proposal: %w", err)
	}
	var proposal review.Proposal
	if err := json.Unmarshal(proposalBytes, &proposal); err != nil {
		return err
	}
	inspection, err := inspect(ctx, coreCommand, options.source, options.schema)
	if err != nil {
		return err
	}
	if proposal.SourceDigest != inspection.SourceDigest {
		return errors.New("proposal source digest does not match the inspected source")
	}
	audit, err := session.OpenAudit(options.project, options.sessionID)
	if err != nil {
		return fmt.Errorf("open audit: %w", err)
	}
	stored, created, err := loadOrCreateApplySession(options, proposal, inspection)
	if err != nil {
		return err
	}
	decisions, comments, err := sessionApplyState(stored, proposal, options.source, options.approved, options.rejected)
	if err != nil {
		return err
	}
	proposalDigest := digestBytes(proposalBytes)
	if created {
		for _, item := range proposal.Items {
			action := "item.rejected"
			if decisions[item.ChangeID] == "approved" {
				action = "item.approved"
			}
			if err := appendAudit(audit, action, []string{item.ChangeID}, nil, inspection.SourceDigest, proposalDigest, ""); err != nil {
				return err
			}
		}
		if err := session.Save(options.project, stored); err != nil {
			return fmt.Errorf("save review session: %w", err)
		}
	}

	checks := []protocol.ValidationResult{}
	if len(options.validators) != 0 {
		candidatePath, candidateDigest, cleanup, err := prepareValidatorCandidate(ctx, coreCommand, options, proposalBytes, decisions, comments, inspection.SourceDigest)
		if err != nil {
			return err
		}
		defer cleanup()
		if options.configPath == "" {
			options.configPath, err = config.DefaultPath()
			if err != nil {
				return err
			}
		}
		cfg, err := config.Load(options.configPath, options.project)
		if err != nil {
			return err
		}
		for _, name := range options.validators {
			registration, ok := findRegistration(cfg.Validators, name)
			if !ok {
				return fmt.Errorf("%w: validator %q is not registered", errValidationBlocked, name)
			}
			validator := runner.Validator{Name: name, Command: runner.Command{Executable: registration.Executable, Args: registration.Args, WorkingDirectory: registration.WorkingDirectory, EnvironmentList: registration.Environment(), Timeout: registration.Timeout}}
			result, validateErr := validator.Validate(ctx, protocol.ExternalValidatorRequest{CandidatePath: candidatePath, CandidateDigest: candidateDigest, SourceDigest: inspection.SourceDigest}, nil)
			if validateErr != nil {
				return fmt.Errorf("%w: validator %q failed", errValidationBlocked, name)
			}
			checks = append(checks, protocol.ValidationResult{SubjectDigest: candidateDigest, Checks: []protocol.CheckResult{{Kind: "external", Status: result.Status, Code: result.Code, Message: "Registered validator accepted the candidate."}}})
		}
	}

	assembly, err := assembleFinal(ctx, coreCommand, options.source, options.schema, inspection.SourceDigest, proposalBytes, decisions, comments, checks)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, assembly.Diff)
	if options.confirmToken == "" {
		if err := appendAudit(audit, "final.previewed", assembly.ApprovedIDs, nil, inspection.SourceDigest, proposalDigest, ""); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "session: %s\nconfirmation token: %s\n", options.sessionID, assembly.ConfirmationNonce)
		return errConfirmationRequired
	}
	if err := appendAudit(audit, "final.confirmed", assembly.ApprovedIDs, nil, inspection.SourceDigest, proposalDigest, "confirmation.accepted"); err != nil {
		return err
	}
	if err := appendAudit(audit, "apply.started", assembly.ApprovedIDs, nil, inspection.SourceDigest, proposalDigest, "apply.started"); err != nil {
		return err
	}
	applyPayload := protocol.ApplyFinal{SourcePath: options.source, SchemaPath: options.schema, SourceDigest: inspection.SourceDigest, FinalChangeDigest: assembly.FinalChangeDigest, ConfirmationNonce: options.confirmToken, Proposal: proposalBytes, Decisions: decisions, Comments: comments, ExternalChecks: checks}
	payload, err := json.Marshal(applyPayload)
	if err != nil {
		return err
	}
	response, err := runner.RunProtocol(ctx, coreCommand, protocol.Request{ProtocolVersion: protocol.Version, RequestID: requestID("apply"), Operation: protocol.OperationApplyFinal, PayloadSchema: "zconfig.apply/1", Payload: payload}, nil)
	if err != nil {
		return err
	}
	if !response.OK {
		_ = appendAudit(audit, "apply.failed", assembly.ApprovedIDs, nil, inspection.SourceDigest, proposalDigest, "apply.rejected")
		return fmt.Errorf("apply rejected: %s", response.Error.Code)
	}
	var result protocol.ApplyResult
	if err := protocol.DecodePayload(response.Result, &result); err != nil {
		_ = restoreFromRecovery(options.source, options.source+".zconfig-recovery")
		return err
	}
	if err := appendAudit(audit, "apply.succeeded", assembly.ApprovedIDs, nil, result.SourceDigestAfter, proposalDigest, "apply.succeeded"); err != nil {
		if restoreErr := restoreFromRecovery(options.source, result.Recovery.Path); restoreErr != nil {
			return fmt.Errorf("success audit failed and recovery failed: %v; %w", restoreErr, err)
		}
		return fmt.Errorf("success audit failed; source restored: %w", err)
	}
	fmt.Fprintf(os.Stdout, "applied %s; recovery: %s\n", result.SourceDigestAfter, result.Recovery.Path)
	return nil
}

func inspect(ctx context.Context, command runner.Command, source, schema string) (protocol.InspectionResult, error) {
	payload, _ := json.Marshal(protocol.InspectSource{SourcePath: source, SchemaPath: schema})
	response, err := runner.RunProtocol(ctx, command, protocol.Request{ProtocolVersion: protocol.Version, RequestID: requestID("inspect"), Operation: protocol.OperationInspectSource, PayloadSchema: "zconfig.inspection/1", Payload: payload}, nil)
	if err != nil {
		return protocol.InspectionResult{}, err
	}
	if !response.OK {
		return protocol.InspectionResult{}, fmt.Errorf("inspect source: %s", response.Error.Code)
	}
	var result protocol.InspectionResult
	if err := protocol.DecodePayload(response.Result, &result); err != nil {
		return protocol.InspectionResult{}, err
	}
	return result, nil
}

func decisionSet(proposal review.Proposal, approved, rejected []string) (map[string]string, error) {
	decisions := make(map[string]string, len(proposal.Items))
	valid := make(map[string]bool, len(proposal.Items))
	for _, item := range proposal.Items {
		valid[item.ChangeID] = true
	}
	for _, group := range []struct {
		ids      []string
		decision string
	}{{approved, "approved"}, {rejected, "rejected"}} {
		for _, id := range group.ids {
			if !valid[id] {
				return nil, fmt.Errorf("unknown change ID %q", id)
			}
			if _, exists := decisions[id]; exists {
				return nil, fmt.Errorf("change ID %q was decided more than once", id)
			}
			decisions[id] = group.decision
		}
	}
	if len(decisions) != len(proposal.Items) {
		return nil, errors.New("every change item must be approved or rejected")
	}
	return decisions, nil
}

func canonicalizeApplyPaths(options *applyOptions) error {
	project, err := os.Getwd()
	if err != nil {
		return err
	}
	if options.project != "" {
		project = options.project
	}
	options.project, err = filepath.Abs(project)
	if err != nil {
		return err
	}
	for label, target := range map[string]*string{
		"proposal": &options.proposal,
		"source":   &options.source,
		"schema":   &options.schema,
		"config":   &options.configPath,
	} {
		if *target == "" {
			continue
		}
		*target, err = filepath.Abs(*target)
		if err != nil {
			return fmt.Errorf("resolve %s path: %w", label, err)
		}
	}
	return nil
}

func loadOrCreateApplySession(options applyOptions, proposal review.Proposal, inspection protocol.InspectionResult) (review.Session, bool, error) {
	stored, err := session.Load(options.project, options.sessionID)
	if err == nil {
		return stored, false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return review.Session{}, false, fmt.Errorf("load review session: %w", err)
	}
	if options.confirmToken != "" {
		return review.Session{}, false, errors.New("confirmation requires the review session created by the preview")
	}
	decisions, err := decisionSet(proposal, options.approved, options.rejected)
	if err != nil {
		return review.Session{}, false, err
	}
	persisted := proposal
	persisted.Items = append([]review.ChangeItem(nil), proposal.Items...)
	applyInspection(&persisted, inspection)
	for i := range persisted.Items {
		persisted.Items[i].Decision = review.Decision(decisions[persisted.Items[i].ChangeID])
		if persisted.Items[i].Sensitivity != review.SensitivityNormal {
			persisted.Items[i].ExpectedOld = nil
			persisted.Items[i].ProposedValue = nil
		}
	}
	now := time.Now().UTC()
	return review.Session{
		SessionSchema: session.SessionSchema,
		ReviewID:      options.sessionID,
		Source: review.Source{
			Path:       options.source,
			Digest:     inspection.SourceDigest,
			ByteLength: inspection.ByteLength,
			NodeCount:  inspection.NodeCount,
			RootType:   "object",
		},
		ActiveProposal: persisted,
		Comments:       []review.Comment{},
		Lifecycle:      review.LifecycleReady,
		UpdatedAt:      now,
	}, true, nil
}

func sessionApplyState(stored review.Session, proposal review.Proposal, source string, approved, rejected []string) (map[string]string, []protocol.FinalComment, error) {
	if stored.Source.Path != source || stored.Source.Digest != proposal.SourceDigest || stored.ActiveProposal.ProposalID != proposal.ProposalID || stored.ActiveProposal.Revision != proposal.Revision || len(stored.ActiveProposal.Items) != len(proposal.Items) {
		return nil, nil, errors.New("review session does not match the active proposal")
	}
	decisions := make(map[string]string, len(proposal.Items))
	for i, item := range proposal.Items {
		storedItem := stored.ActiveProposal.Items[i]
		if storedItem.ChangeID != item.ChangeID || storedItem.Path != item.Path || storedItem.Operation != item.Operation {
			return nil, nil, errors.New("review session proposal identity changed")
		}
		if storedItem.Decision != review.DecisionApproved && storedItem.Decision != review.DecisionRejected {
			return nil, nil, fmt.Errorf("review session has no final decision for %q", item.ChangeID)
		}
		decisions[item.ChangeID] = string(storedItem.Decision)
	}
	if len(approved)+len(rejected) != 0 {
		requested, err := decisionSet(proposal, approved, rejected)
		if err != nil {
			return nil, nil, err
		}
		for id, decision := range requested {
			if decisions[id] != decision {
				return nil, nil, fmt.Errorf("command decision for %q differs from the review session", id)
			}
		}
	}
	comments := make([]protocol.FinalComment, 0, len(stored.Comments))
	for _, comment := range stored.Comments {
		comments = append(comments, protocol.FinalComment{CommentID: comment.CommentID, ChangeID: comment.ChangeID, Status: string(comment.Status)})
	}
	return decisions, comments, nil
}

func restoreFromRecovery(source, recovery string) error {
	prior, err := os.ReadFile(recovery)
	if err != nil {
		return err
	}
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(source), ".zconfig-audit-rollback-*.tmp")
	if err != nil {
		return err
	}
	name := temporary.Name()
	ok := false
	defer func() {
		_ = temporary.Close()
		if !ok {
			_ = os.Remove(name)
		}
	}()
	if err := temporary.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if _, err := temporary.Write(prior); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, source); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(source))
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return err
	}
	ok = true
	return nil
}

func digestBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func appendAudit(audit *session.Audit, action string, changeIDs, commentIDs []string, sourceDigest, proposalDigest, outcome string) error {
	event := session.AuditEvent{
		EventID:        requestID("event"),
		OccurredAt:     time.Now().UTC(),
		ActorType:      "human",
		Action:         action,
		ChangeIDs:      append([]string(nil), changeIDs...),
		CommentIDs:     append([]string(nil), commentIDs...),
		SourceDigest:   sourceDigest,
		ProposalDigest: proposalDigest,
		OutcomeCode:    outcome,
	}
	if err := audit.Append(event); err != nil {
		return fmt.Errorf("audit %s: %w", action, err)
	}
	return nil
}

func assembleFinal(ctx context.Context, command runner.Command, source, schema, digest string, proposal json.RawMessage, decisions map[string]string, comments []protocol.FinalComment, checks []protocol.ValidationResult) (protocol.FinalAssemblyResult, error) {
	if checks == nil {
		checks = []protocol.ValidationResult{}
	}
	payload, err := json.Marshal(protocol.FinalAssembly{SourcePath: source, SchemaPath: schema, SourceDigest: digest, Proposal: proposal, Decisions: decisions, Comments: comments, ExternalChecks: checks})
	if err != nil {
		return protocol.FinalAssemblyResult{}, err
	}
	response, err := runner.RunProtocol(ctx, command, protocol.Request{ProtocolVersion: protocol.Version, RequestID: requestID("assemble"), Operation: protocol.OperationAssembleFinal, PayloadSchema: "zconfig.final-change/1", Payload: payload}, nil)
	if err != nil {
		return protocol.FinalAssemblyResult{}, err
	}
	if !response.OK {
		return protocol.FinalAssemblyResult{}, fmt.Errorf("final assembly rejected: %s", response.Error.Code)
	}
	var result protocol.FinalAssemblyResult
	if err := protocol.DecodePayload(response.Result, &result); err != nil {
		return protocol.FinalAssemblyResult{}, err
	}
	return result, nil
}

func prepareValidatorCandidate(ctx context.Context, command runner.Command, options applyOptions, proposal json.RawMessage, decisions map[string]string, comments []protocol.FinalComment, sourceDigest string) (string, string, func(), error) {
	source, err := os.ReadFile(options.source)
	if err != nil {
		return "", "", func() {}, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(options.source), ".zconfig-candidate-*.json")
	if err != nil {
		return "", "", func() {}, err
	}
	path := temporary.Name()
	recovery := path + ".zconfig-recovery"
	cleanup := func() { _ = os.Remove(path); _ = os.Remove(recovery) }
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		cleanup()
		return "", "", func() {}, err
	}
	if _, err := temporary.Write(source); err != nil {
		temporary.Close()
		cleanup()
		return "", "", func() {}, err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		cleanup()
		return "", "", func() {}, err
	}
	if err := temporary.Close(); err != nil {
		cleanup()
		return "", "", func() {}, err
	}
	assembly, err := assembleFinal(ctx, command, path, options.schema, sourceDigest, proposal, decisions, comments, nil)
	if err != nil {
		cleanup()
		return "", "", func() {}, err
	}
	payload := protocol.ApplyFinal{SourcePath: path, SchemaPath: options.schema, SourceDigest: sourceDigest, FinalChangeDigest: assembly.FinalChangeDigest, ConfirmationNonce: assembly.ConfirmationNonce, Proposal: proposal, Decisions: decisions, Comments: comments, ExternalChecks: []protocol.ValidationResult{}}
	encoded, err := json.Marshal(payload)
	if err != nil {
		cleanup()
		return "", "", func() {}, err
	}
	response, err := runner.RunProtocol(ctx, command, protocol.Request{ProtocolVersion: protocol.Version, RequestID: requestID("candidate"), Operation: protocol.OperationApplyFinal, PayloadSchema: "zconfig.apply/1", Payload: encoded}, nil)
	if err != nil || !response.OK {
		cleanup()
		if err != nil {
			return "", "", func() {}, err
		}
		return "", "", func() {}, fmt.Errorf("candidate preparation rejected: %s", response.Error.Code)
	}
	return path, assembly.FinalChangeDigest, cleanup, nil
}

func runReview(args []string) error {
	options, err := parseReviewArgs(args)
	if err != nil {
		return err
	}
	return executeReview(options)
}

type reviewOptions struct {
	proposal, source, schema, corePath string
	monochrome                         bool
}

func parseReviewArgs(args []string) (reviewOptions, error) {
	fs := flag.NewFlagSet("zconfig review", flag.ContinueOnError)
	source := fs.String("source", "", "JSON source configuration")
	schema := fs.String("schema", "", "optional JSON Schema")
	corePath := fs.String("core", "", "path to zconfig-core")
	monochrome := fs.Bool("monochrome", false, "disable color output")
	proposalPath := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		proposalPath, args = args[0], args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return reviewOptions{}, err
	}
	if proposalPath == "" && fs.NArg() == 1 {
		proposalPath = fs.Arg(0)
	}
	if proposalPath == "" || fs.NArg() > 1 || *source == "" {
		return reviewOptions{}, errors.New("usage: zconfig review <proposal> --source <file> [--schema <file>]")
	}
	return reviewOptions{proposal: proposalPath, source: *source, schema: *schema, corePath: *corePath, monochrome: *monochrome}, nil
}

func executeReview(options reviewOptions) error {
	if options.corePath == "" {
		options.corePath = findCore()
	}
	if options.corePath == "" {
		return errors.New("zconfig core not found; pass --core")
	}
	command := runner.Command{Executable: options.corePath, Timeout: 10 * time.Second}
	ctx := context.Background()
	if err := checkProtocol(ctx, command, "core"); err != nil {
		return err
	}

	proposalBytes, err := os.ReadFile(options.proposal)
	if err != nil {
		return err
	}
	if err := protocol.ValidateContract(protocol.ContractProposal, proposalBytes, ""); err != nil {
		return fmt.Errorf("invalid proposal: %w", err)
	}
	var proposal review.Proposal
	if err := json.Unmarshal(proposalBytes, &proposal); err != nil {
		return err
	}
	inspectionPayload, _ := json.Marshal(protocol.InspectSource{SourcePath: options.source, SchemaPath: options.schema})
	inspectionResponse, err := runner.RunProtocol(ctx, command, protocol.Request{
		ProtocolVersion: protocol.Version, RequestID: requestID("inspect"), Operation: protocol.OperationInspectSource,
		PayloadSchema: "zconfig.inspection/1", Payload: inspectionPayload,
	}, nil)
	if err != nil {
		return fmt.Errorf("inspect source: %w", err)
	}
	if !inspectionResponse.OK {
		return fmt.Errorf("inspect source: %s", inspectionResponse.Error.Code)
	}
	var inspection protocol.InspectionResult
	if err := protocol.DecodePayload(inspectionResponse.Result, &inspection); err != nil {
		return err
	}
	if proposal.SourceDigest != inspection.SourceDigest {
		return errors.New("proposal source digest does not match the inspected source")
	}
	validationResponse, err := runner.RunProtocol(ctx, command, protocol.Request{
		ProtocolVersion: protocol.Version, RequestID: requestID("proposal"), Operation: protocol.OperationValidateProposal,
		PayloadSchema: "zconfig.proposal/1", Payload: proposalBytes,
	}, nil)
	if err != nil {
		return fmt.Errorf("validate proposal: %w", err)
	}
	if !validationResponse.OK {
		return fmt.Errorf("validate proposal: %s", validationResponse.Error.Code)
	}
	applyInspection(&proposal, inspection)
	model := ui.NewReviewModel(review.NewState(proposal.Items))
	model.Monochrome = options.monochrome
	return runReviewProgram(model)
}

func applyInspection(proposal *review.Proposal, inspection protocol.InspectionResult) {
	classifications := make(map[string]string, len(inspection.Settings))
	for _, setting := range inspection.Settings {
		classifications[setting.Path] = setting.Sensitivity
	}
	for i := range proposal.Items {
		value := classifications[proposal.Items[i].Path]
		if value == "" && sensitiveName(proposal.Items[i].Path) {
			value = string(review.SensitivitySuspected)
		}
		proposal.Items[i].Sensitivity = review.Sensitivity(value)
		if proposal.Items[i].Sensitivity == "" {
			proposal.Items[i].Sensitivity = review.SensitivityNormal
		}
	}
}

func sensitiveName(path string) bool {
	part := path
	if at := strings.LastIndexByte(path, '/'); at >= 0 {
		part = path[at+1:]
	}
	part = strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, part)
	switch part {
	case "password", "passwd", "secret", "token", "accesstoken", "refreshtoken", "apikey", "apitoken", "privatekey", "clientsecret", "credential", "credentials":
		return true
	}
	return false
}

func requestID(prefix string) string { return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()) }

var runReviewProgram = func(model tea.Model) error {
	_, err := tea.NewProgram(model).Run()
	return err
}

func checkProtocol(ctx context.Context, command runner.Command, label string) error {
	request := protocol.Request{ProtocolVersion: protocol.Version, RequestID: "handshake-1", Operation: protocol.OperationProtocolInfo, PayloadSchema: "zconfig.empty/1", Payload: json.RawMessage(`{}`)}
	response, err := runner.RunProtocol(ctx, command, request, nil)
	if err != nil {
		return fmt.Errorf("%s handshake: %w", label, err)
	}
	if !response.OK {
		return fmt.Errorf("%s handshake rejected: %s", label, response.Error.Code)
	}
	var info protocol.ProtocolInfo
	if err := protocol.DecodePayload(response.Result, &info); err != nil {
		return fmt.Errorf("%s capabilities: %w", label, err)
	}
	for _, v := range info.ProtocolVersions {
		if v == protocol.Version {
			return nil
		}
	}
	return fmt.Errorf("%s does not support protocol %s", label, protocol.Version)
}
func findRegistration(values []config.Registration, name string) (config.Registration, bool) {
	for _, v := range values {
		if v.Name == name {
			return v, true
		}
	}
	return config.Registration{}, false
}
func findCore() string {
	if value := os.Getenv("ZCONFIG_CORE"); value != "" {
		return value
	}
	self, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(self), "zconfig-core")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if value, err := execLookPath("zconfig-core"); err == nil {
		return value
	}
	return ""
}

var execLookPath = exec.LookPath
