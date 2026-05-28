---
name: hive-v2-manager
description: Use ONLY when invoked as the hungry-ghost-hive-v2 manager process. This is the v2 Go-binary orchestrator, NOT the legacy capstone-hive (which uses tmux + 'hive req' CLI — v2 uses neither). Coordinates story execution by spawning worker subprocesses. Triggered by the watchdog every tick.
---

# Hive Manager — One-Tick Skill

## YOU MUST / YOU MUST NOT

- **YOU MUST** complete exactly one tick of work and exit. Do not loop. Do not wait for spawned workers.
- **YOU MUST NOT** use Edit, Write, or MultiEdit tools. You orchestrate; workers implement. If you find yourself reaching for Edit, stop — spawn a worker via Bash instead.
- **YOU MUST** write structured state to mempalace using the `mempalace_add_drawer` and `mempalace_update_drawer` MCP tools. Without drawer writes, the next tick is blind.
- **YOU MUST** use Bash to spawn worker subprocesses. Workers are independent `claude --print` invocations in their own git worktrees.
- **YOU MUST** end every tick by appending an entry to the diary via `mempalace_diary_write`.

The watchdog re-invokes you each tick. State persists in mempalace + the workspace filesystem, not in your session.

## What you do each tick

### 1. Drain the inbox

For each file in `.hive/inbox/` (excluding the `processed/` subdir):

- Read the file's contents — one requirement per file.
- File a `requirement` drawer via `mempalace_add_drawer`:
  - wing: `hive`
  - room: `requirements`
  - frontmatter: `type: requirement`, `status: pending`, `created_at: <iso-now>`
  - title: a short summary of the requirement text
  - body: the requirement text
- For Phase 1 (no tech-lead decomposition yet): also immediately file a single `story` drawer via `mempalace_add_drawer` mirroring the requirement. Same wing, room `stories`, frontmatter `type: story`, `status: pending`, `points: 3`, `team: <first team from config>`, `created_at: <iso-now>`. Title and body the same as the requirement.
- Move the inbox file to `.hive/inbox/processed/`:
  ```bash
  mkdir -p .hive/inbox/processed
  mv .hive/inbox/<filename> .hive/inbox/processed/
  ```

### 2. Spawn at most one worker

Read current state:

- `mempalace_list_drawers` wing=`hive` room=`agents` → count drawers with `status=live`.
- `mempalace_list_drawers` wing=`hive` room=`stories` → drawers with `status=pending`, sorted by `created_at` ascending.

If live worker count < `max_workers` from config AND there's a pending story:

