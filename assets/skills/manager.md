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
- **YOU MUST** spawn AT MOST 1 concurrent tech-lead + UP TO `max_workers` concurrent workers across this tick. Read `max_workers` from `.hive/config.yaml` (default 3). Tech-lead and workers may spawn in the same tick.

The watchdog re-invokes you each tick. State persists in mempalace + the workspace filesystem, not in your session.

## Tick order (load-bearing — reordered in Phase 2.A)

```
1. Drain inbox     →  file requirement drawers (NO story drawers)
2. Reap            →  detect exited agents, re-pend/escalate abandoned, transition parent requirements
3a. Spawn tech-lead (if needed, slot free)
3b. Spawn-fill workers up to max_workers   →  loop: while live_workers < max_workers && ready stories
4. Diary entry
5. Write .hive/last-tick.json (idle-backoff signal — see step 5)
6. Exit
```

Reap moves BEFORE spawn so a just-finished agent frees its slot for the next role. Step 3a runs first but does NOT block step 3b — both can spawn in the same tick.

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

**QA reaping (Phase 2.H — script-QA agents have an additional outcome file):**
- If the agent-state drawer has `role: qa` AND `qa_kind: script`, this is a script-QA agent. Read `.hive/agents/<id>/qa-result`:
  - **`pass`**: tests passed and merge succeeded. The script already called `hive merge`, so the story drawer is at `status=merged`. Update agent-state: `status=exited`, `exit_reason=passed`, `ended_at=<iso-now>`. Continue to the normal worker-reap requirement-transition logic below (so the parent requirement flips to `complete` when its last story merges).
  - **`fail-test` / `fail-merge` / `fail-setup`**: the script could not auto-resolve. Update the story drawer via `mempalace_update_drawer`: keep `status=review`, set `script_qa_attempted=true` (NEW field — frontmatter addition). Next tick's step 3c will spawn an LLM-QA for this story instead of re-running the script. Update agent-state: `status=exited`, `exit_reason=script-<fail-test|fail-merge|fail-setup>`, `ended_at=<iso-now>`. **Do NOT delete `.hive/agents/<id>/` yet** — leave the qa-fail.log around so the LLM-QA's context.md can point at it for evidence. The next tick's reap pass will clean it up after the LLM-QA also exits.
  - **Missing qa-result file**: the script crashed before writing it. Treat as `fail-setup`.
- LLM-QA agents (no `qa_kind` field, or `qa_kind: llm`) follow the existing worker-reap path below (they update the story drawer themselves via the qa.md skill).

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

### 3. Spawn (tech-lead + workers + QA, in that order)

Compute the split live counts by walking `.hive/agents/<id>/`:

- For each subdirectory, read `worker.pid` and run `kill -0 <pid> 2>/dev/null` to check liveness.
- If alive, look up the agent-state drawer for `agent-<id>` (or read its `role` from the path/context if your drawer lookup is slow) to bucket it:
  - `live_tech_leads` — count of live agents with `role: tech-lead`
  - `live_workers` — count of live agents with `role` in {junior, intermediate, senior}
  - `live_qa` — count of live agents with `role: qa` (Phase 2.C)
  - For each live `qa` agent, also remember its `current_story` so step 3c can avoid double-spawn

Read `max_workers` AND `max_qa` from `.hive/config.yaml` (defaults: 3 and 2).

Continue regardless of counts — step 3a checks `live_tech_leads == 0`, step 3b loops on `live_workers < max_workers`, step 3c loops on `live_qa < max_qa` AND filters out stories already in QA.

#### 3a. Spawn tech-lead (at most one concurrent)

If `live_tech_leads > 0`, SKIP this sub-step entirely.

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
- Spawn (tech-leads have NO worktree — they spawn from workspace root). `--bare --disable-slash-commands` skips hooks, LSP, plugin sync, auto-memory, CLAUDE.md discovery, and skill catalog loading — the role is already provided via `--system-prompt-file` and MCP via `--mcp-config`:
  ```bash
  nohup claude --print --bare --disable-slash-commands --permission-mode acceptEdits \
    --mcp-config "$(pwd)/.claude/mcp.json" --strict-mcp-config \
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
- Increment `live_tech_leads` so step 3b's checks remain accurate within this tick.
- Continue to step 3b (workers may also spawn this same tick).

#### 3b. Spawn-fill workers up to max_workers (loop)

`mempalace_list_drawers` wing=`hive` room=`stories` → all stories. Compute the "ready" set: each story is ready iff:

- `status == pending`
- For each title in `depends_on`: the matching story drawer has `status == merged`
- Empty `depends_on` array counts as all deps satisfied

Sort the ready set by `created_at` ascending (oldest first). This produces topological order naturally — tech-leads emit stories in dependency order, so oldest-first picks the next runnable story consistently.

Now loop:

```
while live_workers < max_workers AND ready set is non-empty:
    1. Pop the oldest ready story.
    2. Determine role from story.points (table below). On non-Fibonacci, file a finding drawer
       (kind=bug, "tech-lead emitted non-Fibonacci points value") and skip this story (continue loop).
    3. Compute base branch:
         if story.feature_branch is set → base_branch = story.feature_branch
         else (legacy Phase 2.A story) → base_branch = main
    4. Generate agent ID: openssl rand -hex 4
    5. Create the worktree based on the feature branch:
         git -C repos/<team> fetch origin <base_branch> --quiet
         git -C repos/<team> worktree add ../<team>--<role>-<id> -b agent/<team>--<role>-<id> origin/<base_branch>
    6. Create .hive/agents/<id>/ with started_at
    7. Write context.md (template below — note the new "Base branch" line)
    8. Spawn the worker subprocess (recipe unchanged from Phase 2.A — see template below)
    9. File the agent-state drawer with current_story = story.title, worktree, pid, started_at
   10. Update the story drawer: status=assigned, assigned_to=<id>
   11. Increment live_workers
