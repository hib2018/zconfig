package review

import (
	"errors"
	"fmt"
	"time"
)

func (s *State) AddComment(changeID, body string, now time.Time) (Comment, error) {
	if body == "" || !s.hasChange(changeID) {
		return Comment{}, errors.New("comment requires an existing change and non-empty body")
	}
	comment := Comment{CommentID: fmt.Sprintf("comment-%d", len(s.Comments)+1), ChangeID: changeID, Body: body, Status: CommentOpen, CreatedAt: now, UpdatedAt: now}
	s.Comments = append(s.Comments, comment)
	return comment, nil
}

func (s *State) EditComment(commentID, body string, now time.Time) error {
	if body == "" {
		return errors.New("comment body is empty")
	}
	comment := s.comment(commentID)
	if comment == nil {
		return errors.New("comment not found")
	}
	comment.Body, comment.UpdatedAt, comment.Status = body, now, CommentOpen
	return nil
}

func (s *State) WithdrawComment(commentID string) error {
	for i := range s.Comments {
		if s.Comments[i].CommentID == commentID {
			s.Comments = append(s.Comments[:i], s.Comments[i+1:]...)
			return nil
		}
	}
	return errors.New("comment not found")
}

func (s *State) ClaimResolved(commentIDs []string) error {
	for _, id := range commentIDs {
		comment := s.comment(id)
		if comment == nil || comment.Status != CommentOpen {
			return errors.New("invalid resolution claim")
		}
		comment.Status = CommentAgentClaimed
	}
	return nil
}

func (s *State) ConfirmComment(commentID string) error {
	comment := s.comment(commentID)
	if comment == nil || comment.Status != CommentAgentClaimed {
		return errors.New("comment is not agent-claimed")
	}
	comment.Status = CommentHumanConfirmed
	return nil
}

func (s *State) ApplyAcceptedRevision(items []ChangeItem, modifiedIDs, resolvedCommentIDs []string) error {
	if len(items) != len(s.Items) {
		return errors.New("revision item count changed")
	}
	modified := make(map[string]bool, len(modifiedIDs))
	for _, id := range modifiedIDs {
		modified[id] = true
	}
	for i := range items {
		if items[i].ChangeID != s.Items[i].ChangeID {
			return errors.New("revision identity changed")
		}
		if modified[items[i].ChangeID] {
			items[i].Decision = DecisionPending
		} else {
			items[i].Decision = s.Items[i].Decision
		}
	}
	if err := s.ClaimResolved(resolvedCommentIDs); err != nil {
		return err
	}
	s.Items = append([]ChangeItem(nil), items...)
	return nil
}

func (s *State) hasChange(id string) bool {
	for _, item := range s.Items {
		if item.ChangeID == id {
			return true
		}
	}
	return false
}

func (s *State) comment(id string) *Comment {
	for i := range s.Comments {
		if s.Comments[i].CommentID == id {
			return &s.Comments[i]
		}
	}
	return nil
}