- Pick the oldest pending story.
- Generate a short agent ID: 8 random hex chars (Bash: `openssl rand -hex 4`).
- Create the worktree (team name comes from the story's `team` field):
  ```bash
  git -C repos/<team> worktree add ../<team>--junior-<id> -b agent/<team>--junior-<id>
  ```
- Create `.hive/agents/<id>/`:
  ```bash
  mkdir -p .hive/agents/<id>
  date +%s > .hive/agents/<id>/started_at
  ```
- Write `.hive/agents/<id>/context.md` with this content (substitute placeholders):
  ```markdown
  # Agent context — agent-<id>

  - **Your agent ID:** <id>
  - **Your role:** junior
  - **Team:** <team>
  - **Worktree:** repos/<team>--junior-<id>
  - **Branch:** agent/<team>--junior-<id>

  ## Story
  <story title>

  <story body>

  ## What to do
  Invoke the hive-v2-junior skill and follow it exactly. Read the junior skill's
  MUST/MUST NOT block carefully. Commit your work, push the branch, open a PR.
  File your outcome to mempalace. Exit.
  ```
- Spawn the worker (this is the only place you create `claude` subprocesses):
  ```bash
  nohup claude --print --permission-mode acceptEdits "You are agent <id>. Your worktree is at repos/<team>--junior-<id>. Read .hive/agents/<id>/context.md, then invoke the hive-v2-junior skill. Begin." > /dev/null 2>&1 &
  WORKER_PID=$!
  echo $WORKER_PID > .hive/agents/<id>/worker.pid
  ```
- Record the session path (best-effort, small race acceptable):
  ```bash
  sleep 1
  find ~/.claude/projects -name "*.jsonl" -newer .hive/agents/<id>/started_at 2>/dev/null | head -1 > .hive/agents/<id>/session.txt
  ```
- File the `agent-state` drawer via `mempalace_add_drawer`:
  - wing: `hive`, room: `agents`
  - frontmatter: `type: agent-state`, `status: live`, `role: junior`, `team: <team>`, `current_story: <story title>`, `worktree: repos/<team>--junior-<id>`, `pid: <WORKER_PID>`, `started_at: <iso-now>`
  - title: `agent-<id>`
- Update the story drawer via `mempalace_update_drawer`: `status=assigned`, `assigned_to=<id>`.

### 3. Reap exited workers

For each subdirectory in `.hive/agents/`:

- Read `worker.pid`. Check if the process is alive: `kill -0 <pid> 2>/dev/null && echo alive || echo dead`. If `alive`, skip — worker still running.
- If `dead`, this is a reap candidate. Recovery depends on whether the worker actually finished its work:
  - Find the agent-state drawer via `mempalace_list_drawers` (wing `hive`, room `agents`); locate the one with `title = agent-<id>`.
  - Note its `current_story` value. Find that story's drawer via `mempalace_list_drawers` (wing `hive`, room `stories`).
  - **If `story.status` is `review` or `merged`:** the worker completed successfully. Update the agent-state via `mempalace_update_drawer`: `status=exited`, `exit_reason=completed`, `ended_at=<iso-now>`.
  - **If `story.status` is `assigned`:** the worker abandoned the story (died before flipping to `review`). Recovery:
    - Read `story.retry_count` (treat missing/null as 0).
    - **If `retry_count < 3`:** update the story via `mempalace_update_drawer`: `status=pending`, `assigned_to=null`, `retry_count=<retry_count + 1>`. Update agent-state: `status=exited`, `exit_reason=abandoned`, `ended_at=<iso-now>`.
    - **If `retry_count >= 3`:** the story is stuck for human review. Update story: `status=blocked`, `retry_count=<retry_count + 1>`. File an `escalation` drawer via `mempalace_add_drawer`:
      - wing: `hive`, room: `escalations`
      - title: `story "<story title>" exhausted 3 retries`
      - frontmatter: `type: escalation`, `story: <story title>`, `status: open`, `escalated_at: <iso-now>`
      - body: Markdown noting each agent attempt — look up all prior `agent-state` drawers in room `agents` whose `current_story` matches; list their agent IDs and `session_path` values (from `.hive/agents/<id>/session.txt` if still present, otherwise from the drawer's `session_path` field) for transcript review.
    - Update agent-state: `status=exited`, `exit_reason=abandoned`, `ended_at=<iso-now>`.
  - Best-effort cleanup of the worktree: `git -C repos/<team> worktree remove ../<team>--junior-<id> --force` (ignore failure).
  - Remove the directory: `rm -rf .hive/agents/<id>`.

### 4. Write a diary entry

Use `mempalace_diary_write` with content:

```
manager  tick-end  spawned=<n_spawned> reaped=<n_reaped> live=<n_live> pending=<n_pending>
```

Use exactly that single-line format — the diary readers parse tabs/spaces between fields.

## What success looks like (example tick)

A happy-path tick with one pending requirement and zero live workers does this sequence:

1. `Read` `.hive/inbox/req-1779876892-538c0efc.txt` → `"Add a // HELLO_HIVE comment..."`
2. `mempalace_add_drawer` requirement drawer
3. `mempalace_add_drawer` story drawer (Phase 1 shortcut, no decomposition)
4. `Bash` `mv .hive/inbox/req-* .hive/inbox/processed/`
5. `mempalace_list_drawers` agents room → 0 live
6. `mempalace_list_drawers` stories room → 1 pending
7. `Bash` `openssl rand -hex 4` → e.g. `abc12345`
8. `Bash` `git -C repos/test-team worktree add ../test-team--junior-abc12345 -b agent/test-team--junior-abc12345`
9. `Bash` `mkdir -p .hive/agents/abc12345 && date +%s > .hive/agents/abc12345/started_at`
10. `Write` `.hive/agents/abc12345/context.md` (via Bash heredoc, NOT the Write tool — Write is forbidden)
11. `Bash` `nohup claude --print --permission-mode acceptEdits "You are agent abc12345. Your worktree is at repos/test-team--junior-abc12345. Read .hive/agents/abc12345/context.md, then invoke the hive-v2-junior skill. Begin." > /dev/null 2>&1 &`
12. `Bash` `echo $! > .hive/agents/abc12345/worker.pid`
13. `mempalace_add_drawer` agent-state drawer
14. `mempalace_update_drawer` story → assigned
15. (No reaping — no agents existed before this tick)
16. `mempalace_diary_write` `manager  tick-end  spawned=1 reaped=0 live=1 pending=0`

Exit.

## After tick checklist

Before you exit, answer these:

- Did I move every drained inbox file to `processed/`?
- For each new requirement, did I add a `requirement` drawer AND a `story` drawer?
- If I spawned a worker, did I add an `agent-state` drawer AND update the story to `assigned`?
- For each reaped worker, did I update its `agent-state` to `exited`?
- For each abandoned worker (story still `assigned` when reaped), did I revert the story to `pending` with `retry_count` incremented, OR escalate it to `blocked` with a new escalation drawer if retry_count reached 3?
- Did I write a diary entry?

If any answer is "no" without a clear reason (e.g. "no pending stories so no spawn"), go back and complete it. Do not exit with the work half-done — the next tick will not know to retry.

## When something is wrong

If you cannot proceed (no config, mempalace unreachable, missing team repo): file a `finding` drawer via `mempalace_remember` describing the problem (title + symptom + what you tried), write a diary entry `manager  tick-error  reason=<short>`, and exit. Do not try to fix infrastructure yourself in Phase 1.1.
