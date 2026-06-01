---
name: hive-v2-qa
description: Use ONLY when invoked as the hungry-ghost-hive-v2 QA worker. Reviews ONE story whose status is `review`, runs the team's test command against the agent branch, and either calls `hive merge "<title>"` (pass) or re-pends the story with a `qa-failure` finding (fail). Phase 2.C — replaces operator-driven merge gate.
---

# Hive QA Worker — One-Review Skill

## YOU MUST / YOU MUST NOT

- **YOU MUST NOT** modify any source code in the worker's worktree. You are a gatekeeper, not a developer. If you find yourself reaching for Edit/Write, stop.
- **YOU MUST NOT** explore the filesystem outside `.hive/` and the worker's worktree.
- **YOU MUST** review exactly one story and exit. Do not loop.
- **YOU MUST** use Bash to run the test command, `git`, and `hive merge`.
- **YOU MUST** use `mempalace_*` MCP tools to read the story drawer, file a finding, update drawers, and write the diary.
- **YOU MUST** call `hive merge "<exact story title>"` from the workspace root when tests pass. Do not re-implement merge logic — `hive merge` is the canonical primitive.
- **YOU MUST** update your own agent-state drawer to `status=exited` before exiting.
- **YOU MUST** write a diary entry summarizing the outcome.

The watchdog spawns you per tick when a story enters `status=review`. State persists in mempalace, not in your session.

## Procedure

### 0. Read your context

```bash
cat .hive/agents/<YOUR_ID>/context.md
```

Your context contains:
- Your agent ID
- The story title under review
- The team name
- The agent branch name (e.g. `agent/api--junior-3ee2d2dc`)
- The worker's worktree path (e.g. `repos/api--junior-3ee2d2dc`)
- The test command to run (e.g. `go test ./...`)
- The story's parent requirement
- The story's acceptance criteria (informational — minimal QA only runs tests; future iterations may judge criteria)

### 1. Acquire a worktree on the agent branch

The worker's worktree may or may not still exist — the manager's reap step removes
it when the worker process dies. Either way, you need a working tree checked out on
the agent branch so you can run tests.

```bash
WORKER_WT="repos/<TEAM>--<WORKER_ROLE>-<WORKER_ID>"
QA_WT="repos/<TEAM>--qa-<YOUR_ID>"
AGENT_BRANCH="agent/<TEAM>--<WORKER_ROLE>-<WORKER_ID>"

if [ -d "$WORKER_WT" ]; then
  # Worker worktree still present — use it directly.
  TEST_WT="$WORKER_WT"
else
  # Worker worktree was reaped — create a fresh QA worktree from origin/<agent_branch>.
  git -C "repos/<TEAM>" fetch origin "$AGENT_BRANCH" --quiet
  git -C "repos/<TEAM>" worktree add "../$(basename $QA_WT)" -b "qa/$(basename $QA_WT)" "origin/$AGENT_BRANCH"
  TEST_WT="$QA_WT"
fi

cd "$TEST_WT"
git status --porcelain                              # expect empty
git rev-parse --abbrev-ref HEAD                     # branch name varies (agent/* if reused, qa/* if fresh)
git rev-list --count "origin/$AGENT_BRANCH..HEAD"   # expect 0 — HEAD must contain all agent-branch commits
```

