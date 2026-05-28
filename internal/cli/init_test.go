package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestDiscoverMempalaceMCP_FindsBlockInClaudeJSON(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	claudeJSON := `{
		"mcpServers": {
			"mempalace": {
				"command": "/some/venv/python",
				"args": ["-m", "mempalace_gateway.server"],
				"env": {"MEMPALACE_ROOT": "/some/path"}
			},
			"other": {"command": "other-thing"}
		}
	}`
	if err := os.WriteFile(filepath.Join(tmp, ".claude.json"), []byte(claudeJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := discoverMempalaceMCP()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !strings.Contains(string(got), "/some/venv/python") {
		t.Errorf("expected command in result, got: %s", got)
	}
	if !strings.Contains(string(got), "/some/path") {
		t.Errorf("expected MEMPALACE_ROOT in result, got: %s", got)
	}
}

func TestDiscoverMempalaceMCP_ReturnsNotExistWhenNoFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	_, err := discoverMempalaceMCP()
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected os.ErrNotExist, got: %v", err)
	}
}

func TestDiscoverMempalaceMCP_ReturnsNotExistWhenNoMempalaceBlock(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	claudeJSON := `{"mcpServers": {"other": {"command": "x"}}}`
	if err := os.WriteFile(filepath.Join(tmp, ".claude.json"), []byte(claudeJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := discoverMempalaceMCP()
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected os.ErrNotExist, got: %v", err)
	}
}
