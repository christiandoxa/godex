package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const loginRuntimeAPIKeyOnly = "Runtime API key only"

type LoginMenuAction uint8

const (
	LoginChatGPT LoginMenuAction = iota
	LoginDeviceCode
	LoginOpenAIAPIKey
	LoginGeminiAPIKeyGuidance
	LoginClaude
	LoginAntigravity
	LoginAnthropicAPIKeyGuidance
	LoginDeepSeekAPIKeyGuidance
	LoginCopilotImport
)

type loginMenuEntry struct {
	title    string
	provider string
	auth     string
	usage    string
	command  string
	action   LoginMenuAction
	guidance bool
}

type loginMenuLayout struct {
	visible int
	compact bool
}

type loginMenuModel struct {
	entries  []loginMenuEntry
	selected int
	offset   int
	height   int
	chosen   *LoginMenuAction
	cancel   bool
	guidance bool
}

func LoginMenuInteractive(in io.Reader, out io.Writer) bool {
	return loginMenuTerminal(in) && loginMenuTerminal(out)
}

func RunLoginMenu(ctx context.Context, in io.Reader, out io.Writer) (LoginMenuAction, error) {
	entries := loginMenuEntries()
	model := newLoginMenuModel(entries, 24)
	program := tea.NewProgram(
		model,
		tea.WithInput(in), tea.WithOutput(out), tea.WithAltScreen(), tea.WithContext(ctx),
	)
	result, err := program.Run()
	if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if err != nil {
		return 0, fmt.Errorf("login menu TUI failed: %w", err)
	}
	final, ok := result.(loginMenuModel)
	if !ok {
		return 0, errors.New("login menu returned invalid state")
	}
	if final.cancel || final.chosen == nil {
		return 0, errors.New("login cancelled")
	}
	return *final.chosen, nil
}

func newLoginMenuModel(entries []loginMenuEntry, height int) loginMenuModel {
	return loginMenuModel{entries: append([]loginMenuEntry(nil), entries...), height: max(1, height)}
}

func (model loginMenuModel) Init() tea.Cmd { return nil }

func (model loginMenuModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.height = max(1, message.Height)
		model.offset = loginMenuWindowOffset(model.selected, model.offset, model.layout().visible, len(model.entries))
	case tea.KeyMsg:
		return model.updateKey(message)
	}
	return model, nil
}

func (model loginMenuModel) updateKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if model.guidance {
		switch strings.ToLower(key.String()) {
		case "enter", "esc", "backspace":
			model.guidance = false
			return model, nil
		case "q", "ctrl+c", "ctrl+z":
			model.cancel = true
			return model, tea.Quit
		default:
			return model, nil
		}
	}
	layout := model.layout()
	if digit := loginMenuDigit(key); digit > 0 {
		return model.selectIndex(digit - 1)
	}
	switch key.String() {
	case "up", "k", "K":
		model.selected = max(0, model.selected-1)
	case "down", "j", "J":
		model.selected = min(len(model.entries)-1, model.selected+1)
	case "pgup", "u", "U":
		model.selected = max(0, model.selected-max(1, layout.visible))
	case "pgdown", "d", "D":
		model.selected = min(len(model.entries)-1, model.selected+max(1, layout.visible))
	case "home", "g":
		model.selected = 0
	case "end", "G":
		model.selected = max(0, len(model.entries)-1)
	case "enter":
		return model.selectIndex(model.selected)
	case "esc", "q", "Q", "ctrl+c", "ctrl+z":
		model.cancel = true
		return model, tea.Quit
	}
	model.offset = loginMenuWindowOffset(model.selected, model.offset, layout.visible, len(model.entries))
	return model, nil
}

func (model loginMenuModel) selectIndex(index int) (tea.Model, tea.Cmd) {
	if index < 0 || index >= len(model.entries) {
		return model, nil
	}
	model.selected = index
	entry := model.entries[index]
	if entry.guidance {
		model.guidance = true
		return model, nil
	}
	action := entry.action
	model.chosen = &action
	return model, tea.Quit
}

func (model loginMenuModel) View() string {
	if len(model.entries) == 0 {
		return "Godex Login\n\nNo login methods available."
	}
	if model.guidance {
		return loginGuidanceView(model.entries[model.selected])
	}
	layout := model.layout()
	offset := loginMenuWindowOffset(model.selected, model.offset, layout.visible, len(model.entries))
	end := min(len(model.entries), offset+layout.visible)
	var output strings.Builder
	fmt.Fprintf(&output, "Godex Login  methods %d-%d of %d\n\n", offset+1, end, len(model.entries))
	for index := offset; index < end; index++ {
		marker := " "
		if index == model.selected {
			marker = ">"
		} else if index == offset && offset > 0 {
			marker = "^"
		} else if index+1 == end && end < len(model.entries) {
			marker = "v"
		}
		fmt.Fprintf(&output, "%s %2d. %s\n", marker, index+1, model.entries[index].title)
	}
	entry := model.entries[model.selected]
	output.WriteString("\n")
	fmt.Fprintf(&output, "Provider: %s\nAuth: %s\nUse: %s\nCommand: %s\n", entry.provider, entry.auth, entry.usage, entry.command)
	if layout.compact {
		output.WriteString("\n↑/↓ move • Enter select • 1-9 direct • q/Esc cancel")
	} else {
		output.WriteString("\n↑/↓ or j/k move • PgUp/PgDn or u/d • g/G Home/End • 1-9 direct • Enter select • q/Esc cancel")
	}
	return output.String()
}