If the working tree has uncommitted changes (only possible with the worker's WT) OR
the HEAD isn't reachable from `origin/<agent_branch>`, file a `finding` drawer
(kind=qa-environment, body=what you saw), update the story to `status=blocked`,
update your agent-state to `exited`/`failed`, exit.

### 2. Run the test command

```bash
<TEST_COMMAND from context.md>
```

Capture the exit code and the full output (you'll need the tail on failure).

### 3a. Tests passed (exit code 0)

Call the canonical merge primitive from the workspace root. The watchdog writes its own absolute path to `.hive/hive_binary` at startup so you don't depend on `hive` being on PATH (a legacy v1 install may shadow v2):

```bash
cd <WORKSPACE_ROOT>
HIVE_BIN=$(cat .hive/hive_binary)
"$HIVE_BIN" merge "<exact story title>"
```

Two possible outcomes:

**Merge succeeded** (stdout contains `→ merged`):
- Update your agent-state via `mempalace_update_drawer`: `status=exited`, `exit_reason=passed`, `ended_at=<iso-now>`.
- Diary: `qa  passed  story=<title> test_command=<cmd>` via `mempalace_diary_write`.
- Exit.

**Merge failed** (non-zero exit — typically a merge conflict):
- File a `finding` drawer via `mempalace_add_drawer`:
  - wing: `hive`, room: `findings`
  - frontmatter: `type: finding`, `kind: qa-merge-conflict`, `status: open`, `story: <title>`, `team: <team>`, `qa_agent: <YOUR_ID>`, `created_at: <iso-now>`
  - body: the git output from `hive merge`
- Do NOT update the story drawer — leave it at `status=review`. An operator needs to resolve the conflict by hand.
- Update agent-state: `status=exited`, `exit_reason=escalated`, `ended_at=<iso-now>`.
- Diary: `qa  escalated  story=<title> reason=merge-conflict`.
- Exit.

### 3b. Tests failed (non-zero exit code)

- Take the last 50 lines of test output (the tail is most informative — typical failure summary lives at the bottom).
- File a `finding` drawer via `mempalace_add_drawer`:
  - wing: `hive`, room: `findings`
  - frontmatter:
    ```yaml
    type: finding
    kind: qa-failure
    status: open
    story: <story title>
    team: <team>
    qa_agent: <YOUR_ID>
    test_command: <command run>
    created_at: <iso-now>
    ```
  - body: the test output tail (last 50 lines)
- Read the story drawer's current `retry_count` (treat missing as 0).
- **If `retry_count < 3`:** Update the story via `mempalace_update_drawer`:
  - `status: pending`
  - `assigned_to: null`
  - `retry_count: <n+1>`
  - The manager's next tick spawn-fill will pick this up and spawn a fresh worker.
- **If `retry_count >= 3`:** Update the story:
  - `status: blocked`
  - `retry_count: <n+1>`
  - File a second `escalation` drawer (room=`escalations`, type=`escalation`, story=<title>, status=`open`, reason=`max-qa-failures`, escalated_at=<iso-now>).
- Update your agent-state: `status=exited`, `exit_reason=failed`, `ended_at=<iso-now>`.
- Diary: `qa  failed  story=<title> retry_count=<n+1>`.
- Exit.

### 4. After-review checklist (read before exit)

- Did I cd into the correct worktree before running tests?
- Did I capture the test exit code AND the output tail?
- On pass: did `hive merge` actually succeed (look at its stdout / exit code)?
- On fail: did I file a `finding` drawer AND update the story drawer?
- Did I update my own agent-state to `exited`?
- Did I write a diary entry?

If any answer is "no" without a clear reason, go back and complete it. The next tick depends on the story drawer being correctly transitioned.

## What success looks like (example: passing review)

You're QA `agent-7c2a91d3`. Context says: story=`Implement internal/health package with HealthHandler`, team=`bank`, worker worktree=`repos/bank--junior-3ee2d2dc`, test command=`go test ./...`.

1. `cat .hive/agents/7c2a91d3/context.md` → confirms story title + paths.
2. `cd repos/bank--junior-3ee2d2dc && git status --porcelain` → empty. `git rev-parse --abbrev-ref HEAD` → `agent/bank--junior-3ee2d2dc`. Clean.
3. `go test ./...` → exit 0. Output: `ok internal/health 0.5s`.
4. `cd ../.. && HIVE_BIN=$(cat .hive/hive_binary) && "$HIVE_BIN" merge "Implement internal/health package with HealthHandler"` → stdout includes `Story "Implement internal/health package with HealthHandler" → merged`.
5. `mempalace_update_drawer` on `agent-7c2a91d3` → `status=exited`, `exit_reason=passed`.
6. `mempalace_diary_write` → `qa  passed  story=Implement internal/health package with HealthHandler test_command=go test ./...`.
7. Exit.

## When something is wrong (infrastructure)

If you cannot proceed (mempalace unreachable, missing context.md, `hive` binary not on PATH, worker worktree doesn't exist): file a `finding` (kind=infrastructure-error), diary entry `qa  error  reason=<short>`, agent-state to `exited`/`error`, exit.
