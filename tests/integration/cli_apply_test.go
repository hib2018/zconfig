package integration

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hib2018/zconfig/tui/internal/review"
	"github.com/hib2018/zconfig/tui/internal/session"
)

func TestCLIValidatorPreviewConfirmationAndApply(t *testing.T) {
	core := buildCore(t)
	root := t.TempDir()
	cli := filepath.Join(root, "zconfig")
	validator := filepath.Join(root, "fixture-validator")
	buildCLI := exec.Command("go", "build", "-o", cli, "../../tui/cmd/zconfig")
	if output, err := buildCLI.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v: %s", err, output)
	}
	buildValidator := exec.Command("go", "build", "-o", validator, "../fixtures/validator")
	if output, err := buildValidator.CombinedOutput(); err != nil {
		t.Fatalf("build validator: %v: %s", err, output)
	}

	source := []byte("{\"theme\":\"light\",\"keep\":true}\n")
	sourcePath := filepath.Join(root, "app.json")
	if err := os.WriteFile(sourcePath, source, 0o640); err != nil {
		t.Fatal(err)
	}
	proposal := map[string]any{
		"proposal_id": "cli", "revision": 1, "source_digest": shaDigest(source), "created_at": "2026-09-20T00:00:00Z",
		"items": []map[string]any{
			{"change_id": "theme", "path": "/theme", "operation": "replace", "expected_old": "light", "proposed_value": "dark", "explanation": "theme"},
			{"change_id": "keep", "path": "/keep", "operation": "replace", "expected_old": true, "proposed_value": false, "explanation": "reject fixture"},
		},
	}
	proposalBytes, _ := json.Marshal(proposal)
	proposalPath := filepath.Join(root, "proposal.json")
	if err := os.WriteFile(proposalPath, proposalBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "commands.json")
	configBytes, _ := json.Marshal(map[string]any{"validators": []map[string]any{
		{"name": "fixture", "executable": validator, "args": []string{"--mode", "passed"}, "working_directory": filepath.Join(root, "validator-work")},
		{"name": "blocking", "executable": validator, "args": []string{"--mode", "failed"}},
	}})
	if err := os.Mkdir(filepath.Join(root, "validator-work"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, configBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"apply", proposalPath, "--source", sourcePath, "--core", core, "--config", configPath, "--session", "cli-review", "--approve", "theme", "--reject", "keep", "--validator", "fixture"}
	preview := exec.Command(cli, args...)
	preview.Dir = root
	output, err := preview.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
		t.Fatalf("preview exit=%v output=%s", err, output)
	}
	if !strings.Contains(string(output), `- "light"`) || !strings.Contains(string(output), `+ "dark"`) {
		t.Fatalf("missing final diff: %s", output)
	}
	token := outputValue(string(output), "confirmation token: ")
	if len(token) != 64 {
		t.Fatalf("missing confirmation token: %s", output)
	}
	if current, _ := os.ReadFile(sourcePath); !bytes.Equal(current, source) {
		t.Fatalf("preview mutated source: %q", current)
	}
	blockedArgs := append([]string(nil), args...)
	blockedArgs[len(blockedArgs)-1] = "blocking"
	blocked := exec.Command(cli, blockedArgs...)
	blocked.Dir = root
	blockedOutput, blockedErr := blocked.CombinedOutput()
	if !errors.As(blockedErr, &exitErr) || exitErr.ExitCode() != 3 || !strings.Contains(string(blockedOutput), "validation blocked application") {
		t.Fatalf("blocked exit=%v output=%s", blockedErr, blockedOutput)
	}
	if current, _ := os.ReadFile(sourcePath); !bytes.Equal(current, source) {
		t.Fatalf("failed validator mutated source: %q", current)
	}
	stored, err := session.Load(root, "cli-review")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	stored.Comments = append(stored.Comments, review.Comment{CommentID: "comment-theme", ChangeID: "theme", Body: "confirm this", Status: review.CommentOpen, CreatedAt: now, UpdatedAt: now})
	if err := session.Save(root, stored); err != nil {
		t.Fatal(err)
	}
	unresolved := exec.Command(cli, append(args, "--confirm-token", token)...)
	unresolved.Dir = root
	if unresolvedOutput, unresolvedErr := unresolved.CombinedOutput(); unresolvedErr == nil || !strings.Contains(string(unresolvedOutput), "final assembly rejected") {
		t.Fatalf("unresolved comment accepted: %v: %s", unresolvedErr, unresolvedOutput)
	}
	stored.Comments[0].Status = review.CommentHumanConfirmed
	stored.Comments[0].UpdatedAt = time.Now().UTC()
	if err := session.Save(root, stored); err != nil {
		t.Fatal(err)
	}
	changedState := exec.Command(cli, append(args, "--confirm-token", token)...)
	changedState.Dir = root
	if changedOutput, changedErr := changedState.CombinedOutput(); changedErr == nil || !strings.Contains(string(changedOutput), "apply rejected") {
		t.Fatalf("token survived comment-state change: %v: %s", changedErr, changedOutput)
	}
	refreshed := exec.Command(cli, args...)
	refreshed.Dir = root
	refreshedOutput, refreshedErr := refreshed.CombinedOutput()
	if !errors.As(refreshedErr, &exitErr) || exitErr.ExitCode() != 2 {
		t.Fatalf("refreshed preview exit=%v output=%s", refreshedErr, refreshedOutput)
	}
	token = outputValue(string(refreshedOutput), "confirmation token: ")

	direct := exec.Command(cli, append(args, "--confirm-token", strings.Repeat("0", 64))...)
	direct.Dir = root
	if directOutput, directErr := direct.CombinedOutput(); directErr == nil || !strings.Contains(string(directOutput), "apply rejected") {
		t.Fatalf("fabricated confirmation accepted: %v: %s", directErr, directOutput)
	}
	if current, _ := os.ReadFile(sourcePath); !bytes.Equal(current, source) {
		t.Fatalf("fabricated confirmation mutated source: %q", current)
	}

	confirmed := exec.Command(cli, append(args, "--confirm-token", token)...)
	confirmed.Dir = root
	output, err = confirmed.CombinedOutput()
	if err != nil {
		t.Fatalf("confirmed apply: %v: %s", err, output)
	}
	if current, _ := os.ReadFile(sourcePath); !bytes.Equal(current, []byte("{\"theme\":\"dark\",\"keep\":true}\n")) {
		t.Fatalf("apply result: %q", current)
	}
	matches, err := filepath.Glob(filepath.Join(root, ".zconfig-candidate-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("candidate files were not removed: %v %v", matches, err)
	}
	auditPath := filepath.Join(root, ".zconfig", "audit", "cli-review.jsonl")
	audit, err := os.ReadFile(auditPath)
	if err != nil || !strings.Contains(string(audit), `"action":"final.confirmed"`) || !strings.Contains(string(audit), `"action":"apply.succeeded"`) {
		t.Fatalf("missing apply audit trail: %v: %s", err, audit)
	}
}

func outputValue(output, prefix string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}
