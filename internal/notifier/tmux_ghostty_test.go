package notifier

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func stubNotifierExecutable(t *testing.T, path string) {
	t.Helper()
	old := notifierExecutablePath
	notifierExecutablePath = func() (string, error) { return path, nil }
	t.Cleanup(func() { notifierExecutablePath = old })
}

func argValue(args []string, flag string) (string, bool) {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1], true
		}
	}
	return "", false
}

func TestBuildGhosttyTmuxNotifierArgs(t *testing.T) {
	stubNotifierExecutable(t, "/opt/plugin/bin/claude-notifications")
	t.Setenv("TMUX", "/private/tmp/tmux-501/default,123,0")

	args, err := buildGhosttyTmuxNotifierArgs("Title", "Body", "%42", "com.mitchellh.ghostty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	execCmd, ok := argValue(args, "-execute")
	if !ok {
		t.Fatalf("no -execute in %v", args)
	}
	for _, want := range []string{
		"'/opt/plugin/bin/claude-notifications' focus-tmux-ghostty",
		"--pane '%42'",
		"--socket '/private/tmp/tmux-501/default'",
		"--tmux-path '",
	} {
		if !strings.Contains(execCmd, want) {
			t.Errorf("-execute %q missing %q", execCmd, want)
		}
	}
	if strings.Contains(execCmd, "switch-client") {
		t.Errorf("-execute must never switch tmux clients: %q", execCmd)
	}
	if _, ok := argValue(args, "-activate"); ok {
		t.Errorf("-activate must not be set: the helper activates Ghostty itself after choosing the tab")
	}
	if v, _ := argValue(args, "-title"); v != "Title" {
		t.Errorf("-title = %q", v)
	}
	if v, _ := argValue(args, "-group"); !strings.HasPrefix(v, "claude-notif-") {
		t.Errorf("-group = %q", v)
	}
}

func TestBuildGhosttyTmuxNotifierArgs_NoSocket(t *testing.T) {
	stubNotifierExecutable(t, "/bin/x")
	t.Setenv("TMUX", "")
	args, err := buildGhosttyTmuxNotifierArgs("T", "M", "%1", "com.mitchellh.ghostty")
	if err != nil {
		t.Fatal(err)
	}
	execCmd, _ := argValue(args, "-execute")
	if strings.Contains(execCmd, "--socket") {
		t.Errorf("no socket expected: %q", execCmd)
	}
}

func TestBuildGhosttyTmuxNotifierArgs_InvalidPane(t *testing.T) {
	stubNotifierExecutable(t, "/bin/x")
	if _, err := buildGhosttyTmuxNotifierArgs("T", "M", "main:1", "com.mitchellh.ghostty"); err == nil {
		t.Fatal("expected error for non-%id pane target")
	}
}

func TestBuildGhosttyTmuxNotifierArgs_QuotesExecutablePath(t *testing.T) {
	stubNotifierExecutable(t, "/Users/o'neil/plugin/bin/claude-notifications")
	args, err := buildGhosttyTmuxNotifierArgs("T", "M", "%1", "com.mitchellh.ghostty")
	if err != nil {
		t.Fatal(err)
	}
	execCmd, _ := argValue(args, "-execute")
	if !strings.HasPrefix(execCmd, `'/Users/o'\''neil/plugin/bin/claude-notifications' `) {
		t.Errorf("executable not shell-quoted: %q", execCmd)
	}
}

func TestBuildTmuxClickArgs_GhosttyUsesTTYHelper(t *testing.T) {
	stubNotifierExecutable(t, "/opt/plugin/bin/claude-notifications")
	t.Setenv("TMUX", "/private/tmp/tmux-501/default,123,0")
	t.Setenv("TMUX_PANE", "%7")

	args, err := buildTmuxClickArgs("T", "M", "com.mitchellh.ghostty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	execCmd, _ := argValue(args, "-execute")
	if !strings.Contains(execCmd, "focus-tmux-ghostty --pane '%7'") {
		t.Errorf("Ghostty + tmux should use focus-tmux-ghostty, got %q", execCmd)
	}
	if strings.Contains(execCmd, "switch-client") {
		t.Errorf("Ghostty + tmux must not switch clients: %q", execCmd)
	}
}

func TestBuildTmuxClickArgs_OtherTerminalsUnchanged(t *testing.T) {
	t.Setenv("TMUX", "/private/tmp/tmux-501/default,123,0")
	t.Setenv("TMUX_PANE", "%7")

	args, err := buildTmuxClickArgs("T", "M", "com.apple.Terminal")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	execCmd, _ := argValue(args, "-execute")
	if strings.Contains(execCmd, "focus-tmux-ghostty") {
		t.Errorf("non-Ghostty terminal must not use the Ghostty helper: %q", execCmd)
	}
}

// fakeTmux records commands and answers from a table keyed by the first arg.
type fakeTmux struct {
	calls   [][]string
	answers map[string]string
	errs    map[string]error
}

func (f *fakeTmux) run(args ...string) (string, error) {
	f.calls = append(f.calls, append([]string(nil), args...))
	if err := f.errs[args[0]]; err != nil {
		return "", err
	}
	return f.answers[args[0]], nil
}

func (f *fakeTmux) sawCommand(name string) bool {
	for _, c := range f.calls {
		if c[0] == name {
			return true
		}
	}
	return false
}

func TestResolveTmuxPaneClientTTYs(t *testing.T) {
	f := &fakeTmux{answers: map[string]string{
		"display-message": "jadlis_J-NT_12\n",
		"list-clients": "/dev/ttys009 xterm-256color\n" +
			"/dev/ttys004 xterm-ghostty\n" +
			"/dev/ttys004 xterm-ghostty\n" +
			"garbage\n\n",
	}}
	session, ttys, err := resolveTmuxPaneClientTTYs(f.run, "%102")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session != "jadlis_J-NT_12" {
		t.Errorf("session = %q", session)
	}
	if want := []string{"/dev/ttys004", "/dev/ttys009"}; !reflect.DeepEqual(ttys, want) {
		t.Errorf("ttys = %v, want %v (Ghostty first, deduped, junk dropped)", ttys, want)
	}
	if got := f.calls[0]; !reflect.DeepEqual(got, []string{"display-message", "-p", "-t", "%102", "#{session_name}"}) {
		t.Errorf("display-message call = %v", got)
	}
	if got := f.calls[1]; got[0] != "list-clients" || got[2] != "=jadlis_J-NT_12" {
		t.Errorf("list-clients must target the exact session name: %v", got)
	}
}

func TestResolveTmuxPaneClientTTYs_Errors(t *testing.T) {
	tests := []struct {
		name string
		pane string
		f    *fakeTmux
	}{
		{"invalid pane", "main:0", &fakeTmux{}},
		{"pane gone", "%1", &fakeTmux{answers: map[string]string{"display-message": "\n"}}},
		{"display fails", "%1", &fakeTmux{errs: map[string]error{"display-message": fmt.Errorf("no server")}}},
		{"no clients", "%1", &fakeTmux{answers: map[string]string{"display-message": "s\n", "list-clients": ""}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := resolveTmuxPaneClientTTYs(tt.f.run, tt.pane); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestSelectTmuxPaneInOwnSession_NoClientCommands(t *testing.T) {
	f := &fakeTmux{}
	if err := selectTmuxPaneInOwnSession(f.run, "%5"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"select-window", "-t", "%5"}, {"select-pane", "-t", "%5"}}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("calls = %v, want %v", f.calls, want)
	}
}

// TestGhosttyTmuxE2E runs the resolution against a real, isolated tmux server
// with real attached clients (ptys from script(1)). Clients use
// TERM=xterm-256color: xterm-ghostty terminfo exists only inside Ghostty, and
// the Ghostty-first ordering is covered by TestResolveTmuxPaneClientTTYs.
// It never touches the user's tmux server.
func TestGhosttyTmuxE2E(t *testing.T) {
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux not found")
	}
	scriptPath, err := exec.LookPath("script")
	if err != nil {
		t.Skip("script(1) not found")
	}
	if os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "" {
		t.Skip("Skipping tmux e2e tests in CI")
	}

	socket := filepath.Join(t.TempDir(), "s")
	target := GhosttyTmuxTarget{TmuxPath: tmuxPath, Socket: socket}
	run := newTmuxCommandRunner(target)
	t.Cleanup(func() { _ = exec.Command(tmuxPath, "-S", socket, "kill-server").Run() })

	mustRunTmux(t, tmuxPath, socket, "new-session", "-d", "-s", "tab-a", "-n", "w0")
	mustRunTmux(t, tmuxPath, socket, "new-window", "-t", "tab-a", "-n", "w1")
	mustRunTmux(t, tmuxPath, socket, "new-session", "-d", "-s", "tab-a-2")
	mustRunTmux(t, tmuxPath, socket, "select-window", "-t", "tab-a:w0")
	paneW1 := getTmuxPaneID(t, tmuxPath, socket, "tab-a:w1")

	// No client yet: resolution reports the session and fails cleanly.
	session, _, err := resolveTmuxPaneClientTTYs(run, paneW1)
	if err == nil || session != "tab-a" {
		t.Fatalf("without clients: session=%q err=%v, want tab-a + error", session, err)
	}

	// Attach one client to tab-a-2 (a prefix-sharing name) and one to tab-a.
	attach := func(sessionName string) {
		cmd := exec.Command(scriptPath, "-q", "/dev/null", tmuxPath, "-S", socket, "attach", "-t", "="+sessionName)
		cmd.Env = append(os.Environ(), "TERM=xterm-256color", "TMUX=")
		// Keep stdin open: script(1) exits on EOF.
		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatalf("attach %s: %v", sessionName, err)
		}
		t.Cleanup(func() { _ = stdin.Close(); _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	}
	attach("tab-a-2")
	attach("tab-a")

	var ttys []string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, ttys, err = resolveTmuxPaneClientTTYs(run, paneW1)
		if err == nil && len(ttys) == 1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil || len(ttys) != 1 || !strings.HasPrefix(ttys[0], "/dev/tty") || strings.ContainsAny(ttys[0], " _\t") {
		t.Fatalf("ttys = %v err = %v, want exactly the tab-a client tty", ttys, err)
	}

	clientsBefore := runTmux(t, tmuxPath, socket, "list-clients", "-F", "#{client_tty} #{client_session}")

	if err := selectTmuxPaneInOwnSession(run, paneW1); err != nil {
		t.Fatalf("select: %v", err)
	}
	if got := strings.TrimSpace(runTmux(t, tmuxPath, socket, "display-message", "-p", "-t", "tab-a", "#{window_name}")); got != "w1" {
		t.Errorf("tab-a current window = %q, want w1", got)
	}
	if after := runTmux(t, tmuxPath, socket, "list-clients", "-F", "#{client_tty} #{client_session}"); after != clientsBefore {
		t.Errorf("clients changed session:\nbefore %q\nafter  %q", clientsBefore, after)
	}
	if got := strings.TrimSpace(runTmux(t, tmuxPath, socket, "list-sessions", "-F", "#{session_name}")); got != "tab-a\ntab-a-2" {
		t.Errorf("sessions = %q, want both alive", got)
	}
}
