package ui

import (
	"fmt"
	"strings"

	"github.com/hib2018/zconfig/tui/internal/review"
)

type ApprovalView struct {
	State       *review.State
	BulkPreview *review.BulkApprovalPreview
	FinalDiff   string
	ApplyArmed  bool
	Recovery    string
}

func (v *ApprovalView) ApproveSelected() error {
	item := v.State.SelectedItem()
	if item == nil {
		return fmt.Errorf("no selected change item")
	}
	return v.State.SetDecision(item.ChangeID, review.DecisionApproved)
}

func (v *ApprovalView) RejectSelected() error {
	item := v.State.SelectedItem()
	if item == nil {
		return fmt.Errorf("no selected change item")
	}
	return v.State.SetDecision(item.ChangeID, review.DecisionRejected)
}

func (v *ApprovalView) PrepareBulk() review.BulkApprovalPreview {
	preview := v.State.PrepareVisibleApproval()
	v.BulkPreview = &preview
	return preview
}

func (v *ApprovalView) ConfirmBulk() error {
	if v.BulkPreview == nil {
		return fmt.Errorf("bulk approval has not been previewed")
	}
	preview := *v.BulkPreview
	v.BulkPreview = nil
	return v.State.ConfirmVisibleApproval(preview)
}

func (v *ApprovalView) OpenFinal(diff string) error {
	readiness := v.State.Readiness()
	if !readiness.Ready {
		return fmt.Errorf("review is not ready")
	}
	v.FinalDiff, v.ApplyArmed = diff, false
	v.State.Mode = review.ModeApproval
	return nil
}

func (v *ApprovalView) ConfirmApply() error {
	if v.State.Mode != review.ModeApproval || v.FinalDiff == "" {
		return fmt.Errorf("final diff is not visible")
	}
	v.ApplyArmed = true
	return nil
}

func (v ApprovalView) Render() string {
	readiness := v.State.Readiness()
	var lines []string
	if v.BulkPreview != nil {
		lines = append(lines, "Bulk approval affects: "+strings.Join(v.BulkPreview.ChangeIDs, ", "))
	}
	if len(readiness.PendingChangeIDs) != 0 {
		lines = append(lines, "Pending: "+strings.Join(readiness.PendingChangeIDs, ", "))
	}
	if len(readiness.UnresolvedCommentIDs) != 0 {
		lines = append(lines, "Unresolved comments: "+strings.Join(readiness.UnresolvedCommentIDs, ", "))
	}
	if len(readiness.FailedCheckChangeIDs) != 0 {
		lines = append(lines, "Failed checks: "+strings.Join(readiness.FailedCheckChangeIDs, ", "))
	}
	if v.FinalDiff != "" {
		lines = append(lines, "Final diff:", v.FinalDiff, "Press y to arm the distinct apply confirmation.")
	}
	if v.ApplyArmed {
		lines = append(lines, "Apply confirmation armed")
	}
	if v.Recovery != "" {
		lines = append(lines, "Recovery: "+v.Recovery)
	}
	return strings.Join(lines, "\n")
}
