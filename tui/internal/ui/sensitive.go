package ui

import (
	"fmt"
	"time"

	"github.com/hib2018/zconfig/tui/internal/review"
)

type SensitiveView struct {
	Store        *review.SecretStore
	PendingShare string
}

func (v SensitiveView) Label(item review.ChangeItem) string {
	switch item.Sensitivity {
	case review.SensitivitySchema:
		return "sensitive (schema)"
	case review.SensitivitySuspected:
		return "suspected sensitive"
	default:
		return "normal"
	}
}

func (v *SensitiveView) ConfirmReveal(changeID, fingerprint string, now time.Time) error {
	return v.Store.Grant(changeID, fingerprint, review.SecretReveal, now.Add(5*time.Minute))
}

func (v *SensitiveView) PrepareShare(changeID string) string {
	v.PendingShare = changeID
	return fmt.Sprintf("Share %s once with the registered agent?", changeID)
}

func (v *SensitiveView) ConfirmShare(changeID, fingerprint string, now time.Time) error {
	if v.PendingShare != changeID {
		return fmt.Errorf("share confirmation does not match selected item")
	}
	v.PendingShare = ""
	return v.Store.Grant(changeID, fingerprint, review.SecretShareOnce, now.Add(time.Minute))
}

func (v *SensitiveView) Conceal(changeID string) {
	v.Store.Conceal(changeID)
	if v.PendingShare == changeID {
		v.PendingShare = ""
	}
}
