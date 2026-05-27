# Hive v2 — Architecture Design

**Status:** Draft (Phase 0 architecture overview) — pending implementation plan
**Date:** 2026-05-27
**Author:** Brainstormed with Claude
**Successor to:** `hungry-ghost-hive` v0.x (frozen on completion of this spec)

---

## 1. Summary

Hive v2 is a **ground-up rewrite** of `hungry-ghost-hive` with a single thesis:

> **Intelligence lives in Claude + skills, not in code.**

The TS codebase that grew to thousands of files becomes a ~700-LOC Go binary whose only job is supervising `claude` subprocesses. All orchestration logic (story decomposition, agent assignment, escalation handling, stuck-detection, recovery) moves into Claude Code skills. State lives in mempalace; the filesystem holds only skills, worktrees, and ephemeral per-agent context.

The design optimizes for:
1. **Minimal code surface** — fewer lines of Go than v1 has TypeScript packages
2. **Persistent memory** — mempalace is the only storage; agents share knowledge across runs
3. **Self-correction** — the manager is a fresh `claude` per tick; new failure modes are handled by editing a skill, not shipping code
4. **Runs for days unattended** — a thin watchdog keeps the manager alive; mempalace state survives any crash

## 2. Goals & Non-Goals

### Goals

- Single Go binary distribution (`go install` or GitHub release download); no Node/npm runtime requirement
- All agent orchestration logic in skill markdown, not in Go source
- Mempalace as the sole storage layer for stories, agent state, escalations, findings, and event log
- Bounded-parallel worker execution, up to N concurrent `claude` subprocesses
- Multi-repo support (one team per repo); workers operate in per-spawn git worktrees
- Jira integration via the Atlassian MCP from inside skill behavior (no TS/Go Jira client)
- A real TUI as observability, designed and shipped alongside Phase 1 in its own follow-up spec

### Non-Goals

- Backwards compatibility with v1 workspaces. v2 reads no v1 state.
- Distributed mode, RAFT, leader election, multi-machine clustering.
- Postgres or SQLite. No relational DB. No migrations.
- Codex or Gemini runtime support. Claude Code only.
- Acting as a long-lived OS service. v2 is started and stopped manually; the watchdog keeps the manager alive **only while running**.
- Sub-second observation latency. Status reads are eventually consistent against mempalace and the diary.
- More than ~5 concurrent workers per machine (bounded by host resources, not by architecture).

## 3. Architecture Topology

```
┌───────────────────────────────────────────────────────────────────┐
│  User                                                             │
│   $ hive run                                                      │
└───────┬───────────────────────────────────────────────────────────┘
        │
        ▼
┌─────────────────────────┐
│ Watchdog                │  ~30 LOC of Go. Sole job: keep
│ (long-lived Go process) │  manager alive. Restarts on crash.
└───────┬─────────────────┘
        │ spawns fresh per tick (default: every 60s)
        ▼
┌────────────────────────────────────────────┐
│ Manager                                    │
│  claude --print --permission-mode          │     reads/writes
│         acceptEdits                        │ ◄────────────────┐
│  Skill: manager.md (+ task skills)         │                  │
│                                            │                  │
│  Each tick:                                │                  │
│   1. Read mempalace: queue, live workers   │                  │
│   2. Decide: spawn/nudge/kill/escalate     │                  │
│   3. Bash: git worktree, spawn workers     │                  │
│   4. Write outcomes to mempalace           │                  │
│   5. Exit                                  │                  │
└──────────────┬─────────────────────────────┘                  │
               │ Bash spawn                                     │
               ▼                                                │
┌────────────────────────────────────────────┐                  │
│ Worker(s) — up to N concurrent             │                  │
│  claude --print --permission-mode          │     reads/writes │
│         acceptEdits                        │ ◄────────────────┤
│  cwd: <worktree>                           │                  │
│  Skill: <role>.md (+ task skills)          │                  │
│                                            │                  │
│  1. Read .hive/agents/<id>/context.md      │                  │
│  2. Invoke role skill                      │                  │
│  3. Do the work (edit, test, PR)           │                  │
│  4. Write findings drawer                  │                  │
│  5. Exit                                   │                  │
└────────────────────────────────────────────┘                  │
                                                                │
                                                                ▼
                                                  ┌──────────────────────┐
                                                  │ Mempalace MCP        │
                                                  │ (single instance,    │
                                                  │  user-level)         │
                                                  │                      │
                                                  │ Wing: hive-<ws>      │
                                                  │ Rooms: stories,      │
                                                  │   agents, escalations│
                                                  │   findings, reqs     │
                                                  │ Diary: hive-<ws>-evt │
                                                  └──────────────────────┘
```

