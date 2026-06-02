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

	"github.com/nikrich/hungry-ghost-hive-v2/assets"
	"github.com/nikrich/hungry-ghost-hive-v2/internal/mempalace"
	"github.com/nikrich/hungry-ghost-hive-v2/internal/proc"
)

// Options controls Run.
type Options struct {
	WorkspaceRoot     string
	TickInterval      time.Duration
	ManagerTimeout    time.Duration
	ClaudeBinary      string        // default "claude"
	ManagerPrompt     string        // appended to system prompt
	IdleBackoffMax    time.Duration // cap on the exponential idle-tick backoff; set ≤ TickInterval to disable
	IdleBackoffFactor float64       // multiplier per consecutive idle tick (e.g. 2.0)
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

	// Phase 2.C: write the absolute path of THIS hive binary into the workspace
	// so the manager and QA subprocesses can invoke `$(cat .hive/hive_binary) merge ...`
	// without depending on a global PATH containing the right v2 binary.
	if exe, err := os.Executable(); err == nil {
		_ = os.WriteFile(filepath.Join(hiveDir, "hive_binary"), []byte(exe+"\n"), 0o644)
	}

	// Phase 2.E: re-sync embedded skills into .claude/skills/ on every run startup
	// so an operator who rebuilds the binary doesn't have to re-init the workspace
	// to pick up new/changed skills. Idempotent — overwrites with current embed.
	if err := assets.SyncSkillsToWorkspace(opts.WorkspaceRoot); err != nil {
		appendLog(watchdogLog, "skill sync error=%v (continuing)", err)
	} else {
		appendLog(watchdogLog, "skills synced from embed")
	}

	appendLog(watchdogLog, "watchdog start pid=%d tick=%s idle_backoff_max=%s factor=%v",
		os.Getpid(), opts.TickInterval, opts.IdleBackoffMax, opts.IdleBackoffFactor)
	defer appendLog(watchdogLog, "watchdog stop")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	tickSummaryPath := filepath.Join(hiveDir, "last-tick.json")
	inboxDir := filepath.Join(hiveDir, "inbox")
	consecutiveIdle := 0

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-sigCh:
			return nil
		default:
		}

		// Phase 2.E: pre-tick maintenance — rotate growing logs, free hung agents.
		rotateIfTooLarge(managerLog)
		rotateIfTooLarge(watchdogLog)
		reapHungAgents(opts.WorkspaceRoot, watchdogLog)

		tickStart := time.Now()
		err := runOneTick(opts, managerPid, managerLog)
		dur := time.Since(tickStart)
		if err != nil {
			appendLog(watchdogLog, "tick error=%v dur=%s", err, dur)
		} else {
			appendLog(watchdogLog, "tick ok dur=%s", dur)
		}

		// Phase 2.G fix: mirror chroma → disk after every tick so `hive status`
		// and operator-facing `.md` files reflect the live state. Without this
		// post-tick sync, the binary's filesystem-rooted view stays frozen at
		// the last `hive merge` invocation — making the orchestrator look
		// stuck even while it's making real progress. Errors are non-fatal:
		// the next merge will still re-sync.
		if syncErr := mempalace.DumpToFilesystem(opts.WorkspaceRoot); syncErr != nil {
			appendLog(watchdogLog, "post-tick sync error=%v (continuing)", syncErr)
		}

		// Idle-tick backoff. The manager writes .hive/last-tick.json at the end
		// of every tick; if everything in it is zero, this tick did no work and
		// we can extend the next sleep. Missing/corrupt summary → assume non-idle
		// (safer than over-backing-off on a manager crash). The poll loop below
		// breaks early if the operator drops a requirement into the inbox.
		summary, summaryErr := ReadTickSummary(tickSummaryPath)
		switch {
		case summaryErr != nil:
			consecutiveIdle = 0
		case summary.IsIdle():
			consecutiveIdle++
		default:
			consecutiveIdle = 0
		}
		sleep := NextDelay(opts.TickInterval, opts.IdleBackoffMax, opts.IdleBackoffFactor, consecutiveIdle)
		if sleep != opts.TickInterval {
			appendLog(watchdogLog, "idle backoff consecutive=%d next_sleep=%s", consecutiveIdle, sleep)
		}
		if sleepInterrupted(ctx, sigCh, inboxDir, sleep) {
			if consecutiveIdle > 0 {
				appendLog(watchdogLog, "inbox change → cancel idle backoff (was consecutive=%d)", consecutiveIdle)
			}
			consecutiveIdle = 0
		}
		select {
		case <-ctx.Done():
			return nil
		case <-sigCh:
			return nil
		default:
		}
	}
}