func (model loginMenuModel) layout() loginMenuLayout {
	return loginMenuLayoutForRows(model.height, len(model.entries))
}

func loginMenuLayoutForRows(rows, entryCount int) loginMenuLayout {
	if entryCount <= 0 {
		return loginMenuLayout{visible: 1, compact: true}
	}
	compact := rows < 18
	visible := entryCount
	if compact {
		visible = min(entryCount, max(1, rows-5))
	}
	return loginMenuLayout{visible: visible, compact: compact}
}

func loginMenuWindowOffset(selected, current, visible, count int) int {
	if count <= 0 || visible <= 0 {
		return 0
	}
	selected = min(max(0, selected), count-1)
	maximum := max(0, count-visible)
	current = min(max(0, current), maximum)
	if selected < current {
		return selected
	}
	if selected >= current+visible {
		return min(maximum, selected-visible+1)
	}
	return current
}

func loginMenuDigit(key tea.KeyMsg) int {
	if key.Type != tea.KeyRunes || len(key.Runes) != 1 {
		return 0
	}
	current := key.Runes[0]
	if current < '1' || current > '9' {
		return 0
	}
	return int(current - '0')
}

func loginGuidanceView(entry loginMenuEntry) string {
	return fmt.Sprintf(
		"Godex Login\n\nProvider guidance: %s\n\nProvider: %s\nAuth: %s\nUse: %s\nCommand: %s\n\nNote: this path is selected at runtime, not stored by godex login.\n\nEnter/Esc return • q cancel",
		entry.title, entry.provider, entry.auth, entry.usage, entry.command,
	)
}

func loginMenuTerminal(value any) bool {
	file, ok := value.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func loginMenuEntries() []loginMenuEntry {
	return []loginMenuEntry{
		{"Sign in with ChatGPT (OpenAI OAuth)", "OpenAI / Codex", "ChatGPT OAuth", "Default quota-aware profile pool for godex run.", "godex login", LoginChatGPT, false},
		{"OpenAI device code", "OpenAI / Codex", "Device-code OAuth", "Same OpenAI/Codex profile type, useful on a terminal without a local browser.", "godex login --device-auth", LoginDeviceCode, false},
		{"Provide your own API key (OpenAI/API-compatible)", "OpenAI / local / OpenAI-compatible endpoint", "API key stored in the selected Godex profile", "Use OpenAI API billing, or provide a compatible base URL for local/custom endpoints.", "godex login --with-api-key [--base-url URL]", LoginOpenAIAPIKey, false},
		{"Google Gemini API key", "Google Gemini", loginRuntimeAPIKeyOnly, "Not a persisted login profile; pass GEMINI_API_KEY(S), GOOGLE_API_KEY(S), or --api-key when launching Gemini.", "GEMINI_API_KEY=<redacted> godex run --provider gemini --model gemini-2.5-pro", LoginGeminiAPIKeyGuidance, true},
		{"Anthropic Claude OAuth", "Anthropic Claude", "Claude Code OAuth profile", "Import an existing Claude Code OAuth profile for godex run --provider anthropic.", "godex profile import claude --activate", LoginClaude, false},
		{"Google Antigravity CLI", "Google Antigravity", "Antigravity CLI keyring / Google Sign-In", "Authenticate the native agy CLI used by the Antigravity provider.", "agy auth login", LoginAntigravity, true},
		{"Anthropic API key", "Anthropic Claude", loginRuntimeAPIKeyOnly, "Not a persisted login profile; pass ANTHROPIC_API_KEY(S) or --api-key.", "ANTHROPIC_API_KEY=<redacted> godex run --provider anthropic --model claude-sonnet-4-6", LoginAnthropicAPIKeyGuidance, true},
		{"DeepSeek API key", "DeepSeek", loginRuntimeAPIKeyOnly, "DeepSeek has no OAuth login; use an API key for the provider adapter.", "DEEPSEEK_API_KEY=<redacted> godex run --provider deepseek --model deepseek-v4-pro", LoginDeepSeekAPIKeyGuidance, true},
		{"GitHub Copilot import", "GitHub Copilot", "Existing Copilot CLI account import", "Record the Copilot identity from local Copilot CLI state, then launch with --provider copilot.", "godex profile import copilot --activate", LoginCopilotImport, false},
	}
}
