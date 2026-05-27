package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ParsesValidYAML(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yaml")
	yaml := `workspace_slug: my-workspace
max_workers: 3
tick_interval_seconds: 60
manager_timeout_seconds: 300
teams:
  - name: bff-web
    repo_url: git@github.com:org/bff-web.git
    repo_path: repos/bff-web
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WorkspaceSlug != "my-workspace" {
		t.Errorf("WorkspaceSlug: got %q, want %q", cfg.WorkspaceSlug, "my-workspace")
	}
	if cfg.MaxWorkers != 3 {
		t.Errorf("MaxWorkers: got %d, want 3", cfg.MaxWorkers)
	}
	if len(cfg.Teams) != 1 || cfg.Teams[0].Name != "bff-web" {
		t.Errorf("Teams: got %+v, want one team named bff-web", cfg.Teams)
	}
}

func TestLoad_ErrorsOnMissingWorkspaceSlug(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(path, []byte("max_workers: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error on missing workspace_slug")
	}
}

func TestLoad_AppliesDefaults(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yaml")
	yaml := `workspace_slug: x
teams: []
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MaxWorkers != 3 {
		t.Errorf("MaxWorkers default: got %d, want 3", cfg.MaxWorkers)
	}
	if cfg.TickIntervalSeconds != 60 {
		t.Errorf("TickIntervalSeconds default: got %d, want 60", cfg.TickIntervalSeconds)
	}
	if cfg.ManagerTimeoutSeconds != 300 {
		t.Errorf("ManagerTimeoutSeconds default: got %d, want 300", cfg.ManagerTimeoutSeconds)
	}
}
