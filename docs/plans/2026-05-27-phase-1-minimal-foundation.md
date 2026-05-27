# Hive v2 — Phase 1: Minimal Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a working hive v2 binary that can `init` a workspace, accept a requirement via `add-req`, spawn a single Claude Code worker via the manager loop, and complete one story end-to-end (PR opened). Happy path only — no multi-team, no QA, no escalation, no retry, no stuck-detection.

**Architecture:** Go binary supervises `claude` subprocesses. The manager is a fresh `claude --print` invocation per watchdog tick. All state lives in mempalace. Skills (`.claude/skills/`) carry the orchestration logic. See [`docs/specs/2026-05-27-architecture-design.md`](../specs/2026-05-27-architecture-design.md) for the full design.

**Tech Stack:**
- Go 1.22+ (uses `embed.FS`, `slog`)
- `github.com/spf13/cobra` — CLI parsing
- `gopkg.in/yaml.v3` — config + drawer frontmatter parsing
- `github.com/fatih/color` — pretty `hive status` output
- `github.com/gofrs/flock` — PID file locks
- `claude` CLI on PATH (the runtime)
- mempalace MCP server (the storage)

**Deliverables when Phase 1 is done:**
- `go build ./cmd/hive` produces a working binary
- `hive init` + `hive add-req "..."` + `hive run` results in one PR being opened on a real repo
- `hive status` shows in-flight + completed work
- `hive stop` cleanly shuts everything down
- Unit tests pass; one documented manual e2e verification passes

**Out of scope for Phase 1** (deliberately): retry logic, stuck-worker detection, escalation handling, multi-team config, QA review flow, tech-lead decomposition (we author requirements that map directly to a single story), TUI, goreleaser/CI.

---

## File Structure

```
hungry-ghost-hive-v2/
├── go.mod
├── go.sum
├── cmd/
│   └── hive/
│       └── main.go                  # cobra root, wires up commands
├── internal/
│   ├── paths/
│   │   ├── paths.go                 # workspace + mempalace path discovery
│   │   └── paths_test.go
│   ├── config/
│   │   ├── config.go                # .hive/config.yaml schema + load
│   │   └── config_test.go
│   ├── drawers/
│   │   ├── drawers.go               # walk mempalace wing, parse YAML frontmatter, filter
│   │   └── drawers_test.go
│   ├── diary/
│   │   ├── diary.go                 # read/tail event log
│   │   └── diary_test.go
│   ├── proc/
│   │   ├── proc.go                  # PID files, flock, kill cascade
│   │   └── proc_test.go
│   ├── watchdog/
│   │   └── watchdog.go              # supervisor loop
│   └── cli/
│       ├── init.go                  # `hive init`
│       ├── run.go                   # `hive run` (forks watchdog)
│       ├── stop.go                  # `hive stop`
│       ├── status.go                # `hive status`
│       ├── addreq.go                # `hive add-req`
│       ├── logs.go                  # `hive logs`
│       └── watchdog_mode.go         # hidden `hive --watchdog` entry
├── assets/
│   ├── assets.go                    # //go:embed declarations
│   ├── skills/
│   │   ├── manager.md
│   │   ├── junior.md
│   │   └── tasks/
│   │       ├── creating-a-pr.md
│   │       └── filing-a-finding.md
│   ├── settings.local.json
│   └── mcp.json
└── docs/
    ├── specs/
    │   └── 2026-05-27-architecture-design.md   (already exists)
    └── plans/
        └── 2026-05-27-phase-1-minimal-foundation.md  (this file)
```

**One responsibility per file.** No file does both CLI wiring and business logic; CLI command files just parse flags and call into `internal/*` packages.

---

## Task 0: Bootstrap Go module

**Files:**
- Create: `go.mod`
- Create: `cmd/hive/main.go`
- Create: `.github/workflows/ci.yml` (skip — out of scope for Phase 1)

- [ ] **Step 0.1: Initialize Go module**

```sh
cd /Users/jannik/development/nikrich/hungry-ghost-hive-v2
go mod init github.com/nikrich/hungry-ghost-hive-v2
```

- [ ] **Step 0.2: Add dependencies**

```sh
go get github.com/spf13/cobra@latest
go get gopkg.in/yaml.v3@latest
go get github.com/fatih/color@latest
go get github.com/gofrs/flock@latest
```

- [ ] **Step 0.3: Create cmd/hive/main.go skeleton**

```go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "hive",
	Short: "AI agent orchestrator — supervises Claude Code subprocesses",
	Long:  `Hive supervises Claude Code subprocesses to coordinate agile-style AI development teams.`,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 0.4: Build to confirm it compiles**

Run: `go build ./cmd/hive`
Expected: Binary `hive` appears; no errors.

- [ ] **Step 0.5: Smoke-test root command**

Run: `./hive --help`
Expected: Cobra-rendered help output naming "hive".

- [ ] **Step 0.6: Commit**

```sh
git add go.mod go.sum cmd/hive/main.go
git commit -m "chore: bootstrap Go module with cobra root command"
```

---

## Task 1: `internal/paths` — workspace + mempalace path discovery

**Files:**
- Create: `internal/paths/paths.go`
- Test: `internal/paths/paths_test.go`

The package answers: "Given some cwd, where is the workspace root (the dir containing `.hive/`)? And where is the mempalace storage root (from `$MEMPALACE_ROOT` env var)?"

- [ ] **Step 1.1: Write the failing tests**

`internal/paths/paths_test.go`:

```go
package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindWorkspaceRoot_FindsAncestorWithHiveDir(t *testing.T) {
	tmp := t.TempDir()
	wsRoot := filepath.Join(tmp, "ws")
	deep := filepath.Join(wsRoot, "a", "b", "c")
	if err := os.MkdirAll(filepath.Join(wsRoot, ".hive"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := FindWorkspaceRoot(deep)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != wsRoot {
		t.Errorf("got %q, want %q", got, wsRoot)
	}
}

func TestFindWorkspaceRoot_ErrorsWhenNoHiveDir(t *testing.T) {
	tmp := t.TempDir()
	_, err := FindWorkspaceRoot(tmp)
	if err == nil {
		t.Fatal("expected error when no .hive dir in ancestry")
	}
}

func TestMempalaceRoot_ReturnsEnvVar(t *testing.T) {
	t.Setenv("MEMPALACE_ROOT", "/some/path")
	got, err := MempalaceRoot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/some/path" {
		t.Errorf("got %q, want %q", got, "/some/path")
	}
}

func TestMempalaceRoot_ErrorsWhenUnset(t *testing.T) {
	t.Setenv("MEMPALACE_ROOT", "")
	_, err := MempalaceRoot()
	if err == nil {
		t.Fatal("expected error when MEMPALACE_ROOT unset")
	}
}
```

- [ ] **Step 1.2: Run tests to verify they fail**

Run: `go test ./internal/paths/`
Expected: FAIL — package has no implementation yet.

- [ ] **Step 1.3: Implement `internal/paths/paths.go`**

```go
// Package paths discovers workspace and mempalace storage roots.
package paths

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrNoWorkspace is returned when no .hive directory is found in the ancestry of the given path.
var ErrNoWorkspace = errors.New("no .hive directory found in ancestry; not inside a hive workspace")

// ErrMempalaceRootUnset is returned when MEMPALACE_ROOT env var is not set.
var ErrMempalaceRootUnset = errors.New("MEMPALACE_ROOT environment variable not set")

// FindWorkspaceRoot walks up from start until it finds a directory containing .hive/.
// Returns the workspace root or ErrNoWorkspace.
func FindWorkspaceRoot(start string) (string, error) {
	cur, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if info, err := os.Stat(filepath.Join(cur, ".hive")); err == nil && info.IsDir() {
			return cur, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", ErrNoWorkspace
		}
		cur = parent
	}
}

// MempalaceRoot returns the path from $MEMPALACE_ROOT.
func MempalaceRoot() (string, error) {
	v := os.Getenv("MEMPALACE_ROOT")
	if v == "" {
		return "", ErrMempalaceRootUnset
	}
	return v, nil
}

