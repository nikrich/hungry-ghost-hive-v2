# Operator skills

Claude Code skills for **operators** of hungry-ghost-hive v2 — humans (or other Claude sessions) who install, run, and feed hive.

These are distinct from the skills under `assets/skills/` — those are hive's INTERNAL roles (manager, tech-lead, junior/intermediate/senior, qa) that get embedded into the `hive` binary and loaded into hive's own subprocess invocations. Skills here run in an **operator's** Claude Code session, not inside hive.

## What's here

| Skill | Use when… |
|---|---|
| `hive-operator.md` | The user wants to install, configure, queue requirements for, monitor, or debug hungry-ghost-hive v2. |

## Install

Claude Code reads skills from `~/.claude/skills/`. Drop the skill file there:

```sh
mkdir -p ~/.claude/skills
cp operator-skills/hive-operator.md ~/.claude/skills/
```

That's it. The skill activates on relevant prompts (per its `description:` frontmatter — "install hive", "set up hive", "run hive", "queue a requirement", etc.) the next time you start a Claude Code session.

To verify it's loaded:

```
> /skills
```

`hive-operator` should appear in the list.

## Updating

When this repo's `hive-operator.md` changes, re-copy the latest:

```sh
cp operator-skills/hive-operator.md ~/.claude/skills/
```

(Or symlink it once: `ln -s "$(pwd)/operator-skills/hive-operator.md" ~/.claude/skills/hive-operator.md` — symlink survives `git pull`s.)

## Authoring more operator skills

Drop additional `*.md` files in this directory. Each follows the standard Claude Code skill frontmatter:

```markdown
---
name: <kebab-case-name>
description: Use when… <one sentence that helps Claude decide when to activate>
---

# Skill body
...
```

Keep them focused — one skill per operator workflow, not one mega-skill per tool. If hive grows debugging tooling, that's a separate `hive-debug` skill. If it grows a cost-reporting CLI, that's a separate `hive-cost` skill.
