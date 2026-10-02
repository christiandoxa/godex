package auth

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestLoginMenuEntriesMatchProdex4351Order(t *testing.T) {
	entries := loginMenuEntries()
	want := []struct {
		title  string
		action LoginMenuAction
	}{
		{"Sign in with ChatGPT (OpenAI OAuth)", LoginChatGPT},
		{"OpenAI device code", LoginDeviceCode},
		{"Provide your own API key (OpenAI/API-compatible)", LoginOpenAIAPIKey},
		{"Google Gemini API key", LoginGeminiAPIKeyGuidance},
		{"Anthropic Claude OAuth", LoginClaude},
		{"Google Antigravity CLI", LoginAntigravity},
		{"Anthropic API key", LoginAnthropicAPIKeyGuidance},
		{"DeepSeek API key", LoginDeepSeekAPIKeyGuidance},
		{"GitHub Copilot import", LoginCopilotImport},
	}
	if len(entries) != len(want) {
		t.Fatalf("entry count = %d, want %d", len(entries), len(want))
	}
	for index, expected := range want {
		entry := entries[index]
		if entry.title != expected.title || entry.action != expected.action || entry.provider == "" || entry.auth == "" || entry.usage == "" || entry.command == "" {
			t.Fatalf("entry %d = %#v", index, entry)
		}
	}
}

func TestLoginMenuLayoutAndWindowMatchReferenceFixtures(t *testing.T) {
	compact := loginMenuLayoutForRows(12, 9)
	if !compact.compact || compact.visible != 7 {
		t.Fatalf("compact layout = %#v", compact)
	}
	roomy := loginMenuLayoutForRows(24, 9)
	if roomy.compact || roomy.visible != 9 {
		t.Fatalf("roomy layout = %#v", roomy)
	}
	if got := loginMenuWindowOffset(6, 0, 4, 9); got != 3 {
		t.Fatalf("window offset = %d, want 3", got)
	}
	if got := loginMenuWindowOffset(1, 3, 4, 9); got != 1 {
		t.Fatalf("window offset = %d, want 1", got)
	}
}

func TestLoginMenuNavigationAndDirectDigitSelection(t *testing.T) {
	model := newLoginMenuModel(loginMenuEntries(), 12)
	model.selected = 2
	updated, command := model.Update(keyRunes('j'))
	model = updated.(loginMenuModel)
	if command != nil || model.selected != 3 {
		t.Fatalf("down = selected:%d command:%v", model.selected, command)
	}
	updated, command = model.Update(keyRunes('u'))
	model = updated.(loginMenuModel)
	if command != nil || model.selected != 0 {
		t.Fatalf("page up = selected:%d command:%v", model.selected, command)
	}
	updated, command = model.Update(keyRunes('9'))
	model = updated.(loginMenuModel)
	if command == nil || model.chosen == nil || *model.chosen != LoginCopilotImport || model.selected != 8 {
		t.Fatalf("digit 9 = %#v command:%v", model, command)
	}
}

func TestLoginMenuGuidanceReturnsToMenuWithoutSelecting(t *testing.T) {
	model := newLoginMenuModel(loginMenuEntries(), 24)
	model.selected = 7
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(loginMenuModel)
	if command != nil || !model.guidance || model.chosen != nil {
		t.Fatalf("guidance enter = %#v command:%v", model, command)
	}
	view := model.View()
	for _, expected := range []string{"Provider guidance: DeepSeek API key", "Runtime API key only", "DEEPSEEK_API_KEY=<redacted>", "Enter/Esc return"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("guidance missing %q: %q", expected, view)
		}
	}
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(loginMenuModel)
	if command != nil || model.guidance || model.chosen != nil {
		t.Fatalf("guidance return = %#v command:%v", model, command)
	}
}

func TestLoginMenuCancelAndView(t *testing.T) {
	model := newLoginMenuModel(loginMenuEntries(), 12)
	view := model.View()
	for _, expected := range []string{"Godex Login", "methods 1-7 of 9", "Sign in with ChatGPT", "Provider: OpenAI / Codex", "1-9 direct"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("view missing %q: %q", expected, view)
		}
	}
	updated, command := model.Update(keyRunes('q'))
	model = updated.(loginMenuModel)
	if command == nil || !model.cancel {
		t.Fatalf("cancel = %#v command:%v", model, command)
	}
}

func TestRunLoginMenuRejectsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := RunLoginMenu(ctx, strings.NewReader(""), &strings.Builder{}); err == nil {
		t.Fatal("cancelled menu context unexpectedly succeeded")
	}
}

func keyRunes(value rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{value}}
}
