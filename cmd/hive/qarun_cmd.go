package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/nikrich/hungry-ghost-hive-v2/internal/cli"
	"github.com/nikrich/hungry-ghost-hive-v2/internal/paths"
	"github.com/spf13/cobra"
)

func init() {
	var storyTitle, agentID string
	cmd := &cobra.Command{
		Use:   "qa-run",
		Short: "Green-path QA: run team tests against an agent branch and merge on pass",
		Long: `Set up a working tree on the worker's agent branch, run the team's
test_command, and on a pass invoke 'hive merge' to flip the story to merged.
Exit codes encode the outcome so the manager can branch without re-reading
the qa-result file:
  0 — pass: tests passed and merge succeeded
  2 — fail-test: test command exited non-zero
  3 — fail-merge: tests passed but merge failed (typically a conflict)
  4 — fail-setup: could not set up worktree, find story, or load config

Every outcome writes .hive/agents/<agent-id>/qa-result (one of pass/fail-*);
failures additionally write qa-fail.log with the last 50 lines of output.

Spawned by the manager's QA step in place of an LLM-QA subprocess on the
happy path. The manager escalates to an LLM-QA on any fail-* result.`,
		RunE: func(c *cobra.Command, args []string) error {
			if storyTitle == "" || agentID == "" {
				return errors.New("--story and --agent-id are both required")
			}
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			ws, err := paths.FindWorkspaceRoot(cwd)
			if err != nil {
				return errors.New("not inside a hive workspace (no .hive/ in ancestry)")
			}
			result, runErr := cli.RunQA(cli.QAOptions{
				WorkspaceRoot: ws,
				StoryTitle:    storyTitle,
				AgentID:       agentID,
			})
			if runErr != nil {
				fmt.Fprintf(os.Stderr, "qa-run: %v\n", runErr)
			}
			fmt.Printf("qa-run result=%s exit=%d\n", result, result.ExitCode())
			os.Exit(result.ExitCode())
			return nil // unreachable; satisfies cobra signature
		},
	}
	cmd.Flags().StringVar(&storyTitle, "story", "", "exact story title (matches the title in the story drawer)")
	cmd.Flags().StringVar(&agentID, "agent-id", "", "the QA agent id the manager assigned (used for .hive/agents/<id>/* output files)")
	rootCmd.AddCommand(cmd)
}
