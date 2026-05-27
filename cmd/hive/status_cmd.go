package main

import (
	"errors"
	"os"
	"path/filepath"

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
			cfg, err := config.Load(filepath.Join(paths.HiveDir(ws), "config.yaml"))
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
