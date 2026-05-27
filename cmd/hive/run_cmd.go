package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nikrich/hungry-ghost-hive-v2/internal/config"
	"github.com/nikrich/hungry-ghost-hive-v2/internal/paths"
	"github.com/nikrich/hungry-ghost-hive-v2/internal/proc"
	"github.com/nikrich/hungry-ghost-hive-v2/internal/watchdog"
	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the watchdog + manager supervisor loop in the foreground",
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			ws, err := paths.FindWorkspaceRoot(cwd)
			if err != nil {
				return errors.New("not inside a hive workspace")
			}

			watchdogPid := filepath.Join(ws, ".hive", "watchdog.pid")
			if pid, _ := proc.ReadPidFile(watchdogPid); pid > 0 && proc.IsAlive(pid) {
				return fmt.Errorf("hive run already active (pid %d)", pid)
			}

			cfg, err := config.Load(filepath.Join(ws, ".hive", "config.yaml"))
			if err != nil {
				return err
			}

			fmt.Printf("hive run started (workspace=%s, tick=%ds, max_workers=%d)\n",
				cfg.WorkspaceSlug, cfg.TickIntervalSeconds, cfg.MaxWorkers)
			fmt.Println("Press Ctrl-C to stop.")

			return watchdog.Run(context.Background(), watchdog.Options{
				WorkspaceRoot:  ws,
				TickInterval:   time.Duration(cfg.TickIntervalSeconds) * time.Second,
				ManagerTimeout: time.Duration(cfg.ManagerTimeoutSeconds) * time.Second,
			})
		},
	}
	rootCmd.AddCommand(cmd)
}
