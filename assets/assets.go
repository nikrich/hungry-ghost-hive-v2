// Package assets bundles default skills, MCP config, and permission settings into the binary.
package assets

import (
	"embed"
	"io/fs"
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
