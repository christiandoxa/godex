package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

const recoverySession04377 = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"

func TestProdex04377LiveTailUsesLatestExactSessionRecoveryEvidence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		fragment string
		want     string
	}{
		{
			name: "native capacity failure remains eligible",
			fragment: `{"type":"event_msg","payload":{"type":"user_message"}}` + "\n" +
				`{"type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"done","output":"saved"}}` + "\n" +
				`{"type":"event_msg","payload":{"type":"task_complete","error":{"message":"Selected model is at capacity.","codex_error_info":"server_overloaded"}}}` + "\n",
			want: "overload",
		},
		{
			name: "later native activity clears failure",
			fragment: `{"type":"event_msg","payload":{"type":"user_message"}}` + "\n" +
				`{"type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"done","output":"saved"}}` + "\n" +
				`{"type":"event_msg","payload":{"type":"task_complete","error":{"codex_error_info":"server_overloaded"}}}` + "\n" +
				`{"type":"event_msg","payload":{"type":"task_started"}}` + "\n" +
				`{"type":"event_msg","payload":{"type":"user_message"}}` + "\n",
		},
		{
			name: "later terminal without acceptance clears failure",
			fragment: `{"type":"event_msg","payload":{"type":"user_message"}}` + "\n" +
				`{"type":"event_msg","payload":{"type":"task_complete","error":{"codex_error_info":"server_overloaded"}}}` + "\n" +
				`{"type":"event_msg","payload":{"type":"task_started"}}` + "\n" +
				`{"type":"event_msg","payload":{"type":"task_complete"}}` + "\n",
		},
		{
			name: "foreign session cannot clear failure",
			fragment: `{"type":"event_msg","payload":{"type":"user_message"}}` + "\n" +
				`{"type":"event_msg","payload":{"type":"task_complete","error":{"codex_error_info":"server_overloaded"}}}` + "\n" +
				`{"type":"event_msg","session_id":"00000000-0000-4000-8000-000000000002","payload":{"type":"task_started"}}` + "\n",
			want: "overload",
		},
		{
			name: "intermediate retry error cannot arm recovery",
			fragment: `{"type":"event_msg","payload":{"type":"user_message"}}` + "\n" +
				`{"type":"event_msg","payload":{"type":"error","will_retry":true,"codex_error_info":"server_overloaded"}}` + "\n",
		},
		{
			name: "successful completion clears acceptance",
			fragment: `{"type":"event_msg","payload":{"type":"user_message"}}` + "\n" +
				`{"type":"event_msg","payload":{"type":"task_complete"}}` + "\n" +
				`{"type":"error","error":{"code":"usage_limit_reached"}}` + "\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "rollout-"+recoverySession04377+".jsonl")
			if err := os.WriteFile(path,
				[]byte(`{"type":"session_meta","payload":{"id":"`+recoverySession04377+`"}}`+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			checkpoint := captureRecoveryCheckpoint04360(path)
			file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := file.WriteString(tc.fragment); err != nil {
				_ = file.Close()
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			class, offset, accepted, activity, scanned := checkpoint.scanAcceptedRecoveryWithActivity04377(
				context.Background(), recoverySession04377, 0, false,
			)
			if !scanned || offset <= checkpoint.offset || class != tc.want {
				t.Fatalf("scan class=%q offset=%d accepted=%t activity=%t scanned=%t; want %q",
					class, offset, accepted, activity, scanned, tc.want)
			}
		})
	}
}

func TestProdex04377LiveRecoveryPendingEvidenceCannotCrossGoalRevision(t *testing.T) {
	for _, nextStatus := range []string{"active", "paused"} {
		t.Run(nextStatus, func(t *testing.T) {
			id := recoverySession04377
			path := filepath.Join(t.TempDir(), "rollout-"+id+".jsonl")
			if err := os.WriteFile(path,
				[]byte(`{"type":"session_meta","payload":{"id":"`+id+`"}}`+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			marker := &sessionStartMarker04360{directory: filepath.Dir(path), path: filepath.Join(filepath.Dir(path), "session.id")}
			if err := os.WriteFile(marker.path, []byte(id+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			state := runtimemodel.GoalRecoveryState{SessionID: id, Status: "blocked", UpdatedAt: 10}
			monitor := &liveGoalRecovery04361{
				marker: marker,
				read: func(context.Context, string) (runtimemodel.GoalRecoveryState, bool) {
					return state, true
				},
				resolve: func(context.Context, string) (sessionmodel.Report, error) {
					return sessionmodel.Report{ID: id, Path: path}, nil
				},
			}
			if _, ok := monitor.readState(t.Context()); !ok {
				t.Fatal("initial goal state was unavailable")
			}
			appendRecoveryEvidence04377(t, path,
				`{"type":"event_msg","payload":{"type":"user_message"}}`+"\n"+
					`{"type":"event_msg","payload":{"type":"task_complete","error":{"codex_error_info":"server_overloaded"}}}`+"\n")
			if got, ok := monitor.readState(t.Context()); !ok || got.Status != "usage_limited" {
				t.Fatalf("new terminal failure did not arm recovery: %#v ok=%t", got, ok)
			}
			state.Status, state.UpdatedAt = nextStatus, 11
			got, ok := monitor.readState(t.Context())
			if !ok || got.Status != nextStatus || monitor.pendingClass != "" {
				t.Fatalf("goal revision retained stale recovery: state=%#v pending=%q ok=%t", got, monitor.pendingClass, ok)
			}
			if nextStatus == "active" {
				state.Status = "usage_limited"
				got, ok = monitor.readState(t.Context())
				if !ok || got.Status != "blocked" {
					t.Fatalf("stale usage row crossed goal revision: state=%#v ok=%t", got, ok)
				}
			}
		})
	}
}

func TestProdex04377SuccessfulNativeCompletionClearsPendingFailure(t *testing.T) {
	id := recoverySession04377
	path := filepath.Join(t.TempDir(), "rollout-"+id+".jsonl")
	if err := os.WriteFile(path,
		[]byte(`{"type":"session_meta","payload":{"id":"`+id+`"}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	marker := &sessionStartMarker04360{directory: filepath.Dir(path), path: filepath.Join(filepath.Dir(path), "session.id")}
	if err := os.WriteFile(marker.path, []byte(id+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := runtimemodel.GoalRecoveryState{SessionID: id, Status: "blocked", UpdatedAt: 10}
	monitor := &liveGoalRecovery04361{
		marker: marker,
		read: func(context.Context, string) (runtimemodel.GoalRecoveryState, bool) {
			return state, true
		},
		resolve: func(context.Context, string) (sessionmodel.Report, error) {
			return sessionmodel.Report{ID: id, Path: path}, nil
		},
	}
	if _, ok := monitor.readState(t.Context()); !ok {
		t.Fatal("initial goal state was unavailable")
	}
	appendRecoveryEvidence04377(t, path,
		`{"type":"event_msg","payload":{"type":"user_message"}}`+"\n"+
			`{"type":"event_msg","payload":{"type":"task_complete","error":{"codex_error_info":"server_overloaded"}}}`+"\n")
	if got, ok := monitor.readState(t.Context()); !ok || got.Status != "usage_limited" {
		t.Fatalf("terminal failure did not arm recovery: %#v ok=%t", got, ok)
	}
	appendRecoveryEvidence04377(t, path,
		`{"type":"event_msg","payload":{"type":"task_complete"}}`+"\n")
	if got, ok := monitor.readState(t.Context()); !ok || got.Status != "blocked" || monitor.pendingClass != "" {
		t.Fatalf("successful completion retained recovery: state=%#v pending=%q ok=%t", got, monitor.pendingClass, ok)
	}
	state.Status = "usage_limited"
	if got, ok := monitor.readState(t.Context()); !ok || got.Status != "blocked" {
		t.Fatalf("stale usage row retriggered after completion: state=%#v ok=%t", got, ok)
	}
}

func appendRecoveryEvidence04377(t *testing.T, path, content string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
