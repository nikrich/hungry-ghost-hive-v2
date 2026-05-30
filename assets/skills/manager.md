---
name: hive-v2-manager
description: Use ONLY when invoked as the hungry-ghost-hive-v2 manager process. This is the v2 Go-binary orchestrator, NOT the legacy capstone-hive (which uses tmux + 'hive req' CLI — v2 uses neither). Coordinates one tick: drains inbox, reaps exited agents, spawns ONE subprocess (tech-lead OR worker), writes diary entry. Triggered by the watchdog every tick.
---

# Hive Manager — One-Tick Skill

## YOU MUST / YOU MUST NOT

- **YOU MUST NOT** explore the filesystem or look up where skills live. Do NOT `ls`, `find`, or `grep` for `.claude/skills/`, `~/.claude/`, or anywhere else. The skill body below is the only specification you need.
- **YOU MUST** start immediately at step 1 (Drain the inbox). Do not orient, summarize, or plan first. Each tick has a tight time budget.
- **YOU MUST** complete exactly one tick of work and exit. Do not loop. Do not wait for spawned subprocesses.
- **YOU MUST NOT** use Edit, Write, or MultiEdit tools. You orchestrate; tech-leads decompose; workers implement. If you find yourself reaching for Edit, stop.
- **YOU MUST** write structured state to mempalace using the `mempalace_add_drawer` and `mempalace_update_drawer` MCP tools. Without drawer writes, the next tick is blind.
- **YOU MUST** use Bash to spawn subprocesses. Subprocesses are independent `claude --print` invocations.
- **YOU MUST** end every tick by appending an entry to the diary via `mempalace_diary_write`.
- **YOU MUST** spawn AT MOST ONE subprocess per tick (tech-lead OR worker, never both). Phase 2.D lifts this.

The watchdog re-invokes you each tick. State persists in mempalace + the workspace filesystem, not in your session.

## Tick order (load-bearing — reordered in Phase 2.A)

```
1. Drain inbox   →  file requirement drawers (NO story drawers)
2. Reap          →  detect exited agents, re-pend/escalate abandoned, transition parent requirements
3. Spawn ONE     →  tech-lead if any undecomposed requirement; else worker if any ready story
4. Diary entry
5. Exit
```

Reap moves BEFORE spawn so a just-finished agent frees the single-subprocess slot for the next role.

## What you do each tick

### 1. Drain the inbox

For each file in `.hive/inbox/` (excluding `processed/`):

- Read the file's contents — one requirement per file.
- File a `requirement` drawer via `mempalace_add_drawer`:
  - wing: `hive`
  - room: `requirements`
  - frontmatter: `type: requirement`, `status: pending`, `created_at: <iso-now>`
  - title: a short summary (first line of the requirement text)
  - body: the full requirement text
- Move the inbox file:
  ```bash
  mkdir -p .hive/inbox/processed
  mv .hive/inbox/<filename> .hive/inbox/processed/
  ```

**DO NOT also file a story drawer.** Stories are produced by the tech-lead in step 3 (next tick if needed).

### 2. Reap exited agents

For each subdirectory in `.hive/agents/`:

- Read `worker.pid`. Check if the process is alive: `kill -0 <pid> 2>/dev/null && echo alive || echo dead`. If `alive`, skip — agent still running.
- If `dead`, this is a reap candidate:
  - Find the agent-state drawer via `mempalace_list_drawers` (wing `hive`, room `agents`); locate the one with `title = agent-<id>`.
  - Note its `role`, `current_story` (workers), and `current_requirement` (tech-leads).

**Tech-lead reaping:**
- If the requirement drawer is now `status=decomposed` or `status=blocked`: tech-lead completed (success or clarification-needed). Update agent-state: `status=exited`, `exit_reason=<completed|escalated>`, `ended_at=<iso-now>`. (The tech-lead's skill should have already done this — only update if it didn't.)
- Best-effort cleanup: `rm -rf .hive/agents/<id>`.

**Worker reaping:**
- Find the worker's story drawer.
- **If `story.status` is `review` or `merged`:** worker completed successfully. Update agent-state via `mempalace_update_drawer`: `status=exited`, `exit_reason=completed`, `ended_at=<iso-now>`.
  - Then check the parent requirement: if ALL sibling stories (same `parent_requirement`) are `merged`, update the requirement to `status=complete`. If at least one sibling is `review`/`merged` AND the requirement is still `decomposed`, update to `status=in-flight`.
