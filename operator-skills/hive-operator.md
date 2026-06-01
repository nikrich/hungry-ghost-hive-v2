---
name: hive-operator
description: Use when the user wants to install, configure, or operate hungry-ghost-hive v2 (the Go-binary AI agent orchestrator). Covers install, workspace init, requirement authoring, queuing, monitoring, merge-conflict recovery, multi-team setup, and "set and forget" daemon mode. Distinct from the legacy capstone-hive skill.
---

# Hive Operator

You help the user operate **hungry-ghost-hive v2** — a Go binary that supervises Claude Code subprocesses to run AI development teams in parallel against real git repos.

## What hive v2 is (1-paragraph elevator)

A single Go binary (`hive`) runs a watchdog that wakes every 60s and spawns a fresh `claude --print` manager subprocess. The manager reads requirements from `.hive/inbox/`, decomposes each into a DAG of stories via a tech-lead role, then spawn-fills up to `max_workers` parallel workers (juniors/intermediates/seniors based on story points). Each worker writes real code in its own git worktree, opens a real PR via `gh pr create`. A QA worker auto-spawns when a story hits `status=review`, runs the team's test command, and merges the PR if tests pass. State lives in mempalace (ChromaDB-backed). Operator drops requirements; the system drains them. See `nikrich/hungry-ghost-hive-v2` on GitHub.

## When to invoke this skill

- User says they want to "install hive", "set up hive", "run hive", "use hive"
- User asks about requirement format, decomposition, story sizing
- User has a hive workspace that's misbehaving (stuck workers, escalations, merge conflicts)
- User wants to add a new team / repo to their workspace
- User is browsing the v2 spec docs and wants context

If the user mentions "capstone-hive" or "hive req" with no `add-req`, that's the LEGACY v1 hive (tmux-based) — politely note v2 is different and redirect if appropriate.

## Architecture cheat-sheet (so you can answer questions accurately)

```
Operator                    Watchdog (Go, always-on)        Manager (claude --print, per-tick)
   │                              │                              │
   │ hive add-req "..."           │                              │
   ├──► .hive/inbox/req-*.txt    │                              │
   │                              │ every 60s, spawn:            │
   │                              ├──────────────────────────────►
   │                              │                              │ 1. drain inbox → requirement drawers
   │                              │                              │ 2. reap dead agents
   │                              │                              │ 3a. spawn tech-lead (1 slot)
   │                              │                              │ 3b. spawn-fill workers (max_workers)
   │                              │                              │ 3c. spawn-fill QA (max_qa)
   │                              │                              │ 4. write diary entry
   │                              │                              │ 5. exit
   │                              │ ◄────────────────────────────┤
   │                              │ wait tick, repeat            │

Workers run in their own worktrees, push agent branches, open PRs.
QA cds into the agent worktree, runs go test ./..., calls hive merge "<title>".
```

**Key invariants:**
- All state persists in mempalace (workspace-local ChromaDB at `.hive/memory/index/chroma/`). Subprocesses are stateless.
- One tech-lead per workspace at a time. Up to `max_workers` workers + `max_qa` QA concurrent.
- One feature branch per (team × requirement). One agent branch per worker.
- `hive merge` is the canonical merge primitive — workers don't merge themselves; either operator (pre-2.C) or QA (post-2.C) calls it.

## Install

```bash
# Option A: go install (recommended — picks up newest main)
go install github.com/nikrich/hungry-ghost-hive-v2/cmd/hive@latest
which hive    # expect ~/go/bin/hive

# Option B: clone + build
git clone https://github.com/nikrich/hungry-ghost-hive-v2.git
cd hungry-ghost-hive-v2
go build -o /usr/local/bin/hive ./cmd/hive
```

Prerequisites:
- Go 1.22+
- `claude` CLI (Claude Code) with OAuth or `ANTHROPIC_API_KEY` configured
- `git` 2.28+ (for `init -b main`)
- `gh` CLI authenticated to the GitHub account that owns the target repos
- `python3` + `mempalace_gateway` package importable (the operator already has this if they installed mempalace; if not, point them at the install via `pip install -e` from the gateway source)

## Initialize a workspace

```bash
mkdir -p ~/work/my-hive-workspace && cd $_

hive init \
  --team "name=backend,url=git@github.com:my-org/backend.git" \
  --team "name=frontend,url=git@github.com:my-org/frontend.git" \
  --workspace-slug myproject

# init writes:
#   .hive/config.yaml       — workspace + team config
#   .hive/inbox/            — requirement drop folder
#   .hive/memory/           — mempalace storage
#   .claude/skills/         — bundled hive role skills
#   .claude/mcp.json        — workspace-local MCP config

# init currently (skip)s the actual git clone — do it manually:
git clone <team_url> repos/<team_name>
git -C repos/<team_name> config user.email <committer-email>
git -C repos/<team_name> config user.name <committer-name>
```

For each team, edit `.hive/config.yaml` to add `test_command:` if it's not Go:

