package review

import (
	"encoding/json"
	"testing"
	"time"
)

func revisionRaw(value string) *json.RawMessage { raw := json.RawMessage(value); return &raw }

func TestCommentLifecycleRequiresHumanConfirmation(t *testing.T) {
	state := NewState([]ChangeItem{{ChangeID: "a", Operation: OperationAdd, ProposedValue: revisionRaw("1")}})
	now := time.Now()
	comment, err := state.AddComment("a", "make it two", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.ClaimResolved([]string{comment.CommentID}); err != nil {
		t.Fatal(err)
	}
	if state.Comments[0].Status != CommentAgentClaimed {
		t.Fatal("agent claim became confirmation")
	}
	if err := state.ConfirmComment(comment.CommentID); err != nil {
		t.Fatal(err)
	}
	if err := state.EditComment(comment.CommentID, "make it three", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if state.Comments[0].Status != CommentOpen {
		t.Fatal("edited comment did not reopen")
	}
	if err := state.WithdrawComment(comment.CommentID); err != nil || len(state.Comments) != 0 {
		t.Fatal("withdraw failed")
	}
}

func TestAcceptedRevisionResetsOnlyModifiedDecisions(t *testing.T) {
	state := NewState([]ChangeItem{
		{ChangeID: "a", Operation: OperationAdd, ProposedValue: revisionRaw("1"), Decision: DecisionApproved},
		{ChangeID: "b", Operation: OperationAdd, ProposedValue: revisionRaw("2"), Decision: DecisionRejected},
	})
	comment, _ := state.AddComment("a", "change", time.Now())
	next := append([]ChangeItem(nil), state.Items...)
	next[0].ProposedValue = revisionRaw("3")
	if err := state.ApplyAcceptedRevision(next, []string{"a"}, []string{comment.CommentID}); err != nil {
		t.Fatal(err)
	}
	if state.Items[0].Decision != DecisionPending || state.Items[1].Decision != DecisionRejected {
		t.Fatal("decision invalidation scope is wrong")
	}
	if state.Comments[0].Status != CommentAgentClaimed {
		t.Fatal("resolution claim missing")
	}
}
