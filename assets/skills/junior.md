---
name: hive-v2-junior
description: Use when spawned as a hungry-ghost-hive-v2 junior worker. This is the v2 Go-binary architecture (NOT the legacy capstone-hive). Reads context.md, implements a story, opens a PR via gh, files outcome to mempalace, exits.
---

# Hive Junior — Worker Skill

## YOU MUST / YOU MUST NOT

- **YOU MUST** read `.hive/agents/<YOUR_ID>/context.md` first — it contains your agent ID, the story, the team, and the branch.
- **YOU MUST** implement exactly one story. Do not pick up other pending stories. Do not invoke the manager skill.
- **YOU MUST** commit and push your work, then open a PR via `gh pr create` (follow the `tasks/creating-a-pr.md` skill).
- **YOU MUST** update your `agent-state` drawer to `status: exited` AND update the story drawer to `status: review, pr_url: <url>` before exiting.
- **YOU MUST** exit cleanly when done. The manager reaps you on the next tick.
- **YOU MUST NOT** prompt the user for input — permission-bypass mode is active.

Your cwd is your git worktree. Your branch already exists (the manager created it).

## Procedure

### 1. Read your context

```bash
cat .hive/agents/<YOUR_ID>/context.md
```

(The path may need adjustment depending on where the manager spawned you — your cwd is the worktree, which is typically `repos/<team>--junior-<id>`, so the workspace root is `../..` from there. The exact path is in the manager's invocation prompt.)

### 2. Orient

Read the team repo's:
- `README.md` (project overview)
- `CLAUDE.md` if present (project-specific instructions)
- The relevant project manifest: `package.json` / `go.mod` / `pyproject.toml` / `Cargo.toml` / etc.

### 3. Implement

Make the minimum change required by the story's acceptance criteria. If the project has tests, update them and run them.

### 4. Commit + push + open PR

Use the `tasks/creating-a-pr.md` skill. It covers commit message conventions, branch push, and `gh pr create`. Capture the PR URL from the `gh pr create` output.

### 5. File your outcome

- Find your `agent-state` drawer:
  - `mempalace_list_drawers` wing=`hive-<workspace-slug>` (from `.hive/config.yaml`) room=`agents`
  - Locate the one with `title = agent-<YOUR_ID>`
- Update it via `mempalace_update_drawer`: `status=exited`, `exit_reason=completed`, `ended_at=<iso-now>`.
- Find your story drawer:
  - `mempalace_list_drawers` wing=same, room=`stories`
  - Locate the one assigned to you (matches `current_story` from your agent-state)
- Update via `mempalace_update_drawer`: `status=review`, `pr_url=<the URL from gh pr create>`.

### 6. Optional: file a finding

If you discovered something durable (a bug pattern, a non-obvious gotcha, a useful trick), use `tasks/filing-a-finding.md` to file it. Otherwise skip.

### 7. Exit

Just stop. The process ends; the manager reaps your PID on the next tick.

## After completion checklist

- Did the PR get created (you have a URL)?
- Did you update your `agent-state` to `exited`?
- Did you update the story to `review` with `pr_url` set?
- Are you about to exit cleanly (no pending tool calls)?

If any answer is "no" and you can complete it now, do so. If you genuinely cannot complete the work (blocker, missing context, can't push), do NOT push a half-done PR — instead:

- Update your `agent-state` to `status=exited`, `exit_reason=escalated`
- File a `finding` drawer (use `tasks/filing-a-finding.md`) describing the blocker
- Exit

Phase 2 will introduce a proper escalation skill; for Phase 1.1 the finding drawer is enough.
