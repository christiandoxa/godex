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
	width                int
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
		model.width = max(1, message.Width)
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
	// Reserve the footer and an optional refresh warning, then let the
	// tagged 0.437.9 page policy fit complete profile sections.
	bodyBudget := model.height - strings.Count(output.String(), "\n") - 2
	if model.err != nil {
		bodyBudget -= 2
	}
	model.writeQuotaBody(&output, max(0, bodyBudget))
	if model.err != nil {
		output.WriteString("\nQuota refresh failed; showing last successful snapshot when available.\n")
	}
	output.WriteString("\n")
	output.WriteString(model.footer())
	return clipQuotaTUIView04379(wrapQuotaText(output.String(), model.width), model.height)
}

func (model quotaTUIModel) writePoolSummary(output *strings.Builder) {
	output.WriteString("\nQuota Overview\n")
	for _, field := range quotaPoolSummaryFields(model.reports, model.updated) {
		_, _ = fmt.Fprintf(output, "%s: %s\n", field.label, field.value)
	}
}

func (model quotaTUIModel) writeQuotaBody(output *strings.Builder, budget int) {
	if len(model.reports) == 0 {
		if model.loading && budget > 0 {
			output.WriteString("Loading quota data...\n")
		}
		return
	}
	if !model.options.All {
		_ = writeQuotaReports(output, model.reports, model.options.detail)
		return
	}
	if budget < 1 {
		return
	}
	reports := model.sortedReports()
	start := min(max(0, model.scrollOffset), len(reports)-1)
	var header strings.Builder
	_ = writeQuotaReports(&header, nil, model.options.detail)
	text := header.String()
	text = wrapQuotaText(text, model.width)
	lines := strings.Count(text, "\n")
	if lines > budget {
		return
	}
	output.WriteString(text)
	shown := 0
	for _, report := range reports[start:] {
		var section strings.Builder
		_ = writeReport(&section, report, model.options.detail)
		text := wrapQuotaText(section.String(), model.width)
		sectionLines := strings.Count(text, "\n")
		plan, ok := planQuotaPage04379(len(reports), start, shown, lines, sectionLines, &budget)
		if !ok || !plan.appendSection {
			break
		}
		output.WriteString(text)
		lines += sectionLines
		shown++
	}
	page, _ := planQuotaPage04379(len(reports), start, shown, lines, 0, &budget)
	if page.showNotice {
		if shown > 0 {
			_, _ = fmt.Fprintf(output, "Showing %d-%d of %d profiles\n",
				start+1, start+shown, len(reports))
		} else {
			_, _ = fmt.Fprintf(output, "No profile rows fit; %d above, %d below\n",
				page.hiddenBefore, page.hiddenAfter)
		}
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

func (model quotaTUIModel) maxScrollOffset() int {
	// The tagged quota page plan permits starting at the final profile
	// even when all other sections would fit in a taller viewport.
	return max(0, len(model.sortedReports())-1)
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
