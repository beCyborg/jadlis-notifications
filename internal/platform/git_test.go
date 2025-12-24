package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetGitBranch(t *testing.T) {
	tests := []struct {
		name     string
		cwd      string
		wantNone bool // if true, expect empty string
	}{
		{
			name:     "Empty cwd",
			cwd:      "",
			wantNone: true,
		},
		{
			name:     "Non-existent directory",
			cwd:      "/non/existent/path",
			wantNone: true,
		},
		{
			name:     "Temp directory (not a git repo)",
			cwd:      os.TempDir(),
			wantNone: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetGitBranch(tt.cwd)
			if tt.wantNone && result != "" {
				t.Errorf("GetGitBranch(%q) = %q, want empty string", tt.cwd, result)
			}
		})
	}
}

func TestGetGitBranch_RealRepo(t *testing.T) {
	// Find the project root (which should be a git repo)
	cwd, err := os.Getwd()
	if err != nil {
		t.Skip("Could not get working directory")
	}

	// Walk up to find .git directory
	for {
		if _, err := os.Stat(filepath.Join(cwd, ".git")); err == nil {
			break
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			t.Skip("Not running in a git repository")
		}
		cwd = parent
	}

	branch := GetGitBranch(cwd)
	if branch == "" {
		t.Error("Expected non-empty branch name for git repository")
	}

	t.Logf("Current branch: %s", branch)
}
