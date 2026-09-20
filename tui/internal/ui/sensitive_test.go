package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/hib2018/zconfig/tui/internal/review"
)

func TestSensitiveLabelsRevealShareAndConceal(t *testing.T) {
	store := review.NewSecretStore()
	view := SensitiveView{Store: store}
	if !strings.Contains(view.Label(review.ChangeItem{Sensitivity: review.SensitivitySuspected}), "suspected") {
		t.Fatal("suspected label missing")
	}
	now := time.Now()
	if err := view.ConfirmReveal("a", "fp", now); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(view.PrepareShare("a"), "once") {
		t.Fatal("one-use warning missing")
	}
	if err := view.ConfirmShare("a", "fp", now); err != nil {
		t.Fatal(err)
	}
	if err := store.Authorize("a", "fp", review.SecretShareOnce, now); err != nil {
		t.Fatal(err)
	}
	if err := store.Authorize("a", "fp", review.SecretShareOnce, now); err == nil {
		t.Fatal("share reused")
	}
	view.Conceal("a")
	if err := store.Authorize("a", "fp", review.SecretReveal, now); err == nil {
		t.Fatal("conceal did not revoke reveal")
	}
}
