package review

import (
	"errors"
	"testing"
)

func TestStableTransitionsAutosaveAndSurfaceFailure(t *testing.T) {
	state := NewState([]ChangeItem{{ChangeID: "a", Operation: OperationAdd, ProposedValue: revisionRaw("1")}})
	calls := 0
	state.OnStableTransition = func(State) error { calls++; return nil }
	if err := state.SetDecision("a", DecisionApproved); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("autosave calls=%d", calls)
	}
	state.OnStableTransition = func(State) error { return errors.New("disk full") }
	if err := state.SetDecision("a", DecisionRejected); err == nil || state.Error == "" {
		t.Fatal("autosave failure was hidden")
	}
}

func TestRestoreRevalidatesSourceAndClearsConfirmation(t *testing.T) {
	session := Session{Source: Source{Digest: "sha256:source"}, ActiveProposal: Proposal{Items: []ChangeItem{{ChangeID: "a", Operation: OperationAdd, ProposedValue: revisionRaw("1"), Decision: DecisionApproved}}}, Lifecycle: LifecycleReady}
	if _, err := RestoreSession(session, "sha256:other"); err == nil {
		t.Fatal("stale source resumed")
	}
	state, err := RestoreSession(session, "sha256:source")
	if err != nil {
		t.Fatal(err)
	}
	if state.FinalConfirmationDigest != "" || state.Lifecycle != LifecycleReady {
		t.Fatalf("invalid restored state: %#v", state)
	}
}

func TestInterruptedApplyReconciliation(t *testing.T) {
	state := NewState(nil)
	state.FinalConfirmationDigest = "confirmed"
	if err := state.ReconcileInterruptedApply("before", "after", "after"); err != nil {
		t.Fatal(err)
	}
	if state.Lifecycle != LifecycleApplied || state.FinalConfirmationDigest != "" {
		t.Fatalf("applied state not reconciled: %#v", state)
	}
	if err := state.ReconcileInterruptedApply("before", "after", "unknown"); err == nil {
		t.Fatal("unknown source state accepted")
	}
}
