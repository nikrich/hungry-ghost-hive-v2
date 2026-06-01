package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

			printBanner(cfg, ws)

			return watchdog.Run(context.Background(), watchdog.Options{
				WorkspaceRoot:  ws,
				TickInterval:   time.Duration(cfg.TickIntervalSeconds) * time.Second,
				ManagerTimeout: time.Duration(cfg.ManagerTimeoutSeconds) * time.Second,
			})
		},
	}
	rootCmd.AddCommand(cmd)
}

// printBanner prints the hive startup banner — honeycomb-framed ASCII "HIVE"
// + a config summary block. Uses ANSI yellow when stdout is a TTY; falls
// back to plain text under pipes/redirects.
func printBanner(cfg *config.Config, workspaceRoot string) {
	const (
		yellow = "\033[38;5;214m"
		dim    = "\033[2m"
		bold   = "\033[1m"
		reset  = "\033[0m"
	)
	c := func(code string) string { return code }
	if fi, err := os.Stdout.Stat(); err != nil || (fi.Mode()&os.ModeCharDevice) == 0 {
		// Not a TTY — strip ANSI.
		c = func(string) string { return "" }
	}

	const hexRow = "⬡ ⬡ ⬡ ⬡ ⬡ ⬡ ⬡ ⬡ ⬡ ⬡ ⬡ ⬡ ⬡ ⬡ ⬡ ⬡ ⬡ ⬡ ⬡ ⬡ ⬡ ⬡"
	const art = `      ██╗  ██╗██╗██╗   ██╗███████╗
      ██║  ██║██║██║   ██║██╔════╝
      ███████║██║██║   ██║█████╗
      ██╔══██║██║╚██╗ ██╔╝██╔══╝
      ██║  ██║██║ ╚████╔╝ ███████╗
      ╚═╝  ╚═╝╚═╝  ╚═══╝  ╚══════╝`

	fmt.Println()
	fmt.Printf("   %s%s%s\n", c(yellow), hexRow, c(reset))
	fmt.Println()
	fmt.Printf("%s%s%s\n", c(yellow), art, c(reset))
	fmt.Println()
	fmt.Printf("      %shungry-ghost-hive · v2 · parallel AI dev teams%s\n", c(dim), c(reset))
	fmt.Println()
	fmt.Printf("   %s%s%s\n", c(yellow), hexRow, c(reset))
	fmt.Println()

	teamNames := make([]string, len(cfg.Teams))
	for i, t := range cfg.Teams {
		teamNames[i] = t.Name
	}
	teamLine := strings.Join(teamNames, ", ")
	if teamLine == "" {
		teamLine = "(none configured)"
	}

	rows := [][2]string{
		{"workspace", cfg.WorkspaceSlug},
		{"tick", fmt.Sprintf("%ds", cfg.TickIntervalSeconds)},
		{"max workers", fmt.Sprintf("%d", cfg.MaxWorkers)},
		{"max qa", fmt.Sprintf("%d", cfg.MaxQA)},
		{"teams", teamLine},
		{"workspace root", workspaceRoot},
	}
	for _, r := range rows {
		fmt.Printf("   %s%-15s%s %s\n", c(bold), r[0], c(reset), r[1])
	}
	fmt.Println()
	fmt.Printf("   %sPress Ctrl-C to stop · drop requirements via `hive add-req \"…\"`%s\n", c(dim), c(reset))
	fmt.Println()
}
