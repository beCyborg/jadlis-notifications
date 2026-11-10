//go:build darwin

package notifier

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/beCyborg/jadlis-notifications/internal/logging"
)

// The owner's session window "tld" (~/.claude/scripts/tld.py): a list of
// Claude Code tmux sessions on the left and the session itself, live, on the
// right. Sessions he works with through that window have no Ghostty tab of
// their own, so the plain click path would open a new tab for every
// notification. When such a window is open, the click shows the session there
// and raises the Ghostty tab that hosts the window.
//
// Contract of `tld.py open <session>` (checked 2026-11-10): exit 0 and one
// stdout line with the tty of the terminal that shows the window; exit 1 with
// no output when no window is open; exit 2 when the session is unknown.

const tldOpenTimeout = 3 * time.Second

// tldSessionOpener asks an open tld window to show session and returns the tty
// of the terminal hosting that window. ok is false when there is no window, no
// script, or the call failed. Overridable in tests.
var tldSessionOpener = runTldOpen

func tldScriptPath() string {
	if p := os.Getenv("JADLIS_TLD_SCRIPT"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "scripts", "tld.py")
}

func runTldOpen(session string) (string, bool) {
	script := tldScriptPath()
	if script == "" || session == "" {
		return "", false
	}
	if _, err := os.Stat(script); err != nil {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), tldOpenTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, script, "open", session)
	// A child of the script that keeps stdout open must not hold the click
	// past the timeout.
	cmd.WaitDelay = 500 * time.Millisecond
	// The window keeps its state files in the per-user temp dir. A click handler
	// started by the notification helper may lack TMPDIR; without it the script
	// would look in /tmp and report "no window".
	cmd.Env = os.Environ()
	if os.Getenv("TMPDIR") == "" {
		if out, err := exec.Command("/usr/bin/getconf", "DARWIN_USER_TEMP_DIR").Output(); err == nil {
			if dir := strings.TrimSpace(string(out)); dir != "" {
				cmd.Env = append(cmd.Env, "TMPDIR="+dir)
			}
		}
	}
	out, err := cmd.Output()
	// ErrWaitDelay: the script exited 0 (the window already shows the session),
	// only a leftover child kept stdout open. Its answer still counts.
	if err != nil && !errors.Is(err, exec.ErrWaitDelay) {
		logging.Debug("focus-tmux-ghostty: tld open %q: %v", session, err)
		return "", false
	}
	tty := strings.TrimSpace(string(out))
	if !strings.HasPrefix(tty, "/dev/") {
		logging.Debug("focus-tmux-ghostty: tld open %q answered %q, not a tty", session, tty)
		return "", false
	}
	return tty, true
}
