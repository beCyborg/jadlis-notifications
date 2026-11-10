package notifier

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Ghostty + plain tmux click-to-focus.
//
// Layout this supports: every Ghostty tab runs its own tmux client attached to
// its own tmux session. On click we must raise the Ghostty tab whose tmux client
// shows the pane that sent the notification. We never run switch-client: that
// would retarget whatever client is current instead of raising the right tab.
//
// Resolution happens at click time (clients can re-attach between the hook and
// the click): pane -> session -> attached client ttys -> Ghostty terminal with
// that tty (Ghostty AppleScript exposes `tty` per terminal) -> focus it.

const ghosttyTmuxFocusCommand = "focus-tmux-ghostty"

// ghosttyClientTermname is the TERM a Ghostty tmux client reports.
const ghosttyClientTermname = "xterm-ghostty"

// notifierExecutablePath returns the absolute, symlink-resolved path of the
// running binary. Overridable in tests.
var notifierExecutablePath = func() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// buildGhosttyTmuxNotifierArgs constructs terminal-notifier arguments for
// Ghostty + tmux. The click runs the binary's focus-tmux-ghostty subcommand,
// which raises the exact Ghostty tab and activates Ghostty itself.
func buildGhosttyTmuxNotifierArgs(title, message, paneTarget, bundleID string) ([]string, error) {
	if !strings.HasPrefix(paneTarget, "%") {
		return nil, fmt.Errorf("invalid tmux pane target: %q", paneTarget)
	}
	exe, err := notifierExecutablePath()
	if err != nil || exe == "" {
		return nil, fmt.Errorf("cannot resolve notifier executable: %v", err)
	}

	cmd := shellQuote(exe) + " " + ghosttyTmuxFocusCommand +
		" --pane " + shellQuote(paneTarget) +
		" --tmux-path " + shellQuote(getTmuxPath())
	if socketPath := getTmuxSocketPath(); socketPath != "" {
		cmd += " --socket " + shellQuote(socketPath)
	}

	return []string{
		"-title", title,
		"-message", message,
		"-execute", cmd,
		"-group", fmt.Sprintf("claude-notif-%d", time.Now().UnixNano()),
	}, nil
}

// GhosttyTmuxTarget identifies a tmux pane on a given tmux server.
type GhosttyTmuxTarget struct {
	Pane     string
	TmuxPath string
	Socket   string
}

// tmuxCommandRunner runs one tmux command against the target's server and
// returns stdout. Overridable in tests.
type tmuxCommandRunner func(args ...string) (string, error)

const tmuxClickCommandTimeout = 2 * time.Second

func newTmuxCommandRunner(target GhosttyTmuxTarget) tmuxCommandRunner {
	tmuxPath := target.TmuxPath
	if tmuxPath == "" {
		tmuxPath = getTmuxPath()
	}
	return func(args ...string) (string, error) {
		full := args
		if target.Socket != "" {
			full = append([]string{"-S", target.Socket}, args...)
		}
		ctx, cancel := context.WithTimeout(context.Background(), tmuxClickCommandTimeout)
		defer cancel()
		out, err := exec.CommandContext(ctx, tmuxPath, full...).Output()
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("tmux %s timed out", strings.Join(args, " "))
		}
		if err != nil {
			return "", fmt.Errorf("tmux %s: %w", strings.Join(args, " "), err)
		}
		return string(out), nil
	}
}

// resolveTmuxPaneClientTTYs returns the session that owns pane and the ttys of
// the clients attached to it. Ghostty clients come first; other clients are
// kept as candidates because a user may override TERM inside Ghostty.
func resolveTmuxPaneClientTTYs(run tmuxCommandRunner, pane string) (string, []string, error) {
	if !strings.HasPrefix(pane, "%") {
		return "", nil, fmt.Errorf("invalid tmux pane target: %q", pane)
	}
	out, err := run("display-message", "-p", "-t", pane, "#{session_name}")
	if err != nil {
		return "", nil, err
	}
	session := strings.TrimSpace(out)
	if session == "" {
		return "", nil, fmt.Errorf("pane %s has no session (pane gone?)", pane)
	}

	// "=" makes tmux match the session name exactly, not as a prefix.
	// A space separates the fields: tmux prints control characters such as a
	// tab as "_" in format output, and a tty path has no spaces.
	out, err = run("list-clients", "-t", "="+session, "-F", "#{client_tty} #{client_termname}")
	if err != nil {
		return session, nil, err
	}

	var ghostty, others []string
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		tty, termname, _ := strings.Cut(line, " ")
		tty = strings.TrimSpace(tty)
		if !strings.HasPrefix(tty, "/dev/") || seen[tty] {
			continue
		}
		seen[tty] = true
		if strings.TrimSpace(termname) == ghosttyClientTermname {
			ghostty = append(ghostty, tty)
		} else {
			others = append(others, tty)
		}
	}
	ttys := append(ghostty, others...)
	if len(ttys) == 0 {
		return session, nil, fmt.Errorf("no tmux client attached to session %q", session)
	}
	return session, ttys, nil
}

// selectTmuxPaneInOwnSession makes pane the current window and pane of its own
// session. It changes no client's session: only the client(s) already showing
// that session see the change.
func selectTmuxPaneInOwnSession(run tmuxCommandRunner, pane string) error {
	if _, err := run("select-window", "-t", pane); err != nil {
		return err
	}
	_, err := run("select-pane", "-t", pane)
	return err
}