```yaml
teams:
  - name: backend
    test_command: go test ./...
  - name: frontend
    test_command: npm test
```

## Authoring requirements that decompose well

**This is the single biggest determinant of whether a hive run succeeds.** A bad requirement produces conflicts; a good one drains cleanly. Coach the operator on this.

### Anatomy of a good requirement

```
Build N independent <thing>s under <module>/. Each <thing> lives in its own
directory and is independent of the others — different file paths, no shared
types, no import dependencies between <things>.

1. <module>/<name1>/ — file <name1>.go exports <function signature>. Test
   <name1>_test.go asserts <observable post-condition 1>, <post-condition 2>.

2. <module>/<name2>/ — file <name2>.go ... [same pattern]

[3, 4, 5, ...]

Common acceptance criteria for every story:
- Implementation file + test file present at listed paths
- <test command> passes
- Commit pushed; PR opened against feature branch via gh pr create
```

### The five rules (drill these in)

1. **Independent files.** Each story owns at least one unique file path. Two stories that both create `README.md` from scratch WILL conflict at merge — we've seen this. Different directories or distant sections of one file is fine.
2. **Concrete acceptance criteria.** Use imperatives a test command can verify: "function returns 200", "response body matches `{...}`", "file ends with `<EOF>`". Avoid soft language ("looks good", "clean", "performant").
3. **Fibonacci sizing.** Stories are 1-2-3-5-8-13 points. 1-3 → junior, 5 → intermediate, 8-13 → senior. Aim for stories ≤3 points unless genuinely cross-cutting.
4. **No story imports another story's package within the same requirement.** Cross-story imports require a `depends_on:` — the importer waits for the importee to merge. Avoid unless truly necessary; serializes work.
5. **Explicitly call out independence.** Saying "stories are INDEPENDENT" in the requirement text helps the tech-lead emit `depends_on: []` correctly.

### Worked example (good)

```
hive add-req "Add five small Go utility modules to the test API repo. Each
lives in its own directory under pkg/ and is INDEPENDENT of the others —
different file paths, no shared types.

1. pkg/healthz/ — file healthz.go exports Handler(w, r) that returns HTTP 200
   with body {\"status\":\"ok\"}. Test asserts status, content-type, body.
2. pkg/readyz/ — file readyz.go exports Handler(w, r) that returns HTTP 200
   with body {\"ready\":true}. Test asserts same.
3. pkg/buildinfo/ — file buildinfo.go exports Build struct {Version, Commit
   string} and Get() Build reading env vars. Test covers both-set, neither-set,
   one-set.
4. pkg/uuidlite/ — file uuidlite.go exports New() string returning 8-char
   lowercase hex. Test asserts length, character set, uniqueness over 100 calls.
5. pkg/clamp/ — file clamp.go exports ClampInt(value, min, max int) int.
   Table-driven test for below-min, above-max, in-range, equal-to-bounds.

Acceptance for each: impl + test files present, go test passes, PR opened
against feature branch."
```

This produces 5 clean PRs that auto-merge. Verified.

### Anti-example (bad)

```
hive add-req "Make the bank better. Add some validation and tests and
maybe some new endpoints."
```

What goes wrong: tech-lead either emits one mega-story (no parallelism) or 5+ vague stories that all touch the same files. Workers write conflicting code. QA refuses on merge conflicts. System stalls.

### Multi-team requirement (Phase 2.F — scoped, not yet built as of 2026-06-01)

When 2.F lands, requirements can span teams:

```
hive add-req --teams backend,frontend "Add a /healthz UI badge:
1. backend: ensure /healthz returns uptime_seconds in the JSON
2. frontend: add a top-bar component that fetches /healthz and shows green/red"
```

Each story gets `team:` set per the team it belongs to. Each team gets its own feature branch on its own repo.

Until 2.F lands, ONE requirement = ONE team. Cross-team work needs separate `hive add-req` per team.

## Run the watchdog

```bash
# Interactive foreground (Ctrl-C to stop)
cd ~/work/my-hive-workspace
hive run

# Backgrounded local
nohup hive run > /tmp/hive.log 2>&1 &

# True always-on (macOS) — use the launchd plist
# See docs/operator/launchd-keepalive.md in the hive repo.
```

The watchdog ticks every 60s by default. Override via `tick_interval_seconds:` in config.

## Queue requirements

```bash
hive add-req "..."                              # single team (uses teams[0])
hive add-req --teams a,b "..."                  # multi-team (2.F)
echo "..." > .hive/inbox/manual-req.txt         # also works — operator drops file directly
```

Queue them whenever you want — at the start of the day, trickled in over hours, via webhook into a script. The manager drains the inbox on its next tick.

**Important:** multiple requirements can be in flight simultaneously. Workers from req A + workers from req B compete for the same `max_workers` pool. Only the tech-lead is single-slot, so requirement DECOMPOSITION is serial; story EXECUTION is parallel across requirements.

## Monitor

