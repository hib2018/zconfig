package integration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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
