package codex

import "testing"

func TestSessionAttachmentPolicyMatchesTaggedScannerContracts(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
		start int
		end   int
		ok    bool
	}{
		{name: "escaped image", input: `<image path=\"/tmp/codex-clipboard-a.png\"`, start: 14, end: 40, ok: true},
		{name: "plain image", input: `<image path="/tmp/codex-clipboard-a.png"`, start: 13, end: 39, ok: true},
		{name: "missing image path", input: `<image src="/tmp/codex-clipboard-a.png"`, ok: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			start, end, ok := sessionImageTagPathRange(test.input)
			if start != test.start || end != test.end || ok != test.ok {
				t.Fatalf("range = (%d,%d,%t), want (%d,%d,%t)", start, end, ok, test.start, test.end, test.ok)
			}
		})
	}

	clipboard := "see /tmp/codex-clipboard-a.png."
	start, end, ok := nextSessionClipboardPath(clipboard, 0)
	if !ok || clipboard[start:end] != "/tmp/codex-clipboard-a.png" {
		t.Fatalf("clipboard range = (%d,%d,%t) %q", start, end, ok, clipboard[start:end])
	}
	attachment := `see /tmp/attachments/thread/image-a.png, next`
	start, end, ok = nextSessionAttachmentPath(attachment, 0)
	if !ok || attachment[start:end] != "/tmp/attachments/thread/image-a.png" {
		t.Fatalf("attachment range = (%d,%d,%t) %q", start, end, ok, attachment[start:end])
	}

	if !sessionClipboardFileName("codex-clipboard-a.png") {
		t.Fatal("clipboard filename rejected")
	}
	for _, name := range []string{"pasted-text-1.txt", "image-1.png", "goal-objective.md"} {
		if !sessionPersistableAttachmentFileName(name) {
			t.Errorf("persistable filename rejected: %s", name)
		}
	}
	if sessionPersistableAttachmentFileName("other.txt") {
		t.Fatal("unrecognized attachment filename accepted")
	}
}

func TestSessionAttachmentPolicyMatchesTaggedPathBoundaries(t *testing.T) {
	for _, test := range []struct {
		name string
		text string
		want string
	}{
		{name: "comma", text: "x=/tmp/codex-clipboard-a.png,tail", want: "x=/tmp/codex-clipboard-a.png"},
		{name: "semicolon", text: "x=/tmp/codex-clipboard-a.png;tail", want: "x=/tmp/codex-clipboard-a.png"},
		{name: "trailing dots", text: "x=/tmp/codex-clipboard-a.png...", want: "x=/tmp/codex-clipboard-a.png"},
		{name: "json quote escape", text: `{"path":"/tmp/codex-clipboard-a.png\\\"ignored"}`, want: "/tmp/codex-clipboard-a.png"},
	} {
		t.Run(test.name, func(t *testing.T) {
			start, end, ok := nextSessionClipboardPath(test.text, 0)
			if !ok || test.text[start:end] != test.want {
				t.Fatalf("range = (%d,%d,%t) %q, want %q", start, end, ok, test.text[start:end], test.want)
			}
		})
	}
}

func TestSessionAttachmentPolicyUsesEarliestAttachmentSeparatorVariant(t *testing.T) {
	text := `C:\\tmp\\attachments\\thread\\image-1.png then /later/attachments/id/image-2.png`
	start, end, ok := nextSessionAttachmentPath(text, 0)
	if !ok || text[start:end] != `C:\\tmp\\attachments\\thread\\image-1.png` {
		t.Fatalf("first attachment = (%d,%d,%t) %q", start, end, ok, text[start:end])
	}
	start2, end2, ok := nextSessionAttachmentPath(text, end)
	if !ok || text[start2:end2] != "/later/attachments/id/image-2.png" {
		t.Fatalf("second attachment = (%d,%d,%t) %q", start2, end2, ok, text[start2:end2])
	}
}

func TestProdex04356SessionScanCursorSkipsJSONEscapes(t *testing.T) {
	for _, fixture := range []struct {
		name   string
		prefix string
	}{
		{name: "newline", prefix: `\n`},
		{name: "unicode", prefix: `\u2028`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			path := `/tmp/deleted-overlay/attachments/11111111-2222-4333-8444-555555555555/image-1.png`
			text := fixture.prefix + path
			start, end, ok := nextSessionAttachmentPath(text, 0)
			if !ok {
				t.Fatal("attachment path was not found")
			}
			if got := text[start:end]; got != path {
				t.Fatalf("attachment range = %q, want %q", got, path)
			}
		})
	}
}
