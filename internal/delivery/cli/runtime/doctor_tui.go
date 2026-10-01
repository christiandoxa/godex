package runtime

import (
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type doctorTUIModel struct {
	panels []doctorPanel
}

func (model doctorTUIModel) Init() tea.Cmd { return tea.Quit }

func (model doctorTUIModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return model, nil }

func (model doctorTUIModel) View() string {
	var output strings.Builder
	_, _ = fmt.Fprintf(&output, "Godex Doctor  %d panel(s)\n\n", len(model.panels))
	for index, panel := range model.panels {
		if index > 0 {
			output.WriteByte('\n')
		}
		output.WriteString(panel.title)
		output.WriteByte('\n')
		for _, field := range panel.fields {
			_, _ = fmt.Fprintf(&output, "%-20s %s\n", field[0], field[1])
		}
	}
	return output.String()
}

func runDoctorTUI(out io.Writer, panels []doctorPanel) error {
	_, err := tea.NewProgram(
		doctorTUIModel{panels: panels},
		tea.WithInput(nil),
		tea.WithOutput(out),
		tea.WithoutSignalHandler(),
	).Run()
	return err
}
