package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

type sessionTUIModel struct {
	reports     []sessionmodel.Report
	offset      int
	height      int
	interactive bool
}

func newSessionTUIModel(reports []sessionmodel.Report, interactive bool, height int) sessionTUIModel {
	return sessionTUIModel{reports: append([]sessionmodel.Report(nil), reports...), interactive: interactive, height: max(8, height)}
}

func (model sessionTUIModel) Init() tea.Cmd {
	if model.interactive {
		return nil
	}
	return tea.Quit
}

func (model sessionTUIModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := message.(tea.WindowSizeMsg); ok {
		model.height = max(8, size.Height)
		model.clampOffset()
		return model, nil
	}
	key, ok := message.(tea.KeyMsg)
	if !ok || !model.interactive {
		return model, nil
	}
	visible := model.visibleRows()
	switch strings.ToLower(key.String()) {
	case "q", "esc", "enter", "ctrl+c", "ctrl+z":
		return model, tea.Quit
	case "j", "down":
		model.offset++
	case "k", "up":
		model.offset--
	case "pgdown":
		model.offset += visible
	case "pgup":
		model.offset -= visible
	case "home":
		model.offset = 0
	case "end":
		model.offset = model.maxOffset()
	}
	model.clampOffset()
	return model, nil
}

func (model sessionTUIModel) View() string {
	lines := sessionTUILines(model.reports)
	visible := model.visibleRows()
	start := min(max(0, model.offset), max(0, len(lines)-visible))
	end := min(len(lines), start+visible)
	var output strings.Builder
	_, _ = fmt.Fprintf(&output, "Godex Sessions  %d session(s)\n\n", len(model.reports))
	if len(lines) == 0 {
		output.WriteString("No sessions found.\n")
	} else {
		for _, line := range lines[start:end] {
			output.WriteString(line)
			output.WriteByte('\n')
		}
	}
	if model.interactive {
		if model.maxOffset() == 0 {
			output.WriteString("\nq / Esc / Enter to exit")
		} else {
			_, _ = fmt.Fprintf(&output, "\nlines %d-%d of %d — j/k/Up/Down/PgUp/PgDn/Home/End to scroll, q/Esc/Enter to exit", start+1, end, len(lines))
		}
	} else {
		output.WriteString("\nuse `godex session resume <session-id>` to resume")
	}
	return output.String()
}

func (model sessionTUIModel) visibleRows() int { return max(1, model.height-5) }

func (model sessionTUIModel) maxOffset() int {
	return max(0, len(sessionTUILines(model.reports))-model.visibleRows())
}

func (model *sessionTUIModel) clampOffset() {
	model.offset = min(max(0, model.offset), model.maxOffset())
}

func sessionTUILines(reports []sessionmodel.Report) []string {
	lines := make([]string, 0, len(reports)*4)
	for _, report := range reports {
		title := displayField(report.ThreadName)
		if strings.TrimSpace(title) == "" {
			title = "Untitled session"
		}
		lines = append(lines,
			fmt.Sprintf("%s  %s", sessionTUIField(report.ID), title),
			fmt.Sprintf("updated %s  profile %s  provider %s", sessionTUIField(report.UpdatedAt), sessionTUIField(report.Profile), sessionTUIField(report.ModelProvider)),
			"cwd "+sessionTUIField(report.CWD),
			"",
		)
	}
	return lines
}

func sessionTUIField(value string) string {
	value = displayField(value)
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func sessionTUIEnabled(out io.Writer, options listOptions) bool {
	return !options.json && !options.idOnly && !options.resumeCommand && sessionTerminal(os.Stdin) && sessionTerminal(out)
}

func runSessionTUI(ctx context.Context, out io.Writer, reports []sessionmodel.Report) error {
	height := sessionTerminalHeight(out)
	interactive := len(sessionTUILines(reports))+6 > height
	options := []tea.ProgramOption{tea.WithInput(nil), tea.WithOutput(out), tea.WithContext(ctx)}
	if interactive {
		options[0] = tea.WithInput(os.Stdin)
		options = append(options, tea.WithAltScreen())
	} else {
		options = append(options, tea.WithoutSignalHandler())
	}
	_, err := tea.NewProgram(newSessionTUIModel(reports, interactive, height), options...).Run()
	if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func sessionTerminal(value io.Writer) bool {
	file, ok := value.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

func sessionTerminalHeight(out io.Writer) int {
	file, ok := out.(*os.File)
	if !ok {
		return 24
	}
	_, height, err := term.GetSize(int(file.Fd()))
	if err != nil || height <= 0 {
		return 24
	}
	return height
}
