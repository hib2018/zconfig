package ui

import (
	"encoding/json"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/hib2018/zconfig/tui/internal/review"
)

func raw(value string) *json.RawMessage { r := json.RawMessage(value); return &r }
func fixtureState() review.State {
	return review.NewState([]review.ChangeItem{
		{ChangeID: "theme", Path: "/ui/theme", Operation: review.OperationReplace, ExpectedOld: raw(`"light"`), ProposedValue: raw(`"dark"`), Explanation: "Use dark theme", Sensitivity: review.SensitivityNormal, Checks: []review.CheckResult{{Kind: "schema", Status: review.CheckPassed, Code: "schema.valid", Message: "valid"}}},
		{ChangeID: "password", Path: "/auth/password", Operation: review.OperationReplace, ExpectedOld: raw(`"hunter2"`), ProposedValue: raw(`"new-secret"`), Explanation: "Rotate secret", Sensitivity: review.SensitivitySuspected, Checks: []review.CheckResult{{Kind: "schema", Status: review.CheckUnverified, Code: "schema.missing", Message: "not configured"}}},
	})
}

func TestReviewNavigationModesAndResize(t *testing.T) {
	m := NewReviewModel(fixtureState())
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	if m.State.Selected != 1 {
		t.Fatalf("selected=%d", m.State.Selected)
	}
	m.Update(tea.KeyPressMsg{Text: "enter", Code: tea.KeyEnter})
	if m.State.Mode != review.ModeDetail {
		t.Fatal("detail did not open")
	}
	m.Update(tea.KeyPressMsg{Text: "d", Code: 'd'})
	if m.State.Mode != review.ModeDiff {
		t.Fatal("diff did not open")
	}
	m.Update(tea.KeyPressMsg{Text: "?", Code: '?'})
	if m.State.Mode != review.ModeHelp {
		t.Fatal("help did not open")
	}
}

func TestFilterAndCheckLabels(t *testing.T) {
	m := NewReviewModel(fixtureState())
	m.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	for _, r := range "password" {
		m.Update(tea.KeyPressMsg{Text: string(r), Code: r})
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	out := m.View().Content
	if strings.Contains(out, "/ui/theme") || !strings.Contains(out, "/auth/password") {
		t.Fatalf("filter output: %s", out)
	}
	if !strings.Contains(out, "UNVERIFIED") {
		t.Fatalf("missing check label: %s", out)
	}
}

func TestSensitiveValuesRedactedBeforeFirstRender(t *testing.T) {
	view := NewReviewModel(fixtureState()).View()
	if !view.AltScreen {
		t.Fatal("review TUI should use alt screen")
	}
	out := view.Content
	if strings.Contains(out, "hunter2") || strings.Contains(out, "new-secret") {
		t.Fatalf("secret rendered: %s", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("redaction missing: %s", out)
	}
}

func TestNarrowMonochromeAndEmptyErrorStates(t *testing.T) {
	m := NewReviewModel(fixtureState())
	m.Monochrome = true
	m.Update(tea.WindowSizeMsg{Width: 24, Height: 8})
	out := m.View().Content
	for _, line := range strings.Split(out, "\n") {
		if len([]rune(line)) > 24 {
			t.Fatalf("line too wide: %q", line)
		}
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatal("monochrome view contains ANSI")
	}
	empty := NewReviewModel(review.NewState(nil)).View().Content
	if !strings.Contains(empty, "No change items") {
		t.Fatalf("empty=%s", empty)
	}
	state := fixtureState()
	state.Error = "proposal is stale"
	if out := NewReviewModel(state).View().Content; !strings.Contains(out, "proposal is stale") {
		t.Fatalf("error=%s", out)
	}
}

func TestScroll(t *testing.T) {
	items := make([]review.ChangeItem, 30)
	for i := range items {
		items[i] = review.ChangeItem{ChangeID: "item", Path: "/item", Operation: review.OperationAdd, ProposedValue: raw("1")}
	}
	m := NewReviewModel(review.NewState(items))
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 8})
	for range 20 {
		m.Update(tea.KeyPressMsg{Text: "j", Code: 'j'})
	}
	if m.State.Offset == 0 {
		t.Fatal("viewport did not scroll")
	}
}
