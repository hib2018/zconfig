package runner

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hib2018/zconfig/tui/internal/protocol"
)

const validatorDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestValidatorHelper(t *testing.T) {
	if os.Getenv("GO_WANT_VALIDATOR_HELPER") != "1" {
		return
	}
	var request protocol.ExternalValidatorRequest
	if json.NewDecoder(os.Stdin).Decode(&request) != nil {
		os.Exit(2)
	}
	mode := os.Getenv("VALIDATOR_MODE")
	if mode == "timeout" {
		time.Sleep(5 * time.Second)
	}
	if mode == "nonzero" {
		os.Stderr.WriteString("failed fixture-secret")
		os.Exit(3)
	}
	if mode == "malformed" {
		os.Stdout.WriteString("not-json")
		os.Exit(0)
	}
	digest := request.CandidateDigest
	if mode == "mismatch" {
		digest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	}
	status := "passed"
	if mode == "failed" {
		status = "failed"
	}
	json.NewEncoder(os.Stdout).Encode(protocol.ExternalValidatorResult{CandidateDigest: digest, Status: status, Code: "validator." + status, Message: "result"})
	os.Exit(0)
}

func validatorCommand(mode string) Command {
	return Command{Executable: os.Args[0], Args: []string{"-test.run=TestValidatorHelper"}, Environment: map[string]string{"GO_WANT_VALIDATOR_HELPER": "1", "VALIDATOR_MODE": mode}, Timeout: 5 * time.Second}
}

func TestValidatorAcceptsPassedDigestBoundResult(t *testing.T) {
	request := protocol.ExternalValidatorRequest{CandidatePath: "/tmp/candidate", CandidateDigest: validatorDigest, SourceDigest: validatorDigest}
	result, err := (Validator{Name: "fixture", Command: validatorCommand("passed")}).Validate(context.Background(), request, nil)
	if err != nil || result.Status != "passed" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestValidatorFailureRules(t *testing.T) {
	request := protocol.ExternalValidatorRequest{CandidatePath: "/tmp/candidate", CandidateDigest: validatorDigest, SourceDigest: validatorDigest}
	for _, mode := range []string{"failed", "mismatch", "malformed", "nonzero"} {
		t.Run(mode, func(t *testing.T) {
			_, err := (Validator{Name: "fixture", Command: validatorCommand(mode)}).Validate(context.Background(), request, []string{"fixture-secret"})
			if err == nil || strings.Contains(err.Error(), "fixture-secret") {
				t.Fatalf("unsafe or missing failure: %v", err)
			}
		})
	}
	command := validatorCommand("timeout")
	command.Timeout = 10 * time.Millisecond
	if _, err := (Validator{Name: "fixture", Command: command}).Validate(context.Background(), request, nil); err == nil {
		t.Fatal("timeout did not block validation")
	}
}

func TestMissingValidatorIsExplicitlyUnverified(t *testing.T) {
	result := UnverifiedValidatorResult(validatorDigest, "validator.missing", "validator is not registered")
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := protocol.ValidateContract(protocol.ContractValidatorResult, data, validatorDigest); err != nil || result.Status != "unverified" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
