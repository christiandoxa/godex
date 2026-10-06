package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

func TestStatusTUIViewAndKeys(t *testing.T) {
	model := newStatusTUIModel(context.Background(), newCLIActivity(), time.Second)
	updated, command := model.Update(statusSnapshotMsg{overview: mustOverview(t), resources: statusResourceSnapshot{available: true, processCount: 2, runtimeProcessCount: 1}})
	model = updated.(statusTUIModel)
	if command != nil {
		t.Fatal("snapshot unexpectedly returned command")
	}
	view := model.View()
	for _, expected := range []string{"Godex Status", "Active profile: work", "q/esc quit", "r refresh"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("view missing %q: %q", expected, view)
		}
	}
	_, quit := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if quit == nil {
		t.Fatal("q did not return quit command")
	}
	refreshed, refresh := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if refresh == nil || !refreshed.(statusTUIModel).loading {
		t.Fatal("r did not trigger refresh")
	}
}

func mustOverview(t *testing.T) runtimemodel.Overview {
	t.Helper()
	overview, err := newCLIActivity().Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return overview
}
