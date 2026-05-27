---
name: hive-creating-a-pr
description: Use when a hive worker needs to commit changes, push, and open a pull request via gh.
---

# Creating a PR

Use after your code changes are complete and tests pass.

## Steps

1. **Commit.** Use a conventional commit prefix matching the change type:
   - `feat:` for new functionality
   - `fix:` for bug fixes
   - `chore:`, `docs:`, `test:`, `refactor:` as appropriate

   Subject line under 70 chars. Body explains *why*, not *what* (the diff explains what).

   ```bash
   git add <specific-files>   # never `git add -A` — risk of committing junk
   git commit -m "$(cat <<'EOF'
   feat: <subject>

   <why this change matters, in 1-3 sentences>
   EOF
   )"
   ```

2. **Push** to the branch name in your context.md (already created by the manager as `agent/<team>--junior-<id>`):
   ```bash
   git push -u origin <branch-name>
   ```

3. **Open the PR with `gh`:**
   ```bash
   gh pr create --title "<feat/fix/etc>: <subject>" --body "$(cat <<'EOF'
   ## Summary
   - <1-3 bullets>

   ## How verified
   - <commands run, manual checks done>

   ## Linked story
   <STORY-ID from your context.md>
   EOF
   )"
   ```

   Capture the PR URL from `gh pr create` output — you'll need it for your outcome drawer.

## Safety

- Never use `--force` or `--no-verify`.
- Never amend an existing public commit.
- If `git push` is rejected (someone updated the branch), do `git pull --rebase` and try again. Do not force-push.
- If pre-commit hooks fail, fix the issue and commit again — do not bypass.
