package ui

import (
	"strings"
	"testing"

	"github.com/hib2018/zconfig/tui/internal/review"
)

func TestApprovalControlsAndDistinctFinalConfirmation(t *testing.T) {
	state := fixtureState()
	view := ApprovalView{State: &state}
	if err := view.ApproveSelected(); err != nil {
		t.Fatal(err)
	}
	state.Move(1)
	if err := view.RejectSelected(); err != nil {
		t.Fatal(err)
	}
	if err := view.OpenFinal("- old\n+ new"); err != nil {
		t.Fatal(err)
	}
	if view.ApplyArmed {
		t.Fatal("item decisions armed apply")
	}
	if err := view.ConfirmApply(); err != nil {
		t.Fatal(err)
	}
	if !view.ApplyArmed || !strings.Contains(view.Render(), "Final diff") {
		t.Fatal("final confirmation was not distinct")
	}
}

func TestBulkApprovalShowsAffectedItemsAndDetectsScopeChange(t *testing.T) {
	state := fixtureState()
	view := ApprovalView{State: &state}
	preview := view.PrepareBulk()
	if len(preview.ChangeIDs) != 2 || !strings.Contains(view.Render(), "theme") {
		t.Fatal("bulk scope was not visible")
	}
	state.Filter = "password"
	if err := view.ConfirmBulk(); err == nil {
		t.Fatal("changed bulk scope was accepted")
	}
}

func TestApprovalReadinessBlockersAndRecoveryMessage(t *testing.T) {
	state := fixtureState()
	state.Items[0].Decision = review.DecisionApproved
	state.Items[0].Checks = []review.CheckResult{{Status: review.CheckFailed}}
	state.Items[1].Decision = review.DecisionRejected
	view := ApprovalView{State: &state, Recovery: "app.json.zconfig-recovery"}
	if err := view.OpenFinal("diff"); err == nil {
		t.Fatal("failed check did not block final preview")
	}
	out := view.Render()
	if !strings.Contains(out, "Failed checks") || !strings.Contains(out, "Recovery:") {
		t.Fatalf("missing blocker or recovery: %s", out)
	}
}
