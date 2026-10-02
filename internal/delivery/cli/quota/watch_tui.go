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
	ctx                  context.Context
	status               statusRunner
	options              showOptions
	reports              []quotamodel.Report
	err                  error
	updated              time.Time
	loading              bool
	height               int
	scrollOffset         int
	sortMode             quotaReportSort
	providerFilter       quotaProviderFilter
	providerFilterLocked bool
}

func newQuotaTUIModel(ctx context.Context, status statusRunner, options showOptions) quotaTUIModel {
	filter := quotaProviderFilterFromString(options.ProviderFilter)
	return quotaTUIModel{
		ctx: ctx, status: status, options: options, loading: true, height: 24,
		sortMode: quotaSortCurrent, providerFilter: filter,
		providerFilterLocked: options.ProviderFilter != "" && filter != quotaProviderAll,
	}
}

func (model quotaTUIModel) Init() tea.Cmd {
	return tea.Batch(model.fetch(), quotaTick())
}

func (model quotaTUIModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.height = max(6, message.Height)
		model.clampScroll()
	case tea.KeyMsg:
		return model.updateKey(message)
	case quotaSnapshotMsg:
		model.loading = false
		model.err = message.err
		if message.err == nil {
			model.reports = append([]quotamodel.Report(nil), message.reports...)
			model.updated = time.Now()
			model.clampScroll()
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

func (model quotaTUIModel) updateKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	value := strings.ToLower(key.String())
	if quotaQuitKey(value) {
		return model, tea.Quit
	}
	if !model.options.All {
		return model, nil
	}
	switch value {
	case "j", "down":
		model.scrollOffset = min(model.scrollOffset+1, model.maxScrollOffset())
	case "k", "up":
		model.scrollOffset = max(0, model.scrollOffset-1)
	case "s":
		model.sortMode = model.sortMode.next()
		model.scrollOffset = 0
	case "f":
		if model.providerFilterLocked {
			return model, nil
		}
		model.providerFilter = model.providerFilter.next()
		model.scrollOffset = 0
		if model.loading {
			return model, nil
		}
		model.loading = true
		return model, model.fetch()
	case "u":
		if model.loading {
			return model, nil
		}
		model.loading = true
		return model, model.fetch()
	}
	return model, nil
}

func quotaQuitKey(value string) bool {
	switch value {
	case "q", "esc", "ctrl+c", "ctrl+z":
		return true
	default:
		return false
	}
}

func (model quotaTUIModel) View() string {
	var output strings.Builder
	output.WriteString("Godex Quota\n")
	if model.options.All {
		_, _ = fmt.Fprintf(&output, "Sort: %s • Provider: %s\n", model.sortMode.label(), model.providerFilter.label())
		model.writePoolSummary(&output)
	} else if !model.updated.IsZero() {
		output.WriteString("Updated: ")
		output.WriteString(model.updated.Format("2006-01-02 15:04:05"))
		output.WriteByte('\n')
	}
	output.WriteByte('\n')
	model.writeQuotaBody(&output)
	if model.err != nil {
		output.WriteString("\nQuota refresh failed; showing last successful snapshot when available.\n")
	}
	output.WriteString("\n")
	output.WriteString(model.footer())
	return output.String()
}

func (model quotaTUIModel) writePoolSummary(output *strings.Builder) {
	output.WriteString("\nQuota Overview\n")
	for _, field := range quotaPoolSummaryFields(model.reports, model.updated) {
		_, _ = fmt.Fprintf(output, "%s: %s\n", field.label, field.value)
	}
}

func (model quotaTUIModel) writeQuotaBody(output *strings.Builder) {
	if len(model.reports) == 0 {
		if model.loading {
			output.WriteString("Loading quota data...\n")
		}
		return
	}
	if !model.options.All {
		_ = writeQuotaReports(output, model.reports, model.options.detail)
		return
	}
	reports := model.sortedReports()
	visible := model.visibleReportCount()
	start := min(model.scrollOffset, len(reports))
	end := min(start+visible, len(reports))
	_ = writeQuotaReports(output, reports[start:end], model.options.detail)
	if len(reports) > visible {
		_, _ = fmt.Fprintf(output, "Showing %d-%d of %d profiles\n", min(start+1, len(reports)), end, len(reports))
	}
}

func (model quotaTUIModel) footer() string {
	if !model.options.All {
		return "q/esc quit"
	}
	filterControl := " • f filter"
	if model.providerFilterLocked {
		filterControl = " • filter locked"
	}
	return "q/esc quit • j/k scroll • s sort" + filterControl + " • u refresh"
}

func (model quotaTUIModel) sortedReports() []quotamodel.Report {
	return quotaSortedReports(model.reports, model.providerFilter, model.sortMode)
}

func (model quotaTUIModel) visibleReportCount() int {
	reserved := 8
	if model.options.All {
		reserved += len(quotaPoolSummaryFields(model.reports, model.updated)) + 2
	}
	return max(1, model.height-reserved)
}

func (model quotaTUIModel) maxScrollOffset() int {
	return max(0, len(model.sortedReports())-model.visibleReportCount())
}

func (model *quotaTUIModel) clampScroll() {
	model.scrollOffset = min(max(0, model.scrollOffset), model.maxScrollOffset())
}

func (model quotaTUIModel) fetch() tea.Cmd {
	return func() tea.Msg {
		options := model.options.Options
		if model.options.All {
			options.ProviderFilter = model.providerFilter.label()
		}
		reports, err := model.status.Run(model.ctx, options)
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
