package ui

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hib2018/zconfig/tui/internal/review"
)

func benchmarkState(count int) review.State {
	items := make([]review.ChangeItem, count)
	for i := range items {
		oldValue, newValue := json.RawMessage(fmt.Sprintf("%d", i)), json.RawMessage(fmt.Sprintf("%d", i+1))
		items[i] = review.ChangeItem{ChangeID: fmt.Sprintf("c%04d", i), Path: fmt.Sprintf("/settings/%d", i), Operation: review.OperationReplace, ExpectedOld: &oldValue, ProposedValue: &newValue, Explanation: "benchmark", Sensitivity: review.SensitivityNormal}
	}
	state := review.NewState(items)
	state.Width, state.Height = 100, 30
	return state
}

func BenchmarkNavigation1000Changes(b *testing.B) {
	state := benchmarkState(1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state.Move(1)
		_ = state.VisibleItems()
	}
}

func BenchmarkRender1000Changes(b *testing.B) {
	model := NewReviewModel(benchmarkState(1000))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = model.View().Content
	}
}
