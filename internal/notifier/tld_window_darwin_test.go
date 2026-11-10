//go:build darwin

package notifier

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// No test may reach the owner's real tld window: by default the opener says
// "no window". Tests that cover the tld path set it themselves.
func init() {
	tldSessionOpener = func(string) (string, bool) { return "", false }
}

func withTldOpener(t *testing.T, fn func(string) (string, bool)) {
	t.Helper()
	prev := tldSessionOpener
	tldSessionOpener = fn
	t.Cleanup(func() { tldSessionOpener = prev })
}

func TestFocusGhosttyTmuxPane_TldWindowComesFirst(t *testing.T) {
	f := &fakeTmux{answers: map[string]string{
		"display-message": "work\n",
		"list-clients":    "/dev/ttys004 xterm-ghostty\n",
	}}
	var asked []string
	withTldOpener(t, func(session string) (string, bool) {
		asked = append(asked, session)
		return "/dev/ttys002", true
	})
	spy := &focusSpy{}
	if err := focusGhosttyTmuxPane(f.run, "%9", spy.focus, spy.open, spy.activate); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(asked, []string{"work"}) {
		t.Errorf("tld asked for %v, want [work]", asked)
	}
	if !reflect.DeepEqual(spy.focusedTTYs, []string{"/dev/ttys002"}) {
		t.Errorf("focused %v, want the tty of the tld window", spy.focusedTTYs)
	}
	if len(spy.openedFor) != 0 {
		t.Errorf("must not open a new tab when the tld window shows the session: %v", spy.openedFor)
	}
	assertNoClientCommands(t, f)
}

func TestFocusGhosttyTmuxPane_TldWindowForSessionWithoutClients(t *testing.T) {
	f := &fakeTmux{answers: map[string]string{
		"display-message": "bg\n",
		"list-clients":    "",
	}}
	withTldOpener(t, func(string) (string, bool) { return "/dev/ttys002", true })
	spy := &focusSpy{}
	if err := focusGhosttyTmuxPane(f.run, "%1", spy.focus, spy.open, spy.activate); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(spy.openedFor) != 0 || !reflect.DeepEqual(spy.focusedTTYs, []string{"/dev/ttys002"}) {
		t.Errorf("opened=%v focused=%v, want no new tab and the tld tab raised", spy.openedFor, spy.focusedTTYs)
	}
}

func TestFocusGhosttyTmuxPane_TldWindowTabNotRaisedActivatesGhostty(t *testing.T) {
	f := &fakeTmux{answers: map[string]string{
		"display-message": "work\n",
		"list-clients":    "",
	}}
	withTldOpener(t, func(string) (string, bool) { return "/dev/ttys099", true })
	spy := &focusSpy{focusErr: errGhosttyTTYNotFound}
	if err := focusGhosttyTmuxPane(f.run, "%1", spy.focus, spy.open, spy.activate); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !spy.activated || len(spy.openedFor) != 0 {
		t.Errorf("activated=%v opened=%v, want Ghostty activated and no new tab", spy.activated, spy.openedFor)
	}
}

func TestFocusGhosttyTmuxPane_NoTldWindowKeepsOldPath(t *testing.T) {
	f := &fakeTmux{answers: map[string]string{
		"display-message": "bg\n",
		"list-clients":    "",
	}}
	withTldOpener(t, func(string) (string, bool) { return "", false })
	spy := &focusSpy{}
	if err := focusGhosttyTmuxPane(f.run, "%1", spy.focus, spy.open, spy.activate); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(spy.openedFor, []string{"bg"}) {
		t.Errorf("opened %v, want a new tab for bg as before", spy.openedFor)
	}
}

func TestRunTldOpen(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "tld.py")
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(script, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("JADLIS_TLD_SCRIPT", script)

	write(`[ "$1" = open ] && [ "$2" = work ] && echo /dev/ttys002`)
	if tty, ok := runTldOpen("work"); !ok || tty != "/dev/ttys002" {
		t.Errorf("window open: got %q %v, want /dev/ttys002 true", tty, ok)
	}
	write(`exit 1`)
	if _, ok := runTldOpen("work"); ok {
		t.Error("exit 1 (no window) must give ok=false")
	}
	write(`echo "not a tty"`)
	if _, ok := runTldOpen("work"); ok {
		t.Error("an answer that is not a tty must give ok=false")
	}
	if _, ok := runTldOpen(""); ok {
		t.Error("an empty session must give ok=false")
	}
	// A leftover child holding stdout must not keep the click waiting, and the
	// answer of a script that exited 0 still counts.
	write(`echo /dev/ttys003; sleep 10 &`)
	start := time.Now()
	if tty, ok := runTldOpen("work"); !ok || tty != "/dev/ttys003" {
		t.Errorf("leftover child: got %q %v, want /dev/ttys003 true", tty, ok)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("leftover child held the call for %s", d)
	}

	t.Setenv("JADLIS_TLD_SCRIPT", filepath.Join(dir, "missing.py"))
	if _, ok := runTldOpen("work"); ok {
		t.Error("a missing script must give ok=false")
	}
}