### Invariants

- **Only the manager spawns workers.** Workers never spawn other workers. Single point of orchestration.
- **Only one manager at a time.** Watchdog enforces at-most-one via PID file + flock.
- **Workers are fire-and-forget.** They write outcomes to mempalace and exit. Manager picks up results on the next tick.
- **No process talks to another process directly.** All coordination flows through mempalace. There is no in-memory orchestration state to corrupt.
- **Worktrees are scratch space.** A worker's worktree can be deleted at any time after PR merge or escalation — no state is lost.

## 4. Design Decisions

The 13 decisions locked during brainstorming, kept here as a quick reference:

| # | Decision | Rationale |
|---|---|---|
| 1 | Skills replace per-role TypeScript prompts | Single source of truth, editable without releases |
| 2 | Claude Code only — drop codex/gemini runtimes | "Skills" and "minimal code" only pay off with Claude Code as runtime |
| 3 | Dynamic per-spawn context via `.hive/agents/<id>/context.md` | Static skills + dynamic context = clean separation, debuggable artifact per spawn |
| 4 | Skills live in workspace `.claude/skills/` (committed to git) | Per-project tunability; visible in PRs; shareable across machines |
| 5 | Persona role skills + shared task sub-skills | Roles stay personality-focused; operational know-how is reused |
| 6 | 5 team-member roles + Manager + extensible pattern | Adding a new role = writing a new `.md`, no code change |
| 7 | Thin subprocess orchestrator — agents are `claude` subprocesses | Claude Code already does tool use, file edits, bash, MCP, skills, sessions |
| 8 | Mempalace is the sole storage layer | One persistence model; no SQLite; no Postgres; survives crashes |
| 9 | Bounded-parallel agent loop (N concurrent workers) | Real throughput; bounded blast radius |
| 10 | Manager = fresh `claude --print` per tick + Go watchdog | Each tick a fresh perspective; no context bloat; new failure modes = skill edit |
| 11 | Trim profile: keep multi-repo + Jira; cut distributed/RAFT/Postgres/blessed-TUI | Match new architecture; cut sources of v1 instability |
| 12 | TUI in scope (not web, not CLI-only) | User explicitly wants a better TUI than v1 |
| 12a | TUI is its own design spec, built alongside Phase 1+ | Each phase exposes data the TUI needs; TUI evolves with the backend |
| 13 | New repo for v2 (v1 frozen) | Total clean slate; no incremental temptation |

Plus the implementation-language decision:

| # | Decision | Rationale |
|---|---|---|
| 14 | Implement in Go | Single binary distribution; better long-running process behavior; native subprocess + signal handling |

## 5. Workspace Layout

A fresh `hive init` produces:

