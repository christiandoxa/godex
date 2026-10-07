package ping

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	pingmodel "github.com/christiandoxa/godex/internal/model/ping"
	pingusecase "github.com/christiandoxa/godex/internal/usecase/ping"
)

var errPingSelectionCancelled = errors.New("Prodex Super prompt cancelled")

type pingChoiceModel struct {
	title     string
	choices   []string
	selected  int
	result    int
	cancelled bool
}

func (model pingChoiceModel) Init() tea.Cmd { return nil }

func (model pingChoiceModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyMsg)
	if !ok {
		return model, nil
	}
	switch key.String() {
	case "up", "k":
		if model.selected == 0 {
			model.selected = len(model.choices) - 1
		} else {
			model.selected--
		}
	case "down", "j":
		model.selected = (model.selected + 1) % len(model.choices)
	case "pgup":
		model.selected = max(0, model.selected-10)
	case "pgdown":
		model.selected = min(len(model.choices)-1, model.selected+10)
	case "home":
		model.selected = 0
	case "end":
		model.selected = len(model.choices) - 1
	case "enter":
		model.result = model.selected
		return model, tea.Quit
	case "esc", "ctrl+c", "ctrl+z":
		model.cancelled = true
		return model, tea.Quit
	}
	return model, nil
}

func (model pingChoiceModel) View() string {
	var output strings.Builder
	fmt.Fprintf(&output, "Godex Ping  %s\n\n", model.title)
	for index, choice := range model.choices {
		prefix := "  "
		if index == model.selected {
			prefix = "› "
		}
		fmt.Fprintf(&output, "%s%s\n", prefix, choice)
	}
	output.WriteString("\n↑/↓ choose  enter select  esc cancel")
	return output.String()
}

func pingShouldPromptSelection(json, interactive bool) bool { return !json && interactive }

func pingInteractiveTerminal() bool {
	return terminalFile(os.Stdin) && terminalFile(os.Stderr)
}

func terminalFile(file *os.File) bool { return file != nil && term.IsTerminal(int(file.Fd())) }

func promptPingOptions(ctx context.Context, options pingmodel.Options) (pingmodel.Options, error) {
	if options.Model == "" {
		labels, models := pingusecase.OpenAIModelChoices()
		selected, err := promptPingChoice(ctx, "OpenAI ping model", labels, 0)
		if err != nil {
			return pingmodel.Options{}, err
		}
		options.Model = models[selected]
	}
	if options.Effort == "" {
		choices := append([]string{"provider default"}, pingusecase.OpenAIEffortChoices(options.Model)...)
		selected, err := promptPingChoice(ctx, "OpenAI ping reasoning effort", choices, 0)
		if err != nil {
			return pingmodel.Options{}, err
		}
		if selected > 0 {
			options.Effort = choices[selected]
		}
	}
	return options, nil
}

func promptPingChoice(ctx context.Context, title string, choices []string, selected int) (int, error) {
	if len(choices) == 0 {
		return 0, errors.New("ping prompt has no choices")
	}
	model := pingChoiceModel{title: title, choices: append([]string(nil), choices...), selected: min(max(0, selected), len(choices)-1)}
	program := tea.NewProgram(
		model,
		tea.WithInput(os.Stdin),
		tea.WithOutput(os.Stderr),
		tea.WithAltScreen(),
		tea.WithContext(ctx),
	)
	result, err := program.Run()
	if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if err != nil {
		return 0, fmt.Errorf("ping prompt TUI failed: %w", err)
	}
	state, ok := result.(pingChoiceModel)
	if !ok {
		return 0, errors.New("ping prompt returned invalid state")
	}
	if state.cancelled {
		return 0, errPingSelectionCancelled
	}
	return state.result, nil
}
