// Package cli contains command implementations (logic separate from cobra wiring).
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nikrich/hungry-ghost-hive-v2/assets"
	"github.com/nikrich/hungry-ghost-hive-v2/internal/config"
	"gopkg.in/yaml.v3"
)

// TeamFlag is one --team flag value.
type TeamFlag struct {
	Name string
	URL  string
}

// InitOptions controls RunInit.
type InitOptions struct {
	Dir           string
	WorkspaceSlug string
	Teams         []TeamFlag
	NoClone       bool
}

// RunInit creates the workspace skeleton.
func RunInit(opts InitOptions) error {
	if opts.Dir == "" {
		opts.Dir = "."
	}
	if opts.WorkspaceSlug == "" {
		return errors.New("--workspace-slug is required")
	}

	hiveDir := filepath.Join(opts.Dir, ".hive")
	if _, err := os.Stat(hiveDir); err == nil {
		return fmt.Errorf("%s already exists; refusing to overwrite", hiveDir)
	} else if !os.IsNotExist(err) {
		return err
	}

	// Resolve absolute workspace path for mcp.json. Without this, MEMPALACE_ROOT
	// would be relative and break when claude is spawned from a worktree subdir.
	absWorkspace, err := filepath.Abs(opts.Dir)
	if err != nil {
		return fmt.Errorf("resolve workspace abspath: %w", err)
	}

	// Warn on local non-bare team repos (workers will fail at git push).
	for _, t := range opts.Teams {
		if isLocalNonBareRepo(t.URL) {
			fmt.Fprintf(os.Stderr,
				"warning: team %q repo (%s) is a non-bare local repo. Workers will fail at 'git push' (denyCurrentBranch). Use a bare repo (git init --bare) or a remote URL.\n",
				t.Name, t.URL)
		}
	}

	// Step 3 from spec lifecycle: ensure mempalace gateway is installed.
	pythonPath, err := ensureMempalace()
	if err != nil {
		return fmt.Errorf("mempalace setup: %w", err)
	}

	// Build config from flags.
	cfg := config.Config{
		WorkspaceSlug:         opts.WorkspaceSlug,
		MaxWorkers:            3,
		TickIntervalSeconds:   60,
		ManagerTimeoutSeconds: 300,
	}
	for _, t := range opts.Teams {
		cfg.Teams = append(cfg.Teams, config.Team{
			Name:     t.Name,
			RepoURL:  t.URL,
			RepoPath: filepath.Join("repos", t.Name),
		})
	}

	// Write .hive/config.yaml + inbox dir.
	if err := os.MkdirAll(hiveDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(hiveDir, "inbox"), 0o755); err != nil {
		return err
	}
	cfgData, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(hiveDir, "config.yaml"), cfgData, 0o644); err != nil {
		return err
	}

	// Step 4: create the workspace-local mempalace data skeleton.
	if err := createMemoryDir(opts.Dir); err != nil {
		return fmt.Errorf("create memory dir: %w", err)
	}

	// Step 5: .hive/.gitignore (so memory/ doesn't get tracked by default).
	if err := os.WriteFile(filepath.Join(hiveDir, ".gitignore"), []byte("memory/\n"), 0o644); err != nil {
		return fmt.Errorf("write .hive/.gitignore: %w", err)
	}

	// Step 6: write embedded skills.
	skillsFS, err := assets.SkillsFS()
	if err != nil {
		return err
	}
	claudeDir := filepath.Join(opts.Dir, ".claude")
	if err := writeEmbedTree(skillsFS, filepath.Join(claudeDir, "skills")); err != nil {
		return fmt.Errorf("write skills: %w", err)
	}

	// Step 7: settings.local.json from embedded asset.
	settings, err := assets.SettingsLocalJSON()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.local.json"), settings, 0o644); err != nil {
		return err
	}

	// Step 8: write workspace-local mcp.json. We always write this fresh — the
	// Phase 1.1 auto-detect from ~/.claude.json is no longer relevant since the
	// gateway points at workspace-local data.
	if err := writeWorkspaceLocalMCP(filepath.Join(claudeDir, "mcp.json"), pythonPath, filepath.Join(absWorkspace, ".hive", "memory")); err != nil {
		return fmt.Errorf("write mcp.json: %w", err)
	}

	// Step 10: friendly hint.
	fmt.Printf("Workspace initialized. Memory at .hive/memory/. Run `hive add-req \"...\"` then `hive run` to start.\n")

	// (Phase 1) Skip clone logic when NoClone.
	if !opts.NoClone {
		for _, t := range opts.Teams {
			fmt.Printf("(skip) clone %s into %s\n", t.URL, filepath.Join(opts.Dir, "repos", t.Name))
		}
	}

	return nil
}

