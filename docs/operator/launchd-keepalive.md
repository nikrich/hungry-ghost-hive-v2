# Running hive unattended via launchd (macOS)

For multi-day runs you want the watchdog itself restarted if it crashes (OOM, panic, signal). On macOS, `launchd` is the right tool. This guide walks you through a one-time setup.

## The plist

Customize the three placeholders below (`<WORKSPACE>`, `<HIVE_BINARY>`, and the `WORKSPACE_LABEL`) and save as `~/Library/LaunchAgents/com.user.hive.<WORKSPACE_LABEL>.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.user.hive.<WORKSPACE_LABEL></string>

  <key>ProgramArguments</key>
  <array>
    <string><HIVE_BINARY></string>
    <string>run</string>
  </array>

  <key>WorkingDirectory</key>
  <string><WORKSPACE></string>

  <key>KeepAlive</key>
  <true/>

  <key>RunAtLoad</key>
  <true/>

  <key>ThrottleInterval</key>
  <integer>10</integer>

  <key>StandardOutPath</key>
  <string><WORKSPACE>/.hive/launchd.out.log</string>

  <key>StandardErrorPath</key>
  <string><WORKSPACE>/.hive/launchd.err.log</string>

  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>/Users/<YOU>/go/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin</string>
  </dict>
</dict>
</plist>
```

Key behaviors:
- `KeepAlive=true` — launchd respawns the process whenever it exits non-zero (or zero, by default).
- `ThrottleInterval=10` — minimum 10s between respawns. Prevents tight-looping if `hive run` crashes immediately at startup.
- `RunAtLoad=true` — starts immediately when loaded.
- `PATH` must include `gh`, `git`, `claude`, and `python3`. Adjust for your shell.

## Load / unload

```sh
# Load (starts immediately)
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.user.hive.<WORKSPACE_LABEL>.plist

# Check status
launchctl print gui/$(id -u)/com.user.hive.<WORKSPACE_LABEL>

# Unload (stop + remove)
launchctl bootout gui/$(id -u)/com.user.hive.<WORKSPACE_LABEL>.plist
```

## Operator checklist for multi-day runs

- [ ] `gh auth status` shows the correct account active, token not near expiry
- [ ] `claude` CLI on PATH; `~/.claude.json` has valid OAuth tokens
- [ ] Backlog queued: `hive add-req "..."` × N for the work you want done
- [ ] Plist loaded with `launchctl bootstrap`
- [ ] First tick fires within ~60s (check `.hive/watchdog.log` for `tick ok`)
- [ ] After 1 hour: `gh pr list --repo <repo> --state merged` shows growth
- [ ] After 24 hours: `du -sh .hive/memory` (chroma growth) and `ls .hive/log/*` (rotation working) look reasonable

## What still requires your attention

Even with launchd keepalive:
- **Conflicts**: when two stories touch the same file, QA refuses on merge. Story → `status=blocked`, escalation filed. Operator resolves by hand and either `git push` the fix + `hive merge` directly, or unblocks the drawer.
- **Gh / Claude token expiry**: workers fail silently. Watch the `findings` room for `infrastructure-error` kinds.
- **Hung mempalace gateway**: shouldn't happen but if it does, restart by killing the workspace's mempalace_gateway python process — claude will respawn it on next subprocess.