```
<workspace>/
├── .hive/
│   ├── config.yaml              # teams, repo paths, N (max workers), tick interval
│   ├── inbox/                   # CLI writes new requirements here; manager drains
│   ├── watchdog.pid             # PID of the live watchdog (flock-held)
│   ├── manager.pid              # PID of the currently-spawned manager (if any)
│   ├── manager.log              # rotating log: one line per manager tick
│   ├── watchdog.log             # rotating log: watchdog lifecycle events
│   └── agents/
│       └── <agent-id>/          # one dir per live worker, deleted on completion
│           ├── context.md       # dynamic per-spawn context (req, story, team, role)
│           ├── worker.pid       # PID of the live claude subprocess
│           ├── started_at       # unix ts
│           └── session.txt      # path to the claude session jsonl
│
├── .claude/
│   ├── skills/
│   │   ├── manager.md
│   │   ├── tech-lead.md
│   │   ├── senior.md
│   │   ├── intermediate.md
│   │   ├── junior.md
│   │   ├── qa.md
│   │   └── tasks/
│   │       ├── creating-a-pr.md
│   │       ├── escalating.md
│   │       ├── reviewing-a-pr.md
│   │       ├── running-tests.md
│   │       ├── jira-sync.md
│   │       └── filing-a-finding.md
│   ├── settings.local.json      # permission allowlist (bash, edit, MCP tools) — gitignored
│   └── mcp.json                 # mempalace MCP config (+ Atlassian if used)
│
└── repos/
    ├── <team-name>/             # canonical clone of each team's repo
    │   └── ...                  # always on `main`, never directly edited
    └── <team-name>--<role>-<id>/  # ephemeral worktree per worker
        └── ...                  # deleted after PR merge or escalation
```

### Notable consequences

