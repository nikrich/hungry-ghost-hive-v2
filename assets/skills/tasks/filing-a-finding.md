---
name: hive-filing-a-finding
description: Use when a hive worker discovers durable knowledge worth keeping across runs — a bug pattern, a non-obvious gotcha, a useful trick, an architectural decision.
---

# Filing a Finding

Use sparingly. Findings are knowledge the *next* engineer (human or agent) would want to discover. Trivia, restatements of obvious code, and one-off task notes do **not** belong here.

## When to file

- A bug whose root cause was non-obvious (and the fix is surprising)
- A local-dev gotcha (e.g., "service X must be running on port Y or feature Z silently fails")
- A useful library or pattern discovery that future workers should know
- An architectural decision worth recording with rationale

## When NOT to file

- "I made the change requested" — that's a PR, not a finding
- "I learned that <library> has a function <X>" — that's just documentation
- Anything already in the codebase as a comment

## How to file

Use the `mempalace_remember` MCP tool with:

- **wing:** `hive-<workspace-slug>` (from `.hive/config.yaml`)
- **room:** `findings`
- **frontmatter:**
  ```yaml
  type: finding
  added_by: agent-<YOUR_ID>
  story: <STORY-ID from your context.md>
  tags: [<2-5 relevant tags>]
  ```
- **content:** A markdown body with:
  - **Title** (one short sentence stating the finding)
  - **Symptom** (what you observed)
  - **Root cause** (what was actually happening)
  - **Resolution** (what fixed it)
  - **Avoid** (what *not* to try next time)

Keep it under 400 words. Future you will thank present you for being terse.
