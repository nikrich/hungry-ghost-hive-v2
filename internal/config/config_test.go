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
	if cfg.MaxQA != 2 {
		t.Errorf("MaxQA default: got %d, want 2", cfg.MaxQA)
	}
	if cfg.TickIntervalSeconds != 60 {
		t.Errorf("TickIntervalSeconds default: got %d, want 60", cfg.TickIntervalSeconds)
	}
	if cfg.ManagerTimeoutSeconds != 480 {
		t.Errorf("ManagerTimeoutSeconds default: got %d, want 480", cfg.ManagerTimeoutSeconds)
	}
	if cfg.IdleBackoffMaxSeconds != 600 {
		t.Errorf("IdleBackoffMaxSeconds default: got %d, want 600", cfg.IdleBackoffMaxSeconds)
	}
	if cfg.IdleBackoffFactor != 2.0 {
		t.Errorf("IdleBackoffFactor default: got %v, want 2.0", cfg.IdleBackoffFactor)
	}
	if cfg.ModelForJunior != "claude-haiku-4-5-20251001" {
		t.Errorf("ModelForJunior default: got %q, want claude-haiku-4-5-20251001", cfg.ModelForJunior)
	}
	if cfg.ModelForIntermediate != "claude-sonnet-4-6" {
		t.Errorf("ModelForIntermediate default: got %q, want claude-sonnet-4-6", cfg.ModelForIntermediate)
	}
	if cfg.ModelForSenior != "claude-opus-4-7" {
		t.Errorf("ModelForSenior default: got %q, want claude-opus-4-7", cfg.ModelForSenior)
	}
}

func TestLoad_RespectsIdleBackoffOverrides(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yaml")
	yaml := `workspace_slug: x
idle_backoff_max_seconds: 1200
idle_backoff_factor: 1.5
teams: []
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.IdleBackoffMaxSeconds != 1200 {
		t.Errorf("IdleBackoffMaxSeconds: got %d, want 1200", cfg.IdleBackoffMaxSeconds)
	}
	if cfg.IdleBackoffFactor != 1.5 {
		t.Errorf("IdleBackoffFactor: got %v, want 1.5", cfg.IdleBackoffFactor)
	}
}

func TestLoad_RespectsModelOverrides(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yaml")
	yaml := `workspace_slug: x
model_for_junior: claude-sonnet-4-6
model_for_intermediate: claude-opus-4-7
model_for_senior: claude-opus-4-7
teams: []
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ModelForJunior != "claude-sonnet-4-6" {
		t.Errorf("ModelForJunior: got %q, want claude-sonnet-4-6", cfg.ModelForJunior)
	}
	if cfg.ModelForIntermediate != "claude-opus-4-7" {
		t.Errorf("ModelForIntermediate: got %q, want claude-opus-4-7", cfg.ModelForIntermediate)
	}
	if cfg.ModelForSenior != "claude-opus-4-7" {
		t.Errorf("ModelForSenior: got %q, want claude-opus-4-7", cfg.ModelForSenior)
	}
}

func TestLoad_DefaultsPerTeamTestCommand(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yaml")
	yaml := `workspace_slug: x
teams:
  - name: a
  - name: b
    test_command: pytest -q
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Teams[0].TestCommand != "go test ./..." {
		t.Errorf("Teams[a].TestCommand default: got %q, want %q", cfg.Teams[0].TestCommand, "go test ./...")
	}
	if cfg.Teams[1].TestCommand != "pytest -q" {
		t.Errorf("Teams[b].TestCommand override: got %q, want %q", cfg.Teams[1].TestCommand, "pytest -q")
	}
}

func TestLoad_ErrorsOnMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/to/config.yaml")
	if err == nil {
		t.Fatal("expected error when file does not exist")
	}
}

func TestLoad_ErrorsOnInvalidYAML(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(path, []byte("invalid: [yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error on malformed YAML")
	}
}

func TestLoad_ErrorsOnMissingTeamName(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yaml")
	yaml := `workspace_slug: x
teams:
  - repo_url: git@github.com:org/test.git
    repo_path: repos/test
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error on missing team name")
	}
}
