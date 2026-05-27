# Hive v2

> Ground-up rewrite of [hungry-ghost-hive](https://github.com/nikrich/hungry-ghost-hive).
> A ~700-LOC Go binary that supervises Claude Code subprocesses.
> All orchestration intelligence lives in skills + mempalace, not in code.

**Status:** Phase 0 (architecture design) complete. Phase 1 (minimal foundation) in planning.

See the full architecture in [`docs/specs/2026-05-27-hive-v2-architecture-design.md`](docs/specs/2026-05-27-hive-v2-architecture-design.md).

## The thesis

> Intelligence lives in Claude + skills, not in code.

- Single Go binary, no Node/npm runtime
- `claude` subprocess per worker, one git worktree each
- [mempalace](https://github.com/...) as the only storage
- New failure modes are handled by editing a skill, not shipping code
- Runs unattended for days; tiny watchdog keeps the manager loop alive

## Install

Not yet — under active development. When Phase 1 ships:

```sh
go install github.com/nikrich/hungry-ghost-hive-v2/cmd/hive@latest
```

## License

MIT (see [LICENSE](LICENSE)).
