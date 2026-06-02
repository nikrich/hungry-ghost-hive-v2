package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nikrich/hungry-ghost-hive-v2/internal/config"
	"github.com/nikrich/hungry-ghost-hive-v2/internal/mempalace"
	"github.com/nikrich/hungry-ghost-hive-v2/internal/paths"
)

// QAOptions is the input to RunQA.
type QAOptions struct {
	WorkspaceRoot string
	StoryTitle    string
	AgentID       string // the QA agent id the manager assigned; used for .hive/agents/<id>/* and the QA-worktree suffix when one is needed
}

// QAResult enumerates the four QA script outcomes. The string value is written
// verbatim to .hive/agents/<id>/qa-result for the manager to read next tick;
// each result also maps to a distinct exit code so the operator can scripts.
type QAResult string

const (
	QAPass      QAResult = "pass"
	QAFailTest  QAResult = "fail-test"
	QAFailMerge QAResult = "fail-merge"
	QAFailSetup QAResult = "fail-setup"
)

// ExitCode returns the process exit code for a result. Distinct codes let the
// caller branch without re-reading the qa-result file.
func (r QAResult) ExitCode() int {
	switch r {
	case QAPass:
		return 0
	case QAFailTest:
		return 2
	case QAFailMerge:
		return 3
	case QAFailSetup:
		return 4
	}
	return 1
}

// RunQA executes the green-path QA flow for one story: set up a worktree on the
// agent branch, run the team's test_command, and on a pass call RunMerge to flip
// the story to merged. Every outcome writes .hive/agents/<AgentID>/qa-result;
// failures additionally write qa-fail.log with the output tail. The manager's
// reap step reads these next tick to decide between cleanup (pass) and escalating
// to an LLM-QA spawn (any fail-*).
//
// Returns (result, nil) on a successful run regardless of QA outcome — the
// outcome is in the result. Returns (QAFailSetup, err) only when the function
// could not even record a result (e.g. unwritable .hive/agents dir).
func RunQA(opts QAOptions) (QAResult, error) {
	if err := mempalace.DumpToFilesystem(opts.WorkspaceRoot); err != nil {
		return finalize(opts, QAFailSetup, fmt.Sprintf("sync chroma: %v", err))
	}

	wingRoot := paths.WorkspaceWingDir(opts.WorkspaceRoot)
	story, err := findStoryByTitle(wingRoot, opts.StoryTitle)
	if err != nil {
		return finalize(opts, QAFailSetup, fmt.Sprintf("find story: %v", err))
	}
	if story.Status != "review" {
		return finalize(opts, QAFailSetup, fmt.Sprintf("story status is %q, expected review", story.Status))
	}
	if story.Team == "" || story.AssignedTo == "" {
		return finalize(opts, QAFailSetup, "story missing team or assigned_to")
	}

	cfg, err := config.Load(filepath.Join(opts.WorkspaceRoot, ".hive", "config.yaml"))
	if err != nil {
		return finalize(opts, QAFailSetup, fmt.Sprintf("load config: %v", err))
	}
	testCmd := teamTestCommand(cfg, story.Team)
	if testCmd == "" {
		return finalize(opts, QAFailSetup, fmt.Sprintf("team %q has no test_command configured", story.Team))
	}

	role, err := lookupAgentRole(wingRoot, story.AssignedTo)
	if err != nil {
		return finalize(opts, QAFailSetup, fmt.Sprintf("lookup worker role: %v", err))
	}

	testWT, err := setupTestWorktree(opts.WorkspaceRoot, story.Team, role, story.AssignedTo, opts.AgentID)
	if err != nil {
		return finalize(opts, QAFailSetup, fmt.Sprintf("setup test worktree: %v", err))
	}

	testOut, testErr := runShell(testWT, testCmd)
	if testErr != nil {
		return finalize(opts, QAFailTest, fmt.Sprintf("test command failed (%v):\n%s", testErr, tail(testOut, 50)))
	}

	if err := RunMerge(MergeOptions{
		WorkspaceRoot: opts.WorkspaceRoot,
		StoryTitle:    opts.StoryTitle,
	}); err != nil {
		return finalize(opts, QAFailMerge, fmt.Sprintf("merge failed: %v", err))
	}

	return finalize(opts, QAPass, "")
}

// finalize writes the qa-result and (on failure) qa-fail.log files. Returns
// (result, nil) on success; (QAFailSetup, err) only if the write itself fails.
func finalize(opts QAOptions, result QAResult, failOutput string) (QAResult, error) {
	agentDir := filepath.Join(opts.WorkspaceRoot, ".hive", "agents", opts.AgentID)
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		return QAFailSetup, fmt.Errorf("mkdir agent dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "qa-result"), []byte(string(result)+"\n"), 0o644); err != nil {
		return QAFailSetup, fmt.Errorf("write qa-result: %w", err)
	}
	if result != QAPass && failOutput != "" {
		if err := os.WriteFile(filepath.Join(agentDir, "qa-fail.log"), []byte(failOutput), 0o644); err != nil {
			return QAFailSetup, fmt.Errorf("write qa-fail.log: %w", err)
		}
	}
	return result, nil
}

func teamTestCommand(cfg *config.Config, team string) string {
	for _, t := range cfg.Teams {
		if t.Name == team {
			return t.TestCommand
		}
	}
	return ""
}

// setupTestWorktree returns a usable working tree for the agent branch.
// Prefers the worker's existing worktree (commits not-yet-pushed are still
// reachable there); falls back to a fresh QA worktree branched off
// origin/<agent_branch> when the worker's was already reaped.
func setupTestWorktree(workspaceRoot, team, role, workerID, qaAgentID string) (string, error) {
	workerWT := filepath.Join(workspaceRoot, "repos", fmt.Sprintf("%s--%s-%s", team, role, workerID))
	if info, err := os.Stat(workerWT); err == nil && info.IsDir() {
		return workerWT, nil
	}

	agentBranch := fmt.Sprintf("agent/%s--%s-%s", team, role, workerID)
	qaWTName := fmt.Sprintf("%s--qa-%s", team, qaAgentID)
	qaWT := filepath.Join(workspaceRoot, "repos", qaWTName)
	repoDir := filepath.Join(workspaceRoot, "repos", team)

	if _, err := os.Stat(repoDir); err != nil {
		return "", fmt.Errorf("team repo %s not found: %w", repoDir, err)
	}
	if err := gitRun(repoDir, "fetch", "origin", agentBranch, "--quiet"); err != nil {
		return "", fmt.Errorf("fetch %s: %w", agentBranch, err)
	}
	if err := gitRun(repoDir, "worktree", "add", "../"+qaWTName, "-b", "qa/"+qaWTName, "origin/"+agentBranch); err != nil {
		return "", fmt.Errorf("worktree add: %w", err)
	}
	return qaWT, nil
}

// runShell evaluates command via bash -c in dir. bash -c is required (not a
// security regression): test_command in .hive/config.yaml is an operator-
// authored shell expression — pipes, &&, env vars, multi-command chains —
// matching the conventions of `npm test`, `pytest -q`, `go test ./...`. The
// trust boundary is .hive/config.yaml itself: anyone who can edit it already
// has the same filesystem privileges this process runs with, so passing
// through bash is no escalation. Do NOT extend this helper to take values
// from a network-reachable source without re-evaluating that trust model.
func runShell(dir, command string) (string, error) {
	cmd := exec.Command("bash", "-c", command)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// tail returns the last n newline-separated lines of s. Trailing blank lines
// (e.g. from a final '\n') do count — matching what an operator would see in `tail -n50`.
func tail(s string, n int) string {
	if n <= 0 || s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}
