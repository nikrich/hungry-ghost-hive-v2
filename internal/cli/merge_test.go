package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeDrawerFile writes a frontmatter+body markdown file to disk and returns the path.
func writeDrawerFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// makeWorkspace returns a workspace root with the minimum .hive layout RunMerge needs.
func makeWorkspace(t *testing.T) string {
	t.Helper()
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".hive", "memory", "wings", "hive", "rooms", "stories"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(ws, ".hive", "memory", "wings", "hive", "rooms", "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".hive", "config.yaml"),
		[]byte("workspace_slug: test\nteams:\n  - name: api\n    repo_path: repos/api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return ws
}

func TestRunMerge_RefusesNonReviewStory(t *testing.T) {
	ws := makeWorkspace(t)
	storiesDir := filepath.Join(ws, ".hive", "memory", "wings", "hive", "rooms", "stories")
	writeDrawerFile(t, storiesDir, "STORY-001.md", `---
type: story
status: pending
title: Implement /healthz
team: api
feature_branch: feature/healthz
assigned_to: abc123
---

Body.
`)

	err := RunMerge(MergeOptions{
		WorkspaceRoot: ws,
		StoryTitle:    "Implement /healthz",
	})
	if err == nil {
		t.Fatal("expected error for non-review story, got nil")
	}
	if !strings.Contains(err.Error(), "cannot merge") {
		t.Errorf("error message: got %q, want substring 'cannot merge'", err.Error())
	}
	if !strings.Contains(err.Error(), "pending") {
		t.Errorf("error should mention current status 'pending', got %q", err.Error())
	}
}

func TestRunMerge_RefusesStoryWithoutFeatureBranch(t *testing.T) {
	ws := makeWorkspace(t)
	storiesDir := filepath.Join(ws, ".hive", "memory", "wings", "hive", "rooms", "stories")
	writeDrawerFile(t, storiesDir, "STORY-001.md", `---
type: story
status: review
title: Legacy P2.A story
team: api
assigned_to: abc123
---

Body.
`)

	err := RunMerge(MergeOptions{
		WorkspaceRoot: ws,
		StoryTitle:    "Legacy P2.A story",
	})
	if err == nil {
		t.Fatal("expected error for story without feature_branch, got nil")
	}
	if !strings.Contains(err.Error(), "feature_branch") {
		t.Errorf("error should mention feature_branch: got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "legacy") {
		t.Errorf("error should mention legacy hint: got %q", err.Error())
	}
}
