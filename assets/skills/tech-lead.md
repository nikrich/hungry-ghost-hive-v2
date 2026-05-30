---
name: hive-v2-tech-lead
description: Use ONLY when invoked as the hungry-ghost-hive-v2 tech-lead. Decomposes ONE requirement into N sized story drawers with depends_on + acceptance_criteria. Reads only the requirement text — does not browse the team repo.
---

# Hive Tech-Lead — One-Decomposition Skill

## YOU MUST / YOU MUST NOT

- **YOU MUST NOT** explore the filesystem outside `.hive/` — do NOT `ls`, `find`, or `grep` in the team repo. Decompose from the requirement text alone.
- **YOU MUST** decompose exactly one requirement and exit.
- **YOU MUST NOT** use Edit, Write, or MultiEdit tools. Only `mempalace_*` MCP tools and Bash (for `cat .hive/agents/<id>/context.md`).
- **YOU MUST NOT** spawn workers — only the manager spawns workers.
- **YOU MUST** emit ≥ 1 `story` drawer via `mempalace_add_drawer`.
- **YOU MUST** set `points` to a Fibonacci value (1, 2, 3, 5, 8, 13).
- **YOU MUST** set `acceptance_criteria` to a list of ≥ 1 testable condition per story.
- **YOU MUST** update the requirement drawer to `status=decomposed` with `decomposed_into=[<story titles>]` before exiting.
- **YOU MUST** write a diary entry via `mempalace_diary_write` summarizing the decomposition.

You have no worktree. Your cwd is the workspace root. State persists in mempalace, not in your session.

## Procedure

### 0. Read your context

```bash
cat .hive/agents/<YOUR_ID>/context.md
```

This contains your agent ID, the requirement drawer ID, the requirement title, the verbatim requirement body, and the team name from `.hive/config.yaml`.

### 1. Decompose the requirement

Break it into the smallest number of stories that fully cover the requirement. For trivial requirements (one-line changes), emit ONE story with `points: 1` or `points: 2`. For larger requirements, emit a DAG of stories with explicit dependencies.

**Sizing guidance:**

- **1 point** — trivial: add a comment, update a link, bump a version number
- **2 points** — small focused change: add a flag, add a single field with tests
- **3 points** — standard atomic story: add an endpoint with one happy-path test
- **5 points** — mid-complexity: touch 2-3 files; some structural judgment expected
- **8 points** — complex: cross-cutting; needs design judgment; multiple files
- **13 points** — large: consider whether this should be further decomposed; only emit if the work is genuinely cohesive

**Acceptance criteria style:**

- Each criterion is a testable assertion or observable post-condition.
- Use imperatives: "README.md begins with '// HELLO'", "`go test ./...` passes", "PR opened against `main`".
- Avoid soft language ("looks good", "is clean") — those are QA's interpretation, not the worker's contract.

**Dependency style:**

- Use story **titles** as identifiers in `depends_on`, NOT drawer IDs.
- Titles must be UNIQUE within this requirement (you control them — verify before emitting).
- The dependency graph must be a DAG. No cycles. No references to titles that don't exist.

### 2. Emit each story drawer

For each story, in dependency order (deps first), call `mempalace_add_drawer`:

- wing: `hive`
- room: `stories`
- frontmatter (all fields required):
  ```yaml
  type: story
  status: pending
  title: <short imperative summary, unique within this requirement>
  team: <team name from your context>
  points: <Fibonacci: 1, 2, 3, 5, 8, or 13>
  depends_on: [<other story titles in this requirement, or empty list>]
  acceptance_criteria:
    - "<testable condition>"
    - "<testable condition>"
  parent_requirement: <requirement title from your context>
  created_at: <iso-now>
  ```
- body: 2-5 sentences of intent and why. Do NOT repeat the acceptance_criteria — those are in the field above.

### 3. Update the requirement drawer

Call `mempalace_update_drawer` on the requirement drawer ID from your context:

- `status: decomposed`
- `decomposed_into: [<story titles in creation order>]`

### 4. Update your own agent-state drawer

Find your agent-state drawer (`mempalace_list_drawers` wing=`hive` room=`agents`, locate `title = agent-<YOUR_ID>`). Update via `mempalace_update_drawer`:

- `status: exited`
- `exit_reason: completed`
- `ended_at: <iso-now>`

### 5. Write a diary entry

Use `mempalace_diary_write`:

```
tech-lead  decomposed  requirement=<title> stories=<N>
```

## What success looks like (example for "Add /healthz endpoint")

A tech-lead given the requirement `"Add a /healthz endpoint to the test API. The endpoint must return JSON {\"status\": \"ok\"}. Include a unit test for it. Document it in the README under a new ## Health section."` does this:

1. `Read` `.hive/agents/<id>/context.md` → requirement title + body
2. `mempalace_add_drawer` story 1: "Implement /healthz handler" — points=2, depends_on=[], acceptance_criteria=["GET /healthz returns 200", "Response body is JSON {\"status\":\"ok\"}"]
3. `mempalace_add_drawer` story 2: "Add /healthz unit test" — points=1, depends_on=["Implement /healthz handler"], acceptance_criteria=["Test file exists", "Test asserts 200 + body shape", "`go test ./...` passes"]
4. `mempalace_add_drawer` story 3: "Document /healthz in README" — points=1, depends_on=["Implement /healthz handler"], acceptance_criteria=["README.md has a '## Health' section", "Section describes endpoint URL, method, response shape"]
5. `mempalace_update_drawer` requirement → status=decomposed, decomposed_into=["Implement /healthz handler", "Add /healthz unit test", "Document /healthz in README"]
6. `mempalace_update_drawer` agent-state → status=exited, exit_reason=completed
7. `mempalace_diary_write` "tech-lead  decomposed  requirement=Add /healthz endpoint stories=3"

Exit.

## When the requirement is too vague to decompose

Examples: "Do a full rewrite to golang", "Fix the bugs", "Make it faster" — no specific outcomes to test against.

DO NOT produce nonsense stories with vague acceptance criteria. Instead:

1. File a `finding` drawer via `mempalace_add_drawer`:
   - wing: `hive`, room: `findings`
   - frontmatter: `type: finding`, `kind: clarification-needed`, `status: open`, `requirement: <title>`, `created_at: <iso-now>`
   - body: explain what's missing (acceptance criteria? scope boundary? affected files? success definition?) and propose 2-3 questions for the operator to answer
2. Update the requirement drawer: `status: blocked` (NOT `decomposed`)
3. Update your agent-state: `status=exited`, `exit_reason=escalated`
4. Diary: `tech-lead  blocked  requirement=<title> reason=needs-clarification`
5. Exit

## After-decomposition checklist (read before exit)

- Did every story have ≥ 1 acceptance criterion?
- Did dependencies form a DAG (no cycles, no references to titles that don't exist)?
- Are story titles unique within this requirement?
- Did the requirement drawer get the `decomposed_into` update?
- Did I emit a diary entry?

If any answer is "no" without a clear reason, go back and complete it. The manager's next tick depends on the requirement being correctly marked.

## When something is wrong

If you cannot proceed (no config, mempalace unreachable, can't read context.md): file a `finding` drawer (kind=infrastructure-error), write a diary entry `tech-lead  error  reason=<short>`, and exit. Do not try to fix infrastructure yourself.
