package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddReq_WritesInboxFile(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".hive", "inbox"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RunAddReq(tmp, "Build a /healthz endpoint"); err != nil {
		t.Fatalf("RunAddReq: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(tmp, ".hive", "inbox"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d files in inbox, want 1", len(entries))
	}
	if !strings.HasPrefix(entries[0].Name(), "req-") {
		t.Errorf("filename: got %q, want req-* prefix", entries[0].Name())
	}
	data, err := os.ReadFile(filepath.Join(tmp, ".hive", "inbox", entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "Build a /healthz endpoint\n" {
		t.Errorf("body: got %q", data)
	}
}

func TestAddReq_ErrorsIfNoWorkspace(t *testing.T) {
	tmp := t.TempDir()
	if err := RunAddReq(tmp, "x"); err == nil {
		t.Fatal("expected error when no .hive/inbox/ exists")
	}
}