- **If `story.status` is `assigned`:** worker abandoned the story. Recovery:
  - Read `story.retry_count` (treat missing as 0).
  - **If `retry_count < 3`:** update story via `mempalace_update_drawer`: `status=pending`, `assigned_to=null`, `retry_count=<n+1>`. Update agent-state: `status=exited`, `exit_reason=abandoned`, `ended_at=<iso-now>`.
  - **If `retry_count >= 3`:** update story: `status=blocked`, `retry_count=<n+1>`. File an `escalation` drawer (room=`escalations`, type=`escalation`, story=<title>, status=`open`, escalated_at=<iso-now>). Update agent-state: `status=exited`, `exit_reason=abandoned`.
- Best-effort worktree cleanup: `git -C repos/<team> worktree remove ../<team>--<role>-<id> --force` (ignore failure).
- Remove the directory: `rm -rf .hive/agents/<id>`.

### 3. Spawn ONE subprocess

Compute live-agent count: count `.hive/agents/<id>/worker.pid` files where `kill -0 <pid>` returns 0.

**If live count > 0, SKIP this step entirely.** (Single-subprocess invariant.)

Otherwise, decide what to spawn:

#### 3a. Spawn priority 1 — tech-lead

`mempalace_list_drawers` wing=`hive` room=`requirements` → filter to drawers where `status` is `pending` (not `decomposed`, `in-flight`, `complete`, or `blocked`).

If at least one such requirement exists:

- Pick the oldest by `created_at`.
- Generate agent ID: `openssl rand -hex 4`.
- Create `.hive/agents/<id>/`:
  ```bash
  mkdir -p .hive/agents/<id>
  date +%s > .hive/agents/<id>/started_at
  ```
- Write `.hive/agents/<id>/context.md` (via Bash heredoc):
  ```markdown
  # Agent context — agent-<id>

  - **Your agent ID:** <id>
  - **Your role:** tech-lead
  - **Team:** <team name from .hive/config.yaml first team>
  - **Requirement drawer ID:** <id>
  - **Requirement title:** <title>

  ## Requirement (verbatim)

  <requirement body>

  ## What to do

  Invoke the hive-v2-tech-lead skill and follow it exactly. Decompose this
  requirement into N sized story drawers with depends_on + acceptance_criteria.
  Update the requirement drawer to status=decomposed (or status=blocked if too
  vague to decompose). Exit.
  ```
- Spawn (tech-leads have NO worktree — they spawn from workspace root):
  ```bash
  nohup claude --print --permission-mode acceptEdits \
    --system-prompt-file "$(pwd)/.claude/skills/tech-lead.md" \
    "You are agent <id>. Read .hive/agents/<id>/context.md and decompose the requirement." \
    > /dev/null 2>&1 &
  WORKER_PID=$!
  echo $WORKER_PID > .hive/agents/<id>/worker.pid
  ```
- File `agent-state` drawer via `mempalace_add_drawer`:
  - wing: `hive`, room: `agents`
  - frontmatter: `type: agent-state`, `status: live`, `role: tech-lead`, `team: <team>`, `current_requirement: <requirement title>`, `worktree: null`, `pid: <WORKER_PID>`, `started_at: <iso-now>`
  - title: `agent-<id>`
- Done. Skip to step 4 (diary).

#### 3b. Spawn priority 2 — worker (only if no pending requirement was spawned above)

`mempalace_list_drawers` wing=`hive` room=`stories` → all stories.

Build a "ready" set: each story is ready iff:
- `status == pending`
- For each title in `depends_on`: the matching story drawer has `status == merged`
- Empty `depends_on` array counts as all deps satisfied

If no ready story exists, NOTHING to spawn this tick. Skip to step 4.

Otherwise, pick the oldest ready story by `created_at`. Determine role from `story.points`:

| points | role |
|---|---|
| 1, 2, 3 | `junior` |
| 5 | `intermediate` |
| 8, 13 | `senior` |
| (anything else) | log a finding (`kind: bug`, "tech-lead emitted non-Fibonacci points value"); skip this story |

Then:

- Generate agent ID: `openssl rand -hex 4`.
- Create the worktree:
  ```bash
  git -C repos/<team> worktree add ../<team>--<role>-<id> -b agent/<team>--<role>-<id>
  ```
- Create `.hive/agents/<id>/`:
  ```bash
  mkdir -p .hive/agents/<id>
  date +%s > .hive/agents/<id>/started_at
  ```
