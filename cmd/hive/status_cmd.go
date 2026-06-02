package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/nikrich/hungry-ghost-hive-v2/internal/cli"
	"github.com/nikrich/hungry-ghost-hive-v2/internal/mempalace"
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
			// Mirror chroma → disk before reading so we don't print a stale
			// snapshot. Without this, the disk view only refreshes during
			// `hive merge` and `hive status` can lag the orchestrator by
			// minutes or hours. Errors are non-fatal — fall through and
			// render whatever the disk currently has.
			if syncErr := mempalace.DumpToFilesystem(ws); syncErr != nil {
				fmt.Fprintf(os.Stderr, "warn: pre-read sync failed: %v (showing last on-disk state)\n", syncErr)
			}
			// Phase 1.2: workspace-local memory at <workspace>/.hive/memory/wings/hive.
			// No env-var lookup, no slug lookup — wing is deterministic.
			return cli.RenderStatus(os.Stdout, paths.WorkspaceWingDir(ws))
		},
	}
	rootCmd.AddCommand(cmd)
}
