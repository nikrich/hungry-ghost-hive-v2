package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindWorkspaceRoot_FindsAncestorWithHiveDir(t *testing.T) {
	tmp := t.TempDir()
	wsRoot := filepath.Join(tmp, "ws")
	deep := filepath.Join(wsRoot, "a", "b", "c")
	if err := os.MkdirAll(filepath.Join(wsRoot, ".hive"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := FindWorkspaceRoot(deep)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != wsRoot {
		t.Errorf("got %q, want %q", got, wsRoot)
	}
}

func TestFindWorkspaceRoot_ErrorsWhenNoHiveDir(t *testing.T) {
	tmp := t.TempDir()
	_, err := FindWorkspaceRoot(tmp)
	if err == nil {
		t.Fatal("expected error when no .hive dir in ancestry")
	}
}

func TestMempalaceRoot_ReturnsEnvVar(t *testing.T) {
	t.Setenv("MEMPALACE_ROOT", "/some/path")
	got, err := MempalaceRoot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/some/path" {
		t.Errorf("got %q, want %q", got, "/some/path")
	}
}

func TestMempalaceRoot_ErrorsWhenUnset(t *testing.T) {
	t.Setenv("MEMPALACE_ROOT", "")
	_, err := MempalaceRoot()
	if err == nil {
		t.Fatal("expected error when MEMPALACE_ROOT unset")
	}
}

func TestWorkspaceWingDir_ReturnsCorrectPath(t *testing.T) {
	got := WorkspaceWingDir("/tmp/ws")
	want := filepath.Join("/tmp/ws", ".hive", "memory", "wings", "hive")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
