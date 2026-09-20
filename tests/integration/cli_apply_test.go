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
		{"name": "fixture", "executable": validator, "args": []string{"--mode", "passed"}},
		{"name": "blocking", "executable": validator, "args": []string{"--mode", "failed"}},
	}})
	if err := os.WriteFile(configPath, configBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"apply", proposalPath, "--source", sourcePath, "--core", core, "--config", configPath, "--approve", "theme", "--reject", "keep", "--validator", "fixture"}
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

	confirmed := exec.Command(cli, append(args, "--confirm")...)
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
}
