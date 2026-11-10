//go:build !darwin

package notifier

import "fmt"

// FocusGhosttyTmuxPane is not supported on non-darwin platforms.
func FocusGhosttyTmuxPane(target GhosttyTmuxTarget) error {
	return fmt.Errorf("focus-tmux-ghostty not supported on this platform")
}
