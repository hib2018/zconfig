package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"

	"github.com/hib2018/zconfig/tui/internal/review"
)

type RevisionRequestPayload struct {
	ProposalID           string            `json:"proposal_id"`
	BaseRevision         int               `json:"base_revision"`
	SourceDigest         string            `json:"source_digest"`
	AllowedChangeItemIDs []string          `json:"allowed_change_item_ids"`
	Comments             []RevisionComment `json:"comments"`
	Proposal             AgentProposal     `json:"proposal"`
	SourceContext        json.RawMessage   `json:"source_context,omitempty"`
}

type RevisionResultPayload struct {
	ProposalID         string        `json:"proposal_id"`
	BaseRevision       int           `json:"base_revision"`
	SourceDigest       string        `json:"source_digest"`
	Proposal           AgentProposal `json:"proposal"`
	ResolvedCommentIDs []string      `json:"resolved_comment_ids"`
}

type RevisionComment struct {
	CommentID string `json:"comment_id"`
	ChangeID  string `json:"change_id"`
	Body      string `json:"body"`
}

type AgentProposal struct {
	ProposalID   string            `json:"proposal_id"`
	Revision     int               `json:"revision"`
	SourceDigest string            `json:"source_digest"`
	CreatedAt    string            `json:"created_at"`
	Producer     string            `json:"producer,omitempty"`
	Items        []AgentChangeItem `json:"items"`
}

type AgentChangeItem struct {
	ChangeID      string           `json:"change_id"`
	Path          string           `json:"path"`
	Operation     review.Operation `json:"operation"`
	ExpectedOld   *json.RawMessage `json:"expected_old,omitempty"`
	ProposedValue *json.RawMessage `json:"proposed_value,omitempty"`
	Explanation   string           `json:"explanation"`
}

func BuildRevisionRequest(requestID string, proposal review.Proposal, comments []review.Comment, sourceContext json.RawMessage) (Request, error) {
	if requestID == "" || len(comments) == 0 {
		return Request{}, errors.New("revision requires request id and comments")
	}
	allowedSet := map[string]bool{}
	requestComments := make([]RevisionComment, 0, len(comments))
	for _, comment := range comments {
		if comment.Status != review.CommentOpen || comment.Body == "" {
			return Request{}, errors.New("only open non-empty comments can be submitted")
		}
		allowedSet[comment.ChangeID] = true
		requestComments = append(requestComments, RevisionComment{CommentID: comment.CommentID, ChangeID: comment.ChangeID, Body: comment.Body})
	}
	allowed := make([]string, 0, len(allowedSet))
	for id := range allowedSet {
		allowed = append(allowed, id)
	}
	sort.Strings(allowed)
	payload := RevisionRequestPayload{
		ProposalID: proposal.ProposalID, BaseRevision: proposal.Revision, SourceDigest: proposal.SourceDigest,
		AllowedChangeItemIDs: allowed, Comments: requestComments, Proposal: toAgentProposal(proposal), SourceContext: sourceContext,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return Request{}, err
	}
	return Request{ProtocolVersion: Version, RequestID: requestID, Operation: OperationReviseProposal, PayloadSchema: "zconfig.revision/1", Payload: encoded}, nil
}

func toAgentProposal(value review.Proposal) AgentProposal {
	items := make([]AgentChangeItem, len(value.Items))
	for i, item := range value.Items {
		items[i] = AgentChangeItem{ChangeID: item.ChangeID, Path: item.Path, Operation: item.Operation, ExpectedOld: item.ExpectedOld, ProposedValue: item.ProposedValue, Explanation: item.Explanation}
	}
	return AgentProposal{ProposalID: value.ProposalID, Revision: value.Revision, SourceDigest: value.SourceDigest, CreatedAt: value.CreatedAt.Format("2006-01-02T15:04:05Z07:00"), Producer: value.Producer, Items: items}
}

func ValidateRevisionScope(active, candidate AgentProposal, allowedIDs []string) ([]string, error) {
	if active.ProposalID != candidate.ProposalID || active.SourceDigest != candidate.SourceDigest || candidate.Revision != active.Revision+1 || len(active.Items) != len(candidate.Items) {
		return nil, errors.New("revision base changed")
	}
	allowed := make(map[string]bool, len(allowedIDs))
	for _, id := range allowedIDs {
		allowed[id] = true
	}
	modified := make([]string, 0)
	for i := range active.Items {
		before, after := active.Items[i], candidate.Items[i]
		if before.ChangeID != after.ChangeID || before.Path != after.Path || before.Operation != after.Operation || !rawEqual(before.ExpectedOld, after.ExpectedOld) {
			return nil, errors.New("revision changed immutable fields")
		}
		changed := !rawEqual(before.ProposedValue, after.ProposedValue) || before.Explanation != after.Explanation
		if changed && !allowed[before.ChangeID] {
			return nil, errors.New("revision changed an uncommented item")
		}
		if changed {
			modified = append(modified, before.ChangeID)
		}
	}
	return modified, nil
}

func rawEqual(a, b *json.RawMessage) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	var left, right any
	if json.Unmarshal(*a, &left) != nil || json.Unmarshal(*b, &right) != nil {
		return bytes.Equal(*a, *b)
	}
	l, _ := json.Marshal(left)
	r, _ := json.Marshal(right)
	return bytes.Equal(l, r)
}
