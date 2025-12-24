package platform

import (
	"os/exec"
	"strings"
)

// GetGitBranch returns the current git branch name for the given directory.
// Returns empty string if not in a git repository or on error.
func GetGitBranch(cwd string) string {
	if cwd == "" {
		return ""
	}

	cmd := exec.Command("git", "-C", cwd, "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	branch := strings.TrimSpace(string(output))

	// "HEAD" is returned when in detached HEAD state
	if branch == "HEAD" {
		return ""
	}

	return branch
}