// writeWorkspaceLocalMCP emits a .claude/mcp.json hard-coded to point at the
// given absolute MEMPALACE_ROOT using the given python3 binary.
func writeWorkspaceLocalMCP(path, pythonPath, mempalaceRoot string) error {
	doc := map[string]any{
		"mcpServers": map[string]any{
			"mempalace": map[string]any{
				"command": pythonPath,
				"args":    []string{"-m", "mempalace_gateway.server"},
				"env": map[string]any{
					"MEMPALACE_ROOT": mempalaceRoot,
				},
			},
		},
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// discoverMempalaceMCP reads $HOME/.claude.json and returns the user's existing
// mempalace MCP server config as a raw JSON message. Returns os.ErrNotExist if
// the file doesn't exist or doesn't contain an mcpServers.mempalace block.
func discoverMempalaceMCP() (json.RawMessage, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, fmt.Errorf("read ~/.claude.json: %w", err)
	}

	var top struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, fmt.Errorf("parse ~/.claude.json: %w", err)
	}
	mempalace, ok := top.MCPServers["mempalace"]
	if !ok || len(mempalace) == 0 {
		return nil, os.ErrNotExist
	}
	return mempalace, nil
}

// ensureMempalace makes sure the mempalace gateway python module is
// importable. Returns the absolute path to python3 (suitable for use in
// mcp.json's "command" field).
//
// Strategy:
//  1. Resolve python3 via PATH. If not found, return error.
//  2. Try `python3 -c "import mempalace_gateway"`. If it exits 0, done.
//  3. Otherwise install. Prefer `uv tool install mempalace` if `uv` is on PATH.
//     Else `python3 -m pip install --user mempalace`.
//  4. Re-check the import. If still failing, return an actionable error.
func ensureMempalace() (string, error) {
	pythonPath, err := exec.LookPath("python3")
	if err != nil {
		return "", fmt.Errorf("python3 not found on PATH — install Python 3.10+ and retry")
	}

	if err := checkMempalaceImport(pythonPath); err == nil {
		return pythonPath, nil
	}

	// Need to install. Prefer uv tool, fall back to pip --user.
	if uvPath, uerr := exec.LookPath("uv"); uerr == nil {
		fmt.Println("installing mempalace via uv tool install (one-time setup)...")
		cmd := exec.Command(uvPath, "tool", "install", "mempalace")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if cerr := cmd.Run(); cerr != nil {
			return "", fmt.Errorf("uv tool install mempalace failed: %w", cerr)
		}
	} else {
		fmt.Println("installing mempalace via pip --user (one-time setup)...")
		cmd := exec.Command(pythonPath, "-m", "pip", "install", "--user", "mempalace")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if cerr := cmd.Run(); cerr != nil {
			return "", fmt.Errorf("pip install --user mempalace failed: %w (install 'uv' (https://docs.astral.sh/uv) or run the pip command manually with elevated permissions)", cerr)
		}
	}

	if err := checkMempalaceImport(pythonPath); err != nil {
		return "", fmt.Errorf("mempalace_gateway still not importable after install: %w", err)
	}
	return pythonPath, nil
}

// checkMempalaceImport returns nil if `python3 -c "import mempalace_gateway"` exits 0.
func checkMempalaceImport(pythonPath string) error {
	cmd := exec.Command(pythonPath, "-c", "import mempalace_gateway")
	return cmd.Run()
}

// createMemoryDir creates the workspace-local mempalace data skeleton:
//   <workspaceRoot>/.hive/memory/wings/hive/rooms/{requirements,stories,agents,escalations,findings}/
//   <workspaceRoot>/.hive/memory/index/
//   <workspaceRoot>/.hive/memory/.mempalace/config.yaml (allowlist: [hive])
func createMemoryDir(workspaceRoot string) error {
	memRoot := filepath.Join(workspaceRoot, ".hive", "memory")
	for _, sub := range []string{
		"wings/hive/rooms/requirements",
		"wings/hive/rooms/stories",
		"wings/hive/rooms/agents",
		"wings/hive/rooms/escalations",
		"wings/hive/rooms/findings",
		"index",
		".mempalace",
	} {
		if err := os.MkdirAll(filepath.Join(memRoot, sub), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", sub, err)
		}
	}

	// Mempalace policy config: allowlist the single 'hive' wing.
	// Schema to be confirmed against upstream mempalace docs during T8 verification;
	// if it differs, update this string.
	cfg := "allowlist:\n  - hive\n"
	return os.WriteFile(filepath.Join(memRoot, ".mempalace", "config.yaml"), []byte(cfg), 0o644)
}

// isLocalNonBareRepo returns true when url is a filesystem path AND the
// directory contains a non-bare git repo. Returns false for any URL with a
// scheme (http://, https://, ssh://, git@), for any path that doesn't contain
// a git repo, and for any error talking to git.
func isLocalNonBareRepo(url string) bool {
	// Quick filter: looks like a remote URL?
	if strings.Contains(url, "://") || strings.HasPrefix(url, "git@") {
		return false
	}
	// Looks like a path. Does it have a git repo?
	out, err := exec.Command("git", "-C", url, "rev-parse", "--is-bare-repository").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "false"
}

// writeEmbedTree walks an fs.FS and mirrors it under dst.
func writeEmbedTree(src fs.FS, dst string) error {
	return fs.WalkDir(src, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dst, p)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(src, p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
