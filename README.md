# Hive

[![License: MIT](https://img.shields.io/github/license/nikrich/hungry-ghost-hive-v2?style=flat-square)](LICENSE)
[![Release](https://img.shields.io/github/v/release/nikrich/hungry-ghost-hive-v2?include_prereleases&style=flat-square&label=release)](https://github.com/nikrich/hungry-ghost-hive-v2/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/nikrich/hungry-ghost-hive-v2?style=flat-square&logo=go)](go.mod)
[![Status: alpha](https://img.shields.io/badge/status-alpha-orange?style=flat-square)](#roadmap)
[![Built with Claude](https://img.shields.io/badge/built%20with-Claude-D97757?style=flat-square&logo=anthropic&logoColor=white)](https://claude.com/claude-code)

> A single Go binary that supervises Claude Code subprocesses to coordinate agile-style AI development teams. All orchestration logic lives in editable skill markdown — not in code.

Hive turns a high-level requirement into a real git PR by spawning a Claude Code worker in its own worktree, watching it work, and recording state in [mempalace](https://github.com/mempalace/mempalace) — the persistent memory layer. The whole orchestrator is ~1100 lines of Go; everything that decides *what to do next* is in `.claude/skills/*.md` files that you can edit without recompiling.

```text
$ hive add-req "Add a /healthz endpoint to the API"
$ hive run

# 90 seconds later, in your remote:
agent/api--junior-abc12345
└─ feat: add /healthz endpoint
   Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
```

---

## Table of contents

- [Why Hive](#why-hive)
- [Install](#install)
- [Quick start](#quick-start)
- [How it works](#how-it-works)
- [Workspace layout](#workspace-layout)
- [Roadmap](#roadmap)
- [Design docs](#design-docs)
- [Contributing](#contributing)
- [License](#license)

---

## Why Hive

**Single binary.** No Node, no Docker, no broker. `go install` puts `hive` on your `$PATH`.

**Claude Code is the runtime.** Hive doesn't talk to the Anthropic SDK. It spawns `claude --print` subprocesses with `--system-prompt-file <role>.md`. Every agent gets Claude Code's full tool surface (Bash, Edit, MCP, skills) without Hive reimplementing any of it.

**Skills are the source of truth.** Want the manager to behave differently? Edit `.claude/skills/manager.md`. Want to add a new role? Drop in `architect.md`. No release, no recompile.

**Mempalace is the only state.** No SQLite, no Postgres, no migrations. Story queues, agent state, escalations, findings — all live as drawers in a mempalace wing scoped to your workspace.

**Self-correcting by construction.** Each tick is a fresh `claude --print` reading current state. The watchdog supervises; the [bufio scanner streams partial output to `manager.log`](docs/specs/2026-05-28-phase-1.3-tick-reliability-design.md) so a SIGKILL'd tick still tells you what happened. Stories that were assigned but abandoned get re-pended with a retry counter; after 3 attempts they're moved to `blocked` and an escalation drawer is filed for human review.

**Designed to run for days.** The architecture is built around supervisor loops, not chat sessions. Multi-day reliability is a Phase 2 goal — the foundation is in place.

---

## Install

### Prerequisites

- **Go 1.22+** — to install the binary
- **Claude Code CLI** — logged in via `claude login` (OAuth) or `ANTHROPIC_API_KEY`
- **Python 3.10+** — for the mempalace gateway (auto-installed on first `hive init` via `uv tool install` or `pip install --user`)
- **`git`** — and `gh` if you want workers to open real PRs

### One-line install

```sh
go install github.com/nikrich/hungry-ghost-hive-v2/cmd/hive@v1.0.0-alpha.1
```

Make sure `$(go env GOPATH)/bin` is on your `$PATH`. Verify:

```sh
hive --help    # should mention "supervises Claude Code subprocesses"
```

---

## Quick start

### 1. Set up a workspace

```sh
mkdir my-hive-run
cd my-hive-run
git clone <your-repo-url> repos/my-team

hive init \
  --workspace-slug my-run \
  --team name=my-team,url=<your-repo-url> \
  --no-clone
```

`hive init` writes a `.hive/` directory (config + memory skeleton + inbox), a `.claude/` directory (skills + MCP config + permissions allowlist), and installs the mempalace gateway if it isn't already importable.

### 2. Queue a requirement

```sh
hive add-req "Add a // HELLO HIVE comment to the top of README.md"
```

This drops a flat file in `.hive/inbox/`. The manager will pick it up on its next tick.

### 3. Run

```sh
hive run                # foreground; ^C to stop
hive logs -f            # in another terminal — tail the manager's per-tick output
```

Or detached:

```sh
nohup hive run > hive.out 2>&1 &
hive status              # snapshot of stories, agents, escalations
hive stop                # graceful cascade (watchdog → manager → workers)
```

### 4. Watch the loop run

```text
$ hive logs -f
Tick complete.

**Summary:**
- Drained inbox: req-1780041960-... → requirement + story drawers (team=my-team)
- Spawned: agent 90f30131 on worktree repos/my-team--junior-90f30131
- Diary: spawned=1 reaped=0 live=1 pending=0
```

The agent will commit + push a branch named `agent/<team>--junior-<id>` and (if `gh` is configured) open a PR.

---

## How it works

```
┌─ user ──┐  add-req       ┌─ .hive/inbox/ ─┐
│         ├───────────────►│ req-<ts>.txt   │
└─────────┘                └────────────────┘
                                    │
                                    ▼
┌─ watchdog ──┐  every 60s  ┌─ manager (per-tick claude) ──┐
│  (Go loop)  │────────────►│  --system-prompt-file        │
│             │             │      manager.md              │
│  pid file   │◄────────────┤  drains inbox → drawers      │
│  log file   │   logs      │  spawns worker subprocesses  │
│             │             │  reaps + re-pends + escalates│
└─────────────┘             └──────────────────────────────┘
                                    │ Bash spawn
                                    ▼
                            ┌─ worker (claude) ─────┐
                            │  cd into worktree     │
                            │  read context.md      │
                            │  implement story      │
                            │  git commit + push    │
                            │  gh pr create         │
                            │  file outcome drawer  │
                            └───────────────────────┘
                                    │ all via MCP
                                    ▼
                            ┌─ mempalace ───────────┐
                            │  wings/hive/rooms/    │
                            │   requirements/       │
                            │   stories/            │
                            │   agents/             │
                            │   escalations/        │
                            │   findings/           │
                            └───────────────────────┘
```

Key design choices, with the spec link if you want the why:

| Decision | Where it's argued |
|---|---|
| Single Go binary supervising `claude --print` subprocesses | [Phase 0 architecture spec](docs/specs/2026-05-27-architecture-design.md) |
| Mempalace as the only storage | [Phase 0](docs/specs/2026-05-27-architecture-design.md) |
| Workspace-local memory at `.hive/memory/` | [Phase 1.2 spec](docs/specs/2026-05-28-phase-1.2-workspace-local-memory-design.md) |
| `--system-prompt-file` instead of skill invocation | [Phase 1.4 spec](docs/specs/2026-05-29-phase-1.4-system-prompt-inlining-design.md) |
| Re-pend abandoned stories with retry cap of 3 | [Phase 1.2 spec §3 decision 5](docs/specs/2026-05-28-phase-1.2-workspace-local-memory-design.md) |
| Line-buffered stdout to `manager.log` so SIGKILL is debuggable | [Phase 1.3 spec](docs/specs/2026-05-28-phase-1.3-tick-reliability-design.md) |

---

## Workspace layout

After `hive init` your workspace looks like this:

```text
my-hive-run/
├── .hive/
│   ├── config.yaml          # teams, tick interval, timeouts, worker limits
│   ├── inbox/               # add-req drops files here; manager drains
│   │   └── processed/       # post-drain archive
│   ├── memory/              # workspace-local mempalace data (gitignored by default)
│   │   ├── wings/hive/
│   │   │   └── rooms/{requirements,stories,agents,escalations,findings}/
│   │   ├── index/chroma/
│   │   └── .mempalace/config.yaml
│   ├── agents/<id>/         # per-spawn ephemeral state (context.md, worker.pid, etc.)
│   ├── watchdog.pid
│   ├── manager.pid
│   ├── manager.log          # streamed line-by-line as the manager runs
│   └── watchdog.log
├── .claude/
│   ├── skills/
│   │   ├── manager.md       # one-tick orchestrator
│   │   ├── junior.md        # implementer role
│   │   └── tasks/
│   │       ├── creating-a-pr.md
│   │       └── filing-a-finding.md
│   ├── settings.local.json  # permission allowlist (gitignored)
│   └── mcp.json             # mempalace MCP gateway pointed at .hive/memory/
└── repos/
    └── <team-name>/         # canonical clone; never directly edited
```

You can `git init` the workspace and commit `.hive/config.yaml` + `.claude/skills/` to version the orchestration logic with your team. Memory and PID files are gitignored by default.

---

## Roadmap

What's shipped today (`v1.0.0-alpha.1`):

- [x] `hive init` / `run` / `stop` / `status` / `add-req` / `logs` CLI
- [x] Workspace-local mempalace memory
- [x] Watchdog supervisor with per-tick `claude --print` spawn
- [x] Manager + junior roles via `--system-prompt-file` inlining
- [x] Inbox drain → drawer writes → worker spawn → git push
- [x] Re-pend / retry / escalate logic for abandoned stories
- [x] Line-buffered `manager.log` (SIGKILL-survivable diagnostics)

In flight (Phase 2):

- [ ] **`tech-lead` role**: decomposes large requirements into sized stories with dependencies
- [ ] **`senior` / `intermediate` roles**: claim higher-point stories
- [ ] **`qa` role**: reviews each PR before the next story starts
- [ ] **Story dependency graph + multi-worker scheduling**
- [ ] **Multi-day reliability validation**

Later:

- [ ] **TUI dashboard** — distinct from `hive status` plain text
- [ ] **Real-remote GitHub PR verification** — currently exercised against a bare local remote
- [ ] **Conditional `claude --bare`** for API-key users (skips user-side plugin hooks)
- [ ] **MCP precedence fix** — drawers should land in workspace-local mempalace, not the user's global instance

See [`docs/specs/`](docs/specs/) for design specs and [`docs/plans/`](docs/plans/) for implementation plans + verification runbooks.

---

## Design docs

Hive was built iteratively across six phases, each with its own design spec and verification runbook. If you want to understand why a decision was made, the spec is usually the answer:

| Phase | Spec | What landed |
|---|---|---|
| 0 | [architecture design](docs/specs/2026-05-27-architecture-design.md) | Go-supervisor-of-Claude-Code approach, mempalace as storage |
| 1 | [minimal foundation](docs/plans/2026-05-27-phase-1-minimal-foundation.md) | CLI commands, watchdog, skill embedding |
| 1.1 | [skill hardening + init polish](docs/specs/2026-05-28-phase-1.1-skill-hardening-design.md) | `hive-v2-*` skill name disambiguation, init UX |
| 1.2 | [workspace-local memory](docs/specs/2026-05-28-phase-1.2-workspace-local-memory-design.md) | `.hive/memory/`, retry/escalate logic |
| 1.3 | [tick reliability](docs/specs/2026-05-28-phase-1.3-tick-reliability-design.md) | `bufio.Scanner` for `manager.log`, `hive status` fix |
| 1.4 | [`--system-prompt-file` inlining](docs/specs/2026-05-29-phase-1.4-system-prompt-inlining-design.md) | Manager + junior become their system prompt |

Verification runbooks live in [`docs/plans/*verification-runbook.md`](docs/plans/) — each documents what worked, what didn't, and what to fix next.

---

## Contributing

This is alpha-quality software with one role implemented. PRs are welcome, especially around:

- **Phase 2 work** — anything from the in-flight list above
- **Skill content** — `manager.md` and `junior.md` are markdown; sharper prose helps Claude
- **Verification scenarios** — runnable e2e scenarios that exercise edge cases
- **Cross-platform support** — currently tested on macOS/arm64; Linux PRs welcome

Before sending a PR:

```sh
go test ./...
go build ./cmd/hive
./hive --help        # smoke test
```

For larger changes, open an issue first to discuss the design — Hive's whole architectural premise is "small, defensible decisions you can read in the specs."

---

## License

[MIT](LICENSE) — © 2026 Jannik Richter.

---

<sub>Hive is the v2 ground-up rewrite of the original [hungry-ghost-hive](https://github.com/nikrich/hungry-ghost-hive). v1 is frozen; only the new repo is actively developed.</sub>
