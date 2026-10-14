package superexpose

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProdex043712TranscriptANSIAndCRNormalization(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
	}{
		{"", ""},
		{"plain ASCII", "plain ASCII"},
		{"\x1b[31mred\x1b[0m", "red"},
		{"\x1b]0;caption\aready", "ready"},
		{"\x1b]0;caption\x1b\\done", "done"},
		{"x\r\ny\rz", "x\ny\nz"},
		{"表🙂 \x1b🙂safe", "表🙂 safe"},
		{"abc\x1b", "abc"},
		{"\x1b[unfinished", "nfinished"},
		{"\x1b[123", ""},
		{"\x1b]unfinished", ""},
		{"a\x1b[8msecret", "asecret"},
		{"a\x1b]text\x1b\\b", "ab"},
	} {
		if got := stripTranscriptANSI043712(test.input); got != test.want {
			t.Fatalf("strip %q = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestProdex043712TranscriptSanitizationRemovesResidualControls(t *testing.T) {
	got := sanitizeOutputText("\x1b[31mred\x1b[0m\x00\r\nnext\tline")
	if want := "red \nnext\tline"; got != want {
		t.Fatalf("sanitized transcript = %q, want %q", got, want)
	}
	if strings.ContainsAny(got, "\x1b\r") {
		t.Fatalf("sanitized transcript retained terminal controls: %q", got)
	}
}

func TestProdex043712TranscriptSanitizationKeepsTruncationBound(t *testing.T) {
	got := sanitizeOutputText("\x1b[32m" + strings.Repeat("x", outputReadMaxTextBytes+100) + "\x1b[0m")
	if len(got) > outputReadMaxTextBytes || !strings.HasSuffix(got, " …[text_truncated]") {
		t.Fatalf("sanitized long transcript length/suffix = %d/%q", len(got), got[len(got)-min(32, len(got)):])
	}
}

func TestProdex043712OutputReaderAppliesTranscriptNormalization(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	record := map[string]any{
		"timestamp": "2026-10-14T10:00:00Z",
		"type":      "response_item",
		"payload": map[string]any{
			"type":   "function_call_output",
			"output": "\x1b[31mred\x1b[0m\r\nnext",
		},
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	batch, err := readOutputEvents(path, 0, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.events) != 1 || batch.events[0].Kind != "tool" || batch.events[0].Text != "red\nnext" {
		t.Fatalf("normalized transcript event = %#v", batch.events)
	}
}