// HiveDir returns <workspaceRoot>/.hive.
func HiveDir(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, ".hive")
}

// ClaudeDir returns <workspaceRoot>/.claude.
func ClaudeDir(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, ".claude")
}

// WingDir returns the on-disk directory for a workspace's mempalace wing.
func WingDir(mempalaceRoot, wingSlug string) string {
	return filepath.Join(mempalaceRoot, "wings", "hive-"+wingSlug)
}
```

- [ ] **Step 1.4: Run tests to verify they pass**

Run: `go test ./internal/paths/ -v`
Expected: PASS — all 4 tests.

- [ ] **Step 1.5: Commit**

```sh
git add internal/paths/
git commit -m "feat(paths): add workspace and mempalace path discovery"
```

---

## Task 2: `internal/config` — `.hive/config.yaml` schema and loader

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

The schema (Phase 1 minimal):

```yaml
workspace_slug: greenlight-freelance     # used as hive-<slug> wing name
max_workers: 3
tick_interval_seconds: 60
manager_timeout_seconds: 300
teams:
  - name: bff-web
    repo_url: git@github.com:org/bff-web.git
    repo_path: repos/bff-web
```

- [ ] **Step 2.1: Write failing tests**

`internal/config/config_test.go`:

```go
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
```

- [ ] **Step 2.2: Run tests to verify they fail**

Run: `go test ./internal/config/`
Expected: FAIL — no package code yet.

- [ ] **Step 2.3: Implement `internal/config/config.go`**

```go
// Package config loads and validates .hive/config.yaml.
package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Team struct {
	Name     string `yaml:"name"`
	RepoURL  string `yaml:"repo_url"`
	RepoPath string `yaml:"repo_path"`
}

type Config struct {
	WorkspaceSlug         string `yaml:"workspace_slug"`
	MaxWorkers            int    `yaml:"max_workers"`
	TickIntervalSeconds   int    `yaml:"tick_interval_seconds"`
	ManagerTimeoutSeconds int    `yaml:"manager_timeout_seconds"`
	Teams                 []Team `yaml:"teams"`
}

// Load reads, parses, validates, and applies defaults to a config file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	cfg.applyDefaults()
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.WorkspaceSlug == "" {
		return errors.New("workspace_slug is required")
	}
	for i, t := range c.Teams {
		if t.Name == "" {
			return fmt.Errorf("teams[%d].name is required", i)
		}
	}
	return nil
}

func (c *Config) applyDefaults() {
	if c.MaxWorkers == 0 {
		c.MaxWorkers = 3
	}
	if c.TickIntervalSeconds == 0 {
		c.TickIntervalSeconds = 60
	}
	if c.ManagerTimeoutSeconds == 0 {
		c.ManagerTimeoutSeconds = 300
	}
}
```

- [ ] **Step 2.4: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS — all 3 tests.

- [ ] **Step 2.5: Commit**

```sh
git add internal/config/
git commit -m "feat(config): add .hive/config.yaml loader with defaults"
```

---

## Task 3: `internal/drawers` — read mempalace drawers from disk

**Files:**
- Create: `internal/drawers/drawers.go`
- Test: `internal/drawers/drawers_test.go`

A drawer on disk is a markdown file with YAML frontmatter:

```markdown
---
title: STORY-001: Add /healthz endpoint
type: story
status: pending
points: 3
team: bff-web
---

