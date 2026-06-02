package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nikrich/hungry-ghost-hive-v2/internal/config"
)

func TestQAResult_ExitCode(t *testing.T) {
	cases := []struct {
		result QAResult
		want   int
	}{
		{QAPass, 0},
		{QAFailTest, 2},
		{QAFailMerge, 3},
		{QAFailSetup, 4},
		{QAResult("unknown"), 1},
	}
	for _, tc := range cases {
		if got := tc.result.ExitCode(); got != tc.want {
			t.Errorf("%s.ExitCode() = %d, want %d", tc.result, got, tc.want)
		}
	}
}

func TestTail(t *testing.T) {
	cases := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"empty in returns empty", "", 50, ""},
		{"n=0 returns input", "a\nb\nc", 0, "a\nb\nc"},
		{"fewer lines than n returns all", "a\nb\nc", 10, "a\nb\nc"},
		{"more lines returns last n", "a\nb\nc\nd\ne", 2, "d\ne"},
		{"exactly n returns all", "a\nb\nc", 3, "a\nb\nc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tail(tc.in, tc.n); got != tc.want {
				t.Errorf("tail(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
			}
		})
	}
}

func TestTeamTestCommand(t *testing.T) {
	cfg := &config.Config{
		Teams: []config.Team{
			{Name: "api", TestCommand: "go test ./..."},
			{Name: "web", TestCommand: "npm test"},
		},
	}
	cases := []struct {
		team, want string
	}{
		{"api", "go test ./..."},
		{"web", "npm test"},
		{"missing", ""},
	}
	for _, tc := range cases {
		if got := teamTestCommand(cfg, tc.team); got != tc.want {
			t.Errorf("teamTestCommand(%q) = %q, want %q", tc.team, got, tc.want)
		}
	}
}

func TestFinalize_WritesResultFile(t *testing.T) {
	ws := t.TempDir()
	opts := QAOptions{WorkspaceRoot: ws, AgentID: "abc123"}

	result, err := finalize(opts, QAPass, "")
	if err != nil {
		t.Fatalf("finalize pass: %v", err)
	}
	if result != QAPass {
		t.Errorf("result: got %s, want pass", result)
	}

	data, err := os.ReadFile(filepath.Join(ws, ".hive", "agents", "abc123", "qa-result"))
	if err != nil {
		t.Fatalf("read qa-result: %v", err)
	}
	if strings.TrimSpace(string(data)) != "pass" {
		t.Errorf("qa-result contents: got %q, want %q", string(data), "pass\n")
	}

	if _, err := os.Stat(filepath.Join(ws, ".hive", "agents", "abc123", "qa-fail.log")); !os.IsNotExist(err) {
		t.Error("qa-fail.log should not exist on pass")
	}
}

func TestFinalize_WritesFailLogOnFailure(t *testing.T) {
	ws := t.TempDir()
	opts := QAOptions{WorkspaceRoot: ws, AgentID: "abc123"}

	result, err := finalize(opts, QAFailTest, "FAIL: TestFoo\n--- got: 1, want: 2")
	if err != nil {
		t.Fatalf("finalize fail-test: %v", err)
	}
	if result != QAFailTest {
		t.Errorf("result: got %s, want fail-test", result)
	}

	data, err := os.ReadFile(filepath.Join(ws, ".hive", "agents", "abc123", "qa-fail.log"))
	if err != nil {
		t.Fatalf("read qa-fail.log: %v", err)
	}
	if !strings.Contains(string(data), "FAIL: TestFoo") {
		t.Errorf("qa-fail.log missing expected content; got %q", string(data))
	}
}

func TestRunQA_FailsSetupWhenStoryMissing(t *testing.T) {
	ws := makeWorkspace(t)
	result, err := RunQA(QAOptions{
		WorkspaceRoot: ws,
		StoryTitle:    "Nonexistent Story",
		AgentID:       "qa-abc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != QAFailSetup {
		t.Errorf("result: got %s, want fail-setup", result)
	}

	failLog, err := os.ReadFile(filepath.Join(ws, ".hive", "agents", "qa-abc", "qa-fail.log"))
	if err != nil {
		t.Fatalf("expected qa-fail.log: %v", err)
	}
	if !strings.Contains(string(failLog), "find story") {
		t.Errorf("qa-fail.log should mention find-story failure; got %q", failLog)
	}
}

func TestRunQA_FailsSetupWhenStoryNotInReview(t *testing.T) {
	ws := makeWorkspace(t)
	storiesDir := filepath.Join(ws, ".hive", "memory", "wings", "hive", "rooms", "stories")
	writeDrawerFile(t, storiesDir, "STORY-001.md", `---
type: story
status: pending
title: My Story
team: api
feature_branch: feature/x
assigned_to: w1
---
Body.
`)

	result, err := RunQA(QAOptions{
		WorkspaceRoot: ws,
		StoryTitle:    "My Story",
		AgentID:       "qa-abc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != QAFailSetup {
		t.Errorf("result: got %s, want fail-setup", result)
	}
}
