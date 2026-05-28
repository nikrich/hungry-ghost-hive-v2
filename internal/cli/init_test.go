package cli

import (
	"errors"
	"os"
	"os/exec"
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

func TestIsLocalNonBareRepo_TrueForNonBareLocalRepo(t *testing.T) {
	tmp := t.TempDir()
	cmd := exec.Command("git", "init", "-q", tmp)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	if !isLocalNonBareRepo(tmp) {
		t.Errorf("expected true for non-bare local repo %s", tmp)
	}
}

func TestIsLocalNonBareRepo_FalseForBareRepo(t *testing.T) {
	tmp := t.TempDir()
	cmd := exec.Command("git", "init", "--bare", "-q", tmp)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init --bare: %v", err)
	}
	if isLocalNonBareRepo(tmp) {
		t.Errorf("expected false for bare repo %s", tmp)
	}
}

func TestIsLocalNonBareRepo_FalseForRemoteURL(t *testing.T) {
	for _, url := range []string{
		"https://github.com/foo/bar.git",
		"git@github.com:foo/bar.git",
		"ssh://git@github.com/foo/bar.git",
	} {
		if isLocalNonBareRepo(url) {
			t.Errorf("expected false for remote URL %q", url)
		}
	}
}

func TestIsLocalNonBareRepo_FalseForNonExistentPath(t *testing.T) {
	if isLocalNonBareRepo("/this/path/does/not/exist/at/all") {
		t.Error("expected false for non-existent path")
	}
}

func TestEnsureMempalace_NoOpWhenAlreadyImportable(t *testing.T) {
	// Construct a fake python3 on PATH that succeeds the importability check.
	// Strategy: create a temp dir, write a script named "python3" that exits 0,
	// chmod +x, prepend to PATH.
	tmp := t.TempDir()
	fake := filepath.Join(tmp, "python3")
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))

	pythonPath, err := ensureMempalace()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pythonPath != fake {
		t.Errorf("expected python path %q, got %q", fake, pythonPath)
	}
}

func TestEnsureMempalace_ErrorsWhenNoPython3(t *testing.T) {
	// Empty PATH → no python3 anywhere.
	t.Setenv("PATH", "")
	_, err := ensureMempalace()
	if err == nil {
		t.Fatal("expected error when python3 is unavailable")
	}
}
