package review

import (
	"reflect"
	"testing"
	"time"
)

func decisionItems() []ChangeItem {
	return []ChangeItem{
		{ChangeID: "color", Path: "/ui/color", Operation: OperationReplace, ExpectedOld: revisionRaw(`"blue"`), ProposedValue: revisionRaw(`"navy"`)},
		{ChangeID: "spacing", Path: "/ui/spacing", Operation: OperationReplace, ExpectedOld: revisionRaw("8"), ProposedValue: revisionRaw("12")},
		{ChangeID: "timeout", Path: "/network/timeout", Operation: OperationReplace, ExpectedOld: revisionRaw("10"), ProposedValue: revisionRaw("20")},
	}
}

func TestSetDecisionAndReadiness(t *testing.T) {
	state := NewState(decisionItems())
	state.FinalConfirmationDigest = "sha256:stale"
	if err := state.SetDecision("color", DecisionApproved); err != nil {
		t.Fatal(err)
	}
	if state.FinalConfirmationDigest != "" {
		t.Fatal("decision change did not invalidate final confirmation")
	}
	if err := state.SetDecision("spacing", DecisionRejected); err != nil {
		t.Fatal(err)
	}
	if err := state.SetDecision("timeout", DecisionApproved); err != nil {
		t.Fatal(err)
	}

	readiness := state.Readiness()
	if !readiness.Ready || !reflect.DeepEqual(readiness.ApprovedChangeIDs, []string{"color", "timeout"}) {
		t.Fatalf("unexpected readiness: %#v", readiness)
	}
	if state.Lifecycle != LifecycleReady {
		t.Fatalf("lifecycle = %q, want ready", state.Lifecycle)
	}
}

func TestReadinessBlocksPendingFailedChecksAndApprovedUnresolvedComments(t *testing.T) {
	state := NewState(decisionItems())
	_ = state.SetDecision("color", DecisionApproved)
	_ = state.SetDecision("spacing", DecisionRejected)
	comment, err := state.AddComment("color", "darker", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	state.Items[0].Checks = []CheckResult{{Kind: "schema", Status: CheckFailed, Code: "schema.minimum"}}

	readiness := state.Readiness()
	if readiness.Ready {
		t.Fatal("state with blockers was ready")
	}
	if !reflect.DeepEqual(readiness.PendingChangeIDs, []string{"timeout"}) ||
		!reflect.DeepEqual(readiness.UnresolvedCommentIDs, []string{comment.CommentID}) ||
		!reflect.DeepEqual(readiness.FailedCheckChangeIDs, []string{"color"}) {
		t.Fatalf("unexpected blockers: %#v", readiness)
	}
	if state.Lifecycle != LifecycleReviewing {
		t.Fatalf("lifecycle = %q, want reviewing", state.Lifecycle)
	}
}

func TestBulkApprovalRequiresExactVisiblePreview(t *testing.T) {
	state := NewState(decisionItems())
	state.Filter = "ui/"
	preview := state.PrepareVisibleApproval()
	if !reflect.DeepEqual(preview.ChangeIDs, []string{"color", "spacing"}) {
		t.Fatalf("preview = %#v", preview.ChangeIDs)
	}

	state.Filter = "timeout"
	if err := state.ConfirmVisibleApproval(preview); err == nil {
		t.Fatal("stale bulk preview was accepted")
	}
	for _, item := range state.Items {
		if item.Decision != DecisionPending {
			t.Fatal("stale preview changed a decision")
		}
	}

	preview = state.PrepareVisibleApproval()
	if err := state.ConfirmVisibleApproval(preview); err != nil {
		t.Fatal(err)
	}
	if state.Items[2].Decision != DecisionApproved || state.Items[0].Decision != DecisionPending {
		t.Fatalf("bulk approval escaped visible scope: %#v", state.Items)
	}
}

func TestEveryRejectedIsReadyWithEmptyApprovedSet(t *testing.T) {
	state := NewState(decisionItems())
	for _, item := range state.Items {
		if err := state.SetDecision(item.ChangeID, DecisionRejected); err != nil {
			t.Fatal(err)
		}
	}
	readiness := state.Readiness()
	if !readiness.Ready || len(readiness.ApprovedChangeIDs) != 0 {
		t.Fatalf("all-rejected review should be decided: %#v", readiness)
	}
}
