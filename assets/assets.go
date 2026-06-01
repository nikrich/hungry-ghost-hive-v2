// Package assets bundles default skills, MCP config, and permission settings into the binary.
package assets

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed skills/* skills/tasks/* settings.local.json mcp.json
var fsys embed.FS

// SkillsFS returns the embedded skills tree (root: "skills").
func SkillsFS() (fs.FS, error) {
	return fs.Sub(fsys, "skills")
}

// SettingsLocalJSON returns the default .claude/settings.local.json contents.
func SettingsLocalJSON() ([]byte, error) {
	return fsys.ReadFile("settings.local.json")
}

// MCPJSON returns the default .claude/mcp.json contents.
func MCPJSON() ([]byte, error) {
	return fsys.ReadFile("mcp.json")
}

// SyncSkillsToWorkspace overwrites <workspaceRoot>/.claude/skills/ with the
// skills embedded in this binary. Called by hive init AND every hive run
// startup so an operator who rebuilds the binary picks up new/changed skills
// without re-initing the workspace (Phase 2.E — defends against the skill
// drift bug that bit Phase 2.C verification).
func SyncSkillsToWorkspace(workspaceRoot string) error {
	src, err := SkillsFS()
	if err != nil {
		return err
	}
	dst := filepath.Join(workspaceRoot, ".claude", "skills")
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
