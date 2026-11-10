//go:build darwin

package notifier

import (
	"fmt"
	"reflect"
	"testing"
)

type focusSpy struct {
	focusedTTYs []string
	focusErr    error
	openedFor   []string
	openErr     error
	activated   bool
}

func (s *focusSpy) focus(ttys []string) (string, error) {
	s.focusedTTYs = ttys
	if s.focusErr != nil {
		return "", s.focusErr
	}
	return ttys[0], nil
}
func (s *focusSpy) open(session string) error {
	s.openedFor = append(s.openedFor, session)
	return s.openErr
}
func (s *focusSpy) activate() error { s.activated = true; return nil }

func assertNoClientCommands(t *testing.T, f *fakeTmux) {
	t.Helper()
	for _, forbidden := range []string{"switch-client", "detach-client", "kill-session", "kill-server"} {
		if f.sawCommand(forbidden) {
			t.Errorf("must never run %s: %v", forbidden, f.calls)
		}
	}
}

func TestFocusGhosttyTmuxPane_FocusesClientTTY(t *testing.T) {
	f := &fakeTmux{answers: map[string]string{
		"display-message": "tab-a\n",
		"list-clients":    "/dev/ttys004 xterm-ghostty\n",
	}}
	spy := &focusSpy{}
	if err := focusGhosttyTmuxPane(f.run, "%9", spy.focus, spy.open, spy.activate); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(spy.focusedTTYs, []string{"/dev/ttys004"}) {
		t.Errorf("focused ttys = %v", spy.focusedTTYs)
	}
	if spy.activated || len(spy.openedFor) > 0 {
		t.Error("no fallback must run when the exact tab was focused")
	}
	if !f.sawCommand("select-window") || !f.sawCommand("select-pane") {
		t.Errorf("pane must be selected inside its own session: %v", f.calls)
	}
	assertNoClientCommands(t, f)
}

func TestFocusGhosttyTmuxPane_OpensTabWhenSessionHasNoClient(t *testing.T) {
	f := &fakeTmux{answers: map[string]string{"display-message": "jadlis_J-NT_13\n", "list-clients": ""}}
	spy := &focusSpy{}
	err := focusGhosttyTmuxPane(f.run, "%3", func([]string) (string, error) {
		t.Fatal("focus must not run without clients")
		return "", nil
	}, spy.open, spy.activate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(spy.openedFor, []string{"jadlis_J-NT_13"}) {
		t.Errorf("opened tabs for %v, want the pane's session", spy.openedFor)
	}
	if !f.sawCommand("select-window") {
		t.Error("pane must be selected before the new tab attaches")
	}
	assertNoClientCommands(t, f)
}

func TestFocusGhosttyTmuxPane_OpensTabWhenNoGhosttyTabShowsSession(t *testing.T) {
	f := &fakeTmux{answers: map[string]string{"display-message": "s\n", "list-clients": "/dev/ttys001 xterm-256color\n"}}
	spy := &focusSpy{focusErr: errGhosttyTTYNotFound}
	if err := focusGhosttyTmuxPane(f.run, "%1", spy.focus, spy.open, spy.activate); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(spy.openedFor, []string{"s"}) {
		t.Errorf("opened tabs for %v", spy.openedFor)
	}
	assertNoClientCommands(t, f)
}

func TestFocusGhosttyTmuxPane_NoDuplicateTabOnAppleScriptFailure(t *testing.T) {
	f := &fakeTmux{answers: map[string]string{"display-message": "s\n", "list-clients": "/dev/ttys004 xterm-ghostty\n"}}
	spy := &focusSpy{focusErr: fmt.Errorf("Ghostty tty AppleScript timed out")}
	if err := focusGhosttyTmuxPane(f.run, "%1", spy.focus, spy.open, spy.activate); err == nil {
		t.Fatal("expected error")
	}
	if len(spy.openedFor) > 0 {
		t.Error("a failed lookup (timeout, permission) must not open a duplicate tab")
	}
	if !spy.activated {
		t.Error("Ghostty must still be activated")
	}
}

func TestFocusGhosttyTmuxPane_FallsBackToActivate(t *testing.T) {
	tests := []struct {
		name string
		f    *fakeTmux
		spy  *focusSpy
	}{
		{"pane gone", &fakeTmux{answers: map[string]string{"display-message": "\n"}}, &focusSpy{}},
		{"tmux server down", &fakeTmux{errs: map[string]error{"display-message": fmt.Errorf("no server")}}, &focusSpy{}},
		{"new tab fails", &fakeTmux{answers: map[string]string{"display-message": "s\n"}}, &focusSpy{openErr: fmt.Errorf("denied")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := focusGhosttyTmuxPane(tt.f.run, "%1", tt.spy.focus, tt.spy.open, tt.spy.activate); err == nil {
				t.Fatal("expected error")
			}
			if !tt.spy.activated {
				t.Error("Ghostty must still be activated so the click is not a no-op")
			}
			assertNoClientCommands(t, tt.f)
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
