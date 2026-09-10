package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/hib2018/zconfig/tui/internal/review"
)

type RevisionView struct {
	State    *review.State
	Previous []review.ChangeItem
	Busy     bool
	Error    string
}

func (v *RevisionView) AddComment(changeID, body string) error {
	_, err := v.State.AddComment(changeID, strings.TrimSpace(body), time.Now())
	return err
}

func (v *RevisionView) ConfirmResolution(commentID string) error {
	return v.State.ConfirmComment(commentID)
}

func (v RevisionView) Render() string {
	if v.Error != "" {
		return "Revision error: " + v.Error + "\nRetry is available; the active proposal is unchanged."
	}
	if v.Busy {
		return "Agent revision in progress…\nCancel to keep the active proposal."
	}
	var lines []string
	for i, item := range v.State.Items {
		before := "—"
		if i < len(v.Previous) {
			before = displayValue(v.Previous[i].ProposedValue, v.Previous[i].Sensitivity)
		}
		after := displayValue(item.ProposedValue, item.Sensitivity)
		if before != after {
			lines = append(lines, fmt.Sprintf("%s: %s -> %s", item.Path, before, after))
		}
	}
	for _, comment := range v.State.Comments {
		lines = append(lines, fmt.Sprintf("[%s] %s", comment.Status, comment.ChangeID))
	}
	if len(lines) == 0 {
		return "No revised change items"
	}
	return strings.Join(lines, "\n")
}
