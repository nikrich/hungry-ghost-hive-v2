package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nikrich/hungry-ghost-hive-v2/internal/paths"
	"github.com/nikrich/hungry-ghost-hive-v2/internal/proc"
	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the running hive watchdog + manager + workers",
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			ws, err := paths.FindWorkspaceRoot(cwd)
			if err != nil {
				return errors.New("not inside a hive workspace")
			}
			hiveDir := paths.HiveDir(ws)

			if pid, _ := proc.ReadPidFile(filepath.Join(hiveDir, "watchdog.pid")); pid > 0 {
				fmt.Printf("stopping watchdog pid=%d\n", pid)
				if err := proc.Terminate(pid, 30*time.Second); err != nil {
					fmt.Fprintf(os.Stderr, "watchdog: %v\n", err)
				}
			}

			if pid, _ := proc.ReadPidFile(filepath.Join(hiveDir, "manager.pid")); pid > 0 && proc.IsAlive(pid) {
				fmt.Printf("killing orphan manager pid=%d\n", pid)
				_ = proc.Terminate(pid, 5*time.Second)
			}

			agentsDir := filepath.Join(hiveDir, "agents")
			entries, _ := os.ReadDir(agentsDir)
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				workerPid := filepath.Join(agentsDir, e.Name(), "worker.pid")
				if pid, _ := proc.ReadPidFile(workerPid); pid > 0 && proc.IsAlive(pid) {
					fmt.Printf("killing worker pid=%d (%s)\n", pid, e.Name())
					_ = proc.Terminate(pid, 5*time.Second)
				}
			}

			fmt.Println("stopped")
			return nil
		},
	}
	rootCmd.AddCommand(cmd)
}
