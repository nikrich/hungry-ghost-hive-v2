package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/nikrich/hungry-ghost-hive-v2/internal/paths"
	"github.com/spf13/cobra"
)

func init() {
	var follow bool
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Print (or tail) the manager log",
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			ws, err := paths.FindWorkspaceRoot(cwd)
			if err != nil {
				return errors.New("not inside a hive workspace")
			}
			logPath := filepath.Join(ws, ".hive", "manager.log")
			return printOrTail(logPath, follow)
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow the log (like tail -f)")
	rootCmd.AddCommand(cmd)
}

func printOrTail(path string, follow bool) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "no manager log yet — run `hive run` first")
			return nil
		}
		return err
	}
	defer f.Close()

	if _, err := io.Copy(os.Stdout, f); err != nil {
		return err
	}
	if !follow {
		return nil
	}

	for {
		time.Sleep(500 * time.Millisecond)
		if _, err := io.Copy(os.Stdout, f); err != nil {
			return err
		}
	}
}
