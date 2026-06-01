package cli

import (
	"os"
	"os/exec"
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

// runGit runs git in dir and fails the test on non-zero exit.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return string(out)
}

// setupBareRemoteWithFeatureBranch creates:
//   - a bare remote at <ws>/repos-bare/api.git with main + feature/healthz branches
//   - a working clone at <ws>/repos/api on feature/healthz
//   - an agent branch agent/api--junior-abc123 pushed to the bare remote
//
// Returns the bare remote path (handy for assertions).
func setupBareRemoteWithFeatureBranch(t *testing.T, ws string) string {
	t.Helper()
	bare := filepath.Join(ws, "repos-bare", "api.git")
	clone := filepath.Join(ws, "repos", "api")
	if err := os.MkdirAll(filepath.Dir(bare), 0o755); err != nil {
		t.Fatal(err)
	}

	runGit(t, "", "init", "--bare", bare)

	scratch := filepath.Join(ws, "scratch")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, scratch, "init", "-b", "main")
	runGit(t, scratch, "config", "user.email", "test@example.com")
	runGit(t, scratch, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(scratch, "README.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, scratch, "add", "README.md")
	runGit(t, scratch, "commit", "-m", "seed")
	runGit(t, scratch, "remote", "add", "origin", bare)
	runGit(t, scratch, "push", "origin", "main")
	runGit(t, scratch, "push", "origin", "main:refs/heads/feature/healthz")

	// Working clone, on feature/healthz.
	runGit(t, "", "clone", "-b", "feature/healthz", bare, clone)
	runGit(t, clone, "config", "user.email", "test@example.com")
	runGit(t, clone, "config", "user.name", "test")

	// Build the agent branch off feature/healthz with a real commit, then push.
	runGit(t, clone, "checkout", "-b", "agent/api--junior-abc123")
	if err := os.WriteFile(filepath.Join(clone, "healthz.go"), []byte("package api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, clone, "add", "healthz.go")
	runGit(t, clone, "commit", "-m", "feat: add healthz")
	runGit(t, clone, "push", "origin", "agent/api--junior-abc123")
	runGit(t, clone, "checkout", "feature/healthz")

	return bare
}

func TestRunMerge_HappyPath(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	ws := makeWorkspace(t)
	bare := setupBareRemoteWithFeatureBranch(t, ws)

	storiesDir := filepath.Join(ws, ".hive", "memory", "wings", "hive", "rooms", "stories")
	storyPath := writeDrawerFile(t, storiesDir, "STORY-001.md", `---
type: story
status: review
title: Implement /healthz
team: api
feature_branch: feature/healthz
assigned_to: abc123
---

Body.
`)

	agentsDir := filepath.Join(ws, ".hive", "memory", "wings", "hive", "rooms", "agents")
	writeDrawerFile(t, agentsDir, "agent-abc123.md", `---
type: agent-state
status: exited
title: agent-abc123
role: junior
team: api
---

Body.
`)

	if err := RunMerge(MergeOptions{
		WorkspaceRoot: ws,
		StoryTitle:    "Implement /healthz",
	}); err != nil {
		t.Fatalf("RunMerge: %v", err)
	}

	// The bare remote's feature/healthz should now have a merge commit referencing the agent branch.
	out := runGit(t, "", "-C", bare, "log", "--oneline", "feature/healthz")
	if !strings.Contains(out, "feat: add healthz") {
		t.Errorf("feature/healthz on bare remote missing agent commit:\n%s", out)
	}

	// The story drawer should now read status=merged with merged_at set.
	data, err := os.ReadFile(storyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "status: merged") {
		t.Errorf("drawer status not flipped to merged:\n%s", data)
	}
	if !strings.Contains(string(data), "merged_at:") {
		t.Errorf("drawer missing merged_at:\n%s", data)
	}
}