```

When the loop exits (either cap reached or no ready stories remain), continue to step 4 (diary).

**Role routing table:**

| points | role | model config key |
|---|---|---|
| 1, 2, 3 | `junior` | `model_for_junior` |
| 5 | `intermediate` | `model_for_intermediate` |
| 8, 13 | `senior` | `model_for_senior` |
| (anything else) | log a finding (`kind: bug`, "tech-lead emitted non-Fibonacci points value"); skip this story | — |

**Model routing (Phase 2.H):** Each role has its own model so trivial stories don't burn Opus tokens. Read the matching key from `.hive/config.yaml` and pass it to `claude --model` in the spawn below. Defaults: junior → `claude-haiku-4-5-20251001`, intermediate → `claude-sonnet-4-6`, senior → `claude-opus-4-7`. If the key is missing in the config, the loader has already applied these defaults — read whatever is there.

**Worktree creation** (note the explicit base branch — different from P2.A):

```bash
git -C repos/<team> fetch origin <base_branch> --quiet
git -C repos/<team> worktree add ../<team>--<role>-<id> -b agent/<team>--<role>-<id> origin/<base_branch>
```

**`context.md` template** (the new `Base branch` line tells the worker where its branch forked from):

```markdown
# Agent context — agent-<id>

- **Your agent ID:** <id>
- **Your role:** <role>
- **Team:** <team>
- **Worktree:** repos/<team>--<role>-<id>
- **Branch:** agent/<team>--<role>-<id>
- **Base branch:** <base_branch>
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

**Spawn (Phase 1.5: pinned MCP config so the mempalace gateway points at workspace-local memory; Phase 2.H: per-tier model so juniors run on Haiku; `--bare --disable-slash-commands` skips hooks, LSP, plugin sync, auto-memory, CLAUDE.md discovery, and skill catalog loading — the role is already provided via `--system-prompt-file`):**

`<model>` below = the value of the matching `model_for_*` config key (see model routing table above). Read it once from `.hive/config.yaml` at the start of step 3b and substitute per spawn.

```bash
nohup claude --print --model <model> --bare --disable-slash-commands --permission-mode acceptEdits \
  --mcp-config "$(pwd)/.claude/mcp.json" --strict-mcp-config \
  --system-prompt-file "$(pwd)/.claude/skills/<role>.md" \
  "You are agent <id>. Worktree: repos/<team>--<role>-<id>. Read .hive/agents/<id>/context.md and begin." \
  > /dev/null 2>&1 &
WORKER_PID=$!
echo $WORKER_PID > .hive/agents/<id>/worker.pid
```

**agent-state drawer:**

- frontmatter: `type: agent-state`, `status: live`, `role: <role>`, `team: <team>`, `current_story: <title>`, `worktree: repos/<team>--<role>-<id>`, `pid: <WORKER_PID>`, `started_at: <iso-now>`

**Update story drawer** via `mempalace_update_drawer`: `status=assigned`, `assigned_to=<id>`.

#### 3c. Spawn-fill QA up to max_qa (loop) — Phase 2.C

`mempalace_list_drawers` wing=`hive` room=`stories` → filter to drawers where `status == review`.

Sort the resulting set by `created_at` ascending (oldest review first — same fairness rule as 3b).

Now loop:

