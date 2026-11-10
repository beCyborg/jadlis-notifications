//go:build darwin

package notifier

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/beCyborg/jadlis-notifications/internal/logging"
)

const ghosttyTTYFocusTimeout = 4 * time.Second

// ghosttyTTYFocusRunner focuses the Ghostty terminal whose tty is one of ttys
// and returns the matched tty. Overridable in tests.
var ghosttyTTYFocusRunner = runGhosttyFocusByTTY

// ghosttyAppActivator brings Ghostty to the front without choosing a tab.
// Overridable in tests.
var ghosttyAppActivator = func() error {
	return exec.Command("/usr/bin/open", "-b", "com.mitchellh.ghostty").Run()
}

// FocusGhosttyTmuxPane raises the Ghostty tab whose tmux client shows target's
// pane. On any failure it still activates Ghostty, so the click is never a
// no-op, and returns the reason.
func FocusGhosttyTmuxPane(target GhosttyTmuxTarget) error {
	return focusGhosttyTmuxPane(newTmuxCommandRunner(target), target.Pane, ghosttyTTYFocusRunner, ghosttyAppActivator)
}

func focusGhosttyTmuxPane(
	run tmuxCommandRunner,
	pane string,
	focusTTY func([]string) (string, error),
	activate func() error,
) error {
	session, ttys, err := resolveTmuxPaneClientTTYs(run, pane)
	if err != nil {
		_ = activate()
		return fmt.Errorf("resolve tmux clients for %s: %w", pane, err)
	}

	// Show the pane inside its own session first, so the tab is already right
	// when it comes to the front. Failure here is not fatal.
	if err := selectTmuxPaneInOwnSession(run, pane); err != nil {
		logging.Debug("focus-tmux-ghostty: select %s in %s failed: %v", pane, session, err)
	}

	tty, err := focusTTY(ttys)
	if err != nil {
		_ = activate()
		return fmt.Errorf("no Ghostty tab for session %q (client ttys %v): %w", session, ttys, err)
	}
	logging.Debug("focus-tmux-ghostty: pane %s session %q -> Ghostty tab on %s", pane, session, tty)
	return nil
}

func runGhosttyFocusByTTY(ttys []string) (string, error) {
	if len(ttys) == 0 {
		return "", fmt.Errorf("no ttys")
	}
	ctx, cancel := context.WithTimeout(context.Background(), ghosttyTTYFocusTimeout)
	defer cancel()

	args := append([]string{"-e", ghosttyFocusByTTYAppleScript, "--"}, ttys...)
	output, err := exec.CommandContext(ctx, "/usr/bin/osascript", args...).CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("Ghostty tty AppleScript timed out after %s", ghosttyTTYFocusTimeout)
	}
	text := strings.TrimSpace(string(output))
	if err != nil {
		if text == "" {
			return "", fmt.Errorf("Ghostty tty AppleScript failed: %w", err)
		}
		return "", fmt.Errorf("Ghostty tty AppleScript failed: %w: %s", err, text)
	}
	return text, nil
}

// ghosttyFocusByTTYAppleScript walks argv in order (Ghostty clients first) and
// focuses the first Ghostty terminal whose tty matches, then activates its
// window so Ghostty comes to the front with that tab selected.
const ghosttyFocusByTTYAppleScript = `
on run argv
	if (count of argv) is 0 then
		error "missing tty" number 1002
	end if
	tell application "Ghostty"
		set allTerms to terminals
		repeat with wanted in argv
			repeat with t in allTerms
				try
					if (tty of t as string) is (wanted as string) then
						focus t
						activate
						return wanted as string
					end if
				end try
			end repeat
		end repeat
	end tell
	error "no Ghostty terminal with these ttys" number 1002
end run
`
