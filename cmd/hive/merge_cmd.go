package main

import (
	"errors"
	"os"

	"github.com/nikrich/hungry-ghost-hive-v2/internal/cli"
	"github.com/nikrich/hungry-ghost-hive-v2/internal/paths"
	"github.com/spf13/cobra"
)

func init() {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "merge \"<story-title>\"",
		Short: "Merge a story's agent branch into its parent requirement's feature branch",
		Long: `Merge the worker's agent branch into the requirement's feature branch and
flip the story drawer's status to "merged". The story must be in status "review"
and must have a feature_branch set (Phase 2.D shape). The agent branch is
reconstructed from the story's assigned_to + role.

Phase 2.C's QA role will call this same primitive once it exists.`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			ws, err := paths.FindWorkspaceRoot(cwd)
			if err != nil {
				return errors.New("not inside a hive workspace (no .hive/ in ancestry)")
			}
			return cli.RunMerge(cli.MergeOptions{
				WorkspaceRoot: ws,
				StoryTitle:    args[0],
				DryRun:        dryRun,
			})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print planned git operations without running them")
	rootCmd.AddCommand(cmd)
}
