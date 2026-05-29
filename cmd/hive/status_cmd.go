package main

import (
	"errors"
	"os"

	"github.com/nikrich/hungry-ghost-hive-v2/internal/cli"
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
			// Phase 1.2: workspace-local memory at <workspace>/.hive/memory/wings/hive.
			// No env-var lookup, no slug lookup — wing is deterministic.
			return cli.RenderStatus(os.Stdout, paths.WorkspaceWingDir(ws))
		},
	}
	rootCmd.AddCommand(cmd)
}
