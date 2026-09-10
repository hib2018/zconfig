package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadCommandsAndDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "commands.json")
	data := []byte(`{"agents":[{"name":"fixture","executable":"agent","args":["--stdio"],"environment_allowlist":["PATH"]}],"validators":[]}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Agents[0].Timeout; got != 120*time.Second {
		t.Fatalf("timeout = %v", got)
	}
	if cfg.Agents[0].WorkingDirectory != dir {
		t.Fatal("project directory default not applied")
	}
}

func TestRejectsShellAndUnknownFields(t *testing.T) {
	for _, input := range []string{
		`{"agents":[{"name":"bad","executable":"sh -c echo","args":[]}],"validators":[]}`,
		`{"agents":[],"validators":[],"extra":true}`,
		`{"agents":[{"name":"bad","executable":"agent","args":["$HOME"]}],"validators":[]}`,
	} {
		path := filepath.Join(t.TempDir(), "commands.json")
		_ = os.WriteFile(path, []byte(input), 0600)
		if _, err := Load(path, t.TempDir()); err == nil {
			t.Errorf("accepted %s", input)
		}
	}
}

func TestEnvironmentIsAllowlisted(t *testing.T) {
	t.Setenv("PATH", "/bin")
	t.Setenv("SECRET_TOKEN", "secret")
	reg := Registration{EnvironmentAllowlist: []string{"PATH"}}
	env := reg.Environment()
	if len(env) != 1 || env[0] != "PATH=/bin" {
		t.Fatalf("environment = %#v", env)
	}
}
