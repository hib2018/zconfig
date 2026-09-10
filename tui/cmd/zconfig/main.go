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
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) > 0 && args[0] == "review" {
		return runReview(args[1:])
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
	case "password", "passwd", "secret", "token", "accesstoken", "refreshtoken", "apikey", "privatekey", "clientsecret", "credential", "credentials":
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
