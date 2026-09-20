package main

import (
	"context"
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
	errConfirmationRequired = errors.New("final confirmation required; inspect the diff and repeat with --confirm")
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
	approved, rejected, validators                 []string
	confirm                                        bool
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
	confirm := fs.Bool("confirm", false, "apply the displayed final change set")
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
		return applyOptions{}, errors.New("usage: zconfig apply <proposal> --source <file> (--approve <id>|--reject <id>)... [--validator <name>] [--confirm]")
	}
	return applyOptions{proposal: proposalPath, source: *source, schema: *schema, corePath: *corePath, configPath: *configPath, approved: approved, rejected: rejected, validators: validators, confirm: *confirm}, nil
}

func executeApply(options applyOptions) error {
	ctx := context.Background()
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
	decisions, err := decisionSet(proposal, options.approved, options.rejected)
	if err != nil {
		return err
	}
	inspection, err := inspect(ctx, coreCommand, options.source, options.schema)
	if err != nil {
		return err
	}
	if proposal.SourceDigest != inspection.SourceDigest {
		return errors.New("proposal source digest does not match the inspected source")
	}

	checks := []protocol.ValidationResult{}
	if len(options.validators) != 0 {
		candidatePath, candidateDigest, cleanup, err := prepareValidatorCandidate(ctx, coreCommand, options, proposalBytes, decisions, inspection.SourceDigest)
		if err != nil {
			return err
		}
		defer cleanup()
		project, err := os.Getwd()
		if err != nil {
			return err
		}
		if options.configPath == "" {
			options.configPath, err = config.DefaultPath()
			if err != nil {
				return err
			}
		}
		cfg, err := config.Load(options.configPath, project)
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

	assembly, err := assembleFinal(ctx, coreCommand, options.source, options.schema, inspection.SourceDigest, proposalBytes, decisions, checks)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, assembly.Diff)
	if !options.confirm {
		return errConfirmationRequired
	}
	applyPayload := protocol.ApplyFinal{SourcePath: options.source, SchemaPath: options.schema, SourceDigest: inspection.SourceDigest, FinalChangeDigest: assembly.FinalChangeDigest, ConfirmationNonce: assembly.ConfirmationNonce, Proposal: proposalBytes, Decisions: decisions, Comments: []protocol.FinalComment{}, ExternalChecks: checks}
	payload, err := json.Marshal(applyPayload)
	if err != nil {
		return err
	}
	response, err := runner.RunProtocol(ctx, coreCommand, protocol.Request{ProtocolVersion: protocol.Version, RequestID: requestID("apply"), Operation: protocol.OperationApplyFinal, PayloadSchema: "zconfig.apply/1", Payload: payload}, nil)
	if err != nil {
		return err
	}
	if !response.OK {
		return fmt.Errorf("apply rejected: %s", response.Error.Code)
	}
	var result protocol.ApplyResult
	if err := protocol.DecodePayload(response.Result, &result); err != nil {
		return err
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

func assembleFinal(ctx context.Context, command runner.Command, source, schema, digest string, proposal json.RawMessage, decisions map[string]string, checks []protocol.ValidationResult) (protocol.FinalAssemblyResult, error) {
	if checks == nil {
		checks = []protocol.ValidationResult{}
	}
	payload, err := json.Marshal(protocol.FinalAssembly{SourcePath: source, SchemaPath: schema, SourceDigest: digest, Proposal: proposal, Decisions: decisions, Comments: []protocol.FinalComment{}, ExternalChecks: checks})
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

func prepareValidatorCandidate(ctx context.Context, command runner.Command, options applyOptions, proposal json.RawMessage, decisions map[string]string, sourceDigest string) (string, string, func(), error) {
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
	assembly, err := assembleFinal(ctx, command, path, options.schema, sourceDigest, proposal, decisions, nil)
	if err != nil {
		cleanup()
		return "", "", func() {}, err
	}
	payload := protocol.ApplyFinal{SourcePath: path, SchemaPath: options.schema, SourceDigest: sourceDigest, FinalChangeDigest: assembly.FinalChangeDigest, ConfirmationNonce: assembly.ConfirmationNonce, Proposal: proposal, Decisions: decisions, Comments: []protocol.FinalComment{}, ExternalChecks: []protocol.ValidationResult{}}
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