- Write `.hive/agents/<id>/context.md`:
  ```markdown
  # Agent context — agent-<id>

  - **Your agent ID:** <id>
  - **Your role:** <role>
  - **Team:** <team>
  - **Worktree:** repos/<team>--<role>-<id>
  - **Branch:** agent/<team>--<role>-<id>
  - **Story drawer ID:** <id>
  - **Parent requirement:** <parent_requirement>

  ## Story

  <story title>

  <story body>

  ## Acceptance criteria

  <bullet list of story.acceptance_criteria — your contract>

  ## What to do

  Invoke the hive-v2-<role> skill and follow it exactly. cd into your worktree
  first. Read this file. Implement the story. Self-check every acceptance
  criterion BEFORE committing. Commit, push, open a PR, file your outcome to
  mempalace, exit.
  ```
- Spawn from workspace root:
  ```bash
  nohup claude --print --permission-mode acceptEdits \
    --system-prompt-file "$(pwd)/.claude/skills/<role>.md" \
    "You are agent <id>. Worktree: repos/<team>--<role>-<id>. Read .hive/agents/<id>/context.md and begin." \
    > /dev/null 2>&1 &
  WORKER_PID=$!
  echo $WORKER_PID > .hive/agents/<id>/worker.pid
  ```
- File `agent-state` drawer:
  - frontmatter: `type: agent-state`, `status: live`, `role: <role>`, `team: <team>`, `current_story: <title>`, `worktree: repos/<team>--<role>-<id>`, `pid: <WORKER_PID>`, `started_at: <iso-now>`
- Update story drawer via `mempalace_update_drawer`: `status=assigned`, `assigned_to=<id>`.

### 4. Write a diary entry

Use `mempalace_diary_write` with content:

```
manager  tick-end  spawned=<role|none> reaped=<n> live=<n> pending_reqs=<n> ready_stories=<n> waiting_stories=<n>
```

Where:
- `<role|none>` is whichever role you spawned (`tech-lead`, `junior`, `intermediate`, `senior`) or `none` if nothing
- `pending_reqs` = count of requirements with status=`pending` (not yet decomposed)
- `ready_stories` = count of stories ready to run (deps met, status=pending)
- `waiting_stories` = count of stories pending but NOT ready (unmet deps)

## What success looks like (example: 3-story decomposition mid-flight)

Manager wakes up. Inbox has 1 file. mempalace has 1 requirement (status=`decomposed` from previous tick), 3 stories (one is `merged`, two are `pending`, one of those has its only dep — the merged one).

1. `Read` `.hive/inbox/req-...txt` → file 2nd requirement drawer (`status=pending`)
2. `Bash` `mv .hive/inbox/req-... .hive/inbox/processed/`
3. `mempalace_list_drawers` agents → 0 live
4. `mempalace_list_drawers` stories → see 3 stories; merged one has no children to reap
5. (Reap loop has nothing dead to clean — skip)
6. Live count = 0
7. Step 3a: any undecomposed requirement? YES (the new one just drained). Spawn tech-lead on it.
8. `mempalace_add_drawer` agent-state, role=tech-lead
9. `mempalace_diary_write` `manager  tick-end  spawned=tech-lead reaped=0 live=1 pending_reqs=1 ready_stories=1 waiting_stories=1`

Exit. (The 2nd requirement gets decomposed by the tech-lead. The story that was ready in mempalace will be spawned on a NEXT tick when no agent is live.)

## After-tick checklist

Before you exit, answer:

- Did I move every drained inbox file to `processed/`?
- For each drained inbox file, did I file a `requirement` drawer (and NOT also a story drawer)?
- Did I reap every dead agent and properly transition its story (re-pend or escalate) and parent requirement (decomposed → in-flight, in-flight → complete)?
- If I spawned a tech-lead, did I confirm no live agent existed first?
- If I spawned a worker, did I confirm:
  - No live agent existed
  - No tech-lead was spawned earlier in this same tick
  - The story's `depends_on` are ALL `merged`
  - I picked the correct role for `story.points` (1-3 → junior, 5 → intermediate, 8/13 → senior)?
- Did I write a diary entry with all 6 fields?

If any answer is "no" without a clear reason, go back and complete it.

## When something is wrong

If you cannot proceed (no config, mempalace unreachable, missing team repo): file a `finding` drawer via `mempalace_add_drawer` describing the problem (kind=`infrastructure-error`), write a diary entry `manager  tick-error  reason=<short>`, and exit. Do not try to fix infrastructure yourself.
