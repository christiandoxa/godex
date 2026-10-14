package codex

import (
	"os"
	"strings"
	"testing"
)

const sessionGuidanceID043712 = "01900000-0000-7000-8000-000000000712"

func TestProdex043712SavedSessionGuidanceUsesCanonicalIDAndCRLF(t *testing.T) {
	text := savedSessionGuidance043712(sessionGuidanceID043712)
	for _, want := range []string{
		"\r\nGodex saved session\r\n",
		"Session ID: " + sessionGuidanceID043712 + "\r\n",
		"Resume: godex s " + sessionGuidanceID043712 + "\r\n",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("guidance missing %q: %q", want, text)
		}
	}
	for index, value := range []byte(text) {
		if value == '\n' && (index == 0 || text[index-1] != '\r') {
			t.Fatalf("guidance contains bare LF: %q", text)
		}
	}
	if strings.ContainsAny(text, "\x1b") || strings.Contains(text, "unix://") {
		t.Fatalf("guidance contains terminal/socket control data: %q", text)
	}
}

func TestProdex043712SavedSessionGuidanceUnknownIDUsesPicker(t *testing.T) {
	text := savedSessionGuidance043712("not-a-session")
	if !strings.Contains(text, "Session ID was not reported by this launch.\r\n") {
		t.Fatalf("unknown identity was not explained: %q", text)
	}
	if !strings.Contains(text, "Resume: godex s resume\r\n") {
		t.Fatalf("unknown identity did not use picker: %q", text)
	}
	if strings.Contains(text, "Session ID: ") {
		t.Fatalf("unknown identity was guessed: %q", text)
	}
}

func TestProdex043712ReadSessionIdentityMarkerBoundsAndRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	marker := root + "/session.id"
	if err := writeTestSessionMarker043712(marker, sessionGuidanceID043712); err != nil {
		t.Fatal(err)
	}
	if got := readSessionIdentityMarker(marker); got != sessionGuidanceID043712 {
		t.Fatalf("marker ID = %q", got)
	}
	if err := writeTestSessionMarker043712(marker, strings.Repeat("x", sessionIdentityMarkerLimit+1)); err != nil {
		t.Fatal(err)
	}
	if got := readSessionIdentityMarker(marker); got != "" {
		t.Fatalf("oversized marker ID = %q", got)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", marker); err != nil {
		t.Fatal(err)
	}
	if got := readSessionIdentityMarker(marker); got != "" {
		t.Fatalf("symlink marker ID = %q", got)
	}
}

func TestProdex043712LaunchMarkerWinsOverResumeFallback(t *testing.T) {
	root := t.TempDir()
	marker := root + "/session.id"
	if err := writeTestSessionMarker043712(marker, sessionGuidanceID043712); err != nil {
		t.Fatal(err)
	}
	other := "01900000-0000-7000-8000-000000000713"
	if got := savedSessionID043712(marker, []string{"resume", other}); got != sessionGuidanceID043712 {
		t.Fatalf("launch marker lost precedence: got %q", got)
	}
	if err := writeTestSessionMarker043712(marker, "invalid"); err != nil {
		t.Fatal(err)
	}
	if got := savedSessionID043712(marker, []string{"resume", other}); got != other {
		t.Fatalf("validated resume fallback = %q, want %q", got, other)
	}
}

func TestProdex043712PrepareSessionIdentityOnlyTracksInteractiveCompanion(t *testing.T) {
	marker, cleanup, args := prepareSessionIdentity(t.TempDir(), []string{"exec", "task"})
	cleanup()
	if marker != "" || len(args) != 2 || args[0] != "exec" {
		t.Fatalf("headless launch received identity marker: marker=%q args=%#v", marker, args)
	}

	marker, cleanup, args = prepareSessionIdentity(t.TempDir(), []string{"--model", "synthetic"})
	if marker == "" || len(args) <= 2 {
		t.Fatalf("interactive launch did not receive identity hook: marker=%q args=%#v", marker, args)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("marker unexpectedly existed before callback: %v", err)
	}
	cleanup()
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("marker directory survived cleanup: %v", err)
	}
}

func TestProdex043712IdentityHooksPreserveConfiguredNotifyAndSessionStart(t *testing.T) {
	home := t.TempDir()
	configured := []string{"-c", `notify=["user-notifier"]`, "--model", "synthetic"}
	args := sessionIdentityArguments(home, configured, "/usr/bin/godex", "/tmp/godex-session-hook/session.id")
	joined := strings.Join(args, "\x00")
	if strings.Count(joined, "notify=") != 1 || !strings.Contains(joined, `notify=["user-notifier"]`) {
		t.Fatalf("configured notify was replaced or duplicated: %#v", args)
	}

	customHook := []string{"-c", `hooks.SessionStart=[{hooks=[{type="command",command="user-hook"}]}]`, "--model", "synthetic"}
	args = sessionIdentityArguments(home, customHook, "/usr/bin/godex", "/tmp/godex-session-hook/session.id")
	joined = strings.Join(args, "\x00")
	if strings.Count(joined, "hooks.SessionStart=") != 1 || !strings.Contains(joined, "user-hook") {
		t.Fatalf("configured SessionStart was replaced or duplicated: %#v", args)
	}
}

func writeTestSessionMarker043712(path, value string) error {
	return os.WriteFile(path, []byte(value+"\n"), 0o600)
}
