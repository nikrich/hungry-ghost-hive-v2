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
