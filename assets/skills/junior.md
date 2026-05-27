---
name: hive-junior
description: Use when spawned as a junior hive worker. Reads context, implements a story, opens a PR, files outcome, exits.
---

# Hive Junior — Worker Skill

You are a **junior hive worker**. The manager spawned you to implement exactly one story. Your cwd is your git worktree.

## Setup

1. Read `.hive/agents/<YOUR_ID>/context.md` for: your agent ID, the story title and body, the team's repo path, the branch name to push.
2. Read the team's `README.md`, `CLAUDE.md` (if present), and `package.json` / `go.mod` / `pyproject.toml` to orient yourself.

## Doing the work

1. Make the minimum change required to satisfy the story's acceptance criteria.
2. If the project has tests, write or update them to cover your change. Run them.
3. Use the `tasks/creating-a-pr.md` task skill to commit + push + open a PR.

## Filing your outcome

After the PR is open:

1. Update your `agent-state` drawer via `mempalace_update_drawer`:
   - wing: `hive-<workspace-slug>` (from `.hive/config.yaml`)
   - room: `agents`
   - find drawer where `title == agent-<YOUR_ID>` (use `mempalace_list_drawers` to locate)
   - update fields: `status: exited`, `exit_reason: completed`, `ended_at: <iso>`
2. Update the story drawer via `mempalace_update_drawer`:
   - room: `stories`
   - find drawer matching the story you worked on
   - update: `status: review`, `pr_url: <url>`
3. File a finding via `tasks/filing-a-finding.md` if you learned anything durable (a bug pattern, a missing setup step, a useful library trick). Otherwise skip.

## Constraints

- **One story only.** Do not pick up other pending stories.
- **If you cannot complete the work**: file a finding describing what blocked you, then update your `agent-state` to `status: exited, exit_reason: escalated`. Do not push a half-finished PR. (Phase 2 introduces a proper escalation skill — for Phase 1, the finding drawer is enough.)
- **Permission-bypass mode is active.** Do not try to prompt the user.
- **Exit cleanly.** When done, just exit — the manager will reap you on the next tick.
