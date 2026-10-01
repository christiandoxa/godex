package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

type logTickMsg time.Time

type logSnapshotMsg struct {
	events []runtimemodel.Event
	err    error
}

type logTUIModel struct {
	ctx              context.Context
	activity         *runtimeusecase.Activity
	options          logOptions
	events           []runtimemodel.Event
	err              error
	search           string
	editingSearch    bool
	scrollFromBottom int
	height           int
	loading          bool
}

func newLogTUIModel(ctx context.Context, activity *runtimeusecase.Activity, options logOptions) logTUIModel {
	return logTUIModel{ctx: ctx, activity: activity, options: options, height: 24, loading: true}
}

func (model logTUIModel) Init() tea.Cmd {
	return tea.Batch(model.fetch(), logTick())
}

func (model logTUIModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.height = max(6, message.Height)
	case tea.KeyMsg:
		return model.updateKey(message)
	case logSnapshotMsg:
		model.loading = false
		model.err = message.err
		if message.err == nil {
			model.events = append([]runtimemodel.Event(nil), message.events...)
			model.clampScroll()
		}
	case logTickMsg:
		if model.loading {
			return model, logTick()
		}
		model.loading = true
		return model, tea.Batch(model.fetch(), logTick())
	}
	return model, nil
}

func (model logTUIModel) updateKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	value := strings.ToLower(key.String())
	if model.editingSearch {
		switch value {
		case "enter", "esc":
			model.editingSearch = false
		case "backspace":
			runes := []rune(model.search)
			if len(runes) > 0 {
				model.search = string(runes[:len(runes)-1])
			}
			model.scrollFromBottom = 0
		case "ctrl+u":
			model.search = ""
			model.scrollFromBottom = 0
		default:
			if key.Type == tea.KeyRunes {
				model.search += string(key.Runes)
				model.scrollFromBottom = 0
			}
		}
		return model, nil
	}

	switch value {
	case "q", "esc", "ctrl+c", "ctrl+z":
		return model, tea.Quit
	case "/":
		model.search = ""
		model.editingSearch = true
		model.scrollFromBottom = 0
	case "c":
		model.search = ""
		model.scrollFromBottom = 0
	case "up", "k":
		model.scrollFromBottom++
	case "down", "j":
		model.scrollFromBottom = max(0, model.scrollFromBottom-1)
	case "pgup":
		model.scrollFromBottom += 10
	case "pgdown":
		model.scrollFromBottom = max(0, model.scrollFromBottom-10)
	case "home":
		model.scrollFromBottom = len(model.visibleEvents())
	case "end":
		model.scrollFromBottom = 0
	case "u", "r":
		if model.loading {
			return model, nil
		}
		model.loading = true
		return model, model.fetch()
	}
	model.clampScroll()
	return model, nil
}

func (model logTUIModel) View() string {
	events := model.visibleEvents()
	visible := max(1, model.height-5)
	maxOffset := max(0, len(events)-visible)
	offset := min(model.scrollFromBottom, maxOffset)
	end := len(events) - offset
	start := max(0, end-visible)

	var output strings.Builder
	_, _ = fmt.Fprintf(&output, "Godex Log  mode=%s  events=%d\n\n", model.options.mode, len(events))
	if len(events) == 0 {
		output.WriteString("No matching runtime events.\n")
	} else {
		for _, event := range events[start:end] {
			output.WriteString(formatLogEvent(event))
			output.WriteByte('\n')
		}
	}
	if model.err != nil {
		output.WriteString("\nLog refresh failed; showing last successful snapshot.\n")
	}
	output.WriteByte('\n')
	if model.editingSearch {
		_, _ = fmt.Fprintf(&output, "q quit • ↑/↓ scroll • search: /%s_", model.search)
	} else if strings.TrimSpace(model.search) != "" {
		_, _ = fmt.Fprintf(&output, "q quit • ↑/↓ scroll • / search • search: /%s (c clear)", strings.TrimSpace(model.search))
	} else {
		output.WriteString("q/esc quit • ↑/↓ scroll • PgUp/PgDn • Home/End • / search • u/r refresh")
	}
	return output.String()
}

func (model logTUIModel) fetch() tea.Cmd {
	return func() tea.Msg {
		events, err := model.activity.Events(model.ctx, 256)
		if err == nil {
			events = filterLogEvents(events, model.options.mode)
		}
		return logSnapshotMsg{events: events, err: err}
	}
}

func (model logTUIModel) visibleEvents() []runtimemodel.Event {
	query := strings.ToLower(strings.TrimSpace(model.search))
	if query == "" {
		return model.events
	}
	filtered := make([]runtimemodel.Event, 0, len(model.events))
	for _, event := range model.events {
		if strings.Contains(strings.ToLower(formatLogEvent(event)), query) {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

func (model *logTUIModel) clampScroll() {
	visible := max(1, model.height-5)
	maximum := max(0, len(model.visibleEvents())-visible)
	model.scrollFromBottom = min(model.scrollFromBottom, maximum)
}

func logTick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(at time.Time) tea.Msg { return logTickMsg(at) })
}

func runLogTUI(ctx context.Context, activity *runtimeusecase.Activity, out io.Writer, options logOptions) error {
	program := tea.NewProgram(
		newLogTUIModel(ctx, activity, options),
		tea.WithInput(os.Stdin),
		tea.WithOutput(out),
		tea.WithAltScreen(),
		tea.WithContext(ctx),
	)
	_, err := program.Run()
	if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("log TUI failed: %w", err)
	}
	return nil
}
