# Phase 1 Verification Runbook

**Date executed:** _<fill in YYYY-MM-DD when you run this>_
**Hive binary commit:** _<output of `git rev-parse HEAD`>_
**Tested against:** claude `_<version from `claude --version`>_`, mempalace `_<version>_`

This is the gate that proves the Phase 1 architecture works end-to-end against a real Claude Code CLI + real mempalace + a real test repo. It can't be automated — the manager and worker skills make real LLM calls and write real PRs.

## Pre-flight

Before starting, verify on the host machine:

- [ ] `which claude` returns a path (Claude Code CLI installed)
- [ ] `$MEMPALACE_ROOT` is set and the dir exists, OR the mempalace MCP works via `claude` directly
- [ ] You have a small throwaway test repo you control (e.g., a fork of something trivial). The story will add a one-line comment to its README — pick something where that's safe.
- [ ] You have an `nikrich`-authenticated `gh` for the test repo (or whatever auth lets the worker push branches + open PRs)

## Verification steps

### 1. Init a verification workspace

```sh
mkdir /tmp/hive-verify
cd /tmp/hive-verify
git clone <your-test-repo-url> repos/test-team
/path/to/hive-v2/hive init \
  --workspace-slug verify \
  --team name=test-team,url=<your-test-repo-url> \
  --no-clone
ls -R .hive .claude
```

- [ ] Output shows the full workspace tree: `.hive/config.yaml`, `.hive/inbox/`, `.claude/skills/manager.md`, `.claude/skills/junior.md`, `.claude/skills/tasks/creating-a-pr.md`, `.claude/skills/tasks/filing-a-finding.md`, `.claude/settings.local.json`, `.claude/mcp.json`

### 2. Add a requirement

```sh
/path/to/hive-v2/hive add-req "Add a // HELLO_HIVE comment at the top of README.md"
ls .hive/inbox/
cat .hive/inbox/*.txt
```

- [ ] One `req-*.txt` file in `.hive/inbox/` with the body text

### 3. Run hive for one tick (background)

```sh
/path/to/hive-v2/hive run &
HIVE_PID=$!
sleep 90        # let one tick happen (60s tick + spawn time)
/path/to/hive-v2/hive status
cat .hive/watchdog.log
cat .hive/manager.log
ls .hive/agents/        # should show one live agent dir after a spawn
```

- [ ] `watchdog.log` shows at least one `tick ok` line
- [ ] `manager.log` shows the manager did meaningful work (drained inbox or filed drawers)
- [ ] `hive status` shows non-zero counts (story drawer at least pending → assigned)

### 4. Wait for the worker to finish and verify the PR

Let the manager run another ~5-10 minutes.

```sh
/path/to/hive-v2/hive status
gh pr list -R <your-test-repo>
```

- [ ] A PR exists against `<your-test-repo>` on a branch named `agent/test-team--junior-<id>`
- [ ] The diff adds `// HELLO_HIVE` (or similar) to README.md
- [ ] `hive status` shows the story progressed: `pending → assigned → review` (or `→ merged` if QA gate auto-merged, which Phase 1 doesn't do — review is the expected end-state)

### 5. Stop hive cleanly

```sh
/path/to/hive-v2/hive stop
ls .hive/*.pid 2>/dev/null || echo "(no stale pid files — good)"
ps aux | grep -E "claude|hive" | grep -v grep   # should be empty
```

- [ ] No stale PID files
- [ ] No leftover `claude` or `hive` processes
- [ ] `hive stop` printed "stopped"

## Issues discovered

_<list anything that broke; file follow-up issues>_

## Time to first PR

_<elapsed time from `hive run` to PR opened — useful baseline for Phase 4 self-correction work>_

## Notes for the executor

- The `<path/to/hive-v2>` binary is built from this repo: `go build ./cmd/hive` in the repo root.
- The mempalace MCP must be configured in `.claude/mcp.json` (it is, via `hive init`). If the mempalace gateway binary isn't named `mempalace-gateway` on your machine, edit `.claude/mcp.json` after init.
- If the worker's `claude` invocation can't find skills, verify `.claude/skills/` exists in the worktree dir (Claude Code looks in cwd + ancestors). If not, that's a Phase 1 verification finding — file it under "Issues discovered" above.
- The first run may fail in interesting ways. That's the *point* of this runbook — empirical verification of architectural assumptions.

## When complete

After all boxes are checked and any issues are filed:

```sh
git add docs/plans/2026-05-27-phase-1-verification-runbook.md
git commit -m "docs: phase 1 verification runbook with results"
git push
```

Phase 1 is then officially done.
