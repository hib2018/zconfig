package main

import (
	"encoding/json"
	"errors"
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
	options, err := parseApplyArgs([]string{"proposal.json", "--source", "app.json", "--schema", "app.schema.json", "--approve", "a", "--reject", "b", "--validator", "check", "--confirm"})
	if err != nil {
		t.Fatal(err)
	}
	if options.proposal != "proposal.json" || options.source != "app.json" || options.schema != "app.schema.json" || len(options.approved) != 1 || len(options.rejected) != 1 || len(options.validators) != 1 || !options.confirm {
		t.Fatalf("options=%+v", options)
	}
	if exitCode(errConfirmationRequired) != 2 || exitCode(errValidationBlocked) != 3 || exitCode(errors.New("other")) != 1 {
		t.Fatal("stable exit codes changed")
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
