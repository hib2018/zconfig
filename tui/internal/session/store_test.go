package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hib2018/zconfig/tui/internal/review"
)

const testDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func raw(value string) *json.RawMessage { v := json.RawMessage(value); return &v }

func testSession() review.Session {
	return review.Session{
		SessionSchema: "zconfig.review-session/1", ReviewID: "review-1",
		Source:         review.Source{Path: "/tmp/app.json", Digest: testDigest, ByteLength: 12, NodeCount: 2, RootType: "object"},
		ActiveProposal: review.Proposal{ProposalID: "proposal-1", Revision: 1, SourceDigest: testDigest, CreatedAt: time.Unix(1, 0).UTC(), Items: []review.ChangeItem{{ChangeID: "theme", Path: "/theme", Operation: review.OperationReplace, ExpectedOld: raw(`"light"`), ProposedValue: raw(`"dark"`), Decision: review.DecisionApproved}}},
		Comments:       []review.Comment{{CommentID: "comment-1", ChangeID: "theme", Body: "use dark", Status: review.CommentHumanConfirmed, CreatedAt: time.Unix(2, 0).UTC(), UpdatedAt: time.Unix(3, 0).UTC()}},
		Lifecycle:      review.LifecycleReady, UpdatedAt: time.Unix(4, 0).UTC(),
	}
}

func TestStoreRoundTripAndRestrictivePermissions(t *testing.T) {
	root := t.TempDir()
	want := testSession()
	if err := Save(root, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(root, want.ReviewID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ReviewID != want.ReviewID || got.Comments[0].Body != "use dark" || got.ActiveProposal.Items[0].Decision != review.DecisionApproved {
		t.Fatalf("round trip lost review state: %#v", got)
	}
	info, err := os.Stat(Path(root, want.ReviewID))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("session permissions = %o", info.Mode().Perm())
	}
}

func TestStoreRejectsSensitiveRawValues(t *testing.T) {
	s := testSession()
	s.ActiveProposal.Items[0].Sensitivity = review.SensitivitySchema
	if err := Save(t.TempDir(), s); err == nil {
		t.Fatal("persisted a sensitive raw value")
	}
}

func TestLoadRejectsUnknownVersionAndFields(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".zconfig", "reviews")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"old":    `{"session_schema":"zconfig.review-session/2"}`,
		"extra":  `{"session_schema":"zconfig.review-session/1","review_id":"extra","final_confirmation":"must-not-restore"}`,
		"grant":  `{"session_schema":"zconfig.review-session/1","review_id":"grant","authorized_secrets":{}}`,
		"output": `{"session_schema":"zconfig.review-session/1","review_id":"output","process_output":"secret"}`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name+".json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(root, name); err == nil {
			t.Fatalf("accepted %s session", name)
		}
	}
}

func TestAuditRejectsUnknownFieldsAndTruncatedLines(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".zconfig", "audit")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "review-1.jsonl")
	if err := os.WriteFile(path, []byte(`{"unknown":"field"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyAudit(path, "review-1"); err == nil {
		t.Fatal("invalid audit line passed")
	}
}

func TestAuditChainContainsNoValuesCommentsOrOutput(t *testing.T) {
	root := t.TempDir()
	a, err := OpenAudit(root, "review-1")
	if err != nil {
		t.Fatal(err)
	}
	first := AuditEvent{EventID: "event-1", OccurredAt: time.Unix(10, 0).UTC(), ActorType: "human", Action: "comment.created", ChangeIDs: []string{"theme"}, CommentIDs: []string{"comment-1"}, SourceDigest: testDigest}
	if err := a.Append(first); err != nil {
		t.Fatal(err)
	}
	second := AuditEvent{EventID: "event-2", OccurredAt: time.Unix(11, 0).UTC(), ActorType: "human", Action: "item.approved", ChangeIDs: []string{"theme"}, SourceDigest: testDigest}
	if err := a.Append(second); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(a.Path())
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"use dark", "light", "dark", "stdout", "stderr", "final_confirmation", "authorized_secrets"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("audit leaked %q: %s", forbidden, data)
		}
	}
	events, err := VerifyAudit(a.Path(), "review-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].PreviousEventDigest == nil || *events[1].PreviousEventDigest != events[0].EventDigest {
		t.Fatalf("invalid chain: %#v", events)
	}
	data[len(data)/2] ^= 1
	if err := os.WriteFile(a.Path(), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyAudit(a.Path(), "review-1"); err == nil {
		t.Fatal("tampered audit passed")
	}
}

func TestAuditFailureBlocksAuthoritativeAction(t *testing.T) {
	a := &Audit{sessionID: "review-1", appendLine: func([]byte) error { return os.ErrPermission }}
	called := false
	err := a.Authorize(AuditEvent{EventID: "event-1", OccurredAt: time.Now().UTC(), ActorType: "human", Action: "item.approved", ChangeIDs: []string{"theme"}, SourceDigest: testDigest}, func() error { called = true; return nil })
	if err == nil || called {
		t.Fatal("authoritative action ran after audit failure")
	}
}
