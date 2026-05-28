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
	// Stub python3 + uv away so RunInit doesn't actually try to install anything.
	// The fake python3 succeeds the importability check (exit 0).
	stubBin, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pythonStub := filepath.Join(stubBin, "python3")
	if err := os.WriteFile(pythonStub, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubBin)

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
		".hive/.gitignore",
		".hive/memory/wings/hive/rooms/requirements",
		".hive/memory/wings/hive/rooms/stories",
		".hive/memory/wings/hive/rooms/agents",
		".hive/memory/wings/hive/rooms/escalations",
		".hive/memory/wings/hive/rooms/findings",
		".hive/memory/index",
		".hive/memory/.mempalace/config.yaml",
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

	// Verify mcp.json points at workspace-local memory with our stub python.
	mcpData, err := os.ReadFile(filepath.Join(tmp, ".claude", "mcp.json"))
	if err != nil {
		t.Fatalf("read mcp.json: %v", err)
	}
	mcpStr := string(mcpData)
	if !strings.Contains(mcpStr, pythonStub) {
		t.Errorf("expected mcp.json command to be %q, got:\n%s", pythonStub, mcpStr)
	}
	expectedMemRoot := filepath.Join(tmp, ".hive", "memory")
	if !strings.Contains(mcpStr, expectedMemRoot) {
		t.Errorf("expected mcp.json MEMPALACE_ROOT to be %q, got:\n%s", expectedMemRoot, mcpStr)
	}
	if !strings.Contains(mcpStr, "mempalace_gateway.server") {
		t.Errorf("expected mcp.json args to mention mempalace_gateway.server, got:\n%s", mcpStr)
	}

	// Verify .gitignore contains "memory/".
	giData, err := os.ReadFile(filepath.Join(tmp, ".hive", ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !strings.Contains(string(giData), "memory/") {
		t.Errorf("expected .hive/.gitignore to contain 'memory/', got: %q", giData)
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

func TestCreateMemoryDir_CreatesExpectedTree(t *testing.T) {
	tmp := t.TempDir()
	if err := createMemoryDir(tmp); err != nil {
		t.Fatalf("createMemoryDir: %v", err)
	}

	memRoot := filepath.Join(tmp, ".hive", "memory")
	for _, sub := range []string{
		"wings/hive/rooms/requirements",
		"wings/hive/rooms/stories",
		"wings/hive/rooms/agents",
		"wings/hive/rooms/escalations",
		"wings/hive/rooms/findings",
		"index",
		".mempalace",
	} {
		path := filepath.Join(memRoot, sub)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected dir %s: %v", sub, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("expected %s to be a directory", sub)
		}
	}

	cfgPath := filepath.Join(memRoot, ".mempalace", "config.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("expected mempalace config.yaml: %v", err)
	}
	if !strings.Contains(string(data), "allowlist") || !strings.Contains(string(data), "hive") {
		t.Errorf("expected allowlist with 'hive', got:\n%s", data)
	}
}