- **No `.hive/stories/` directory.** Stories are mempalace drawers in the `hive-<workspace>` wing, room `stories`.
- **No `.hive/db.*`.** No SQLite; no Postgres.
- **No `.hive/logs/<agent-id>/`.** Worker transcripts live in `~/.claude/projects/...` (Claude Code's own session store); `session.txt` tracks the path.
- **`.hive/agents/<id>/` is ephemeral.** Created on spawn, deleted on completion. If `rm -rf .hive/agents/`, the manager re-discovers live work from mempalace.
- **`.claude/skills/` is committed to git.** Skills are project state. The v2 binary's bundled defaults are written at `hive init` time; users tune them per-project thereafter.
- **`.claude/settings.local.json` is gitignored.** Permission allowlist is per-machine.

## 6. Mempalace Data Model

One **wing per workspace**, so multiple hive workspaces share a mempalace instance cleanly.

```
wing: hive-<workspace-slug>     (e.g. hive-greenlight-freelance)
│
├── room: requirements          # top-level asks from the user
├── room: stories               # implementable units; what workers work on
├── room: agents                # state of every spawned worker
├── room: escalations           # blocked work needing input
└── room: findings              # durable knowledge

diary: hive-<workspace-slug>-events    # append-only event log
```

### Drawer schemas

**Requirement** (`type: requirement`)
```yaml
title: REQ-042: User profile editing
type: requirement
status: pending | decomposed | in-flight | complete
stories: [STORY-001, STORY-002]
created_by: human
created_at: <iso>
```

**Story** (`type: story`) — the most-mutated drawer
```yaml
title: STORY-001: Add /healthz endpoint
type: story
status: pending | assigned | in-progress | review | merged | abandoned | blocked
points: 3
team: bff-web
requirement_id: REQ-042
depends_on: [STORY-000]
assigned_to: <agent-id> | null
pr_url: <url> | null
retry_count: 0
created_at: <iso>
updated_at: <iso>
```

**Agent state** (`type: agent-state`)
```yaml
title: agent-<short-id>
type: agent-state
role: junior | intermediate | senior | tech-lead | qa
team: bff-web
status: live | exited
exit_reason: completed | crashed | killed-stuck | killed-budget | escalated | null
worktree: repos/bff-web--junior-abc123
session_path: ~/.claude/projects/.../session.jsonl
pid: 12345
current_story: STORY-001
started_at: <iso>
last_heartbeat: <iso>     # updated by manager from session-file mtime
ended_at: <iso> | null
```

**Escalation** (`type: escalation`)
```yaml
title: STORY-001 blocked: missing API contract
type: escalation
story: STORY-001
escalated_by: <agent-id>
status: open | resolved
resolution: <text> | null
escalated_at: <iso>
resolved_at: <iso> | null
```

**Finding** — mempalace's existing native shape, unchanged.

### Diary (event log)

The `hive-<workspace>-events` diary is the source of truth for the TUI and `hive status`. One entry per significant event:

```
2026-05-27T14:00:00  manager  tick-start
2026-05-27T14:00:01  manager  spawn       agent=abc123 role=junior story=STORY-001 team=bff-web
2026-05-27T14:00:01  manager  tick-end    spawned=1 nudged=0 killed=0
2026-05-27T14:05:00  worker   exit        agent=abc123 reason=completed pr=https://...
2026-05-27T14:30:00  worker   escalation  agent=def456 story=STORY-002 reason=missing-contract
```

Append-only. The TUI reads the last N entries and renders timeline + current-state.

### Filtering model

`mempalace_list_drawers` returns previews + IDs; filtering by `status` happens client-side in the consumer (manager skill or `hive status`). At hive's scale (dozens to low hundreds of drawers per workspace) this is fine. If a workspace ever grew to thousands of active drawers, the model would need to add indexed structured queries.

### Compaction / decay

- `agent-state` drawers for exited agents older than 7 days → archived to an `agents-archive` room (managed by the manager skill in periodic-hygiene cycles).
- `story` drawers with status `merged` or `abandoned` older than 30 days → archived.
- `finding` drawers → never auto-decay; rely on `mempalace-compact` skill periodically.

## 7. Skills Inventory

Each skill is a Claude Code skill markdown file: YAML frontmatter (`name`, `description`) + body. For role skills, the `description` says "Use when operating as the X agent for hive."

### Role skills (one always-on per spawned claude)

| Skill | When invoked | Core responsibilities |
|---|---|---|
| `manager.md` | Always by the manager process | Read mempalace queue + live worker state; pick what to do this tick; spawn workers; intervene on stuck ones; escalate via drawer; drain `.hive/inbox/`; periodic hygiene |
| `tech-lead.md` | Worker spawned as `tech-lead` | Decompose a requirement drawer into story drawers; estimate complexity; assign to a team; handle cross-team dependencies; respond to escalations |
| `senior.md` | Worker spawned as `senior` | Handle 5-8 point stories; design + implement; can be called in to unstick juniors |
| `intermediate.md` | Worker spawned as `intermediate` | Handle 3-point stories; straightforward implementation |
| `junior.md` | Worker spawned as `junior` | Handle 1-3 point stories; escalate early when stuck |
| `qa.md` | Worker spawned as `qa` | Review a PR drawer; run acceptance criteria; approve/reject; file findings |

### Shared task skills (any role can invoke)

| Skill | Invoked when |
|---|---|
| `tasks/creating-a-pr.md` | Worker has working code and needs to open a PR |
| `tasks/escalating.md` | Worker is stuck and needs to escalate |
| `tasks/reviewing-a-pr.md` | QA reviewing a worker's PR |
| `tasks/running-tests.md` | Worker needs to run + interpret the project's test suite |
| `tasks/jira-sync.md` | Any role needs to push state to Jira via Atlassian MCP |
| `tasks/filing-a-finding.md` | Any role files a durable finding to mempalace |

### Adding a new role later

1. Write `<role>.md` (persona + when-to-invoke)
2. Compose existing task skills by referencing them
3. Add the role to `hive-agents.allowed_types` in `.hive/config.yaml`
4. Done — manager skill is told "available roles: $(ls .claude/skills/*.md)"; it picks new role when stories warrant it

No Go code change required.

### What is deliberately not a skill

- **Process-level mechanics** (spawning subprocesses, lockfiles, watchdog supervision) — Go. Skills cannot reliably do `fork/exec` with cleanup guarantees.
- **The CLI commands themselves** (`hive init`, `hive run`, etc.) — Go, because they must exist before any skill can be read.

### Skill content sourcing

v1 `getSystemPrompt()` in `src/agents/*.ts` is reference material for **role boundaries** only. Skills are rewritten from scratch — v1 prompts were written for LLM-as-API-call; v2 skills are written for Claude-Code-with-tools-and-mempalace-MCP. Different audience, different verbs.

## 8. Lifecycle Flows

### 8.1 `hive init`

```
1. CLI reads workspace path (default: cwd)
2. Prompts (or accepts flags) for: workspace slug, teams (name + repo URL pairs),
   max concurrent workers N, manager tick interval (default 60s)
3. Writes .hive/config.yaml
4. Writes .claude/skills/* from the embedded defaults
5. Writes .claude/mcp.json registering mempalace (and optionally Atlassian)
6. Writes .claude/settings.local.json with the permission allowlist
7. Clones each team's repo into repos/<team>/
8. Detects existing hive-<workspace-slug> wing in mempalace by checking
   for `$MEMPALACE_ROOT/wings/hive-<slug>/` on disk (consistent with the
   no-MCP-client rule — see §9).
   - If exists: abort unless --force
   - Otherwise: wing is created on first manager-skill drawer write
9. Prints next steps
```

### 8.2 `hive run`

```
1. CLI checks .hive/watchdog.pid: if alive, exit "already running, PID X"
2. CLI double-forks the watchdog process, writes PID, flock-held
3. CLI exits; watchdog detaches

Watchdog loop (forever, until SIGTERM):
  a. Spawn manager: claude --print --permission-mode acceptEdits ...
     in cwd=<workspace>; record PID
  b. Wait for exit (timeout: 5min hard cap)
  c. Log exit code + duration; append diary entry if non-zero
  d. Sleep <tick_interval> (default 60s)
  e. Repeat
```

### 8.3 Story execution (golden path)

```
T+0:    Human runs `hive add-req "..."` → file in .hive/inbox/
T+60s:  Manager tick. Drains inbox → writes requirement drawer (status=pending).
        Sees REQ-042 needs decomposition. Spawns tech-lead worker:
          - git worktree add repos/<team>--tech-lead-<id> from <team>/main
          - Writes .hive/agents/<id>/context.md
          - claude --print in worktree
        Writes agent-state drawer, diary entry, exits.

T+5m:   Tech-lead worker decomposes REQ → writes STORY-001, STORY-002 drawers.
        Updates REQ-042 status=decomposed. Updates own agent-state. Exits.

T+6m:   Manager tick. STORY-001 pending, 3 pts → spawn junior.

T+15m:  Junior worker pushes branch, opens PR via gh CLI.
        Updates STORY-001 status=review, pr_url. Exits.

T+16m:  Manager tick. Sees STORY-001 status=review → spawn QA worker.

T+25m:  QA approves. STORY-001 status=merged.

T+26m:  Manager tick. All stories for REQ-042 merged → REQ-042 status=complete.
        Hygiene: delete merged-story worktrees.
```

### 8.4 Self-correction (stuck worker)

```
Manager tick:
1. For each agent-state where status=live:
   - Stat session_path. If mtime > 20min ago → "stuck candidate"
   - Read last 50 lines of session jsonl
2. For each stuck candidate, manager skill decides:
   - Permission prompt waiting? → kill, respawn with broader allowlist
   - Tool-use loop (same tool, same args, 5+ times)? → kill, file escalation
   - Genuinely thinking (active tool calls in last 5min)? → leave alone
   - No activity at all? → kill -9 PID, requeue story, diary entry
3. If killed:
   - Update agent-state.status=exited, exit_reason=killed-stuck
   - Revert story.status to pending, clear assigned_to, increment retry_count
   - If retry_count >= 3: file escalation, leave story.status=blocked
   - Delete worktree
   - Diary entry
```

### 8.5 Escalation (worker blocks)

```
Worker (per tasks/escalating.md skill):
1. Files escalation drawer
2. Updates story.status=blocked
3. Updates own agent-state.status=exited, exit_reason=escalated
4. Process exits

Manager next tick:
1. Sees open escalation
2. Skill decides: can manager resolve autonomously?
   - "Missing CLAUDE.md" → manager Bash-writes the file, marks resolved
   - "Missing API contract from another team" → spawn tech-lead worker to mediate
   - Genuinely needs human → leave open, surface in `hive status` + TUI
3. If resolved: requeue story (status=pending), diary entry
```

### 8.6 `hive stop`

```
1. Read .hive/watchdog.pid → SIGTERM
2. Watchdog catches SIGTERM → SIGTERM the current manager (if any)
3. Watchdog SIGTERMs all .hive/agents/*/worker.pid processes
4. Wait 30s for graceful shutdown
5. Anything still alive → SIGKILL
6. Clear PID files
7. Worktrees and agent-state drawers remain (cleanup is next `hive run`'s job)
```

## 9. Go Implementation Surface

### Repo structure

```
hungry-ghost-hive-v2/             (new repo, separate from v1)
├── go.mod
├── go.sum
├── cmd/
│   └── hive/
│       └── main.go
├── internal/
│   ├── config/                   # load/validate .hive/config.yaml
│   ├── paths/                    # workspace/mempalace path discovery
│   ├── drawers/                  # walk mempalace files, parse YAML, filter
│   ├── diary/                    # read/tail diary log
│   ├── proc/                     # spawn detached, PID files + flock, SIGTERM cascade
│   ├── watchdog/                 # the supervisor loop
│   └── cli/                      # command implementations
├── assets/                       # embedded via //go:embed
│   ├── skills/                   # 12 default skill .md files
│   ├── settings.local.json
│   └── mcp.json
└── README.md
```

### CLI commands

| Command | What it does | ~LOC |
|---|---|---|
| `hive init` | Prompts → writes .hive/config.yaml, embedded skills/MCP/settings, clones repos | 200 |
| `hive run` | Checks .hive/watchdog.pid; double-forks watchdog; exits | 80 |
| `hive stop` | Reads PIDs; SIGTERM → 30s → SIGKILL cascade | 60 |
| `hive status` | Walks $MEMPALACE_ROOT drawers + .hive/agents/ + diary; prints table | 150 |
| `hive add-req "<text>"` | Writes .hive/inbox/req-<ts>.txt | 20 |
| `hive logs [-f]` | tail/follow .hive/manager.log | 30 |

### Watchdog (Go-idiomatic)

```go
for {
    if shutdownRequested() { break }
    start := time.Now()
    cmd := exec.Command("claude", "--print", "--permission-mode", "acceptEdits",
                        "--append-system-prompt", "Invoke manager skill, do one tick")
    cmd.Dir = workspaceRoot
    cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
    if err := cmd.Start(); err != nil { /* log, sleep, retry */ continue }
    writePidFile(".hive/manager.pid", cmd.Process.Pid)

    done := make(chan error, 1)
    go func() { done <- cmd.Wait() }()
    select {
    case err := <-done:
        appendLog(".hive/watchdog.log", "tick exit=%v dur=%v", err, time.Since(start))
    case <-time.After(5 * time.Minute):
        syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
        appendLog(".hive/watchdog.log", "tick timeout dur=%v", time.Since(start))
    }
    clearPidFile(".hive/manager.pid")
    time.Sleep(tickInterval)
}
```

`Setpgid` + `Kill(-pid, ...)` guarantees clean cleanup of all manager child processes if a tick times out.

### Critical design choice: no MCP client in the CLI

The CLI **never talks to mempalace via MCP or HTTP**:

1. **Reads are file-based.** Mempalace stores drawers as YAML-frontmatter markdown under `$MEMPALACE_ROOT/wings/<wing>/rooms/<room>/`. `hive status` walks the directory and parses files. Zero IPC. **Verification item:** confirm mempalace's on-disk format is stable enough to depend on (Phase 1).
2. **Writes go through claude, not the CLI.** `hive add-req` writes a flat file to `.hive/inbox/`; the manager's first action each tick drains the inbox and files drawers via the mempalace MCP. Claude is the only writer to mempalace — single point of consistency, proper embedding generation.

This deletes an entire dependency (`@modelcontextprotocol/sdk` equivalent in Go) and removes "mempalace gateway unreachable" as a CLI failure mode.

### Bundled assets via `embed`

```go
//go:embed assets/skills/* assets/settings.local.json assets/mcp.json
var assets embed.FS
```

`hive init` reads from `assets.ReadFile(...)` and writes to the workspace. No external file dependency.

### Dependencies (`go.mod`)

```go
require (
    github.com/spf13/cobra v1.x        // CLI parsing
    gopkg.in/yaml.v3 v3.x              // config + drawer frontmatter parsing
    github.com/fatih/color v1.x        // pretty status output
    github.com/gofrs/flock v0.x        // PID file flock
)
```

Four runtime dependencies, all statically linked into the binary.

### Total estimate

**~600-800 LOC of Go** for the entire v2 CLI + watchdog, plus the bundled markdown assets.

### Distribution

- `go install github.com/.../hungry-ghost-hive-v2/cmd/hive@latest`
- GitHub Releases with prebuilt binaries (`hive_darwin_arm64`, `hive_linux_amd64`, etc.) via `goreleaser`
- No npm package. No Node required.

## 10. Phasing

This spec covers Phase 0 only. Subsequent phases each get their own spec/plan/implementation cycle, referencing this architecture.

| Phase | Scope | Output |
|---|---|---|
| **0 (this spec)** | Architecture overview | Design doc only — no code |
| **1** | Minimal foundation | `hive init` + `hive run` + `hive stop` + a single working role (e.g., junior); watchdog + manager skill + 1 worker skill; happy path only; one team, one repo; tiny TUI scaffold |
| **2** | Roles & skills | All 6 role skills + 6 task skills; story decomposition (tech-lead); QA review flow; full lifecycle drawers |
| **3** | Multi-agent orchestration | N>1 concurrent workers; dependency-aware story routing; multi-repo support |
| **4** | Self-correcting manager | Stuck detection, escalation handling, hygiene cycles, days-long autonomous runs validated |
| **5** | TUI overhaul (its own brainstorm) | Real TUI design + build against the now-stable backend |

Phase 1 is the foundation; Phases 2-4 are largely "expand the manager skill + add more skills" with bounded Go changes. Phase 5 is a separate creative project (the original "UI overhaul" sub-project).

## 11. Open Risks & Verification Items

To resolve during Phase 1:

| Risk | Verification |
|---|---|
| Mempalace's on-disk drawer format is stable enough to depend on for direct reads | Verify by reading `$MEMPALACE_ROOT` structure; document the contract |
| Claude Code's skill discovery in worker worktrees (when worktree is sibling to workspace, not under it) | Test: spawn `claude` in a worktree dir; confirm it sees skills from workspace `.claude/skills/` or requires injection into worktree |
| Atlassian MCP reliability for production Jira sync | Run Jira sync from a manager-skill behavior for a week; measure failure rate; if unreliable, add a thin Go fallback as Phase 2 work |
| Permission allowlist completeness | Empirically: run a full story end-to-end with no human prompts; iterate on `settings.local.json` |
| Single-manager assumption holds under crash recovery | Kill the manager mid-tick; confirm next tick recovers state correctly from mempalace |
| Rate limits / API outages during multi-day runs | Manager skill needs operational hygiene: "if recent worker failures look like rate limits, back off"; verify by deliberate throttling |

## 12. Out of Scope (this spec)

- Detailed skill content (what each skill *says*) — Phase 1+ drafts each one
- TUI design — Phase 5
- Migration path for v1 users — none; v1 is frozen
- Cloud/SaaS hosted hive — v2 is local-first
- Multi-user collaboration on a single workspace — single-user assumed