```bash
# Watchdog ticks
tail -F .hive/watchdog.log

# Manager tick summaries (most informative)
tail -F .hive/manager.log

# Live claude subprocesses
ps aux | grep "claude --print --permission-mode" | grep -v grep

# Story states (queries chroma directly)
hive status

# PRs in flight
gh pr list --repo <owner>/<repo>

# Per-tick diary (if you want history)
# Diary entries are stored in mempalace as drawers in the "diary" room
```

Diary entries look like:
```
manager  tick-end  spawned=junior,junior,junior reaped=1
                   live_workers=3/3 live_tech_leads=0/1 live_qa=1/2
                   pending_reqs=0 ready_stories=2 waiting_stories=1
                   review_stories=2
```

If `spawned=none` for many consecutive ticks AND `ready_stories=0` AND `review_stories=0` AND `pending_reqs=0`, the system is idle — drop more requirements.

## Recovering from common failure modes

### Worker stuck at `status=assigned` for >20 min

Phase 2.E's hung-agent reaper SIGKILLs after 20 min. The manager's next tick reaps the dead agent, re-pends the story (with `retry_count++`). After 3 retries the story → `blocked` and an escalation is filed.

Operator action: read the `findings` room to see why it kept failing. If transient (network blip), reset retry_count via mempalace and let it re-pend. If genuine bug in the requirement, fix the requirement text and re-queue.

### QA refuses on merge conflict

`hive merge` errors with `merge failed: ...CONFLICT...` and prints the manual recovery command:
```
git -C repos/<team> checkout <feature_branch> && git merge origin/<agent_branch> && git push
```

Operator action: resolve manually, push, then update the story drawer to `status=merged` via mempalace (or re-run `hive merge` after fixing the conflict).

### Story at `status=blocked` after 3 QA failures

A `qa-failure` finding is filed each retry. Read the findings to see test output. Common causes:
- Acceptance criteria are ambiguous (worker satisfied one interpretation, QA tested another)
- Test command is wrong for the team
- Worker hallucinated an import or a function signature

Operator action: fix the requirement text (more concrete criteria), reset `retry_count=0` + `status=pending` on the story drawer, let the system retry. Or write the code yourself and `hive merge`.

### Mempalace gateway unreachable

Symptom: `hive merge` fails with `decode dump output: unexpected end of JSON input` or worker subprocesses hang.

Operator action: check `ps aux | grep mempalace_gateway`. If absent, the next claude subprocess will respawn it. If hung, kill it. Verify `MEMPALACE_PALACE_PATH` env in the spawn is `<workspace>/.hive/memory/index/chroma`.

### Skill drift after rebuild

Phase 2.E added skill re-sync on `hive run` startup. If you suspect drift, restart the watchdog (which re-syncs from the embedded skills in the binary).

## "Set and forget" mode

For multi-day unattended operation:
1. Bump `max_workers` and `max_qa` in `.hive/config.yaml` if you want more throughput (cap by Anthropic rate limits)
2. Queue 10-50 requirements upfront
3. Run via launchd (see `docs/operator/launchd-keepalive.md`)
4. Check `gh pr list --state merged` every few hours

For 100+ parallel workers, you need Phase 3.A (Fargate). Not built yet — point the operator at the scoping spec.

## What's built vs scoped (as of 2026-06-01)

**Built and verified end-to-end:**
- Phase 1.x — single-subprocess foundation, workspace-local memory, watchdog reliability, `--system-prompt-file` inlining
- Phase 1.5 — MCP precedence fix + chroma↔filesystem bridge
- Phase 2.A — tech-lead + 3 worker roles (junior/intermediate/senior)
- Phase 2.D — multi-worker parallelism + feature branches + `hive merge`
- Phase 2.C — QA auto-merge (the loop-closing piece)
- Phase 2.E — reliability rails (skill sync, log rotation, hung-agent reaper, launchd plist)

**Scoped but not implemented:**
- Phase 2.F — multi-team requirements (`docs/specs/2026-06-01-phase-2.f-...md`)
- Phase 3.A — Fargate cloud runtime via Bedrock (`docs/specs/2026-06-01-phase-3.a-...md`)

## When NOT to use hive

- Single-story changes: just write the code yourself. Spawning a tech-lead + a worker + a QA for one line is wasteful.
- Requirements where every story must be sequential: no parallelism win, just adds orchestration latency.
- Highly stateful work that doesn't fit the "PR-per-story" model: schema migrations, multi-step deploys.
- Anything where correctness > throughput: hive's QA bar is `go test ./...`. If you need security review, code-style review, architecture review, etc., do those by hand (or wait for the LLM-judgment QA in a future phase).

## Skills you might want to chain with

- For the operator who's NEW to hive and is debugging: walk them through `tail .hive/manager.log` + `hive status` + `gh pr list` first, before changing anything.
- For the operator authoring a new requirement: rehearse it through you (this skill) to spot anti-patterns before queuing.
- For the operator stuck on a merge conflict: walk them through the manual recovery commands the error message prints.
