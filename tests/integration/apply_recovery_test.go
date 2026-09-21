package integration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hib2018/zconfig/tui/internal/review"
	"github.com/hib2018/zconfig/tui/internal/session"
)

type coreResponse struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code string `json:"code"`
	} `json:"error"`
}

type finalPreview struct {
	FinalChangeDigest string   `json:"final_change_digest"`
	SourceDigest      string   `json:"source_digest"`
	ApprovedIDs       []string `json:"approved_change_item_ids"`
	Diff              string   `json:"diff"`
	Nonce             string   `json:"confirmation_nonce"`
}

func TestSubsetApprovalResumeStaleSourceAndRecovery(t *testing.T) {
	core := buildCore(t)
	root := t.TempDir()
	source := []byte("{\"a\":1,\"b\":2}\n")
	if err := os.WriteFile(filepath.Join(root, "config.json"), source, 0o640); err != nil {
		t.Fatal(err)
	}
	digest := shaDigest(source)
	proposal := map[string]any{
		"proposal_id": "p1", "revision": 1, "source_digest": digest, "created_at": "2026-09-20T00:00:00Z",
		"items": []map[string]any{
			{"change_id": "a", "path": "/a", "operation": "replace", "expected_old": 1, "proposed_value": 10, "explanation": "change a"},
			{"change_id": "b", "path": "/b", "operation": "replace", "expected_old": 2, "proposed_value": 20, "explanation": "change b"},
		},
	}
	finalPayload := map[string]any{"source_path": "config.json", "source_digest": digest, "proposal": proposal, "decisions": map[string]string{"a": "approved", "b": "rejected"}, "comments": []any{}, "external_checks": []any{}}
	preview := assemble(t, core, root, finalPayload)
	if len(preview.ApprovedIDs) != 1 || preview.ApprovedIDs[0] != "a" {
		t.Fatalf("approved subset = %#v", preview.ApprovedIDs)
	}

	saved := review.Session{
		SessionSchema: session.SessionSchema, ReviewID: "resume-1",
		Source:         review.Source{Path: "config.json", Digest: digest, ByteLength: int64(len(source)), NodeCount: 3, RootType: "object"},
		ActiveProposal: review.Proposal{ProposalID: "p1", Revision: 1, SourceDigest: digest, CreatedAt: time.Now().UTC(), Items: []review.ChangeItem{{ChangeID: "a", Path: "/a", Operation: review.OperationReplace, Decision: review.DecisionApproved}, {ChangeID: "b", Path: "/b", Operation: review.OperationReplace, Decision: review.DecisionRejected}}},
		Lifecycle:      review.LifecycleReady, UpdatedAt: time.Now().UTC(),
	}
	if err := session.Save(root, saved); err != nil {
		t.Fatal(err)
	}
	loaded, err := session.Load(root, saved.ReviewID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := review.RestoreSession(loaded, digest); err != nil {
		t.Fatalf("resume: %v", err)
	}

	stalePayload := cloneMap(t, finalPayload)
	stalePreview := assemble(t, core, root, stalePayload)
	if err := os.WriteFile(filepath.Join(root, "config.json"), []byte("{\"a\":1,\"b\":3}\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if response := apply(t, core, root, stalePayload, stalePreview); response.OK || response.Error == nil || response.Error.Code != "apply.rejected" {
		t.Fatalf("stale apply response = %+v", response)
	}

	if err := os.WriteFile(filepath.Join(root, "config.json"), source, 0o640); err != nil {
		t.Fatal(err)
	}
	preview = assemble(t, core, root, finalPayload)
	if response := apply(t, core, root, finalPayload, preview); !response.OK {
		t.Fatalf("apply failed: %+v", response)
	}
	got, err := os.ReadFile(filepath.Join(root, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte("{\"a\":10,\"b\":2}\n")) {
		t.Fatalf("subset result = %q", got)
	}
	recovery, err := os.ReadFile(filepath.Join(root, "config.json.zconfig-recovery"))
	if err != nil || !bytes.Equal(recovery, source) {
		t.Fatalf("recovery = %q, %v", recovery, err)
	}
	if response := apply(t, core, root, finalPayload, preview); response.OK {
		t.Fatal("one-use confirmation nonce was accepted twice")
	}

	secretSource := []byte("{\"visible\":\"light\",\"credential\":\"old-schema-secret\",\"api_token\":\"old-name-secret\"}\n")
	if err := os.WriteFile(filepath.Join(root, "secrets.json"), secretSource, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "secrets.schema.json"), []byte(`{"type":"object","properties":{"credential":{"type":"string","x-zconfig-sensitive":true}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	secretDigest := shaDigest(secretSource)
	secretProposal := map[string]any{
		"proposal_id": "secret-preview", "revision": 1, "source_digest": secretDigest, "created_at": "2026-09-20T00:00:00Z",
		"items": []map[string]any{
			{"change_id": "visible", "path": "/visible", "operation": "replace", "expected_old": "light", "proposed_value": "dark", "explanation": "visible"},
			{"change_id": "credential", "path": "/credential", "operation": "replace", "expected_old": "old-schema-secret", "proposed_value": "new-schema-secret", "explanation": "schema sensitive"},
			{"change_id": "token", "path": "/api_token", "operation": "replace", "expected_old": "old-name-secret", "proposed_value": "new-name-secret", "explanation": "name sensitive"},
		},
	}
	secretPayload := map[string]any{"source_path": "secrets.json", "schema_path": "secrets.schema.json", "source_digest": secretDigest, "proposal": secretProposal, "decisions": map[string]string{"visible": "approved", "credential": "approved", "token": "approved"}, "comments": []any{}, "external_checks": []any{}}
	secretPreview := assemble(t, core, root, secretPayload)
	for _, forbidden := range []string{"old-schema-secret", "new-schema-secret", "old-name-secret", "new-name-secret"} {
		if strings.Contains(secretPreview.Diff, forbidden) {
			t.Fatalf("final preview leaked %q: %s", forbidden, secretPreview.Diff)
		}
	}
	if !strings.Contains(secretPreview.Diff, `- "light"`) || !strings.Contains(secretPreview.Diff, `+ "dark"`) || strings.Count(secretPreview.Diff, "[REDACTED]") != 4 {
		t.Fatalf("unexpected redacted final diff: %s", secretPreview.Diff)
	}
}

func TestFinalCandidateSchemaIsRevalidatedBeforePreviewAndApply(t *testing.T) {
	core := buildCore(t)
	root := t.TempDir()
	source := []byte("{\"count\":1}\n")
	if err := os.WriteFile(filepath.Join(root, "config.json"), source, 0o600); err != nil {
		t.Fatal(err)
	}
	proposal := map[string]any{
		"proposal_id": "schema-final", "revision": 1, "source_digest": shaDigest(source), "created_at": "2026-09-20T00:00:00Z",
		"items": []map[string]any{{"change_id": "count", "path": "/count", "operation": "replace", "expected_old": 1, "proposed_value": 10, "explanation": "count"}},
	}
	payload := map[string]any{"source_path": "config.json", "schema_path": "schema.json", "source_digest": shaDigest(source), "proposal": proposal, "decisions": map[string]string{"count": "approved"}, "comments": []any{}, "external_checks": []any{}}
	if err := os.WriteFile(filepath.Join(root, "schema.json"), []byte(`{"type":"object","properties":{"count":{"type":"integer","maximum":5}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if response := callCore(t, core, root, "assemble_final", "zconfig.final-change/1", payload); response.OK {
		t.Fatal("schema-invalid complete candidate reached preview")
	}
	if err := os.WriteFile(filepath.Join(root, "schema.json"), []byte(`{"type":"object","properties":{"count":{"type":"integer","maximum":10}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	preview := assemble(t, core, root, payload)
	if err := os.WriteFile(filepath.Join(root, "schema.json"), []byte(`{"type":"object","properties":{"count":{"type":"integer","maximum":5}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if response := apply(t, core, root, payload, preview); response.OK {
		t.Fatal("candidate was not revalidated after schema changed")
	}
	if got, err := os.ReadFile(filepath.Join(root, "config.json")); err != nil || !bytes.Equal(got, source) {
		t.Fatalf("schema rejection changed source: %q %v", got, err)
	}
}

func buildCore(t *testing.T) string {
	t.Helper()
	prefix := t.TempDir()
	command := exec.Command("zig", "build", "-Doptimize=ReleaseSafe", "--prefix", prefix, "--cache-dir", filepath.Join(prefix, "cache"), "--global-cache-dir", filepath.Join(prefix, "global-cache"))
	command.Dir = filepath.Join("..", "..")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build core: %v: %s", err, output)
	}
	return filepath.Join(prefix, "bin", "zconfig-core")
}

func assemble(t *testing.T, core, dir string, payload map[string]any) finalPreview {
	t.Helper()
	response := callCore(t, core, dir, "assemble_final", "zconfig.final-change/1", payload)
	if !response.OK {
		t.Fatalf("assemble failed: %+v", response)
	}
	var preview finalPreview
	if err := json.Unmarshal(response.Result, &preview); err != nil {
		t.Fatal(err)
	}
	return preview
}

func apply(t *testing.T, core, dir string, payload map[string]any, preview finalPreview) coreResponse {
	t.Helper()
	request := cloneMap(t, payload)
	request["final_change_digest"] = preview.FinalChangeDigest
	request["confirmation_nonce"] = preview.Nonce
	return callCore(t, core, dir, "apply_final", "zconfig.apply/1", request)
}

func callCore(t *testing.T, core, dir, operation, schema string, payload any) coreResponse {
	t.Helper()
	request := map[string]any{"protocol_version": "1.0", "request_id": operation + "-1", "operation": operation, "payload_schema": schema, "payload": payload}
	input, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(core)
	command.Dir, command.Stdin = dir, bytes.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run core: %v: %s", err, output)
	}
	var response coreResponse
	if err := json.Unmarshal(output, &response); err != nil {
		t.Fatalf("decode core response: %v: %s", err, output)
	}
	return response
}

func shaDigest(value []byte) string {
	sum := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func cloneMap(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var cloned map[string]any
	if err := json.Unmarshal(data, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}
