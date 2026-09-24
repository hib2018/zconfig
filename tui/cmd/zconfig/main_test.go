package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hib2018/zconfig/tui/internal/protocol"
	"github.com/hib2018/zconfig/tui/internal/review"
)

func TestApplyInspectionAndSensitiveFallback(t *testing.T) {
	value := json.RawMessage(`"old"`)
	proposal := review.Proposal{Items: []review.ChangeItem{
		{Path: "/ui/theme", ExpectedOld: &value},
		{Path: "/auth/api_key", ExpectedOld: &value},
	}}
	applyInspection(&proposal, protocol.InspectionResult{Settings: []protocol.Setting{{Path: "/ui/theme", Sensitivity: "normal"}}})
	if proposal.Items[0].Sensitivity != review.SensitivityNormal {
		t.Fatal("normal classification lost")
	}
	if proposal.Items[1].Sensitivity != review.SensitivitySuspected {
		t.Fatal("sensitive-name fallback missing")
	}
	if sensitiveName("/ui/tokenizer") || !sensitiveName("/auth/API_Key") || !sensitiveName("/auth/api_token") {
		t.Fatal("sensitive-name matching is not conservative")
	}
}

func TestReviewUsageRequiresSourceAndProposal(t *testing.T) {
	err := runReview(nil)
	if err == nil || !strings.Contains(err.Error(), "usage:") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAutoReviewOptionsFindsRootFiles(t *testing.T) {
	root := t.TempDir()
	source := []byte(`{"ui":{"theme":"light"}}`)
	if err := os.WriteFile(filepath.Join(root, "app.json"), source, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app.schema.json"), []byte(`{"type":"object"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	proposal := `{"proposal_id":"p","revision":1,"source_digest":"` + digestBytes(source) + `","created_at":"2026-09-24T00:00:00Z","items":[{"change_id":"theme","path":"/ui/theme","operation":"replace","expected_old":"light","proposed_value":"dark","explanation":"change theme"}]}`
	if err := os.WriteFile(filepath.Join(root, "proposal.json"), []byte(proposal), 0o600); err != nil {
		t.Fatal(err)
	}
	options, err := autoReviewOptions(root, "core")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(options.proposal) != "proposal.json" || filepath.Base(options.source) != "app.json" || filepath.Base(options.schema) != "app.schema.json" || options.corePath != "core" {
		t.Fatalf("options=%+v", options)
	}
}

func TestParseReviewArgsSupportsDocumentedOrdering(t *testing.T) {
	options, err := parseReviewArgs([]string{"proposal.json", "--source", "app.json", "--schema", "app.schema.json", "--monochrome"})
	if err != nil {
		t.Fatal(err)
	}
	if options.proposal != "proposal.json" || options.source != "app.json" || options.schema != "app.schema.json" || !options.monochrome {
		t.Fatalf("options=%+v", options)
	}
}

func TestParseApplyArgsAndExitCodes(t *testing.T) {
	options, err := parseApplyArgs([]string{"proposal.json", "--source", "app.json", "--schema", "app.schema.json", "--session", "review-1", "--approve", "a", "--reject", "b", "--validator", "check", "--confirm-token", "token"})
	if err != nil {
		t.Fatal(err)
	}
	if options.proposal != "proposal.json" || options.source != "app.json" || options.schema != "app.schema.json" || options.sessionID != "review-1" || options.confirmToken != "token" || len(options.approved) != 1 || len(options.rejected) != 1 || len(options.validators) != 1 {
		t.Fatalf("options=%+v", options)
	}
	if exitCode(errConfirmationRequired) != 2 || exitCode(errValidationBlocked) != 3 || exitCode(errors.New("other")) != 1 {
		t.Fatal("stable exit codes changed")
	}
}

func TestApplyRequiresReviewSession(t *testing.T) {
	if _, err := parseApplyArgs([]string{"proposal.json", "--source", "app.json", "--approve", "a"}); err == nil || !strings.Contains(err.Error(), "--session") {
		t.Fatalf("missing session accepted: %v", err)
	}
}

func TestDecisionSetRequiresExactlyOneDecisionPerItem(t *testing.T) {
	proposal := review.Proposal{Items: []review.ChangeItem{{ChangeID: "a"}, {ChangeID: "b"}}}
	decisions, err := decisionSet(proposal, []string{"a"}, []string{"b"})
	if err != nil || decisions["a"] != "approved" || decisions["b"] != "rejected" {
		t.Fatalf("decisions=%v err=%v", decisions, err)
	}
	if _, err := decisionSet(proposal, []string{"a"}, nil); err == nil {
		t.Fatal("pending item accepted")
	}
	if _, err := decisionSet(proposal, []string{"a"}, []string{"a", "b"}); err == nil {
		t.Fatal("duplicate decision accepted")
	}
	if _, err := decisionSet(proposal, []string{"unknown"}, []string{"b"}); err == nil {
		t.Fatal("unknown item accepted")
	}
}
