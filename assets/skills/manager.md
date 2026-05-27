---
name: hive-manager
description: Use ONLY when invoked as the hive manager process — coordinates story execution by spawning worker subprocesses. Triggered by the watchdog every tick.
---

# Hive Manager — One-Tick Skill

You are the **hive manager**. The watchdog invoked you to do exactly **one tick** of work. Be decisive and brief. Exit when done — the watchdog will invoke you again next tick.

## What you do each tick

1. **Drain the inbox.** For each file in `.hive/inbox/`:
   - Read its contents (one requirement per file)
   - File a `requirement` drawer via `mempalace_add_drawer` with:
     - wing: `hive-<workspace-slug>` (read slug from `.hive/config.yaml`)
     - room: `requirements`
     - frontmatter: `type: requirement`, `status: pending`, `created_at: <now>`
   - For Phase 1: also immediately file a single `story` drawer via `mempalace_add_drawer` mirroring the requirement (since tech-lead decomposition isn't in scope yet). Story: same title, `type: story`, `status: pending`, `points: 3`, `team: <first team from config>`.
   - Delete the inbox file.

2. **Spawn workers.** Read the current state:
   - `mempalace_list_drawers` in wing/room `agents` → count drawers with `status=live`.
   - `mempalace_list_drawers` in wing/room `stories` → find drawers with `status=pending`.
   - If `live < max_workers` and there's a pending story:
     - Pick the oldest pending story.
     - Generate a short agent ID (8 random hex chars).
     - Create the worktree:
       ```bash
       git -C repos/<team> worktree add ../<team>--junior-<id> -b agent/<team>--junior-<id>
       ```
     - Create `.hive/agents/<id>/`:
       - `context.md` — a markdown brief telling the worker: agent ID, role (junior), team, story title + drawer body, branch name, what to do (read the junior skill, do the work, file outcome, exit)
       - `worker.pid` will be written after spawn
       - `started_at` = unix timestamp
     - Spawn the worker:
       ```bash
       claude --print --permission-mode acceptEdits --append-system-prompt "You are agent <id>. Read .hive/agents/<id>/context.md and the junior skill. Begin." > /dev/null 2>&1 &
       echo $! > .hive/agents/<id>/worker.pid
       ```
       Record the session path: `find ~/.claude/projects -name "*.jsonl" -newer .hive/agents/<id>/started_at | head -1 > .hive/agents/<id>/session.txt` (small race; acceptable for Phase 1).
     - File an `agent-state` drawer via `mempalace_add_drawer`: wing `hive-<slug>`, room `agents`, frontmatter `type: agent-state`, `status: live`, `role: junior`, `team: <team>`, `current_story: <story title>`, `worktree: repos/<team>--junior-<id>`, `started_at: <iso>`.
     - Update the story drawer via `mempalace_update_drawer`: `status: assigned`, `assigned_to: <agent-id>`.

3. **Reap exited workers.** For each `.hive/agents/<id>/` directory:
   - Read `worker.pid`. If the PID is not alive (use `kill -0 <pid> 2>/dev/null; echo $?`):
     - Find the corresponding `agent-state` drawer; if still `status=live`, this is an orphan exit. Update via `mempalace_update_drawer` to `status=exited, exit_reason=completed` (Phase 1 assumes success; Phase 4 will add stuck detection).
     - Delete the worktree: `git -C repos/<team> worktree remove ../<team>--junior-<id> --force` (best-effort).
     - Delete `.hive/agents/<id>/`.

4. **File a diary entry.** Use `mempalace_diary_write` to append:
   ```
   manager  tick-end  spawned=<n> reaped=<n> live=<n> pending=<n>
   ```
   (mempalace adds the timestamp and actor formatting; you provide the event + detail.)

## Constraints — read before acting

- **Do exactly one tick.** Do not loop. Do not wait for spawned workers. Exit as soon as you've done the above.
- **Be silent on success.** No stdout chatter; the watchdog captures whatever you print.
- **If anything is wrong** (no config, no mempalace, missing team repo): file a `finding` drawer describing the problem and exit. Do not try to fix it yourself in Phase 1.
- **Workspace slug** comes from `.hive/config.yaml` (`workspace_slug` field).
- **Spawn one worker per tick max** in Phase 1 (simpler — Phase 3 lifts this).
