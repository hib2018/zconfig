package protocol

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCompleteGoldenAcceptanceMatrix(t *testing.T) {
	_, sourceFile, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", "..", "tests", "contract", "fixtures"))
	files, err := filepath.Glob(filepath.Join(root, "*", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 50 {
		t.Fatalf("golden matrix unexpectedly small: %d files", len(files))
	}
	for _, path := range files {
		path := path
		t.Run(filepath.Base(filepath.Dir(path))+"/"+filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			contract, expectedDigest := fixtureContract(filepath.Base(filepath.Dir(path)), filepath.Base(path))
			err = ValidateContract(contract, data, expectedDigest)
			wantValid := strings.HasPrefix(filepath.Base(path), "valid-")
			if wantValid && err != nil {
				t.Fatalf("valid fixture rejected: %v", err)
			}
			if !wantValid && err == nil {
				t.Fatal("invalid fixture accepted")
			}
		})
	}
}

func fixtureContract(directory, name string) (Contract, string) {
	if directory == "proposals" {
		if strings.Contains(name, "setting") {
			return ContractSetting, ""
		}
		return ContractProposal, ""
	}
	if directory == "validators" {
		if strings.Contains(name, "request") {
			return ContractValidatorRequest, ""
		}
		return ContractValidatorResult, "sha256:" + strings.Repeat("c", 64)
	}
	switch {
	case strings.Contains(name, "protocol-info-request"):
		return ContractEmpty, ""
	case strings.Contains(name, "protocol-info-result"):
		return ContractProtocolInfo, ""
	case strings.Contains(name, "inspection-request"):
		return ContractInspectionRequest, ""
	case strings.Contains(name, "inspection-result"):
		return ContractInspectionResult, ""
	case strings.Contains(name, "revision-request"):
		return ContractRevisionRequest, ""
	case strings.Contains(name, "revision-result"):
		return ContractRevisionResult, ""
	case strings.Contains(name, "validation-result"):
		return ContractValidationResult, ""
	case strings.Contains(name, "final-change-request"):
		return ContractFinalRequest, ""
	case strings.Contains(name, "final-change-result"):
		return ContractFinalResult, ""
	case strings.Contains(name, "apply-request"):
		return ContractApplyRequest, ""
	case strings.Contains(name, "apply-result"):
		return ContractApplyResult, ""
	case strings.Contains(name, "audit-event"):
		return ContractAuditEvent, ""
	default:
		return ContractEnvelope, ""
	}
}

func TestDecodeRequestStrict(t *testing.T) {
	valid := `{"protocol_version":"1.0","request_id":"r1","operation":"protocol_info","payload_schema":"zconfig.empty/1","payload":{}}`
	request, err := DecodeRequest(strings.NewReader(valid), MaxMessageBytes)
	if err != nil || request.RequestID != "r1" {
		t.Fatalf("DecodeRequest() = %#v, %v", request, err)
	}

	invalid := []string{
		`{"protocol_version":"1.0","request_id":"r1","request_id":"r2","operation":"protocol_info","payload_schema":"zconfig.empty/1","payload":{}}`,
		`{"protocol_version":"1.0","request_id":"r1","operation":"protocol_info","payload_schema":"zconfig.empty/1","payload":{},"unknown":true}`,
		`{"protocol_version":"2.0","request_id":"r1","operation":"protocol_info","payload_schema":"zconfig.empty/1","payload":{}}`,
		valid + `{}`,
	}
	for _, input := range invalid {
		if _, err := DecodeRequest(strings.NewReader(input), MaxMessageBytes); err == nil {
			t.Errorf("DecodeRequest accepted %s", input)
		}
	}
}

func TestDecodeLimit(t *testing.T) {
	if _, err := DecodeRequest(bytes.NewReader(bytes.Repeat([]byte(" "), 33)), 32); err == nil {
		t.Fatal("oversized input accepted")
	}
}

func TestTypedRedactionAndSanitization(t *testing.T) {
	value := RedactedValue{Visibility: VisibilityRedacted, SecretRef: "s1", ValueType: ValueTypeString, Fingerprint: "hmac-sha256:" + strings.Repeat("a", 64)}
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := Sanitize([]byte("before hunter2 after"), []string{"hunter2"}); string(got) != "before [REDACTED] after" {
		t.Fatalf("unexpected sanitized output: %q", got)
	}
}

func TestDecodeResponseMatchesRequest(t *testing.T) {
	response := `{"protocol_version":"1.0","request_id":"other","ok":true,"result_schema":"zconfig.protocol-info/1","result":{}}`
	if _, err := DecodeResponse(strings.NewReader(response), MaxMessageBytes, "r1"); err == nil {
		t.Fatal("mismatched request id accepted")
	}
}
