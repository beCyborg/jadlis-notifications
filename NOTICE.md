# NOTICE

`jadlis-notifications` is a fork of
[**claude-notifications-go**](https://github.com/777genius/claude-notifications-go) by **777genius**,
licensed under the **GNU General Public License v3.0**. This fork is distributed under the same
license; the full text is in [LICENSE](LICENSE).

Fork base: upstream tag `v1.41.0`.

## Changes made in this fork

| # | Change | Where |
|---|---|---|
| 1 | Notification subtitle is the Claude Code session title read from `custom-title.json`, instead of `branch · folder` | `internal/notifier/notifier.go` |
| 2 | The action-summary suffix (`📝 1 new ▶ 2 cmds ⏱ 41s`) is dropped from the notification body | `internal/hooks/hooks.go` |
| 3 | A Stop-hook gate that suppresses "task complete" while background agents or workflows are still pending | `bin/stop-gate.sh`, `bin/pending-bg-tasks.py`, `hooks/hooks.json` |
| 4 | Shipped default config: Russian status titles, sound off, no session label, iTerm2 click-to-focus, `teamMode: wait-all` | `config/config.json` |
| 5 | Plugin renamed to `jadlis-notifications`; Go module path, `REPO` in `bin/install.sh` and `bin/bootstrap.sh` point at this fork, and the installer pins downloads to this repository's `v<version>` release | `.claude-plugin/`, `go.mod`, `bin/` |
| 6 | The patched `darwin/arm64` binary is committed to `bin/` and published as a release asset | `bin/claude-notifications-darwin-arm64`, `.gitignore` |
| 7 | Upstream CI workflows replaced by a single validation workflow; upstream cross-promotion removed from the installers | `.github/workflows/`, `bin/` |

Changes 1 and 2 are also kept as a standalone patch that
[`tools/sync-upstream.sh`](tools/sync-upstream.sh) re-applies on top of a newer upstream tag.

## Unchanged upstream paths

Two filesystem paths are hardcoded in upstream Go code and were deliberately **not** renamed, so an
existing installation keeps working:

- user config override: `~/.claude/claude-notifications-go/config.json`
- iTerm2 Python API venv: `~/.claude/claude-notifications-go/iterm2-venv/`

The upstream README is preserved at [docs/UPSTREAM-README.md](docs/UPSTREAM-README.md), and the
upstream changelog at [CHANGELOG.md](CHANGELOG.md).
