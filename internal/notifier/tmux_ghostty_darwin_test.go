//go:build darwin

package notifier

import (
	"fmt"
	"reflect"
	"testing"
)

func TestFocusGhosttyTmuxPane_FocusesClientTTY(t *testing.T) {
	f := &fakeTmux{answers: map[string]string{
		"display-message": "tab-a\n",
		"list-clients":    "/dev/ttys004 xterm-ghostty\n",
	}}
	var gotTTYs []string
	activated := false
	err := focusGhosttyTmuxPane(f.run, "%9",
		func(ttys []string) (string, error) { gotTTYs = ttys; return ttys[0], nil },
		func() error { activated = true; return nil },
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(gotTTYs, []string{"/dev/ttys004"}) {
		t.Errorf("focused ttys = %v", gotTTYs)
	}
	if activated {
		t.Error("plain activation must not run when the exact tab was focused")
	}
	if !f.sawCommand("select-window") || !f.sawCommand("select-pane") {
		t.Errorf("pane must be selected inside its own session: %v", f.calls)
	}
	for _, forbidden := range []string{"switch-client", "detach-client", "kill-session"} {
		if f.sawCommand(forbidden) {
			t.Errorf("must never run %s: %v", forbidden, f.calls)
		}
	}
}

func TestFocusGhosttyTmuxPane_FallsBackToActivate(t *testing.T) {
	tests := []struct {
		name  string
		f     *fakeTmux
		focus func([]string) (string, error)
	}{
		{"no clients", &fakeTmux{answers: map[string]string{"display-message": "s\n"}},
			func([]string) (string, error) { t.Fatal("focus must not run"); return "", nil }},
		{"no Ghostty tab with that tty", &fakeTmux{answers: map[string]string{
			"display-message": "s\n", "list-clients": "/dev/ttys001 xterm-256color\n"}},
			func([]string) (string, error) { return "", fmt.Errorf("not found") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			activated := false
			err := focusGhosttyTmuxPane(tt.f.run, "%1", tt.focus, func() error { activated = true; return nil })
			if err == nil {
				t.Fatal("expected error")
			}
			if !activated {
				t.Error("Ghostty must still be activated so the click is not a no-op")
			}
			if tt.f.sawCommand("switch-client") {
				t.Error("must never switch-client")
			}
		})
	}
}

func TestMaybeCaptureGhosttyTerminalID_SkippedInsideTmux(t *testing.T) {
	t.Setenv("TMUX", "/private/tmp/tmux-501/default,1,0")
	old := ghosttyFrontmostTerminalInfoRunner
	called := false
	ghosttyFrontmostTerminalInfoRunner = func() (ghosttyFrontmostTerminalInfo, error) {
		called = true
		return ghosttyFrontmostTerminalInfo{}, fmt.Errorf("stub")
	}
	t.Cleanup(func() { ghosttyFrontmostTerminalInfoRunner = old })

	MaybeCaptureGhosttyTerminalID("com.mitchellh.ghostty", "sess", "/tmp")
	if called {
		t.Error("inside tmux the frontmost-terminal lookup must be skipped")
	}
}
