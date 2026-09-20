//go:build unix

package integration

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestCoreInspectionRejectsEveryUnsupportedUnixFileKind(t *testing.T) {
	core := buildCore(t)
	root := t.TempDir()
	regular := filepath.Join(root, "regular.json")
	if err := os.WriteFile(regular, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "directory")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(root, "link.json")
	if err := os.Symlink(regular, symlink); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(root, "pipe")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{"directory": directory, "symlink": symlink, "fifo": fifo, "device": "/dev/null"} {
		t.Run(name, func(t *testing.T) {
			response := callCore(t, core, root, "inspect_source", "zconfig.inspection/1", map[string]any{"source_path": path})
			if response.OK || response.Error == nil || response.Error.Code != "source.invalid" {
				t.Fatalf("unsupported file accepted: %+v", response)
			}
		})
	}
}
