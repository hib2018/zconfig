package ui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hib2018/zconfig/tui/internal/review"
)

func TestRevisionViewCommentProgressComparisonAndRecovery(t *testing.T) {
	one, two := json.RawMessage("1"), json.RawMessage("2")
	state := review.NewState([]review.ChangeItem{{ChangeID: "a", Path: "/a", Operation: review.OperationAdd, ProposedValue: &two}})
	view := RevisionView{State: &state, Previous: []review.ChangeItem{{ChangeID: "a", Path: "/a", Operation: review.OperationAdd, ProposedValue: &one}}}
	if err := view.AddComment("a", " use two "); err != nil {
		t.Fatal(err)
	}
	if out := view.Render(); !strings.Contains(out, "/a: 1 -> 2") || !strings.Contains(out, "[open]") {
		t.Fatalf("out=%s", out)
	}
	view.Busy = true
	if !strings.Contains(view.Render(), "progress") {
		t.Fatal("progress state missing")
	}
	view.Busy, view.Error = false, "timeout"
	if !strings.Contains(view.Render(), "unchanged") {
		t.Fatal("recoverable error missing")
	}
}