```
while live_qa < max_qa AND review set is non-empty:
    1. Pop the oldest review story.
    2. If any live QA agent has current_story == story.title → skip (already in QA), continue loop.
    3. Decide QA path (Phase 2.H):
         if story.script_qa_attempted == true → SPAWN LLM-QA (the existing claude --print qa.md path)
         else → SPAWN SCRIPT-QA (`hive qa-run` Go subcommand, no claude subprocess)
       Script-QA handles the happy path (tests pass + clean merge) for zero LLM tokens.
       On any failure it writes .hive/agents/<id>/qa-result=fail-* and the reap step
       below flips story.script_qa_attempted=true so the next tick spawns the LLM.
    4. Generate agent ID: openssl rand -hex 4
    5. Look up the worker who produced this story via story.assigned_to → expected worktree
       is repos/<team>--<role>-<assigned_to>. Look up role from the worker's agent-state drawer
       (room=agents, body contains "agent-<assigned_to>") so the worktree path is correct.
       (Workers run in junior/intermediate/senior, so role is one of those three.)
    6. Create .hive/agents/<id>/ with started_at
    7. Script-QA path: no context.md needed (the Go binary reads everything from drawers + config).
       LLM-QA path: write context.md (template below — includes story title, team, test_command,
       agent branch, worker worktree path, AND the path to the prior qa-fail.log so the LLM has
       evidence of what the script tripped on).
    8. Spawn the QA subprocess (one of the two recipes below, per step 3 decision)
    9. File agent-state drawer: role=qa, qa_kind=script|llm, status=live, current_story=<story title>,
       team=<team>, pid=<pid>, started_at=<iso-now>
   10. DO NOT modify the story drawer — QA owns that transition
   11. Increment live_qa AND record current_story for the no-double-spawn guard above
```

When the loop exits, continue to step 4 (diary).

**`context.md` template for QA spawn:**

```markdown
# Agent context — agent-<id>

- **Your agent ID:** <id>
- **Your role:** qa
- **Team:** <team>
- **Story under review:** <story title>
- **Test command:** <test_command from .hive/config.yaml teams[].test_command>
- **Worker's agent branch:** agent/<team>--<worker_role>-<assigned_to>
- **Worker's worktree:** repos/<team>--<worker_role>-<assigned_to>
- **Parent requirement:** <parent_requirement>

## Acceptance criteria (informational — minimal QA only runs the test command)

<bullet list of story.acceptance_criteria>

## What to do

Invoke the hive-v2-qa skill and follow it exactly. cd into the worker's worktree,
run the test command, then on pass call `$(cat <WORKSPACE_ROOT>/.hive/hive_binary) merge "<exact story title>"`;
on fail file a finding and flip the story back to status=pending with retry_count++.

Note: the watchdog writes the absolute hive binary path to `.hive/hive_binary` at startup
so you don't depend on `hive` being on PATH (a legacy v1 install may shadow v2).
```

**Script-QA spawn (Phase 2.H — happy path, zero claude tokens):**

```bash
HIVE_BIN=$(cat .hive/hive_binary)
nohup "$HIVE_BIN" qa-run --story "<exact story title>" --agent-id "<id>" \
  > .hive/agents/<id>/qa.log 2>&1 &
WORKER_PID=$!
echo $WORKER_PID > .hive/agents/<id>/worker.pid
```

**LLM-QA spawn (fallback after a script-QA failure — Phase 1.5 + 2.C: pinned MCP config, qa.md skill; Phase 2.H: `--bare --disable-slash-commands` cuts startup overhead — see worker spawn above for rationale):**

```bash
nohup claude --print --bare --disable-slash-commands --permission-mode acceptEdits \
  --mcp-config "$(pwd)/.claude/mcp.json" --strict-mcp-config \
  --system-prompt-file "$(pwd)/.claude/skills/qa.md" \
  "You are QA agent <id>. Story under review: <story title>. Prior script-QA failed — see .hive/agents/<prior_qa_id>/qa-fail.log. Read .hive/agents/<id>/context.md and begin." \
  > /dev/null 2>&1 &
WORKER_PID=$!
echo $WORKER_PID > .hive/agents/<id>/worker.pid
```

### 4. Write a diary entry

Use `mempalace_diary_write` with content:

```
manager  tick-end  spawned=<roles-list|none> reaped=<n> live_workers=<n>/<max> live_tech_leads=<n>/1 live_qa=<n>/<max_qa> pending_reqs=<n> ready_stories=<n> waiting_stories=<n> review_stories=<n>
```

Where:
- `<roles-list|none>` is a comma-joined list of every role spawned this tick (e.g. `tech-lead,junior,junior,qa`) — order = spawn order. `qa` may appear alongside workers. Use `none` only if NOTHING spawned this tick.
- `<max>` in `live_workers=N/<max>` is the value of `max_workers` you read from `.hive/config.yaml`.
- `live_tech_leads` is `0/1` or `1/1` — the slot is fixed at 1.
- `<max_qa>` in `live_qa=N/<max_qa>` is the value of `max_qa` (default 2) from `.hive/config.yaml` (Phase 2.C).
- `pending_reqs` = count of requirements with status=`pending` (not yet decomposed).
- `ready_stories` = count of stories ready to run (deps met, status=pending).
- `waiting_stories` = count of stories pending but NOT ready (unmet deps).
- `review_stories` = count of stories at status=`review` (Phase 2.C — these are the ones QA will spawn for).

