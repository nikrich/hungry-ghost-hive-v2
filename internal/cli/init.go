// Package cli contains command implementations (logic separate from cobra wiring).
package cli

import (
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
	mcp, err := assets.MCPJSON()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "mcp.json"), mcp, 0o644); err != nil {
		return err
	}

	// (Phase 1) Skip clone logic when NoClone. Real cloning is out of scope for T7.
	if !opts.NoClone {
		for _, t := range opts.Teams {
			fmt.Printf("(skip) clone %s into %s\n", t.URL, filepath.Join(opts.Dir, "repos", t.Name))
		}
	}

	return nil
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
