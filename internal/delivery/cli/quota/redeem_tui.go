package quota

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type redeemPromptModel struct {
	profile   string
	label     string
	resetTime string
	confirmed bool
}

func (model redeemPromptModel) Init() tea.Cmd { return nil }

func (model redeemPromptModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyMsg)
	if !ok {
		return model, nil
	}
	switch strings.ToLower(key.String()) {
	case "y":
		model.confirmed = true
		return model, tea.Quit
	case "n", "enter", "esc", "ctrl+c", "ctrl+z":
		model.confirmed = false
		return model, tea.Quit
	default:
		return model, nil
	}
}

func (model redeemPromptModel) View() string {
	return fmt.Sprintf(
		"Godex Redeem\n\nQuota reset is near.\n\nProfile %q has a %s reset near at %s.\nRedeem one reset credit anyway?\n\ny redeem • n/enter/esc cancel",
		model.profile,
		model.label,
		model.resetTime,
	)
}

func runRedeemPromptTUI(ctx context.Context, in, out *os.File, profile, label, resetTime string) (bool, error) {
	program := tea.NewProgram(
		redeemPromptModel{profile: profile, label: label, resetTime: resetTime},
		tea.WithInput(in),
		tea.WithOutput(out),
		tea.WithAltScreen(),
		tea.WithContext(ctx),
	)
	result, err := program.Run()
	if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
		return false, ctx.Err()
	}
	if err != nil {
		return false, fmt.Errorf("redeem prompt TUI failed: %w", err)
	}
	model, ok := result.(redeemPromptModel)
	if !ok {
		return false, errors.New("redeem prompt returned invalid state")
	}
	return model.confirmed, nil
}

func redeemTerminalIO(in io.Reader, out io.Writer) (*os.File, *os.File, bool) {
	input, inputOK := in.(*os.File)
	output, outputOK := out.(*os.File)
	if !inputOK || !outputOK || !terminalFile(input) || !terminalFile(output) {
		return nil, nil, false
	}
	return input, output, true
}
