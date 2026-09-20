package review

import (
	"testing"
	"time"
)

func TestSecretRevealShareExpiryFingerprintAndReset(t *testing.T) {
	now := time.Now()
	store := NewSecretStore()
	if err := store.Grant("a", "fp", SecretReveal, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := store.Authorize("a", "wrong", SecretReveal, now); err == nil {
		t.Fatal("wrong fingerprint accepted")
	}
	if err := store.Authorize("a", "fp", SecretReveal, now); err != nil {
		t.Fatal(err)
	}
	if err := store.Authorize("a", "fp", SecretReveal, now); err != nil {
		t.Fatal("reveal should remain valid in session")
	}
	if err := store.Grant("a", "fp", SecretShareOnce, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := store.Authorize("a", "fp", SecretShareOnce, now); err != nil {
		t.Fatal(err)
	}
	if err := store.Authorize("a", "fp", SecretShareOnce, now); err == nil {
		t.Fatal("share grant reused")
	}
	if err := store.Grant("b", "fp2", SecretReveal, now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := store.Authorize("b", "fp2", SecretReveal, now); err == nil {
		t.Fatal("expired reveal accepted")
	}
	store.Reset()
	if err := store.Authorize("a", "fp", SecretReveal, now); err == nil {
		t.Fatal("authorization survived reset")
	}
}

func TestFalsePositiveCanRemainConcealed(t *testing.T) {
	store := NewSecretStore()
	store.Conceal("tokenizer")
	if err := store.Authorize("tokenizer", "fp", SecretReveal, time.Now()); err == nil {
		t.Fatal("concealed suspected value was revealed")
	}
}
