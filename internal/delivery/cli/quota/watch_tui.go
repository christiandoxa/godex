package quota

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type quotaTickMsg time.Time

type quotaSnapshotMsg struct {
	reports []quotamodel.Report
	err     error
}

type quotaTUIModel struct {
	ctx     context.Context
	status  statusRunner
	options showOptions
	reports []quotamodel.Report
	err     error
	updated time.Time
	loading bool
}

func newQuotaTUIModel(ctx context.Context, status statusRunner, options showOptions) quotaTUIModel {
	return quotaTUIModel{ctx: ctx, status: status, options: options, loading: true}
}

func (model quotaTUIModel) Init() tea.Cmd {
	return tea.Batch(model.fetch(), quotaTick())
}

func (model quotaTUIModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyMsg:
		switch strings.ToLower(message.String()) {
		case "q", "esc", "ctrl+c", "ctrl+z":
			return model, tea.Quit
		case "u", "r":
			if model.loading {
				return model, nil
			}
			model.loading = true
			return model, model.fetch()
		}
	case quotaSnapshotMsg:
		model.loading = false
		model.err = message.err
		if message.err == nil {
			model.reports = append([]quotamodel.Report(nil), message.reports...)
			model.updated = time.Now()
		}
	case quotaTickMsg:
		if model.loading {
			return model, quotaTick()
		}
		model.loading = true
		return model, tea.Batch(model.fetch(), quotaTick())
	}
	return model, nil
}

func (model quotaTUIModel) View() string {
	var output strings.Builder
	output.WriteString("Godex Quota\n")
	if !model.updated.IsZero() {
		output.WriteString("Updated: ")
		output.WriteString(model.updated.Format("2006-01-02 15:04:05"))
		output.WriteByte('\n')
	}
	output.WriteByte('\n')
	if len(model.reports) > 0 {
		_ = writeQuotaReports(&output, model.reports, model.options.detail)
	} else if model.loading {
		output.WriteString("Loading quota data...\n")
	}
	if model.err != nil {
		output.WriteString("\nQuota refresh failed; showing last successful snapshot when available.\n")
	}
	output.WriteString("\nq/esc quit • u/r refresh")
	return output.String()
}

func (model quotaTUIModel) fetch() tea.Cmd {
	return func() tea.Msg {
		reports, err := model.status.Run(model.ctx, model.options.Options)
		return quotaSnapshotMsg{reports: reports, err: err}
	}
}

func quotaTick() tea.Cmd {
	return tea.Tick(quotaWatchInterval, func(at time.Time) tea.Msg { return quotaTickMsg(at) })
}

func runQuotaTUI(ctx context.Context, status statusRunner, out io.Writer, options showOptions) error {
	program := tea.NewProgram(
		newQuotaTUIModel(ctx, status, options),
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
		return fmt.Errorf("quota TUI failed: %w", err)
	}
	return nil
}

func quotaTerminalWriter(out io.Writer) bool {
	file, ok := out.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
