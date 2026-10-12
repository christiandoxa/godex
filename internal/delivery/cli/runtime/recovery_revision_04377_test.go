package runtime

import (
	"context"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	"os"
	"path/filepath"
	"testing"
)

func TestProdex04377StaleUsageRevisionCannotRetriggerAfterNativeActivity(t *testing.T) {
	id := recoverySession04377
	path := filepath.Join(t.TempDir(), "rollout-"+id+".jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"session_meta","payload":{"id":"`+id+`"}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	marker := &sessionStartMarker04360{directory: filepath.Dir(path), path: filepath.Join(filepath.Dir(path), "session.id")}
	if err := os.WriteFile(marker.path, []byte(id+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := runtimemodel.GoalRecoveryState{SessionID: id, Status: "blocked", UpdatedAt: 10}
	monitor := &liveGoalRecovery04361{marker: marker, read: func(context.Context, string) (runtimemodel.GoalRecoveryState, bool) { return state, true }, resolve: func(context.Context, string) (sessionmodel.Report, error) {
		return sessionmodel.Report{ID: id, Path: path}, nil
	}}
	if _, ok := monitor.readState(t.Context()); !ok {
		t.Fatal("initial state unavailable")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(`{"type":"event_msg","payload":{"type":"user_message"}}` + "\n" + `{"type":"event_msg","payload":{"type":"task_complete","error":{"codex_error_info":"server_overloaded"}}}` + "\n")
	if err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if got, ok := monitor.readState(t.Context()); !ok || got.Status != "usage_limited" {
		t.Fatalf("failure did not arm: %#v %t", got, ok)
	}
	state.Status, state.UpdatedAt = "active", 11
	if got, ok := monitor.readState(t.Context()); !ok || got.Status != "active" {
		t.Fatalf("new revision not observed: %#v %t", got, ok)
	}
	state.Status = "usage_limited"
	if got, ok := monitor.readState(t.Context()); !ok || got.Status != "blocked" {
		t.Fatalf("stale usage row retriggered: %#v %t", got, ok)
	}
}
