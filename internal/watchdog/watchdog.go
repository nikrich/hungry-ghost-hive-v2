// Package watchdog runs the supervisor loop that keeps the manager process alive.
package watchdog

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/nikrich/hungry-ghost-hive-v2/internal/proc"
)

// Options controls Run.
type Options struct {
	WorkspaceRoot  string
	TickInterval   time.Duration
	ManagerTimeout time.Duration
	ClaudeBinary   string // default "claude"
	ManagerPrompt  string // appended to system prompt
}

// Run executes the supervisor loop until SIGTERM/SIGINT or context cancellation.
func Run(ctx context.Context, opts Options) error {
	if opts.ClaudeBinary == "" {
		opts.ClaudeBinary = "claude"
	}
	if opts.ManagerPrompt == "" {
		opts.ManagerPrompt = "Invoke the hive manager skill and do one tick."
	}

	hiveDir := filepath.Join(opts.WorkspaceRoot, ".hive")
	watchdogPid := filepath.Join(hiveDir, "watchdog.pid")
	managerPid := filepath.Join(hiveDir, "manager.pid")
	watchdogLog := filepath.Join(hiveDir, "watchdog.log")
	managerLog := filepath.Join(hiveDir, "manager.log")

	if err := proc.WritePidFile(watchdogPid, os.Getpid()); err != nil {
		return fmt.Errorf("write watchdog pid: %w", err)
	}
	defer proc.ClearPidFile(watchdogPid)

	appendLog(watchdogLog, "watchdog start pid=%d tick=%s", os.Getpid(), opts.TickInterval)
	defer appendLog(watchdogLog, "watchdog stop")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-sigCh:
			return nil
		default:
		}

		tickStart := time.Now()
		err := runOneTick(opts, managerPid, managerLog)
		dur := time.Since(tickStart)
		if err != nil {
			appendLog(watchdogLog, "tick error=%v dur=%s", err, dur)
		} else {
			appendLog(watchdogLog, "tick ok dur=%s", dur)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-sigCh:
			return nil
		case <-time.After(opts.TickInterval):
		}
	}
}

func runOneTick(opts Options, managerPid, managerLog string) error {
	logFile, err := os.OpenFile(managerLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer logFile.Close()

	cmd := exec.Command(
		opts.ClaudeBinary,
		"--print",
		"--permission-mode", "acceptEdits",
		"--append-system-prompt", opts.ManagerPrompt,
	)
	cmd.Dir = opts.WorkspaceRoot
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start manager: %w", err)
	}
	if err := proc.WritePidFile(managerPid, cmd.Process.Pid); err != nil {
		return err
	}
	defer proc.ClearPidFile(managerPid)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(opts.ManagerTimeout):
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		return fmt.Errorf("manager timed out after %s", opts.ManagerTimeout)
	}
}

func appendLog(path, format string, args ...any) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s "+format+"\n", append([]any{time.Now().Format(time.RFC3339)}, args...)...)
}
