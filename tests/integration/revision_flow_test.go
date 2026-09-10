package integration

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/hib2018/zconfig/tui/internal/protocol"
	"github.com/hib2018/zconfig/tui/internal/review"
	"github.com/hib2018/zconfig/tui/internal/runner"
)

func TestAgentRevisionAcceptsScopedAndRejectsExpansion(t *testing.T) {
	agentPath := filepath.Join(t.TempDir(), "fixture-agent")
	build := exec.Command("go", "build", "-o", agentPath, "../fixtures/agent")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build agent: %v: %s", err, output)
	}
	one, two := json.RawMessage("1"), json.RawMessage("2")
	active := review.Proposal{ProposalID: "p", Revision: 1, SourceDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CreatedAt: time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC), Items: []review.ChangeItem{
		{ChangeID: "a", Path: "/a", Operation: review.OperationAdd, ProposedValue: &one, Explanation: "a"},
		{ChangeID: "b", Path: "/b", Operation: review.OperationAdd, ProposedValue: &two, Explanation: "b"},
	}}
	comments := []review.Comment{{CommentID: "c1", ChangeID: "a", Body: "revise", Status: review.CommentOpen}}
	request, err := protocol.BuildRevisionRequest("r1", active, comments, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		mode  string
		valid bool
	}{{"success", true}, {"scope-expansion", false}} {
		t.Run(tc.mode, func(t *testing.T) {
			agent := runner.Agent{Command: runner.Command{Executable: agentPath, Args: []string{"--mode", tc.mode}, Timeout: 5 * time.Second}}
			result, err := agent.Revise(context.Background(), request, nil)
			if err != nil {
				t.Fatal(err)
			}
			activeWire := protocol.AgentProposal{ProposalID: active.ProposalID, Revision: active.Revision, SourceDigest: active.SourceDigest, CreatedAt: active.CreatedAt.Format(time.RFC3339), Items: []protocol.AgentChangeItem{
				{ChangeID: "a", Path: "/a", Operation: review.OperationAdd, ProposedValue: &one, Explanation: "a"},
				{ChangeID: "b", Path: "/b", Operation: review.OperationAdd, ProposedValue: &two, Explanation: "b"},
			}}
			_, validationErr := protocol.ValidateRevisionScope(activeWire, result.Proposal, []string{"a"})
			if tc.valid && validationErr != nil {
				t.Fatal(validationErr)
			}
			if !tc.valid && validationErr == nil {
				t.Fatal("scope expansion accepted")
			}
		})
	}
}