### 5. Write the idle-backoff signal (`.hive/last-tick.json`)

After the diary entry, write a JSON snapshot of this tick to `.hive/last-tick.json`. The watchdog reads this to decide whether to extend the next sleep — if every field is zero the system is fully idle and the next tick can wait longer than the base `tick_interval_seconds`. Without this file the watchdog assumes non-idle and never backs off.

Use Bash heredoc:

```bash
cat > .hive/last-tick.json <<JSON
{
  "spawned": <total roles spawned this tick — 0 if none>,
  "pending_reqs": <count from diary>,
  "ready_stories": <count from diary>,
  "review_stories": <count from diary>,
  "live_workers": <count from diary, post-spawn>,
  "live_tech_leads": <count from diary, post-spawn>,
  "live_qa": <count from diary, post-spawn>,
  "ts": $(date +%s)
}
JSON
```

All integer fields. If something in step 1-4 failed and you're about to exit early, still write this file with whatever counts you have — the watchdog falls back to the base interval on a missing/corrupt file, which is safe but wastes a tick.

## What success looks like (example: spawn-fill mid-flight)

Manager wakes up. mempalace has 1 requirement (status=`decomposed`), 3 stories all with `depends_on: []` and `feature_branch: feature/healthz` (just emitted by a tech-lead in the previous tick), zero live agents. `max_workers=3`.

1. `Bash` `ls .hive/inbox/` → empty (skip drain).
2. `mempalace_list_drawers` agents → 0 live (skip reap loop body).
3. Compute split counts: `live_workers=0`, `live_tech_leads=0`.
4. Step 3a: `live_tech_leads == 0` AND a pending requirement exists? NO (it's `decomposed`). Skip 3a.
5. Step 3b loop:
   - Iter 1: ready=3, `live_workers=0/3`. Pop oldest story; points=2 → junior. Compute base=`feature/healthz`. Worktree from `origin/feature/healthz`. Spawn. `live_workers=1`.
   - Iter 2: ready=2, `live_workers=1/3`. Pop next; points=1 → junior. Spawn. `live_workers=2`.
   - Iter 3: ready=1, `live_workers=2/3`. Pop next; points=1 → junior. Spawn. `live_workers=3`.
   - Iter 4: `live_workers=3 == max`. Stop.
6. Diary: `manager  tick-end  spawned=junior,junior,junior reaped=0 live_workers=3/3 live_tech_leads=0/1 pending_reqs=0 ready_stories=0 waiting_stories=0`.

Exit. Next tick reaps as workers exit at `review`; operator runs `hive merge` on each; subsequent ticks transition the requirement to `complete`.

## After-tick checklist

Before you exit, answer:

- Did I move every drained inbox file to `processed/`?
- For each drained inbox file, did I file a `requirement` drawer (and NOT also a story drawer)?
- Did I reap every dead agent and properly transition its story (re-pend or escalate) and parent requirement (decomposed → in-flight, in-flight → complete)?
- Did I compute `live_workers` and `live_tech_leads` separately (not as one combined count)?
- If I spawned a tech-lead, did I confirm `live_tech_leads == 0` before doing so?
- If I spawned workers, for each one did I confirm:
  - `live_workers < max_workers` at the time of spawn (the loop counter is correct, but double-check)
  - The story's `depends_on` are ALL `merged`
  - I picked the correct role for `story.points` (1-3 → junior, 5 → intermediate, 8/13 → senior)
  - I passed `--model <model>` to claude using the matching `model_for_<role>` from `.hive/config.yaml` (Phase 2.H)
  - I based the worktree on `story.feature_branch` (or `main` for legacy P2.A stories without that field)?
- For each QA I spawned (Phase 2.C), did I confirm:
  - `live_qa < max_qa` at the time of spawn
  - No other live QA already had `current_story == this story.title`
  - The QA context.md included the team's `test_command` from `.hive/config.yaml`
- Did I write a diary entry with all 9 fields (`spawned`, `reaped`, `live_workers=N/max`, `live_tech_leads=N/1`, `live_qa=N/max_qa`, `pending_reqs`, `ready_stories`, `waiting_stories`, `review_stories`)?
- Did I write `.hive/last-tick.json` with the 7 integer counts and `ts`? (Required for the watchdog's idle-backoff logic — without it the watchdog never extends its sleep.)

If any answer is "no" without a clear reason, go back and complete it.

## When something is wrong

If you cannot proceed (no config, mempalace unreachable, missing team repo): file a `finding` drawer via `mempalace_add_drawer` describing the problem (kind=`infrastructure-error`), write a diary entry `manager  tick-error  reason=<short>`, and exit. Do not try to fix infrastructure yourself.
