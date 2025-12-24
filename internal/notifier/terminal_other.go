//go:build !darwin

package notifier

import "fmt"

// GetTerminalBundleID returns empty string on non-macOS platforms
// as terminal bundle IDs are a macOS-specific concept.
func GetTerminalBundleID(configOverride string) string {
	return ""
}

// GetTerminalNotifierPath returns an error on non-macOS platforms
// as terminal-notifier is macOS-only.
func GetTerminalNotifierPath() (string, error) {
	return "", fmt.Errorf("terminal-notifier is only available on macOS")
}

// IsTerminalNotifierAvailable returns false on non-macOS platforms.
func IsTerminalNotifierAvailable() bool {
	return false
}

// EnsureClaudeNotificationsApp is a no-op on non-macOS platforms.
func EnsureClaudeNotificationsApp() error {
	return nil
}
