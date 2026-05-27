package assets

import (
	"io/fs"
	"testing"
)

func TestSkillsFS_IncludesManagerSkill(t *testing.T) {
	skillsFS, err := SkillsFS()
	if err != nil {
		t.Fatalf("SkillsFS: %v", err)
	}
	if _, err := fs.Stat(skillsFS, "manager.md"); err != nil {
		t.Errorf("expected manager.md in skills FS: %v", err)
	}
}

func TestSettingsLocalJSON_NonEmpty(t *testing.T) {
	data, err := SettingsLocalJSON()
	if err != nil {
		t.Fatalf("SettingsLocalJSON: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty settings.local.json")
	}
}

func TestMCPJSON_NonEmpty(t *testing.T) {
	data, err := MCPJSON()
	if err != nil {
		t.Fatalf("MCPJSON: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty mcp.json")
	}
}
