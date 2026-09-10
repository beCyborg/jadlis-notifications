package notifier

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSessionFixture(t *testing.T, cwd, sid, customFile, transcript string) {
	t.Helper()
	configDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	sessionDir, transcriptPath := claudeSessionPaths(sid, cwd)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if customFile != "" {
		if err := os.WriteFile(filepath.Join(sessionDir, "custom-title.json"), []byte(customFile), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if transcript != "" {
		if err := os.WriteFile(transcriptPath, []byte(transcript), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReadClaudeSessionTitle_CustomFileWins(t *testing.T) {
	writeSessionFixture(t, "/Users/x/Jadlis", "sid-1",
		`{"customTitle":"Ручное имя"}`,
		`{"type":"ai-title","aiTitle":"Автоимя","sessionId":"sid-1"}`+"\n")
	if got := readClaudeSessionTitle("sid-1", "/Users/x/Jadlis"); got != "Ручное имя" {
		t.Fatalf("got %q", got)
	}
}

func TestReadClaudeSessionTitle_AITitleFromTranscript(t *testing.T) {
	writeSessionFixture(t, "/Users/x/Jadlis", "sid-2", "",
		`{"type":"user","message":{"role":"user","content":"hi"}}`+"\n"+
			`{"type":"ai-title","aiTitle":"Первое имя","sessionId":"sid-2"}`+"\n"+
			`{"type":"ai-title","aiTitle":"Поездка в Гданьск","sessionId":"sid-2"}`+"\n")
	if got := readClaudeSessionTitle("sid-2", "/Users/x/Jadlis"); got != "Поездка в Гданьск" {
		t.Fatalf("got %q", got)
	}
}

func TestReadClaudeSessionTitle_TranscriptCustomBeatsAI(t *testing.T) {
	writeSessionFixture(t, "/Users/x/Jadlis", "sid-3", "",
		`{"type":"custom-title","customTitle":"Дайджест №2","sessionId":"sid-3"}`+"\n"+
			`{"type":"ai-title","aiTitle":"Слаг после плана","sessionId":"sid-3"}`+"\n")
	if got := readClaudeSessionTitle("sid-3", "/Users/x/Jadlis"); got != "Дайджест №2" {
		t.Fatalf("got %q", got)
	}
}

func TestReadClaudeSessionTitle_SkipsOversizedLines(t *testing.T) {
	huge := `{"type":"user","message":{"content":"` + strings.Repeat("x", 3<<20) + `"}}` + "\n"
	writeSessionFixture(t, "/Users/x/Jadlis", "sid-4", "",
		huge+`{"type":"ai-title","aiTitle":"После длинной строки","sessionId":"sid-4"}`+"\n"+huge)
	if got := readClaudeSessionTitle("sid-4", "/Users/x/Jadlis"); got != "После длинной строки" {
		t.Fatalf("got %q", got)
	}
}

func TestReadClaudeSessionTitle_Absent(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	if got := readClaudeSessionTitle("nope", "/Users/x/Jadlis"); got != "" {
		t.Fatalf("got %q", got)
	}
	if got := readClaudeSessionTitle("", "/Users/x/Jadlis"); got != "" {
		t.Fatalf("got %q", got)
	}
}
