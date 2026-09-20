package contract

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hib2018/zconfig/tui/internal/protocol"
)

func TestEveryGoldenDocumentAgreesAcrossGoAndZig(t *testing.T) {
	prefix := t.TempDir()
	build := exec.Command("zig", "build", "-Doptimize=ReleaseSafe", "--prefix", prefix, "--cache-dir", filepath.Join(prefix, "cache"), "--global-cache-dir", filepath.Join(prefix, "global-cache"))
	build.Dir = filepath.Join("..", "..")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build Zig contract probe: %v: %s", err, output)
	}
	probe := filepath.Join(prefix, "bin", "zconfig-contract")
	files, err := filepath.Glob(filepath.Join("fixtures", "*", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 50 {
		t.Fatalf("golden matrix unexpectedly small: %d", len(files))
	}
	for _, path := range files {
		path := path
		t.Run(filepath.ToSlash(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			goContract, zigContract, expectedDigest := fixtureContract(filepath.Base(filepath.Dir(path)), filepath.Base(path))
			goValid := protocol.ValidateContract(goContract, data, expectedDigest) == nil
			args := []string{zigContract}
			if expectedDigest != "" {
				args = append(args, expectedDigest)
			}
			command := exec.Command(probe, args...)
			command.Stdin = bytes.NewReader(data)
			output, err := command.Output()
			if err != nil {
				t.Fatalf("run Zig contract probe: %v", err)
			}
			zigValid := strings.TrimSpace(string(output)) == "valid"
			wantValid := strings.HasPrefix(filepath.Base(path), "valid-")
			if goValid != wantValid || zigValid != wantValid || goValid != zigValid {
				t.Fatalf("acceptance mismatch: want=%t Go=%t Zig=%t output=%q", wantValid, goValid, zigValid, output)
			}
		})
	}
}

func fixtureContract(directory, name string) (protocol.Contract, string, string) {
	if directory == "proposals" {
		if strings.Contains(name, "setting") {
			return protocol.ContractSetting, "setting", ""
		}
		return protocol.ContractProposal, "proposal", ""
	}
	if directory == "validators" {
		if strings.Contains(name, "request") {
			return protocol.ContractValidatorRequest, "validator_request", ""
		}
		digest := "sha256:" + strings.Repeat("c", 64)
		return protocol.ContractValidatorResult, "validator_result", digest
	}
	cases := []struct {
		fragment string
		goKind   protocol.Contract
		zigKind  string
	}{
		{"protocol-info-request", protocol.ContractEmpty, "empty"},
		{"protocol-info-result", protocol.ContractProtocolInfo, "protocol_info"},
		{"inspection-request", protocol.ContractInspectionRequest, "inspection_request"},
		{"inspection-result", protocol.ContractInspectionResult, "inspection_result"},
		{"revision-request", protocol.ContractRevisionRequest, "revision_request"},
		{"revision-result", protocol.ContractRevisionResult, "revision_result"},
		{"validation-result", protocol.ContractValidationResult, "validation_result"},
		{"final-change-request", protocol.ContractFinalRequest, "final_request"},
		{"final-change-result", protocol.ContractFinalResult, "final_result"},
		{"apply-request", protocol.ContractApplyRequest, "apply_request"},
		{"apply-result", protocol.ContractApplyResult, "apply_result"},
		{"audit-event", protocol.ContractAuditEvent, "audit_event"},
	}
	for _, item := range cases {
		if strings.Contains(name, item.fragment) {
			return item.goKind, item.zigKind, ""
		}
	}
	return protocol.ContractEnvelope, "envelope", ""
}