## Description
...
```

The package walks `<wing>/<room>/*.md`, parses the frontmatter, and lets callers filter by type/status.

- [ ] **Step 3.1: Write failing tests**

`internal/drawers/drawers_test.go`:

```go
package drawers

import (
	"os"
	"path/filepath"
	"testing"
)

func writeDrawer(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestList_ParsesFrontmatter(t *testing.T) {
	tmp := t.TempDir()
	writeDrawer(t, filepath.Join(tmp, "stories", "STORY-001.md"),
		"---\ntitle: STORY-001\ntype: story\nstatus: pending\n---\n\nBody\n")

	got, err := List(tmp, "stories")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d drawers, want 1", len(got))
	}
	if got[0].Title != "STORY-001" {
		t.Errorf("Title: got %q, want STORY-001", got[0].Title)
	}
	if got[0].Type != "story" {
		t.Errorf("Type: got %q, want story", got[0].Type)
	}
	if got[0].Status != "pending" {
		t.Errorf("Status: got %q, want pending", got[0].Status)
	}
}

func TestList_SkipsNonMarkdown(t *testing.T) {
	tmp := t.TempDir()
	writeDrawer(t, filepath.Join(tmp, "stories", "STORY-001.md"),
		"---\ntitle: t\ntype: story\n---\nBody\n")
	writeDrawer(t, filepath.Join(tmp, "stories", "notes.txt"), "ignored\n")

	got, err := List(tmp, "stories")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1 (only .md)", len(got))
	}
}

func TestList_ReturnsEmptyOnMissingRoom(t *testing.T) {
	tmp := t.TempDir()
	got, err := List(tmp, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d, want 0", len(got))
	}
}

func TestFilterByStatus(t *testing.T) {
	all := []Drawer{
		{Title: "a", Status: "pending"},
		{Title: "b", Status: "in-progress"},
		{Title: "c", Status: "pending"},
	}
	got := FilterByStatus(all, "pending")
	if len(got) != 2 {
		t.Errorf("got %d, want 2", len(got))
	}
}
```

- [ ] **Step 3.2: Run tests to verify they fail**

Run: `go test ./internal/drawers/`
Expected: FAIL — no implementation.

- [ ] **Step 3.3: Implement `internal/drawers/drawers.go`**

```go
// Package drawers reads mempalace drawers from disk.
package drawers

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Drawer is the parsed view of a single mempalace drawer file.
// Captures the frontmatter fields hive cares about; the full file is kept in Raw for callers that need more.
type Drawer struct {
	Title       string   `yaml:"title"`
	Type        string   `yaml:"type"`
	Status      string   `yaml:"status"`
	Points      int      `yaml:"points"`
	Team        string   `yaml:"team"`
	AssignedTo  string   `yaml:"assigned_to"`
	Role        string   `yaml:"role"`
	Story       string   `yaml:"story"`
	PRURL       string   `yaml:"pr_url"`
	RetryCount  int      `yaml:"retry_count"`
	CreatedAt   string   `yaml:"created_at"`
	UpdatedAt   string   `yaml:"updated_at"`

	Path string `yaml:"-"` // filesystem path, set by List
	Body string `yaml:"-"` // markdown body after the frontmatter
}

// List walks <wingRoot>/rooms/<room>/*.md and returns parsed drawers.
// Missing room directory returns an empty slice (not an error).
func List(wingRoot, room string) ([]Drawer, error) {
	dir := filepath.Join(wingRoot, "rooms", room)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read room dir: %w", err)
	}

	var out []Drawer
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		d, err := parseFile(path)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		out = append(out, d)
	}
	return out, nil
}

// FilterByStatus returns drawers whose Status equals the given value.
func FilterByStatus(in []Drawer, status string) []Drawer {
	var out []Drawer
	for _, d := range in {
		if d.Status == status {
			out = append(out, d)
		}
	}
	return out
}

// FilterByType returns drawers whose Type equals the given value.
func FilterByType(in []Drawer, drawerType string) []Drawer {
	var out []Drawer
	for _, d := range in {
		if d.Type == drawerType {
			out = append(out, d)
		}
	}
	return out
}

func parseFile(path string) (Drawer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Drawer{}, err
	}
	d, err := parse(data)
	if err != nil {
		return Drawer{}, err
	}
	d.Path = path
	return d, nil
}

// parse extracts YAML frontmatter (between leading `---` markers) and the body.
func parse(data []byte) (Drawer, error) {
	// A drawer file looks like:
	//   ---\n
	//   key: value\n
	//   ...
	//   ---\n
	//   body...
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return Drawer{}, errors.New("missing leading frontmatter marker")
	}
	rest := data[4:]
	end := bytes.Index(rest, []byte("\n---\n"))
	if end < 0 {
		return Drawer{}, errors.New("missing trailing frontmatter marker")
	}
	front := rest[:end]
	body := rest[end+5:]

	var d Drawer
	if err := yaml.Unmarshal(front, &d); err != nil {
		return Drawer{}, fmt.Errorf("yaml: %w", err)
	}
	d.Body = string(body)
	return d, nil
}
```

- [ ] **Step 3.4: Run tests to verify they pass**

Run: `go test ./internal/drawers/ -v`
Expected: PASS — all 4 tests.

- [ ] **Step 3.5: Commit**

```sh
git add internal/drawers/
git commit -m "feat(drawers): parse mempalace drawers from disk"
```

---

## Task 4: `internal/diary` — read/tail the event log

**Files:**
- Create: `internal/diary/diary.go`
- Test: `internal/diary/diary_test.go`

Diary format (append-only, one line per event):

```
2026-05-27T14:00:00Z  manager  tick-start
2026-05-27T14:00:01Z  manager  spawn  agent=abc123 role=junior story=STORY-001
```

The diary file lives at `<wingRoot>/diary.log` (mempalace's diary write goes through the MCP; the on-disk file is what hive reads).

- [ ] **Step 4.1: Write failing tests**

`internal/diary/diary_test.go`:

```go
package diary

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRead_ParsesEntries(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "diary.log")
	content := `2026-05-27T14:00:00Z	manager	tick-start
2026-05-27T14:00:01Z	manager	spawn	agent=abc123 role=junior
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Read(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got[0].Actor != "manager" || got[0].Event != "tick-start" {
		t.Errorf("entry 0: got %+v", got[0])
	}
	if got[1].Detail != "agent=abc123 role=junior" {
		t.Errorf("entry 1 Detail: got %q", got[1].Detail)
	}
}

func TestRead_ReturnsEmptyOnMissingFile(t *testing.T) {
	got, err := Read("/nonexistent/path/diary.log")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d, want 0", len(got))
	}
}

func TestTail_ReturnsLastN(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "diary.log")
	content := ""
	for i := 0; i < 10; i++ {
		content += "2026-05-27T14:00:00Z\tmanager\ttick-start\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Tail(path, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("got %d, want 3", len(got))
	}
}
```

- [ ] **Step 4.2: Run tests to verify they fail**

Run: `go test ./internal/diary/`
Expected: FAIL.

- [ ] **Step 4.3: Implement `internal/diary/diary.go`**

```go
// Package diary reads the mempalace event log.
package diary

import (
	"bufio"
	"errors"
	"os"
	"strings"
)

// Entry is one event-log line.
type Entry struct {
	Timestamp string // ISO 8601
	Actor     string // "manager", "worker", "watchdog"
	Event     string // short event name
	Detail    string // free-form rest of line
}

// Read returns all entries from the given diary file.
// Missing file returns an empty slice (not an error).
func Read(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []Entry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if e, ok := parseLine(scanner.Text()); ok {
			out = append(out, e)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Tail returns the last n entries.
func Tail(path string, n int) ([]Entry, error) {
	all, err := Read(path)
	if err != nil {
		return nil, err
	}
	if len(all) <= n {
		return all, nil
	}
	return all[len(all)-n:], nil
}

func parseLine(line string) (Entry, bool) {
	parts := strings.SplitN(line, "\t", 4)
	if len(parts) < 3 {
		return Entry{}, false
	}
	e := Entry{
		Timestamp: parts[0],
		Actor:     parts[1],
		Event:     parts[2],
	}
	if len(parts) == 4 {
		e.Detail = parts[3]
	}
	return e, true
}
```

- [ ] **Step 4.4: Run tests to verify they pass**

Run: `go test ./internal/diary/ -v`
Expected: PASS.

- [ ] **Step 4.5: Commit**

```sh
git add internal/diary/
git commit -m "feat(diary): read and tail mempalace event log"
```

---

## Task 5: `internal/proc` — PID files, flock, kill cascade

**Files:**
- Create: `internal/proc/proc.go`
- Test: `internal/proc/proc_test.go`

Responsibilities:
- Write/read/clear PID files
- Acquire/release exclusive flock on PID files (single-instance enforcement)
- Check if a PID is alive (used by `hive run` and `hive stop`)
- Send SIGTERM, then SIGKILL after a grace period

- [ ] **Step 5.1: Write failing tests**

`internal/proc/proc_test.go`:

```go
package proc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAndReadPidFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.pid")
	if err := WritePidFile(path, 12345); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := ReadPidFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got != 12345 {
		t.Errorf("got %d, want 12345", got)
	}
}

func TestReadPidFile_ReturnsZeroOnMissing(t *testing.T) {
	got, err := ReadPidFile("/nonexistent/path.pid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

func TestClearPidFile_IsIdempotent(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.pid")
	if err := WritePidFile(path, 1); err != nil {
		t.Fatal(err)
	}
	if err := ClearPidFile(path); err != nil {
		t.Fatalf("first clear: %v", err)
	}
	if err := ClearPidFile(path); err != nil {
		t.Fatalf("second clear (idempotent): %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file gone, got err=%v", err)
	}
}

func TestIsAlive_ReturnsFalseForZero(t *testing.T) {
	if IsAlive(0) {
		t.Error("IsAlive(0) should be false")
	}
}

func TestIsAlive_ReturnsTrueForCurrentProcess(t *testing.T) {
	if !IsAlive(os.Getpid()) {
		t.Error("IsAlive(getpid) should be true")
	}
}

func TestIsAlive_ReturnsFalseForLikelyDeadPid(t *testing.T) {
	// PID 999999 is almost certainly not alive on a typical system.
	if IsAlive(999999) {
		t.Skip("PID 999999 happens to be alive; skipping")
	}
}
```

- [ ] **Step 5.2: Run tests to verify they fail**

Run: `go test ./internal/proc/`
Expected: FAIL.

- [ ] **Step 5.3: Implement `internal/proc/proc.go`**

```go
// Package proc handles PID files and process lifecycle.
package proc

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// WritePidFile writes pid to path atomically (write then rename).
func WritePidFile(path string, pid int) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strconv.Itoa(pid)), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReadPidFile returns the PID stored at path, or 0 if the file doesn't exist.
func ReadPidFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// ClearPidFile removes the PID file. Missing file is not an error.
func ClearPidFile(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// IsAlive reports whether the process with the given PID exists.
// On Unix this uses signal 0 which checks existence without sending anything.
func IsAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// Terminate sends SIGTERM to pid, waits up to grace, then sends SIGKILL.
// Returns nil on clean exit, error if SIGKILL was needed or if the process never appeared dead.
func Terminate(pid int, grace time.Duration) error {
	if !IsAlive(pid) {
		return nil
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("SIGTERM %d: %w", pid, err)
	}
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if !IsAlive(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		return fmt.Errorf("SIGKILL %d: %w", pid, err)
	}
	return fmt.Errorf("pid %d required SIGKILL", pid)
}
```

- [ ] **Step 5.4: Run tests to verify they pass**

Run: `go test ./internal/proc/ -v`
Expected: PASS — all 6 tests.

- [ ] **Step 5.5: Commit**

```sh
git add internal/proc/
git commit -m "feat(proc): add PID file helpers and process termination"
```

---

## Task 6: `assets` package — embed skill markdown and config files

**Files:**
- Create: `assets/assets.go`
- Create: `assets/skills/manager.md` (placeholder — full content in Task 13)
- Create: `assets/skills/junior.md` (placeholder — full content in Task 14)
- Create: `assets/skills/tasks/creating-a-pr.md` (placeholder)
- Create: `assets/skills/tasks/filing-a-finding.md` (placeholder)
- Create: `assets/settings.local.json`
- Create: `assets/mcp.json`

Placeholders here mean: a one-line file with a TODO comment that names what goes there. Real skill content is authored in Tasks 13-16 so the spec sections about skills can be addressed in one focused task each.

- [ ] **Step 6.1: Create placeholder skill files**

```sh
mkdir -p assets/skills/tasks
printf "# Manager skill (placeholder — see Task 13)\n" > assets/skills/manager.md
printf "# Junior skill (placeholder — see Task 14)\n" > assets/skills/junior.md
printf "# Creating-a-PR task skill (placeholder — see Task 15)\n" > assets/skills/tasks/creating-a-pr.md
printf "# Filing-a-finding task skill (placeholder — see Task 16)\n" > assets/skills/tasks/filing-a-finding.md
```

- [ ] **Step 6.2: Create `assets/settings.local.json`**

```json
{
  "permissions": {
    "allow": [
      "Bash(*)",
      "Read(**)",
      "Write(**)",
      "Edit(**)",
      "Glob(**)",
      "Grep(*)",
      "WebFetch(*)",
      "mcp__mempalace__*",
      "TodoWrite"
    ]
  }
}
```

Write this file at `assets/settings.local.json`.

- [ ] **Step 6.3: Create `assets/mcp.json`**

```json
{
  "mcpServers": {
    "mempalace": {
      "command": "mempalace-gateway",
      "args": ["serve", "--stdio"]
    }
  }
}
```

Write this file at `assets/mcp.json`. (The exact command depends on how the mempalace MCP is installed on the user's machine; this default is what the mempalace-bootstrap skill configures. Tunable per-workspace after init.)

- [ ] **Step 6.4: Implement `assets/assets.go`**

```go
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
```

- [ ] **Step 6.5: Add a basic test that embed works**

Create `assets/assets_test.go`:

```go
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
```

- [ ] **Step 6.6: Build and test**

Run: `go test ./assets/ -v`
Expected: PASS — all 3 tests.

- [ ] **Step 6.7: Commit**

```sh
git add assets/
git commit -m "feat(assets): embed skill placeholders and default config files"
```

---

## Task 7: `hive init` command

**Files:**
- Create: `internal/cli/init.go`
- Modify: `cmd/hive/main.go` (register command)
- Test: `internal/cli/init_test.go`

Behavior: in the cwd (or `--dir`), create `.hive/config.yaml`, `.hive/inbox/`, `.claude/skills/*`, `.claude/settings.local.json`, `.claude/mcp.json`. Clones each team repo into `repos/<name>/`.

For Phase 1: skip the interactive prompts. Accept the config via flags or a one-line JSON in `--config`. Simplest: support `--workspace-slug` and one `--team name=<n>,url=<u>` flag (repeatable). Iterate later.

Also for Phase 1: skip git cloning if `--no-clone` flag is set (useful for tests and for users who pre-clone).

- [ ] **Step 7.1: Write failing test**

`internal/cli/init_test.go`:

```go
package cli

import (
	"os"
	"path/filepath"
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
```

- [ ] **Step 7.2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestInit -v`
Expected: FAIL.

- [ ] **Step 7.3: Implement `internal/cli/init.go`**

```go
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

	// (Phase 1) Skip clone logic when NoClone. Real cloning added in Task 8.
	if !opts.NoClone {
		// Placeholder for clone logic; for now, just print intent.
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
```

- [ ] **Step 7.4: Wire the command into cobra**

Modify `cmd/hive/main.go` — add an `init.go` file alongside it:

`cmd/hive/init_cmd.go`:

```go
package main

import (
	"strings"

	"github.com/nikrich/hungry-ghost-hive-v2/internal/cli"
	"github.com/spf13/cobra"
)

func init() {
	var (
		dir, slug string
		teams     []string
		noClone   bool
	)
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new hive workspace",
		RunE: func(c *cobra.Command, args []string) error {
			opts := cli.InitOptions{
				Dir:           dir,
				WorkspaceSlug: slug,
				NoClone:       noClone,
			}
			for _, t := range teams {
				name, url, ok := splitTeam(t)
				if !ok {
					return c.Help()
				}
				opts.Teams = append(opts.Teams, cli.TeamFlag{Name: name, URL: url})
			}
			return cli.RunInit(opts)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "workspace directory")
	cmd.Flags().StringVar(&slug, "workspace-slug", "", "workspace slug (used as hive-<slug> wing name)")
	cmd.Flags().StringSliceVar(&teams, "team", nil, "team in name=<n>,url=<u> form (repeatable)")
	cmd.Flags().BoolVar(&noClone, "no-clone", false, "skip cloning team repos")
	rootCmd.AddCommand(cmd)
}

func splitTeam(s string) (name, url string, ok bool) {
	parts := strings.Split(s, ",")
	for _, p := range parts {
		if kv := strings.SplitN(p, "=", 2); len(kv) == 2 {
			switch kv[0] {
			case "name":
				name = kv[1]
			case "url":
				url = kv[1]
			}
		}
	}
	return name, url, name != "" && url != ""
}
```

- [ ] **Step 7.5: Run tests + smoke test**

Run: `go test ./internal/cli/ -run TestInit -v`
Expected: PASS — both tests.

Smoke test:

```sh
go build ./cmd/hive
TMPDIR=$(mktemp -d)
./hive init --dir "$TMPDIR" --workspace-slug demo --team name=bff,url=git@github.com:org/bff.git --no-clone
ls -R "$TMPDIR"
```

Expected: directory tree under `$TMPDIR` containing `.hive/config.yaml`, `.claude/skills/manager.md`, etc.

- [ ] **Step 7.6: Commit**

```sh
git add internal/cli/init.go internal/cli/init_test.go cmd/hive/init_cmd.go
git commit -m "feat(cli): add 'hive init' command"
```

---

## Task 8: `hive add-req` command

**Files:**
- Create: `internal/cli/addreq.go`
- Create: `cmd/hive/addreq_cmd.go`
- Test: `internal/cli/addreq_test.go`

Behavior: write `<workspaceRoot>/.hive/inbox/req-<unix-ts>-<short-id>.txt` with the text as body. Manager drains on next tick.

- [ ] **Step 8.1: Write failing test**

`internal/cli/addreq_test.go`:

```go
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddReq_WritesInboxFile(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".hive", "inbox"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RunAddReq(tmp, "Build a /healthz endpoint"); err != nil {
		t.Fatalf("RunAddReq: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(tmp, ".hive", "inbox"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d files in inbox, want 1", len(entries))
	}
	if !strings.HasPrefix(entries[0].Name(), "req-") {
		t.Errorf("filename: got %q, want req-* prefix", entries[0].Name())
	}
	data, err := os.ReadFile(filepath.Join(tmp, ".hive", "inbox", entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "Build a /healthz endpoint\n" {
		t.Errorf("body: got %q", data)
	}
}

func TestAddReq_ErrorsIfNoWorkspace(t *testing.T) {
	tmp := t.TempDir()
	if err := RunAddReq(tmp, "x"); err == nil {
		t.Fatal("expected error when no .hive/inbox/ exists")
	}
}
```

- [ ] **Step 8.2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestAddReq -v`
Expected: FAIL.

- [ ] **Step 8.3: Implement `internal/cli/addreq.go`**

```go
package cli

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RunAddReq writes a requirement file to <workspaceRoot>/.hive/inbox/.
func RunAddReq(workspaceRoot, text string) error {
	inbox := filepath.Join(workspaceRoot, ".hive", "inbox")
	if info, err := os.Stat(inbox); err != nil || !info.IsDir() {
		return fmt.Errorf("inbox not found at %s: run `hive init` first", inbox)
	}

	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return err
	}
	name := fmt.Sprintf("req-%d-%s.txt", time.Now().Unix(), hex.EncodeToString(suffix))

	if !endsWithNewline(text) {
		text += "\n"
	}
	return os.WriteFile(filepath.Join(inbox, name), []byte(text), 0o644)
}

func endsWithNewline(s string) bool {
	return len(s) > 0 && s[len(s)-1] == '\n'
}
```

- [ ] **Step 8.4: Wire the command**

`cmd/hive/addreq_cmd.go`:

```go
package main

import (
	"errors"
	"os"
	"strings"

	"github.com/nikrich/hungry-ghost-hive-v2/internal/cli"
	"github.com/nikrich/hungry-ghost-hive-v2/internal/paths"
	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "add-req [text...]",
		Short: "Add a requirement to the manager's inbox",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			ws, err := paths.FindWorkspaceRoot(cwd)
			if err != nil {
				return errors.New("not inside a hive workspace (no .hive/ in ancestry)")
			}
			return cli.RunAddReq(ws, strings.Join(args, " "))
		},
	}
	rootCmd.AddCommand(cmd)
}
```

- [ ] **Step 8.5: Run tests + smoke test**

```sh
go test ./internal/cli/ -run TestAddReq -v
go build ./cmd/hive
TMPDIR=$(mktemp -d)
./hive init --dir "$TMPDIR" --workspace-slug demo --no-clone
cd "$TMPDIR" && /Users/jannik/development/nikrich/hungry-ghost-hive-v2/hive add-req "Build a healthz endpoint"
ls .hive/inbox/
cat .hive/inbox/req-*.txt
```

Expected: PASS for tests; smoke test shows a `req-*.txt` file with the body.

- [ ] **Step 8.6: Commit**

```sh
cd /Users/jannik/development/nikrich/hungry-ghost-hive-v2
git add internal/cli/addreq.go internal/cli/addreq_test.go cmd/hive/addreq_cmd.go
git commit -m "feat(cli): add 'hive add-req' command"
```

---

## Task 9: `hive logs` command

**Files:**
- Create: `cmd/hive/logs_cmd.go` (no separate `internal/cli/logs.go` — too thin to abstract)

- [ ] **Step 9.1: Implement `cmd/hive/logs_cmd.go`**

```go
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/nikrich/hungry-ghost-hive-v2/internal/paths"
	"github.com/spf13/cobra"
)

func init() {
	var follow bool
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Print (or tail) the manager log",
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			ws, err := paths.FindWorkspaceRoot(cwd)
			if err != nil {
				return errors.New("not inside a hive workspace")
			}
			logPath := filepath.Join(ws, ".hive", "manager.log")
			return printOrTail(logPath, follow)
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow the log (like tail -f)")
	rootCmd.AddCommand(cmd)
}

func printOrTail(path string, follow bool) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "no manager log yet — run `hive run` first")
			return nil
		}
		return err
	}
	defer f.Close()

	if _, err := io.Copy(os.Stdout, f); err != nil {
		return err
	}
	if !follow {
		return nil
	}

	for {
		time.Sleep(500 * time.Millisecond)
		if _, err := io.Copy(os.Stdout, f); err != nil {
			return err
		}
	}
}
```

- [ ] **Step 9.2: Build + smoke test**

```sh
go build ./cmd/hive
TMPDIR=$(mktemp -d)
./hive init --dir "$TMPDIR" --workspace-slug demo --no-clone
cd "$TMPDIR" && /Users/jannik/development/nikrich/hungry-ghost-hive-v2/hive logs
```

Expected: "no manager log yet" message.

- [ ] **Step 9.3: Commit**

```sh
cd /Users/jannik/development/nikrich/hungry-ghost-hive-v2
git add cmd/hive/logs_cmd.go
git commit -m "feat(cli): add 'hive logs' command"
```

---

## Task 10: `hive status` command

**Files:**
- Create: `internal/cli/status.go`
- Create: `cmd/hive/status_cmd.go`
- Test: `internal/cli/status_test.go`

Behavior: read drawers (stories, agents, escalations) from the workspace's mempalace wing, read the diary tail, print a colored summary table.

- [ ] **Step 10.1: Write failing test**

`internal/cli/status_test.go`:

```go
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderStatus_IncludesStoryCounts(t *testing.T) {
	tmp := t.TempDir()
	wingRoot := filepath.Join(tmp, "wings", "hive-test")
	storyDir := filepath.Join(wingRoot, "rooms", "stories")
	if err := os.MkdirAll(storyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i, s := range []string{"pending", "pending", "merged"} {
		path := filepath.Join(storyDir, "STORY-00"+string(rune('1'+i))+".md")
		body := "---\ntitle: t\ntype: story\nstatus: " + s + "\n---\nbody\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var buf bytes.Buffer
	if err := RenderStatus(&buf, wingRoot); err != nil {
		t.Fatalf("RenderStatus: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "stories: 3") {
		t.Errorf("expected 'stories: 3' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "pending: 2") {
		t.Errorf("expected 'pending: 2' in output, got:\n%s", out)
	}
}
```

- [ ] **Step 10.2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestRenderStatus -v`
Expected: FAIL.

- [ ] **Step 10.3: Implement `internal/cli/status.go`**

```go
package cli

import (
	"fmt"
	"io"

	"github.com/nikrich/hungry-ghost-hive-v2/internal/drawers"
)

// RenderStatus writes a status summary for the given wing root to out.
func RenderStatus(out io.Writer, wingRoot string) error {
	stories, err := drawers.List(wingRoot, "stories")
	if err != nil {
		return err
	}
	agents, err := drawers.List(wingRoot, "agents")
	if err != nil {
		return err
	}
	escalations, err := drawers.List(wingRoot, "escalations")
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "stories: %d\n", len(stories))
	statusCounts := map[string]int{}
	for _, s := range stories {
		statusCounts[s.Status]++
	}
	for _, k := range []string{"pending", "assigned", "in-progress", "review", "merged", "blocked", "abandoned"} {
		if c := statusCounts[k]; c > 0 {
			fmt.Fprintf(out, "  %s: %d\n", k, c)
		}
	}

	live := drawers.FilterByStatus(agents, "live")
	fmt.Fprintf(out, "agents live: %d\n", len(live))
	for _, a := range live {
		fmt.Fprintf(out, "  %s (%s) -> %s\n", a.Title, a.Role, a.Story)
	}

	open := drawers.FilterByStatus(escalations, "open")
	fmt.Fprintf(out, "open escalations: %d\n", len(open))
	for _, e := range open {
		fmt.Fprintf(out, "  %s\n", e.Title)
	}
	return nil
}
```

- [ ] **Step 10.4: Wire the command**

`cmd/hive/status_cmd.go`:

```go
package main

import (
	"errors"
	"os"

	"github.com/nikrich/hungry-ghost-hive-v2/internal/cli"
	"github.com/nikrich/hungry-ghost-hive-v2/internal/config"
	"github.com/nikrich/hungry-ghost-hive-v2/internal/paths"
	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Print current hive state from mempalace",
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			ws, err := paths.FindWorkspaceRoot(cwd)
			if err != nil {
				return errors.New("not inside a hive workspace")
			}
			cfg, err := config.Load(paths.HiveDir(ws) + "/config.yaml")
			if err != nil {
				return err
			}
			mempalaceRoot, err := paths.MempalaceRoot()
			if err != nil {
				return err
			}
			wingRoot := paths.WingDir(mempalaceRoot, cfg.WorkspaceSlug)
			return cli.RenderStatus(os.Stdout, wingRoot)
		},
	}
	rootCmd.AddCommand(cmd)
}
```

- [ ] **Step 10.5: Run tests + smoke test**

```sh
go test ./internal/cli/ -run TestRenderStatus -v
go build ./cmd/hive
TMPDIR=$(mktemp -d)
./hive init --dir "$TMPDIR" --workspace-slug demo --no-clone
MEMPALACE_ROOT=/tmp/fake-mempalace cd "$TMPDIR" && /Users/jannik/development/nikrich/hungry-ghost-hive-v2/hive status
```

Expected: tests PASS; smoke test shows zero counts (no mempalace wing yet).

- [ ] **Step 10.6: Commit**

```sh
cd /Users/jannik/development/nikrich/hungry-ghost-hive-v2
git add internal/cli/status.go internal/cli/status_test.go cmd/hive/status_cmd.go
git commit -m "feat(cli): add 'hive status' command"
```

---

## Task 11: `internal/watchdog` package and `hive run` / `hive stop` commands

**Files:**
- Create: `internal/watchdog/watchdog.go`
- Create: `cmd/hive/run_cmd.go`
- Create: `cmd/hive/stop_cmd.go`

The watchdog runs in the foreground for Phase 1 (no double-fork detach yet — keep it simple, document that the user runs it under `nohup`/`tmux`/etc.). This is a pragmatic Phase 1 shortcut; Phase 2+ can add proper daemonization.

- [ ] **Step 11.1: Implement `internal/watchdog/watchdog.go`**

```go
// Package watchdog runs the supervisor loop that keeps the manager process alive.
package watchdog

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/nikrich/hungry-ghost-hive-v2/internal/proc"
)

// Options controls Run.
type Options struct {
	WorkspaceRoot    string
	TickInterval     time.Duration
	ManagerTimeout   time.Duration
	ClaudeBinary     string // default "claude"
	ManagerPrompt    string // appended to system prompt
}

// Run executes the supervisor loop until SIGTERM/SIGINT or context cancellation.
func Run(ctx context.Context, opts Options) error {
	if opts.ClaudeBinary == "" {
		opts.ClaudeBinary = "claude"
	}
	if opts.ManagerPrompt == "" {
		opts.ManagerPrompt = "Invoke the hive manager skill and do one tick."
	}

	hiveDir := filepath.Join(opts.WorkspaceRoot, ".hive")
	watchdogPid := filepath.Join(hiveDir, "watchdog.pid")
	managerPid := filepath.Join(hiveDir, "manager.pid")
	watchdogLog := filepath.Join(hiveDir, "watchdog.log")
	managerLog := filepath.Join(hiveDir, "manager.log")

	if err := proc.WritePidFile(watchdogPid, os.Getpid()); err != nil {
		return fmt.Errorf("write watchdog pid: %w", err)
	}
	defer proc.ClearPidFile(watchdogPid)

	appendLog(watchdogLog, "watchdog start pid=%d tick=%s", os.Getpid(), opts.TickInterval)
	defer appendLog(watchdogLog, "watchdog stop")

	// Honor SIGTERM/SIGINT.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-sigCh:
			return nil
		default:
		}

		tickStart := time.Now()
		err := runOneTick(opts, managerPid, managerLog)
		dur := time.Since(tickStart)
		if err != nil {
			appendLog(watchdogLog, "tick error=%v dur=%s", err, dur)
		} else {
			appendLog(watchdogLog, "tick ok dur=%s", dur)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-sigCh:
			return nil
		case <-time.After(opts.TickInterval):
		}
	}
}

func runOneTick(opts Options, managerPid, managerLog string) error {
	logFile, err := os.OpenFile(managerLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer logFile.Close()

	cmd := exec.Command(
		opts.ClaudeBinary,
		"--print",
		"--permission-mode", "acceptEdits",
		"--append-system-prompt", opts.ManagerPrompt,
	)
	cmd.Dir = opts.WorkspaceRoot
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start manager: %w", err)
	}
	if err := proc.WritePidFile(managerPid, cmd.Process.Pid); err != nil {
		return err
	}
	defer proc.ClearPidFile(managerPid)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(opts.ManagerTimeout):
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		return fmt.Errorf("manager timed out after %s", opts.ManagerTimeout)
	}
}

func appendLog(path, format string, args ...any) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s "+format+"\n", append([]any{time.Now().Format(time.RFC3339)}, args...)...)
}
```

- [ ] **Step 11.2: Implement `cmd/hive/run_cmd.go`**

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nikrich/hungry-ghost-hive-v2/internal/config"
	"github.com/nikrich/hungry-ghost-hive-v2/internal/paths"
	"github.com/nikrich/hungry-ghost-hive-v2/internal/proc"
	"github.com/nikrich/hungry-ghost-hive-v2/internal/watchdog"
	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the watchdog + manager supervisor loop in the foreground",
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			ws, err := paths.FindWorkspaceRoot(cwd)
			if err != nil {
				return errors.New("not inside a hive workspace")
			}

			// Single-instance check.
			watchdogPid := filepath.Join(ws, ".hive", "watchdog.pid")
			if pid, _ := proc.ReadPidFile(watchdogPid); pid > 0 && proc.IsAlive(pid) {
				return fmt.Errorf("hive run already active (pid %d)", pid)
			}

			cfg, err := config.Load(filepath.Join(ws, ".hive", "config.yaml"))
			if err != nil {
				return err
			}

			fmt.Printf("hive run started (workspace=%s, tick=%ds, max_workers=%d)\n",
				cfg.WorkspaceSlug, cfg.TickIntervalSeconds, cfg.MaxWorkers)
			fmt.Println("Press Ctrl-C to stop.")

			return watchdog.Run(context.Background(), watchdog.Options{
				WorkspaceRoot:  ws,
				TickInterval:   time.Duration(cfg.TickIntervalSeconds) * time.Second,
				ManagerTimeout: time.Duration(cfg.ManagerTimeoutSeconds) * time.Second,
			})
		},
	}
	rootCmd.AddCommand(cmd)
}
```

- [ ] **Step 11.3: Implement `cmd/hive/stop_cmd.go`**

```go
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nikrich/hungry-ghost-hive-v2/internal/paths"
	"github.com/nikrich/hungry-ghost-hive-v2/internal/proc"
	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the running hive watchdog + manager + workers",
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			ws, err := paths.FindWorkspaceRoot(cwd)
			if err != nil {
				return errors.New("not inside a hive workspace")
			}
			hiveDir := paths.HiveDir(ws)

			// Stop watchdog (it'll cascade to manager via its own signal handler).
			if pid, _ := proc.ReadPidFile(filepath.Join(hiveDir, "watchdog.pid")); pid > 0 {
				fmt.Printf("stopping watchdog pid=%d\n", pid)
				if err := proc.Terminate(pid, 30*time.Second); err != nil {
					fmt.Fprintf(os.Stderr, "watchdog: %v\n", err)
				}
			}

			// Kill any orphan manager (shouldn't normally exist).
			if pid, _ := proc.ReadPidFile(filepath.Join(hiveDir, "manager.pid")); pid > 0 && proc.IsAlive(pid) {
				fmt.Printf("killing orphan manager pid=%d\n", pid)
				_ = proc.Terminate(pid, 5*time.Second)
			}

			// Kill any orphan workers.
			agentsDir := filepath.Join(hiveDir, "agents")
			entries, _ := os.ReadDir(agentsDir)
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				workerPid := filepath.Join(agentsDir, e.Name(), "worker.pid")
				if pid, _ := proc.ReadPidFile(workerPid); pid > 0 && proc.IsAlive(pid) {
					fmt.Printf("killing worker pid=%d (%s)\n", pid, e.Name())
					_ = proc.Terminate(pid, 5*time.Second)
				}
			}

			fmt.Println("stopped")
			return nil
		},
	}
	rootCmd.AddCommand(cmd)
}
```

- [ ] **Step 11.4: Build and confirm no compile errors**

Run: `go build ./cmd/hive`
Expected: clean build.

- [ ] **Step 11.5: Smoke test (without claude CLI)**

This is a partial smoke test — the watchdog will try to invoke `claude` and fail, which exercises the error logging path:

```sh
TMPDIR=$(mktemp -d)
./hive init --dir "$TMPDIR" --workspace-slug demo --no-clone
cd "$TMPDIR" && timeout 5 /Users/jannik/development/nikrich/hungry-ghost-hive-v2/hive run || true
cat .hive/watchdog.log
cat .hive/manager.log 2>/dev/null || echo "(no manager log — expected since claude not present)"
```

Expected: watchdog.log shows tick errors (claude not found); process exits cleanly on timeout.

- [ ] **Step 11.6: Commit**

```sh
cd /Users/jannik/development/nikrich/hungry-ghost-hive-v2
git add internal/watchdog/ cmd/hive/run_cmd.go cmd/hive/stop_cmd.go
git commit -m "feat(watchdog): add supervisor loop and run/stop commands"
```

---

## Task 12: Author the manager skill (`assets/skills/manager.md`)

**Files:**
- Modify: `assets/skills/manager.md` (overwrite placeholder)

The skill must instruct claude (running per tick) to:
1. Drain `.hive/inbox/` — for each file, file a requirement drawer in mempalace, then remove the file.
2. List stories with `status=pending`. If any exist and concurrent worker count < `max_workers`, pick one and spawn a worker.
3. Filing-a-finding semantics, escalations: out of scope for Phase 1.
4. Append a diary entry summarizing the tick.

- [ ] **Step 12.1: Write the manager skill**

Overwrite `assets/skills/manager.md` with:

```markdown
---
name: hive-manager
description: Use ONLY when invoked as the hive manager process — coordinates story execution by spawning worker subprocesses. Triggered by the watchdog every tick.
---

# Hive Manager — One-Tick Skill

You are the **hive manager**. The watchdog invoked you to do exactly **one tick** of work. Be decisive and brief. Exit when done — the watchdog will invoke you again next tick.

## What you do each tick

1. **Drain the inbox.** For each file in `.hive/inbox/`:
   - Read its contents (one requirement per file)
   - File a `requirement` drawer via `mempalace_add_drawer` with:
     - wing: `hive-<workspace-slug>` (read slug from `.hive/config.yaml`)
     - room: `requirements`
     - frontmatter: `type: requirement`, `status: pending`, `created_at: <now>`
   - For Phase 1: also immediately file a single `story` drawer via `mempalace_add_drawer` mirroring the requirement (since tech-lead decomposition isn't in scope yet). Story: same title, `type: story`, `status: pending`, `points: 3`, `team: <first team from config>`.
   - Delete the inbox file.

2. **Spawn workers.** Read the current state:
   - `mempalace_list_drawers` in wing/room `agents` → count drawers with `status=live`.
   - `mempalace_list_drawers` in wing/room `stories` → find drawers with `status=pending`.
   - If `live < max_workers` and there's a pending story:
     - Pick the oldest pending story.
     - Generate a short agent ID (8 random hex chars).
     - Create the worktree:
       ```bash
       git -C repos/<team> worktree add ../<team>--junior-<id> -b agent/<team>--junior-<id>
       ```
     - Create `.hive/agents/<id>/`:
       - `context.md` — a markdown brief telling the worker: agent ID, role (junior), team, story title + drawer body, branch name, what to do (read the junior skill, do the work, file outcome, exit)
       - `worker.pid` will be written after spawn
       - `started_at` = unix timestamp
     - Spawn the worker:
       ```bash
       claude --print --permission-mode acceptEdits --append-system-prompt "You are agent <id>. Read .hive/agents/<id>/context.md and the junior skill. Begin." > /dev/null 2>&1 &
       echo $! > .hive/agents/<id>/worker.pid
       ```
       Record the session path: `find ~/.claude/projects -name "*.jsonl" -newer .hive/agents/<id>/started_at | head -1 > .hive/agents/<id>/session.txt` (small race; acceptable for Phase 1).
     - File an `agent-state` drawer via `mempalace_add_drawer`: wing `hive-<slug>`, room `agents`, frontmatter `type: agent-state`, `status: live`, `role: junior`, `team: <team>`, `current_story: <story title>`, `worktree: repos/<team>--junior-<id>`, `started_at: <iso>`.
     - Update the story drawer via `mempalace_update_drawer`: `status: assigned`, `assigned_to: <agent-id>`.

3. **Reap exited workers.** For each `.hive/agents/<id>/` directory:
   - Read `worker.pid`. If the PID is not alive (use `kill -0 <pid> 2>/dev/null; echo $?`):
     - Find the corresponding `agent-state` drawer; if still `status=live`, this is an orphan exit. Update via `mempalace_update_drawer` to `status=exited, exit_reason=completed` (Phase 1 assumes success; Phase 4 will add stuck detection).
     - Delete the worktree: `git -C repos/<team> worktree remove ../<team>--junior-<id> --force` (best-effort).
     - Delete `.hive/agents/<id>/`.

4. **File a diary entry.** Use `mempalace_diary_write` to append:
   ```
   manager  tick-end  spawned=<n> reaped=<n> live=<n> pending=<n>
   ```
   (mempalace adds the timestamp and actor formatting; you provide the event + detail.)

## Constraints — read before acting

- **Do exactly one tick.** Do not loop. Do not wait for spawned workers. Exit as soon as you've done the above.
- **Be silent on success.** No stdout chatter; the watchdog captures whatever you print.
- **If anything is wrong** (no config, no mempalace, missing team repo): file a `finding` drawer describing the problem and exit. Do not try to fix it yourself in Phase 1.
- **Workspace slug** comes from `.hive/config.yaml` (`workspace_slug` field).
- **Spawn one worker per tick max** in Phase 1 (simpler — Phase 3 lifts this).
```

- [ ] **Step 12.2: Verify the skill is included in the embedded FS**

Run: `go test ./assets/ -v`
Expected: PASS (the existing `TestSkillsFS_IncludesManagerSkill` test verifies the file is present).

- [ ] **Step 12.3: Commit**

```sh
git add assets/skills/manager.md
git commit -m "feat(skills): author manager skill for phase 1 happy path"
```

---

## Task 13: Author the junior skill (`assets/skills/junior.md`)

**Files:**
- Modify: `assets/skills/junior.md`

- [ ] **Step 13.1: Write the junior skill**

Overwrite `assets/skills/junior.md`:

```markdown
---
name: hive-junior
description: Use when spawned as a junior hive worker. Reads context, implements a story, opens a PR, files outcome, exits.
---

# Hive Junior — Worker Skill

You are a **junior hive worker**. The manager spawned you to implement exactly one story. Your cwd is your git worktree.

## Setup

1. Read `.hive/agents/<YOUR_ID>/context.md` for: your agent ID, the story title and body, the team's repo path, the branch name to push.
2. Read the team's `README.md`, `CLAUDE.md` (if present), and `package.json` / `go.mod` / `pyproject.toml` to orient yourself.

## Doing the work

1. Make the minimum change required to satisfy the story's acceptance criteria.
2. If the project has tests, write or update them to cover your change. Run them.
3. Use the `tasks/creating-a-pr.md` task skill to commit + push + open a PR.

## Filing your outcome

After the PR is open:

1. Update your `agent-state` drawer via `mempalace_update_drawer`:
   - wing: `hive-<workspace-slug>` (from `.hive/config.yaml`)
   - room: `agents`
   - find drawer where `title == agent-<YOUR_ID>` (use `mempalace_list_drawers` to locate)
   - update fields: `status: exited`, `exit_reason: completed`, `ended_at: <iso>`
2. Update the story drawer via `mempalace_update_drawer`:
   - room: `stories`
   - find drawer matching the story you worked on
   - update: `status: review`, `pr_url: <url>`
3. File a finding via `tasks/filing-a-finding.md` if you learned anything durable (a bug pattern, a missing setup step, a useful library trick). Otherwise skip.

## Constraints

- **One story only.** Do not pick up other pending stories.
- **If you cannot complete the work**: file a finding describing what blocked you, then update your `agent-state` to `status: exited, exit_reason: escalated`. Do not push a half-finished PR. (Phase 2 introduces a proper escalation skill — for Phase 1, the finding drawer is enough.)
- **Permission-bypass mode is active.** Do not try to prompt the user.
- **Exit cleanly.** When done, just exit — the manager will reap you on the next tick.
```

- [ ] **Step 13.2: Commit**

```sh
git add assets/skills/junior.md
git commit -m "feat(skills): author junior worker skill for phase 1"
```

---

## Task 14: Author `tasks/creating-a-pr.md`

**Files:**
- Modify: `assets/skills/tasks/creating-a-pr.md`

- [ ] **Step 14.1: Write the task skill**

Overwrite `assets/skills/tasks/creating-a-pr.md`:

```markdown
---
name: hive-creating-a-pr
description: Use when a hive worker needs to commit changes, push, and open a pull request via gh.
---

# Creating a PR

Use after your code changes are complete and tests pass.

## Steps

1. **Commit.** Use a conventional commit prefix matching the change type:
   - `feat:` for new functionality
   - `fix:` for bug fixes
   - `chore:`, `docs:`, `test:`, `refactor:` as appropriate

   Subject line under 70 chars. Body explains *why*, not *what* (the diff explains what).

   ```bash
   git add <specific-files>   # never `git add -A` — risk of committing junk
   git commit -m "$(cat <<'EOF'
   feat: <subject>
   
   <why this change matters, in 1-3 sentences>
   EOF
   )"
   ```

2. **Push** to the branch name in your context.md (already created by the manager as `agent/<team>--junior-<id>`):
   ```bash
   git push -u origin <branch-name>
   ```

3. **Open the PR with `gh`:**
   ```bash
   gh pr create --title "<feat/fix/etc>: <subject>" --body "$(cat <<'EOF'
   ## Summary
   - <1-3 bullets>
   
   ## How verified
   - <commands run, manual checks done>
   
   ## Linked story
   <STORY-ID from your context.md>
   EOF
   )"
   ```

   Capture the PR URL from `gh pr create` output — you'll need it for your outcome drawer.

## Safety

- Never use `--force` or `--no-verify`.
- Never amend an existing public commit.
- If `git push` is rejected (someone updated the branch), do `git pull --rebase` and try again. Do not force-push.
- If pre-commit hooks fail, fix the issue and commit again — do not bypass.
```

- [ ] **Step 14.2: Commit**

```sh
git add assets/skills/tasks/creating-a-pr.md
git commit -m "feat(skills): author creating-a-pr task skill"
```

---

## Task 15: Author `tasks/filing-a-finding.md`

**Files:**
- Modify: `assets/skills/tasks/filing-a-finding.md`

- [ ] **Step 15.1: Write the task skill**

Overwrite `assets/skills/tasks/filing-a-finding.md`:

```markdown
---
name: hive-filing-a-finding
description: Use when a hive worker discovers durable knowledge worth keeping across runs — a bug pattern, a non-obvious gotcha, a useful trick, an architectural decision.
---

# Filing a Finding

Use sparingly. Findings are knowledge the *next* engineer (human or agent) would want to discover. Trivia, restatements of obvious code, and one-off task notes do **not** belong here.

## When to file

- A bug whose root cause was non-obvious (and the fix is surprising)
- A local-dev gotcha (e.g., "service X must be running on port Y or feature Z silently fails")
- A useful library or pattern discovery that future workers should know
- An architectural decision worth recording with rationale

## When NOT to file

- "I made the change requested" — that's a PR, not a finding
- "I learned that <library> has a function <X>" — that's just documentation
- Anything already in the codebase as a comment

## How to file

Use the `mempalace_remember` MCP tool with:

- **wing:** `hive-<workspace-slug>` (from `.hive/config.yaml`)
- **room:** `findings`
- **frontmatter:**
  ```yaml
  type: finding
  added_by: agent-<YOUR_ID>
  story: <STORY-ID from your context.md>
  tags: [<2-5 relevant tags>]
  ```
- **content:** A markdown body with:
  - **Title** (one short sentence stating the finding)
  - **Symptom** (what you observed)
  - **Root cause** (what was actually happening)
  - **Resolution** (what fixed it)
  - **Avoid** (what *not* to try next time)

Keep it under 400 words. Future you will thank present you for being terse.
```

- [ ] **Step 15.2: Commit**

```sh
git add assets/skills/tasks/filing-a-finding.md
git commit -m "feat(skills): author filing-a-finding task skill"
```

---

## Task 16: End-to-end manual verification

**Files:**
- Create: `docs/plans/2026-05-27-phase-1-verification-runbook.md` (records the verification steps and results)

This is the gate that proves Phase 1 actually works. Requires a real `claude` CLI on PATH and a running mempalace MCP.

- [ ] **Step 16.1: Pre-flight checks**

Verify on your machine:

```sh
which claude          # must exist
echo $MEMPALACE_ROOT  # must be set
ls $MEMPALACE_ROOT    # mempalace storage directory must exist
```

If any fail: install/configure those before continuing.

- [ ] **Step 16.2: Pick a real test repo**

Use a small throwaway repo you control (or fork something tiny). The story will be: "Add a `// HELLO_HIVE` comment at the top of README.md". Trivial — but it exercises the full pipeline.

- [ ] **Step 16.3: Init a verification workspace**

```sh
mkdir /tmp/hive-verify
cd /tmp/hive-verify
git clone <your-test-repo-url> repos/test-team
/Users/jannik/development/nikrich/hungry-ghost-hive-v2/hive init \
  --workspace-slug verify \
  --team name=test-team,url=<your-test-repo-url> \
  --no-clone
ls -R .hive .claude
```

Expected: full workspace tree present.

- [ ] **Step 16.4: Add a requirement**

```sh
/Users/jannik/development/nikrich/hungry-ghost-hive-v2/hive add-req "Add a // HELLO_HIVE comment at the top of README.md"
ls .hive/inbox/
cat .hive/inbox/*.txt
```

Expected: inbox has one `req-*.txt` file with the body.

- [ ] **Step 16.5: Run hive for one tick (5-min cap)**

```sh
/Users/jannik/development/nikrich/hungry-ghost-hive-v2/hive run &
HIVE_PID=$!
sleep 90        # let one tick happen (60s tick + spawn time)
/Users/jannik/development/nikrich/hungry-ghost-hive-v2/hive status
cat .hive/watchdog.log
cat .hive/manager.log
ls .hive/agents/        # should show one live agent dir
```

Expected: at least one tick logged; one agent spawned; story status in mempalace moved from `pending` → `assigned` → eventually `review`.

- [ ] **Step 16.6: Wait for completion + verify PR**

Let the manager run for ~5-10 minutes. Then:

```sh
/Users/jannik/development/nikrich/hungry-ghost-hive-v2/hive status
gh pr list -R <your-test-repo>
```

Expected: A PR open against `<your-test-repo>` from a branch named `agent/test-team--junior-<id>`, adding the `// HELLO_HIVE` comment to README.md.

- [ ] **Step 16.7: Stop hive cleanly**

```sh
/Users/jannik/development/nikrich/hungry-ghost-hive-v2/hive stop
ls .hive/*.pid 2>/dev/null || echo "(no stale pid files — good)"
ps aux | grep -E "claude|hive" | grep -v grep   # should be empty
```

Expected: no stale PIDs, no leftover claude/hive processes.

- [ ] **Step 16.8: Document the verification result**

Create `docs/plans/2026-05-27-phase-1-verification-runbook.md` with:

```markdown
# Phase 1 Verification Runbook

Date: <YYYY-MM-DD>
Tested against: claude <version>, mempalace <version>

## Result

- [ ] Workspace initialized cleanly
- [ ] add-req wrote inbox file
- [ ] Manager drained inbox into requirement + story drawer
- [ ] Worker spawned in worktree
- [ ] PR opened with expected change
- [ ] Story drawer status moved pending → assigned → review
- [ ] hive stop cleanly terminated all processes

## Issues discovered

<list anything that broke; file follow-up issues>

## Time to first PR

<elapsed time from `hive run` to PR opened>
```

- [ ] **Step 16.9: Commit the runbook**

```sh
git add docs/plans/2026-05-27-phase-1-verification-runbook.md
git commit -m "docs: phase 1 verification runbook with results"
```

- [ ] **Step 16.10: Push everything**

```sh
git push
```

Expected: all Phase 1 commits visible on GitHub.

---

## Done criteria

Phase 1 is complete when:

1. `go test ./...` passes (all unit tests in `internal/*` and `assets/`)
2. `go build ./cmd/hive` produces a binary
3. The runbook in Task 16 has every box checked
4. The repo's `main` branch has all commits from Tasks 0-16

## Next phase

Phase 2 (separate plan) adds:
- All 5 role skills (tech-lead, senior, intermediate, junior, qa)
- Tech-lead decomposition flow (requirement → multiple stories)
- QA review skill + flow
- Remaining task skills (`escalating.md`, `reviewing-a-pr.md`, `running-tests.md`, `jira-sync.md`)
- Proper escalation handling
- Story dependencies
