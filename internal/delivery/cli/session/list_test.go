package session

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

func TestArgumentsValidateFiltersAndOutputModes(t *testing.T) {
	for _, args := range [][]string{{"--json", "--id-only"}, {"--parent-only", "--include-subagents"}, {"--limit", "-1"}, {"extra"}, {"--cwd", "."}, {"--unknown"}} {
		if _, err := parseArguments(false, args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	options, err := parseArguments(true, []string{"--cwd", ".", "--profile", "work", "--query", "repair", "--limit", "0", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(options.query.CurrentDir) || !options.query.LimitSet || options.query.Limit != 0 || !options.json {
		t.Fatalf("options = %#v", options)
	}
}
func TestOutputJSONIDsCommandsAndControlCharacters(t *testing.T) {
	reports := []sessionmodel.Report{{ID: "00000000-0000-4000-8000-000000000001", Profile: "work", ThreadName: "name\x1b[2J\nnext"}}
	for _, options := range []listOptions{{json: true}, {idOnly: true}, {resumeCommand: true}, {}} {
		var out bytes.Buffer
		if err := printReports(&out, reports, options); err != nil {
			t.Fatal(err)
		}
		if options.json {
			var decoded []sessionmodel.Report
			if err := json.Unmarshal(out.Bytes(), &decoded); err != nil || len(decoded) != 1 {
				t.Fatalf("json = %s: %v", out.String(), err)
			}
		} else if strings.ContainsRune(out.String(), '\x1b') {
			t.Fatalf("unsafe terminal output = %q", out.String())
		}
		if options.resumeCommand && !strings.HasPrefix(out.String(), "godex session resume ") {
			t.Fatalf("command = %q", out.String())
		}
	}
	var out bytes.Buffer
	if err := printReports(&out, []sessionmodel.Report{}, listOptions{json: true}); err != nil || strings.TrimSpace(out.String()) != "[]" {
		t.Fatalf("empty = %q, %v", out.String(), err)
	}
}

func TestSessionTUIViewAndReferenceScrollKeys(t *testing.T) {
	reports := []sessionmodel.Report{
		{ID: "00000000-0000-4000-8000-000000000001", Profile: "work", ThreadName: "first", UpdatedAt: "2026-10-01T00:00:00Z", CWD: "/repo", ModelProvider: "openai"},
		{ID: "00000000-0000-4000-8000-000000000002", Profile: "work", ThreadName: "second", UpdatedAt: "2026-10-01T00:01:00Z", CWD: "/repo", ModelProvider: "openai"},
	}
	model := newSessionTUIModel(reports, true, 8)
	view := model.View()
	for _, expected := range []string{"Godex Sessions", "2 session(s)", "first", "j/k/Up/Down", "q/Esc/Enter"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("view missing %q: %q", expected, view)
		}
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = updated.(sessionTUIModel)
	if model.offset == 0 {
		t.Fatal("j did not scroll down")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyHome})
	model = updated.(sessionTUIModel)
	if model.offset != 0 {
		t.Fatalf("home offset = %d", model.offset)
	}
	_, quit := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if quit == nil {
		t.Fatal("enter did not quit")
	}
}

func TestSessionTUILinesSanitizeUntrustedMetadata(t *testing.T) {
	lines := sessionTUILines([]sessionmodel.Report{{
		ID: "id\x1b[2J", ThreadName: "name\nnext", Profile: "", ModelProvider: "", CWD: "",
	}})
	rendered := strings.Join(lines, "\n")
	if strings.ContainsRune(rendered, '\x1b') || strings.Contains(rendered, "name\nnext") {
		t.Fatalf("unsafe TUI lines = %q", rendered)
	}
	if !strings.Contains(rendered, "profile -  provider -") || !strings.Contains(rendered, "cwd -") {
		t.Fatalf("missing empty fallbacks: %q", rendered)
	}
}

func TestSessionStaticTUIAutoQuits(t *testing.T) {
	model := newSessionTUIModel(nil, false, 24)
	if model.Init() == nil {
		t.Fatal("static TUI did not auto-quit")
	}
	if !strings.Contains(model.View(), "No sessions found") {
		t.Fatalf("empty view = %q", model.View())
	}
}
