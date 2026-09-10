#!/bin/sh
# stop-gate.sh — gate in front of the plugin's Stop hook.
#
# While background agents or workflows are still running in the session it
# swallows the "task complete" notification (intermediate Stop events).
# Once everything has finished it forwards the hook event to hook-wrapper.sh
# unchanged.
#
# Wired up in hooks/hooks.json (Stop section):
#   sh ${CLAUDE_PLUGIN_ROOT}/bin/stop-gate.sh handle-hook Stop
#
# Everything is resolved relative to this script's own directory, so the gate
# keeps working from any plugin cache path and survives plugin updates.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
COUNTER="$SCRIPT_DIR/pending-bg-tasks.py"

INPUT=$(cat)

PYTHON=""
for candidate in "${CLAUDE_NOTIFICATIONS_PYTHON:-}" /usr/bin/python3 python3; do
    [ -n "$candidate" ] || continue
    if command -v "$candidate" >/dev/null 2>&1; then
        PYTHON="$candidate"
        break
    fi
done

PENDING=0
if [ -n "$PYTHON" ] && [ -f "$COUNTER" ]; then
    PENDING=$(printf '%s' "$INPUT" | "$PYTHON" "$COUNTER" 2>/dev/null || echo 0)
fi
case "$PENDING" in ''|*[!0-9]*) PENDING=0 ;; esac

LOG="${TMPDIR:-/tmp}/jadlis-notifications-stop-gate.log"
if [ -f "$LOG" ] && [ "$(wc -c < "$LOG" 2>/dev/null || echo 0)" -gt 262144 ]; then
    : > "$LOG"
fi
DECISION=pass
[ "$PENDING" -gt 0 ] && DECISION=suppress
printf '%s pending=%s %s\n' "$(date '+%F %T')" "$PENDING" "$DECISION" >> "$LOG" 2>/dev/null || true

[ "$DECISION" = "suppress" ] && exit 0

# Plugin root: the env var Claude Code exports, else this script's parent dir.
PLUGIN_ROOT="${CLAUDE_PLUGIN_ROOT:-}"
if [ -z "$PLUGIN_ROOT" ] || [ ! -f "$PLUGIN_ROOT/bin/hook-wrapper.sh" ]; then
    PLUGIN_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
fi
if [ ! -f "$PLUGIN_ROOT/bin/hook-wrapper.sh" ]; then
    exit 0
fi

printf '%s' "$INPUT" | sh "$PLUGIN_ROOT/bin/hook-wrapper.sh" "$@"
exit 0
