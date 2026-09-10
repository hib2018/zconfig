package review

import (
	"encoding/json"
	"testing"
	"time"
)

func TestChangeItemValuePresence(t *testing.T) {
	nullValue := json.RawMessage("null")
	item := ChangeItem{ChangeID: "c1", Path: "/enabled", Operation: OperationAdd, ProposedValue: &nullValue, Decision: DecisionPending}
	if err := item.Validate(); err != nil {
		t.Fatal(err)
	}
	item.ExpectedOld = &nullValue
	if err := item.Validate(); err == nil {
		t.Fatal("add with expected_old accepted")
	}
}

func TestSessionRejectsEphemeralLifecycle(t *testing.T) {
	s := Session{SessionSchema: "zconfig.review-session/1", ReviewID: "r", Lifecycle: LifecycleApplying, UpdatedAt: time.Now().UTC()}
	if err := s.Validate(); err == nil {
		t.Fatal("persisted applying state accepted")
	}
}
