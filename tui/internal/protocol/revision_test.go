package protocol

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hib2018/zconfig/tui/internal/review"
)

func TestBuildRevisionRequestContainsOnlyAuthorizedTargets(t *testing.T) {
	value := json.RawMessage("1")
	proposal := review.Proposal{ProposalID: "p", Revision: 2, SourceDigest: "sha256:" + string(make([]byte, 64)), CreatedAt: time.Now(), Items: []review.ChangeItem{{ChangeID: "b", Path: "/b", Operation: review.OperationAdd, ProposedValue: &value}, {ChangeID: "a", Path: "/a", Operation: review.OperationAdd, ProposedValue: &value}}}
	comments := []review.Comment{{CommentID: "c1", ChangeID: "b", Body: "change b", Status: review.CommentOpen}}
	request, err := BuildRevisionRequest("r1", proposal, comments, nil)
	if err != nil {
		t.Fatal(err)
	}
	var payload RevisionRequestPayload
	if err := json.Unmarshal(request.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.AllowedChangeItemIDs) != 1 || payload.AllowedChangeItemIDs[0] != "b" {
		t.Fatalf("allowed=%v", payload.AllowedChangeItemIDs)
	}
	if len(payload.Proposal.Items) != 2 {
		t.Fatal("proposal context was truncated")
	}
}
