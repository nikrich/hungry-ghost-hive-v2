// Package cli contains command implementations (logic separate from cobra wiring).
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

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

	// Write config.
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

	// Write embedded skills.
	skillsFS, err := assets.SkillsFS()
	if err != nil {
		return err
	}
	claudeDir := filepath.Join(opts.Dir, ".claude")
	if err := writeEmbedTree(skillsFS, filepath.Join(claudeDir, "skills")); err != nil {
		return fmt.Errorf("write skills: %w", err)
	}

	// Write settings.local.json and mcp.json.
	settings, err := assets.SettingsLocalJSON()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.local.json"), settings, 0o644); err != nil {
		return err
	}
	// Write mcp.json — bundled placeholder by default, auto-detected from
	// ~/.claude.json if a mempalace MCP block exists there.
	mcp, err := assets.MCPJSON()
	if err != nil {
		return err
	}
	mcpPath := filepath.Join(claudeDir, "mcp.json")
	if err := os.WriteFile(mcpPath, mcp, 0o644); err != nil {
		return err
	}
	if discovered, derr := discoverMempalaceMCP(); derr == nil {
		// Splice the discovered block into the just-written mcp.json.
		var doc map[string]any
		if jerr := json.Unmarshal(mcp, &doc); jerr == nil {
			servers, _ := doc["mcpServers"].(map[string]any)
			if servers == nil {
				servers = map[string]any{}
				doc["mcpServers"] = servers
			}
			var disc any
			if jerr := json.Unmarshal(discovered, &disc); jerr == nil {
				servers["mempalace"] = disc
				if out, jerr := json.MarshalIndent(doc, "", "  "); jerr == nil {
					_ = os.WriteFile(mcpPath, out, 0o644)
					fmt.Println("mempalace MCP: auto-configured from ~/.claude.json")
				}
			}
		}
	} else if errors.Is(derr, os.ErrNotExist) {
		fmt.Fprintln(os.Stderr, "warning: mempalace MCP config is a placeholder. Edit .claude/mcp.json with your install's command before running 'hive run'.")
	} else {
		fmt.Fprintf(os.Stderr, "warning: could not read ~/.claude.json for MCP auto-detect: %v\n", derr)
	}

	// (Phase 1) Skip clone logic when NoClone. Real cloning is out of scope for T7.
	if !opts.NoClone {
		for _, t := range opts.Teams {
			fmt.Printf("(skip) clone %s into %s\n", t.URL, filepath.Join(opts.Dir, "repos", t.Name))
		}
	}

	return nil
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
