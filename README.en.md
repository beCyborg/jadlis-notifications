English · [Русский](README.md)

# The task finished and you missed it

A macOS notification arrives when Claude Code is done, asks a question, or has a plan ready.
The subtitle is the session name, so across ten windows you can tell which one freed up.
Clicking the notification switches to the right iTerm2 tab.

This is a fork of [claude-notifications-go](https://github.com/777genius/claude-notifications-go)
(GPL-3.0) with four changes: subtitle = session name, no counter suffix, a background-task gate,
and Russian titles by default.

## What it is

A Claude Code plugin: five hooks (Stop, SubagentStop, PreToolUse, Notification, TeammateIdle)
call a Go binary that sends a native macOS notification.

Differences from upstream:

- **Subtitle is the real session name.** Upstream writes `branch · folder`. The fork reads Claude
  Code's `custom-title.json` and shows the name you see inside Claude Code itself.
- **No counter suffix.** Upstream appends `📝 1 new ▶ 2 cmds ⏱ 41s` to the body. The fork keeps
  only the summary text.
- **Background-task gate.** "Task complete" is held back while background agents or workflows are
  still running in the session.
- **Russian titles, sound off** — in the shipped default config.

## Install

```bash
claude plugin marketplace add https://github.com/beCyborg/jadlis-hub
claude plugin install jadlis-notifications@jadlis
```

Installing it separately is usually unnecessary: it comes in as a dependency of `jadlis-claudecode`.

The patched `darwin/arm64` binary is committed to the repository, so the plugin works right after
install. If the binary goes missing or its version drifts from `plugin.json`,
`bin/hook-wrapper.sh` downloads it from this fork's `v<version>` release — it never reaches
upstream.

## What you see

| Event | Title |
|---|---|
| Claude finished | ✅ Задача выполнена |
| Review finished | 🔍 Ревью завершено |
| A question was asked | ❓ Есть вопрос |
| A plan is ready | 📋 План готов |

The subtitle is the session name. Sound is off, the session label in the title is off, and a click
raises iTerm2 and the right tab.

Your own values go into `~/.claude/claude-notifications-go/config.json` — that path is hardcoded in
the upstream code and was deliberately left alone in the fork, so an existing config is picked up
as is. That file wins over the plugin's own `config/config.json`.

## Background-task gate

`bin/stop-gate.sh` sits in front of the Stop hook. It counts unfinished background agents and
workflows from the session transcript (`bin/pending-bg-tasks.py`); while that count is above zero
the notification is suppressed, otherwise the event is forwarded to `bin/hook-wrapper.sh`
unchanged. Any counter error means zero, i.e. the notification goes through. A launch older than
6 hours with no completion notice is treated as dead.

The decision log is `${TMPDIR}/jadlis-notifications-stop-gate.log`, truncated at 256 KB.

## Click-to-focus in iTerm2

Hitting the exact tab needs the iTerm2 Python API. One-time, by hand:

1. iTerm2 → Settings → General → Magic → **Enable Python API**, then restart iTerm2.
2. `bin/install.sh` creates the venv on first install; manually it is
   `python3 -m venv ~/.claude/claude-notifications-go/iterm2-venv` plus
   `~/.claude/claude-notifications-go/iterm2-venv/bin/pip install iterm2`.

That venv path is hardcoded upstream too and was left alone. Without the Python API a click just
raises iTerm2 without picking a tab.

## License

GPL-3.0, same as upstream. The original is
[777genius/claude-notifications-go](https://github.com/777genius/claude-notifications-go) by
777genius. What exactly was changed is in [NOTICE.md](NOTICE.md); the upstream README is kept at
[docs/UPSTREAM-README.md](docs/UPSTREAM-README.md).
