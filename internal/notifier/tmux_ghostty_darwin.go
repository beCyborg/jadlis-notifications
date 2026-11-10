//go:build darwin

package notifier

import (
	"context"
	"errors"
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

// ghosttyNewTabRunner opens a new Ghostty tab that runs command and brings it
// to the front. Overridable in tests.
var ghosttyNewTabRunner = runGhosttyNewTab

// errGhosttyTTYNotFound means Ghostty answered but has no tab on these ttys
// (the session is shown elsewhere, e.g. another terminal or ssh).
var errGhosttyTTYNotFound = errors.New("no Ghostty terminal with these ttys")

// FocusGhosttyTmuxPane raises the Ghostty tab whose tmux client shows target's
// pane. When the owner's tld window is open, the session is shown there and the
// window's tab is raised instead. When no Ghostty tab shows the session, it
// opens a new Ghostty tab attached to that session. On any other failure it still activates Ghostty,
// so the click is never a no-op, and returns the reason.
func FocusGhosttyTmuxPane(target GhosttyTmuxTarget) error {
	openTab := func(session string) error {
		return ghosttyNewTabRunner(ghosttyTmuxAttachCommand(target, session))
	}
	return focusGhosttyTmuxPane(newTmuxCommandRunner(target), target.Pane, ghosttyTTYFocusRunner, openTab, ghosttyAppActivator)
}

func focusGhosttyTmuxPane(
	run tmuxCommandRunner,
	pane string,
	focusTTY func([]string) (string, error),
	openTab func(session string) error,
	activate func() error,
) error {
	session, ttys, err := resolveTmuxPaneClientTTYs(run, pane)
	if err != nil && !(errors.Is(err, errNoTmuxClient) && session != "") {
		_ = activate()
		return fmt.Errorf("resolve tmux clients for %s: %w", pane, err)
	}

	// Show the pane inside its own session first, so the tab is already right
	// when it comes to the front. Failure here is not fatal.
	if err := selectTmuxPaneInOwnSession(run, pane); err != nil {
		logging.Debug("focus-tmux-ghostty: select %s in %s failed: %v", pane, session, err)
	}

	// An open tld window is the owner's cockpit: show the session there and
	// raise the tab that hosts the window, instead of a tab per session.
	if tty, ok := tldSessionOpener(session); ok {
		if _, err := focusTTY([]string{tty}); err != nil {
			// The window shows the session, but its terminal is not a Ghostty
			// tab we can raise: bring Ghostty forward, open nothing new.
			logging.Debug("focus-tmux-ghostty: tld window shows %q on %s, tab not raised: %v", session, tty, err)
			_ = activate()
			return nil
		}
		logging.Debug("focus-tmux-ghostty: pane %s session %q -> tld window on %s", pane, session, tty)
		return nil
	}

	if len(ttys) > 0 {
		tty, err := focusTTY(ttys)
		if err == nil {
			logging.Debug("focus-tmux-ghostty: pane %s session %q -> Ghostty tab on %s", pane, session, tty)
			return nil
		}
		if !errors.Is(err, errGhosttyTTYNotFound) {
			_ = activate()
			return fmt.Errorf("focus Ghostty tab for session %q (client ttys %v): %w", session, ttys, err)
		}
	}

	if err := openTab(session); err != nil {
		_ = activate()
		return fmt.Errorf("open Ghostty tab for session %q: %w", session, err)
	}
	logging.Debug("focus-tmux-ghostty: pane %s session %q had no Ghostty tab -> opened a new tab attached to it", pane, session)
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
		if strings.Contains(text, errGhosttyTTYNotFound.Error()) {
			return "", errGhosttyTTYNotFound
		}
		if text == "" {
			return "", fmt.Errorf("Ghostty tty AppleScript failed: %w", err)
		}
		return "", fmt.Errorf("Ghostty tty AppleScript failed: %w: %s", err, text)
	}
	return text, nil
}

const ghosttyNewTabTimeout = 4 * time.Second

func runGhosttyNewTab(command string) error {
	ctx, cancel := context.WithTimeout(context.Background(), ghosttyNewTabTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, "/usr/bin/osascript", "-e", ghosttyNewTabAppleScript, "--", command).CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("Ghostty new-tab AppleScript timed out after %s", ghosttyNewTabTimeout)
	}
	if err != nil {
		return fmt.Errorf("Ghostty new-tab AppleScript failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// ghosttyNewTabAppleScript opens a tab running argv[1] in the front Ghostty
// window (a new window when none is open), selects it and brings Ghostty to
// the front.
const ghosttyNewTabAppleScript = `
on run argv
	if (count of argv) is 0 then
		error "missing command" number 1002
	end if
	tell application "Ghostty"
		set cfg to new surface configuration
		set command of cfg to (item 1 of argv)
		if (count of windows) is 0 then
			set w to new window with configuration cfg
		else
			set w to front window
			set t to new tab in w with configuration cfg
			select tab t
		end if
		activate window w
		activate
	end tell
end run
`

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
