package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInit_CreatesExpectedLayout(t *testing.T) {
	tmp := t.TempDir()
	opts := InitOptions{
		Dir:           tmp,
		WorkspaceSlug: "test-ws",
		Teams:         []TeamFlag{{Name: "bff-web", URL: "git@github.com:org/bff-web.git"}},
		NoClone:       true,
	}
	if err := RunInit(opts); err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	for _, want := range []string{
		".hive/config.yaml",
		".hive/inbox",
		".claude/skills/manager.md",
		".claude/skills/junior.md",
		".claude/skills/tasks/creating-a-pr.md",
		".claude/settings.local.json",
		".claude/mcp.json",
	} {
		path := filepath.Join(tmp, want)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}
}

func TestInit_ErrorsIfAlreadyInitialized(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".hive"), 0o755); err != nil {
		t.Fatal(err)
	}
	opts := InitOptions{Dir: tmp, WorkspaceSlug: "x", NoClone: true}
	if err := RunInit(opts); err == nil {
		t.Fatal("expected error when .hive/ already exists")
	}
}
