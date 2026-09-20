package integration

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hib2018/zconfig/tui/internal/protocol"
	"github.com/hib2018/zconfig/tui/internal/review"
	"github.com/hib2018/zconfig/tui/internal/runner"
	"github.com/hib2018/zconfig/tui/internal/session"
	"github.com/hib2018/zconfig/tui/internal/ui"
)

func TestDisclosedSecretDoesNotCrossOutputOrPersistenceBoundaries(t *testing.T) {
	const secret = "fixture-secret"
	rawSecret := json.RawMessage(`"` + secret + `"`)
	state := review.NewState([]review.ChangeItem{{
		ChangeID: "password", Path: "/auth/password", Operation: review.OperationReplace,
		ExpectedOld: &rawSecret, ProposedValue: &rawSecret, Explanation: "rotate",
		Sensitivity: review.SensitivitySuspected,
	}})
	if output := ui.NewReviewModel(state).View().Content; strings.Contains(output, secret) {
		t.Fatalf("TUI leaked secret: %s", output)
	}

	agentPath := filepath.Join(t.TempDir(), "fixture-agent")
	build := exec.Command("go", "build", "-o", agentPath, "../fixtures/agent")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fixture agent: %v: %s", err, output)
	}
	request := protocol.Request{
		ProtocolVersion: protocol.Version, RequestID: "secret-1", Operation: protocol.OperationReviseProposal,
		PayloadSchema: "zconfig.revision/1", Payload: json.RawMessage(`{}`),
	}
	_, err := (runner.Agent{Command: runner.Command{Executable: agentPath, Args: []string{"--mode", "secret-echo"}, Timeout: 5 * time.Second}}).Revise(context.Background(), request, []string{secret})
	if err == nil || strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("agent diagnostic was not remasked: %v", err)
	}

	s := review.Session{
		SessionSchema: session.SessionSchema, ReviewID: "secret-review",
		Source:         review.Source{Path: "/tmp/source.json", Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ByteLength: 2, NodeCount: 1, RootType: "object"},
		ActiveProposal: review.Proposal{ProposalID: "p", Revision: 1, SourceDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CreatedAt: time.Now().UTC(), Items: state.Items},
		Lifecycle:      review.LifecycleReviewing, UpdatedAt: time.Now().UTC(),
	}
	root := t.TempDir()
	if err := session.Save(root, s); err == nil {
		t.Fatal("session accepted a sensitive raw value")
	}
	if data, err := os.ReadFile(session.Path(root, s.ReviewID)); err == nil && strings.Contains(string(data), secret) {
		t.Fatal("session file leaked secret")
	}

	audit, err := session.OpenAudit(root, s.ReviewID)
	if err != nil {
		t.Fatal(err)
	}
	if err := audit.Append(session.AuditEvent{EventID: "e1", OccurredAt: time.Now().UTC(), ActorType: "system", Action: "secret.shared_once", ChangeIDs: []string{"password"}, SourceDigest: s.Source.Digest, OutcomeCode: "secret.shared"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(audit.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secret) {
		t.Fatalf("audit leaked secret: %s", data)
	}
}
