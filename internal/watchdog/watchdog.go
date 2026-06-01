// Package watchdog runs the supervisor loop that keeps the manager process alive.
package watchdog

import (
	"bufio"
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
		// Disambiguate from the legacy capstone-hive skill which may also
		// be installed user-globally. v2 skills use the hive-v2-* prefix.
		opts.ManagerPrompt = "Invoke the hive-v2-manager skill and do exactly one tick of work for this hungry-ghost-hive-v2 workspace. Do NOT invoke capstone-hive or any other hive skill — those are for a different (legacy) architecture."
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

	// Phase 1.4: pass the manager skill as claude's system prompt via
	// --system-prompt-file. This replaces the prior "ask claude to invoke
	// the hive-v2-manager skill" pattern — manager IS the manager because
	// the system prompt says so. P1.3 verification showed the previous
	// pattern resulted in 0 skill invocations and 10 exploratory Bash
	// calls per 5-min tick; inlining removes the discovery cycle entirely.
	managerSkillPath := filepath.Join(opts.WorkspaceRoot, ".claude", "skills", "manager.md")
	mcpConfigPath := filepath.Join(opts.WorkspaceRoot, ".claude", "mcp.json")
	// Phase 1.5: pin MCP config to the workspace-local mcp.json so the
	// mempalace gateway picks up MEMPALACE_ROOT=<workspace>/.hive/memory.
	// --strict-mcp-config ignores the user-level ~/.claude.json which would
	// otherwise point the gateway at a global memory store.
	cmd := exec.Command(
		opts.ClaudeBinary,
		"--print",
		"--permission-mode", "acceptEdits",
		"--mcp-config", mcpConfigPath,
		"--strict-mcp-config",
		"--system-prompt-file", managerSkillPath,
		"Do one tick now.",
	)
	cmd.Dir = opts.WorkspaceRoot

	// Stream child output to manager.log line-by-line so partial output
	// survives SIGKILL on timeout. Without this, claude's buffered stdout
	// is lost when the watchdog kills the manager mid-tick.
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("pipe manager stdout: %w", err)
	}
	// stdout: pipe → scanner → logFile (line-buffered, survives SIGKILL).
	// stderr: direct fd → logFile (typically line/unbuffered, also survives SIGKILL).
	// The earlier `cmd.Stderr = cmd.Stdout` form is a no-op because cmd.Stdout is
	// nil after StdoutPipe() — would have silently discarded stderr.
	cmd.Stderr = logFile

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start manager: %w", err)
	}
	if err := proc.WritePidFile(managerPid, cmd.Process.Pid); err != nil {
		return err
	}
	defer proc.ClearPidFile(managerPid)

	// Drain the pipe in a goroutine; each line lands in manager.log immediately.
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		// Allow up to 1 MiB per line (claude can emit large JSON chunks).
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			_, _ = fmt.Fprintln(logFile, scanner.Text())
		}
	}()

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