// sleepInterrupted blocks for up to total, polling inboxDir every pollInterval.
// Returns true when an inbox file appeared/disappeared (operator dropped a
// requirement) so the caller can reset backoff. Returns false on natural
// expiry, ctx cancel, or signal. Polling is cheap and avoids an fsnotify dep.
func sleepInterrupted(ctx context.Context, sigCh <-chan os.Signal, inboxDir string, total time.Duration) bool {
	const pollInterval = 5 * time.Second
	if total <= pollInterval {
		select {
		case <-ctx.Done():
		case <-sigCh:
		case <-time.After(total):
		}
		return false
	}
	deadline := time.Now().Add(total)
	baseline := inboxFingerprint(inboxDir)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-sigCh:
			return false
		case <-ticker.C:
			if inboxFingerprint(inboxDir) != baseline {
				return true
			}
			if time.Now().After(deadline) {
				return false
			}
		}
	}
}

// inboxFingerprint is a cheap "did anything change" hash of inbox contents.
// We avoid pulling fsnotify; entry count + newest mtime is enough to catch
// the operator dropping a new req-*.txt file. Missing dir → zero.
func inboxFingerprint(inboxDir string) int64 {
	entries, err := os.ReadDir(inboxDir)
	if err != nil {
		return 0
	}
	var newest int64
	count := int64(0)
	for _, e := range entries {
		if e.IsDir() {
			continue // skip processed/
		}
		count++
		if info, err := e.Info(); err == nil {
			if mt := info.ModTime().UnixNano(); mt > newest {
				newest = mt
			}
		}
	}
	return count*1_000_000_000 + newest
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
	// Phase 2.H: --bare skips hooks, LSP, plugin sync, auto-memory, CLAUDE.md
	// auto-discovery, and other startup features hive subprocesses don't use.
	// --disable-slash-commands skips skill catalog loading (the role is already
	// inlined as the system prompt via --system-prompt-file). Together they
	// shave a sizable chunk off every manager tick's spawn cost. MCP and the
	// explicit system prompt still work — see `claude --help` for `--bare`'s
	// "explicitly provide context via" list, which includes --mcp-config and
	// --system-prompt-file.
	cmd := exec.Command(
		opts.ClaudeBinary,
		"--print",
		"--bare",
		"--disable-slash-commands",
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

// Phase 2.E: cap log files at logMaxBytes. When exceeded, rename to <path>.1
// (overwriting any previous rotation) and start fresh. Called pre-tick from the
// supervisor loop so growth is bounded across multi-day runs.
const logMaxBytes = 50 * 1024 * 1024 // 50 MB

func rotateIfTooLarge(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < logMaxBytes {
		return
	}
	_ = os.Rename(path, path+".1")
}

// Phase 2.E: hung-worker detection. The manager skill writes the spawn epoch
// into .hive/agents/<id>/started_at. Any live agent older than hungAgentMaxAge
// gets SIGKILL'd here — the manager's reap step picks up the corpse next tick
// and re-pends (or escalates) the story per normal abandonment recovery.
//
// 20 min is generous for slow worker runs (one tick we saw was 5m, and workers
// can take 5-10 min for non-trivial stories) but tight enough that a truly
// stuck agent is freed within one operator-coffee-break window.
const hungAgentMaxAge = 20 * time.Minute

func reapHungAgents(workspaceRoot, watchdogLog string) {
	agentsDir := filepath.Join(workspaceRoot, ".hive", "agents")
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return // no agents dir = nothing to reap
	}
	now := time.Now()
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		startedAtBytes, err := os.ReadFile(filepath.Join(agentsDir, id, "started_at"))
		if err != nil {
			continue // no started_at yet — agent is mid-spawn, skip
		}
		var startedAt int64
		if _, err := fmt.Sscanf(string(startedAtBytes), "%d", &startedAt); err != nil {
			continue
		}
		age := now.Sub(time.Unix(startedAt, 0))
		if age <= hungAgentMaxAge {
			continue
		}
		pidBytes, err := os.ReadFile(filepath.Join(agentsDir, id, "worker.pid"))
		if err != nil {
			continue
		}
		var pid int
		if _, err := fmt.Sscanf(string(pidBytes), "%d", &pid); err != nil {
			continue
		}
		// kill -0 to check liveness; if alive past the cap, SIGKILL the process group.
		if err := syscall.Kill(pid, 0); err != nil {
			continue // not alive — manager will reap normally
		}
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		appendLog(watchdogLog, "hung-agent reap id=%s pid=%d age=%s", id, pid, age.Round(time.Second))
	}
}
