# Phase 1 Verification Runbook — Executed

**Date executed:** 2026-05-27
**Hive binary commit:** `bfcd548` (`fix(watchdog): pass manager prompt as positional arg to claude --print`)
**Tested against:** claude `claude-opus-4-7`, mempalace gateway running locally (`MEMPALACE_ROOT=/Users/jannik/sft-capstone-memory`)
**Target repo:** local-only `/tmp/hive-verify-target` (single-commit init, no GitHub remote)

## Result summary

**Overall: PARTIAL PASS** — the architecture works end-to-end (watchdog → claude → real git commits in the right repo), but two skill-level issues need follow-up before the manager produces the *full* designed flow (worker subprocesses + mempalace drawer state).

**Issues fixed inline during verification** (committed to main):
1. `fix(watchdog): pass manager prompt as positional arg` (commit `bfcd548`) — `claude --print` requires a positional prompt; `--append-system-prompt` only sets the system prompt and leaves the user message empty.

**Issues filed for Phase 1.1 / Phase 2:**
- Bundled `mcp.json` template uses the placeholder command `mempalace-gateway`. On this host the working command is `/Users/jannik/sft-capstone-memory/gateway/.venv/bin/python -m mempalace_gateway.server`. The template either needs to be machine-detected at `hive init` time or expressed as a placeholder users must edit.
- Manager skill needs reinforcement: claude did the code change directly instead of spawning a worker subprocess. The skill text says "spawn a worker" but the model interpreted that more loosely. Either the skill needs sharper imperatives, or we need a Bash-only enforcement (no Edit tool in the manager's permission set).
- Manager skill didn't write any mempalace drawers (no `wings/hive-verify/` dir was created). Likely related to the above — the model treated the task as a one-shot rather than the orchestration pattern.
- Manager tick took 3m31s — over the spec's typical 1-2 min target, close to the 5 min hard cap. Likely a function of doing the work in-session rather than just dispatching.
- Local "remote" (`/tmp/hive-verify-target`) rejected push with `denyCurrentBranch`. Real flows must use a bare remote — document this in init prompts.

## Pre-flight (executed)

- [x] `which claude` → `/opt/homebrew/bin/claude`
- [x] `MEMPALACE_ROOT=/Users/jannik/sft-capstone-memory` (manually set; not in shell profile)
- [x] Test repo: `/tmp/hive-verify-target` (local git init, no remote)
- [x] No `gh` needed for this run — PR-opening was skipped due to push failure

## Verification steps (executed)

### 1. Init a verification workspace — ✅ PASS

```sh
mkdir /tmp/hive-verify-ws
git clone /tmp/hive-verify-target /tmp/hive-verify-ws/repos/test-team
hive init --dir /tmp/hive-verify-ws --workspace-slug verify \
  --team name=test-team,url=/tmp/hive-verify-target --no-clone
```

- [x] Workspace tree includes `.hive/config.yaml`, `.hive/inbox/`, `.claude/skills/{manager,junior}.md`, `.claude/skills/tasks/{creating-a-pr,filing-a-finding}.md`, `.claude/settings.local.json`, `.claude/mcp.json`

### 2. Add a requirement — ✅ PASS

```sh
hive add-req "Add a // HELLO_HIVE comment at the top of README.md"
```

- [x] `req-1779876892-538c0efc.txt` written to `.hive/inbox/` with exact body

### 3. Run hive (one tick) — ⚠️ PARTIAL

```sh
MEMPALACE_ROOT=/Users/jannik/sft-capstone-memory hive run &
```

- [x] Watchdog started; `watchdog.log` shows `watchdog start pid=60019 tick=1m0s`
- [x] Manager spawned, ran to completion: `tick ok dur=3m31.470918583s`
- [x] Manager output (`manager.log`):
  ```
  tick=1 manager=claude-opus-4-7
    inbox.drain: 1 req
    req=req-1779876892-538c0efc team=test-team
      requirement="Add a // HELLO_HIVE comment at the top of README.md"
      action=apply file=repos/test-team/README.md
      commit=360f957 msg="Add HELLO_HIVE marker"
      push=skipped reason="remote /tmp/hive-verify-target has main checked out (denyCurrentBranch); local clone is canonical for phase-1 verify"
      inbox.move=processed/req-1779876892-538c0efc.txt
    tick.done status=ok stories=1 completed=1 open=0
  ```
- [x] Inbox file moved to `.hive/inbox/processed/` (skill-improvised cleanup pattern — worth codifying)
- [x] README change visible: `// HELLO_HIVE` prepended; commit `360f957` on `main` of `repos/test-team/`
- [ ] **No worker subprocess spawned.** Manager did the work in-session.
- [ ] **No mempalace drawers created.** `wings/hive-verify/` does not exist.

### 4. Verify the PR — ⚠️ SKIPPED (no real remote)

Not applicable: local git "remote" rejects push to checked-out branch. The change is in the workspace's worktree clone of `test-team`, not pushed anywhere.

For a real-remote verification, the test repo must be either:
- A GitHub repo with `gh auth` set up
- A bare local repo (`git init --bare`)

### 5. Stop hive cleanly — ✅ PASS

```sh
hive stop
```

- [x] Output: `stopping watchdog pid=60019` → `stopped`
- [x] No leftover `.hive/*.pid` files
- [x] No leftover `hive run` or `claude --print` processes
- [x] Watchdog log records `watchdog stop`

## Time to first action

- Watchdog start → manager exit (work committed): **3m32s**
- Of which: claude spawn + opus thinking + Edit + git ops + log emit: ~3m20s
- This is one Opus tick. With Sonnet or with the skill rewritten to "delegate, don't implement" it would be much faster.

## Follow-up items (to file before Phase 2)

| Priority | Issue | Owner / Phase |
|---|---|---|
| **High** | Manager skill must enforce "spawn workers, don't do work directly." Options: (a) sharper imperatives in skill body, (b) restrict the manager's permission allowlist to `Bash` + `mcp__mempalace__*` only (no Edit/Write), forcing it to dispatch | Phase 1.1 (skill rewrite) |
| **High** | Manager skill must enforce mempalace drawer writes. Same options as above. Alternatively: require manager to print a structured tick-summary that the next tick can verify against drawer state | Phase 1.1 |
| **Medium** | Bundled `mcp.json` is wrong on most machines (uses placeholder `mempalace-gateway` command). Either auto-detect from user's `~/.claude.json` at `hive init` time, or document the post-init edit | Phase 1.1 |
| **Medium** | `hive init` should warn (or block) when the team `repo_url` is a path to a non-bare local repo — that path will fail `git push` | Phase 1.1 |
| **Low** | Manager tick takes 3m+ on Opus; Phase 4 work depends on faster ticks. Consider sonnet for the manager | Phase 4 |
| **Low** | Manager improvised `.hive/inbox/processed/` for drained files — better than the skill's "delete" instruction. Codify in the skill | Phase 1.1 |

## Conclusion

Phase 1's architecture is sound: a Go watchdog → fresh `claude --print` per tick → real code changes in a real git repo → clean shutdown. The shell-level state model (PID files, inbox dir, log files) is reliable.

The **skill content** is the weak link — claude exercises judgment about *how* to interpret instructions, and the current manager.md leaves room for the "I'll just do it myself" shortcut. Phase 1.1 should be a focused skill-rewrite + bundled-config fix pass before Phase 2 starts adding role complexity.

The watchdog bug (positional-prompt) was a real Phase 1 defect that this runbook caught. Without empirical verification we'd have shipped a binary that errored on every tick.
