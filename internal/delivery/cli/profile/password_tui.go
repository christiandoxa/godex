package profile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

type passwordModeModel struct {
	protect bool
	done    bool
}

func (model passwordModeModel) Init() tea.Cmd { return nil }

func (model passwordModeModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyMsg)
	if !ok {
		return model, nil
	}
	switch strings.ToLower(key.String()) {
	case "y", "enter":
		model.protect, model.done = true, true
		return model, tea.Quit
	case "n", "esc", "ctrl+c", "ctrl+z":
		model.protect, model.done = false, true
		return model, tea.Quit
	default:
		return model, nil
	}
}

func (model passwordModeModel) View() string {
	return "Profile Export  bundle protection\n\n" +
		"Password-protect export file containing profile tokens?\n\n" +
		"Protected bundles require a password to import. Unprotected bundles are plain JSON and may contain reusable credentials.\n\n" +
		"y/enter protect • n/esc unprotected"
}

type passwordEntryModel struct {
	title     string
	label     string
	detail    string
	password  []rune
	accepted  bool
	cancelled bool
}

func (model passwordEntryModel) Init() tea.Cmd { return nil }

func (model passwordEntryModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyMsg)
	if !ok {
		return model, nil
	}
	switch strings.ToLower(key.String()) {
	case "enter":
		model.accepted = true
		return model, tea.Quit
	case "esc", "ctrl+c", "ctrl+z":
		model.cancelled = true
		return model, tea.Quit
	case "backspace":
		if len(model.password) > 0 {
			model.password[len(model.password)-1] = 0
			model.password = model.password[:len(model.password)-1]
		}
	default:
		if key.Type == tea.KeyRunes {
			model.password = append(model.password, key.Runes...)
		}
	}
	return model, nil
}

func (model passwordEntryModel) View() string {
	return fmt.Sprintf(
		"%s  %s\n\n%s\n\n%s\n\n> %s_\n\nenter accept • backspace delete • esc cancel",
		model.title,
		model.label,
		model.label,
		model.detail,
		strings.Repeat("*", len(model.password)),
	)
}

func runPasswordModeTUI(ctx context.Context) (bool, error) {
	result, err := runProfileTUI(ctx, passwordModeModel{})
	if err != nil {
		return false, err
	}
	model, ok := result.(passwordModeModel)
	if !ok || !model.done {
		return false, errors.New("profile export protection prompt returned invalid state")
	}
	return model.protect, nil
}

func runPasswordEntryTUI(ctx context.Context, title, label, detail string) (string, error) {
	result, err := runProfileTUI(ctx, passwordEntryModel{title: title, label: label, detail: detail})
	if err != nil {
		return "", err
	}
	model, ok := result.(passwordEntryModel)
	if !ok {
		return "", errors.New("profile password prompt returned invalid state")
	}
	if model.cancelled {
		clearPasswordRunes(model.password)
		return "", errors.New("profile password input cancelled")
	}
	if !model.accepted {
		clearPasswordRunes(model.password)
		return "", errors.New("profile password prompt did not complete")
	}
	value := string(model.password)
	clearPasswordRunes(model.password)
	return value, nil
}

func runProfileTUI(ctx context.Context, model tea.Model) (tea.Model, error) {
	program := tea.NewProgram(
		model,
		tea.WithInput(os.Stdin),
		tea.WithOutput(os.Stderr),
		tea.WithAltScreen(),
		tea.WithContext(ctx),
	)
	result, err := program.Run()
	if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("profile prompt TUI failed: %w", err)
	}
	return result, nil
}

func profilePasswordTUIAvailable() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stderr.Fd()))
}

func clearPasswordRunes(value []rune) {
	for index := range value {
		value[index] = 0
	}
}
