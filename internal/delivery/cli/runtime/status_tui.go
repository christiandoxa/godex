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

type statusTickMsg time.Time

type statusSnapshotMsg struct {
	overview  runtimemodel.Overview
	resources statusResourceSnapshot
	err       error
}

type statusTUIModel struct {
	ctx       context.Context
	activity  *runtimeusecase.Activity
	resources *statusResourceTracker
	interval  time.Duration
	overview  *runtimemodel.Overview
	resource  statusResourceSnapshot
	err       error
	loading   bool
}

func newStatusTUIModel(ctx context.Context, activity *runtimeusecase.Activity, interval time.Duration) statusTUIModel {
	return statusTUIModel{
		ctx: ctx, activity: activity, resources: newStatusResourceTracker(),
		interval: interval, loading: true,
	}
}

func (model statusTUIModel) Init() tea.Cmd {
	return tea.Batch(model.fetch(), statusTick(model.interval))
}

func (model statusTUIModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyMsg:
		switch strings.ToLower(message.String()) {
		case "q", "esc", "ctrl+c", "ctrl+z":
			return model, tea.Quit
		case "r":
			if model.loading {
				return model, nil
			}
			model.loading = true
			return model, model.fetch()
		}
	case statusSnapshotMsg:
		model.loading = false
		model.err = message.err
		if message.err == nil {
			overview := message.overview
			model.overview = &overview
			model.resource = message.resources
		}
	case statusTickMsg:
		if model.loading {
			return model, statusTick(model.interval)
		}
		model.loading = true
		return model, tea.Batch(model.fetch(), statusTick(model.interval))
	}
	return model, nil
}

func (model statusTUIModel) View() string {
	var output strings.Builder
	output.WriteString("Godex Status\n\n")
	if model.overview != nil {
		output.WriteString("Updated: ")
		output.WriteString(time.Now().Format("2006-01-02 15:04:05"))
		output.WriteByte('\n')
		for _, field := range statusFields(*model.overview, model.resource) {
			_, _ = fmt.Fprintf(&output, "%s: %s\n", field[0], field[1])
		}
	} else if model.loading {
		output.WriteString("Loading runtime status...\n")
	}
	if model.err != nil {
		output.WriteString("\nStatus refresh failed: ")
		output.WriteString(model.err.Error())
		output.WriteByte('\n')
	}
	output.WriteString("\nq/esc quit • r refresh")
	return output.String()
}

func (model statusTUIModel) fetch() tea.Cmd {
	return func() tea.Msg {
		overview, err := model.activity.Overview(model.ctx)
		resources := model.resources.sample()
		return statusSnapshotMsg{overview: overview, resources: resources, err: err}
	}
}

func statusTick(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(at time.Time) tea.Msg { return statusTickMsg(at) })
}

func runStatusTUI(ctx context.Context, activity *runtimeusecase.Activity, out io.Writer, interval time.Duration) error {
	program := tea.NewProgram(
		newStatusTUIModel(ctx, activity, interval),
		tea.WithInput(os.Stdin),
		tea.WithOutput(out),
		tea.WithAltScreen(),
		tea.WithContext(ctx),
	)
	_, err := program.Run()
	if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
