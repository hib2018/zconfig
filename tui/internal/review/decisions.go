package review

import (
	"errors"
	"slices"
)

// BulkApprovalPreview is the exact set shown to the maintainer before a bulk
// decision. Confirmation fails atomically if the visible pending set changes.
type BulkApprovalPreview struct {
	ChangeIDs []string
}

// ReadinessResult explains why a review can or cannot proceed to final
// assembly. Unverified checks stay visible but do not block application.
type ReadinessResult struct {
	Ready                bool
	ApprovedChangeIDs    []string
	PendingChangeIDs     []string
	UnresolvedCommentIDs []string
	FailedCheckChangeIDs []string
}

func (s *State) SetDecision(changeID string, decision Decision) error {
	if decision != DecisionApproved && decision != DecisionRejected {
		return errors.New("decision must be approved or rejected")
	}
	for i := range s.Items {
		if s.Items[i].ChangeID == changeID {
			if s.Items[i].Decision != decision {
				s.Items[i].Decision = decision
				s.invalidateFinalConfirmation()
			}
			s.Readiness()
			return nil
		}
	}
	return errors.New("change item not found")
}

func (s *State) PrepareVisibleApproval() BulkApprovalPreview {
	visible := s.VisibleItems()
	ids := make([]string, 0, len(visible))
	for _, item := range visible {
		if item.Decision == DecisionPending {
			ids = append(ids, item.ChangeID)
		}
	}
	return BulkApprovalPreview{ChangeIDs: ids}
}

func (s *State) ConfirmVisibleApproval(preview BulkApprovalPreview) error {
	current := s.PrepareVisibleApproval()
	if len(preview.ChangeIDs) == 0 {
		return errors.New("bulk approval preview is empty")
	}
	if !slices.Equal(preview.ChangeIDs, current.ChangeIDs) {
		return errors.New("visible pending items changed; review them again")
	}
	wanted := make(map[string]struct{}, len(preview.ChangeIDs))
	for _, id := range preview.ChangeIDs {
		wanted[id] = struct{}{}
	}
	for i := range s.Items {
		if _, ok := wanted[s.Items[i].ChangeID]; ok {
			s.Items[i].Decision = DecisionApproved
		}
	}
	s.invalidateFinalConfirmation()
	s.Readiness()
	return nil
}

func (s *State) Readiness() ReadinessResult {
	result := ReadinessResult{}
	failed := make(map[string]struct{})
	for _, item := range s.Items {
		switch item.Decision {
		case DecisionApproved:
			result.ApprovedChangeIDs = append(result.ApprovedChangeIDs, item.ChangeID)
			for _, check := range item.Checks {
				if check.Status == CheckFailed {
					failed[item.ChangeID] = struct{}{}
				}
			}
		case DecisionRejected:
		default:
			result.PendingChangeIDs = append(result.PendingChangeIDs, item.ChangeID)
		}
	}
	for _, comment := range s.Comments {
		if comment.Status != CommentHumanConfirmed && s.isApproved(comment.ChangeID) {
			result.UnresolvedCommentIDs = append(result.UnresolvedCommentIDs, comment.CommentID)
		}
	}
	for _, item := range s.Items {
		if _, ok := failed[item.ChangeID]; ok {
			result.FailedCheckChangeIDs = append(result.FailedCheckChangeIDs, item.ChangeID)
		}
	}
	result.Ready = len(result.PendingChangeIDs) == 0 &&
		len(result.UnresolvedCommentIDs) == 0 &&
		len(result.FailedCheckChangeIDs) == 0
	if result.Ready {
		s.Lifecycle = LifecycleReady
	} else {
		s.Lifecycle = LifecycleReviewing
	}
	return result
}

func (s *State) isApproved(changeID string) bool {
	for _, item := range s.Items {
		if item.ChangeID == changeID {
			return item.Decision == DecisionApproved
		}
	}
	return false
}

func (s *State) invalidateFinalConfirmation() {
	s.FinalConfirmationDigest = ""
	s.Lifecycle = LifecycleReviewing
}
