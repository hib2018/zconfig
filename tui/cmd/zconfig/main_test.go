package main

import (
	"encoding/json"
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
	if sensitiveName("/ui/tokenizer") || !sensitiveName("/auth/API_Key") {
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
