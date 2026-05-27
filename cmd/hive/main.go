package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "hive",
	Short: "AI agent orchestrator — supervises Claude Code subprocesses",
	Long:  `Hive supervises Claude Code subprocesses to coordinate agile-style AI development teams.`,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
