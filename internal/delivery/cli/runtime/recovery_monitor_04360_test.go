package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProdex04360ChildExitRecoveryUsesOnlyNewStructuredAcceptedSignal(t *testing.T) {
	const session = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	for _, tc := range []struct {
		name, append string
		want         bool
	}{
		{"accepted then structured limit", `{"type":"response_item","payload":{"type":"message","role":"user","content":[]}}` + "\n" +
			`{"type":"error","error":{"code":"usage_limit_reached"}}` + "\n", true},
		{"accepted then nested codex limit", `{"type":"event_msg","payload":{"type":"user_message"}}` + "\n" +
			`{"type":"event_msg","payload":{"type":"error","codex_error_info":{"usage_limit_exceeded":{}}}}` + "\n", true},
		{"marker without user acceptance", `{"type":"error","error":{"code":"usage_limit_reached"}}` + "\n", false},
		{"accepted then unrelated error", `{"type":"response_item","payload":{"type":"message","role":"user"}}` + "\n" +
			`{"type":"error","error":{"code":"invalid_api_key"}}` + "\n", false},
		{"accepted then fake log string", `{"type":"response_item","payload":{"role":"user"}}` + "\n" +
			`{"type":"event_msg","payload":{"message":"You've hit your usage limit."}}` + "\n", false},
		{"old turn accepted then new turn starts without user acceptance",
			`{"type":"response_item","payload":{"role":"user"}}` + "\n" +
				`{"type":"event_msg","payload":{"type":"turn_started"}}` + "\n" +
				`{"type":"error","error":{"code":"usage_limit_reached"}}` + "\n", false},
		{"old turn accepted then app server completed",
			`{"type":"response_item","payload":{"role":"user"}}` + "\n" +
				`{"method":"turn/completed","params":{"turn":{"status":"completed"}}}` + "\n" +
				`{"type":"error","error":{"code":"usage_limit_reached"}}` + "\n", false},
		{"old turn accepted then native turn started without user acceptance",
			`{"type":"response_item","payload":{"role":"user"}}` + "\n" +
				`{"method":"turn/started"}` + "\n" +
				`{"type":"event_msg","payload":{"type":"error","codex_error_info":"usage_limit_exceeded"}}` + "\n", false},
		{"old turn committed then unrelated error",
			`{"type":"response_item","payload":{"role":"user"}}` + "\n" +
				`{"type":"turn.completed","turn":{"status":"completed"}}` + "\n" +
				`{"type":"error","error":{"code":"usage_limit_reached"}}` + "\n", false},
		{"accepted then foreign ID", `{"type":"response_item","payload":{"role":"user"}}` + "\n" +
			`{"type":"error","session_id":"00000000-0000-4000-8000-000000000002","error":{"code":"usage_limit_reached"}}` + "\n", false},
		{"accepted then foreign nested session", `{"type":"response_item","payload":{"role":"user"}}` + "\n" +
			`{"type":"error","error":{"sessionId":"00000000-0000-4000-8000-000000000002","code":"usage_limit_reached"}}` + "\n", false},
		{"accepted then trailing incomplete record", `{"type":"response_item","payload":{"role":"user"}}` + "\n" +
			`{"type":"error","error":{"code":"usage_limit_reached"}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "rollout-"+session+".jsonl")
			prior := `{"type":"session_meta","payload":{"id":"` + session + `"}}` + "\n" +
				`{"type":"error","error":{"code":"usage_limit_reached"}}` + "\n"
			if err := os.WriteFile(path, []byte(prior), 0600); err != nil {
				t.Fatal(err)
			}
			checkpoint := captureRecoveryCheckpoint04360(path)
			if !checkpoint.valid {
				t.Fatal("failed to capture existing regular rollout")
			}
			file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = file.WriteString(tc.append); err != nil {
				t.Fatal(err)
			}
			if err = file.Close(); err != nil {
				t.Fatal(err)
			}
			got := checkpoint.newAcceptedUsageLimit04360(context.Background(), session)
			if got != tc.want {
				t.Fatalf("post-child usage-limit=%t want %t", got, tc.want)
			}
		})
	}
}

func TestProdex04360ChildExitMonitorRejectsMissingSymlinkAndOversize(t *testing.T) {
	root := t.TempDir()
	missing := captureRecoveryCheckpoint04360(filepath.Join(root, "missing.jsonl"))
	if missing.valid {
		t.Fatal("missing rollout unexpectedly valid")
	}
	target := filepath.Join(root, "real.jsonl")
	if err := os.WriteFile(target, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.jsonl")
	if err := os.Symlink(target, link); err == nil && captureRecoveryCheckpoint04360(link).valid {
		t.Fatal("symlink rollout must be fail-closed")
	}
	path := filepath.Join(root, "rollout.jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	checkpoint := captureRecoveryCheckpoint04360(path)
	huge := `{"type":"response_item","payload":{"role":"user"}}` + "\n" +
		strings.Repeat(" ", (1<<20)+1) +
		`{"type":"error","error":{"code":"usage_limit_reached"}}` + "\n"
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.WriteString(huge); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	if checkpoint.newAcceptedUsageLimit04360(context.Background(), "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9") {
		t.Fatal("oversize untrusted rollout marker triggered recovery")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if checkpoint.newAcceptedUsageLimit04360(ctx, "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9") {
		t.Fatal("cancelled launch scheduled recovery")
	}
}

func TestProdex04361ChildExitMonitorRejectsInPlaceStaleRolloutRewrite(t *testing.T) {
	const session = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	path := filepath.Join(t.TempDir(), "rollout-"+session+".jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"session_meta","payload":{"id":"`+session+`"}}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	checkpoint := captureRecoveryCheckpoint04360(path)
	if !checkpoint.valid {
		t.Fatal("failed to capture regular rollout checkpoint")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatal(err)
	}
	padding := make([]byte, checkpoint.offset)
	if _, err := file.Write(padding); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"type":"response_item","payload":{"role":"user"}}` + "\n" +
		`{"type":"error","error":{"code":"usage_limit_reached"}}` + "\n"); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if checkpoint.newAcceptedUsageLimit04360(context.Background(), session) {
		t.Fatal("in-place rewritten rollout reused stale checkpoint evidence")
	}
}

func TestProdex04374NativeCompletionBoundariesResetOnlySuccessfulTurns(t *testing.T) {
	const session = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	for _, tc := range []struct {
		name, append string
	}{
		{"task started resets old acceptance", `{"type":"response_item","payload":{"role":"user"}}
{"type":"event_msg","payload":{"type":"task_started"}}
{"type":"event_msg","payload":{"type":"task_complete","error":{"codex_error_info":"server_overloaded"}}}
		`},
		{"turn started resets old acceptance", `{"type":"response_item","payload":{"role":"user"}}
{"type":"turn_started"}
{"type":"turn_complete","error":{"codex_error_info":"server_overloaded"}}
		`},
		{"successful native completion blocks later stale error", `{"type":"response_item","payload":{"role":"user"}}
{"type":"event_msg","payload":{"type":"task_complete","error":null}}
{"type":"error","error":{"code":"usage_limit_reached"}}
		`},
		{"failed native completion arms recovery", `{"type":"response_item","payload":{"role":"user"}}
{"type":"event_msg","payload":{"type":"task_complete","error":{"codex_error_info":"server_overloaded"}}}
		`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "rollout-"+session+".jsonl")
			if err := os.WriteFile(path, []byte(`{"type":"session_meta","payload":{"id":"`+session+`"}}
`), 0600); err != nil {
				t.Fatal(err)
			}
			checkpoint := captureRecoveryCheckpoint04360(path)
			file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = file.WriteString(strings.TrimSpace(tc.append) + "\n"); err != nil {
				t.Fatal(err)
			}
			if err = file.Close(); err != nil {
				t.Fatal(err)
			}
			got := checkpoint.newAcceptedRecoveryClass04360(context.Background(), session)
			if tc.name == "failed native completion arms recovery" {
				if got != "overload" {
					t.Fatalf("class=%q want overload", got)
				}
				return
			}
			if got != "" {
				t.Fatalf("class=%q, want no recovery", got)
			}
		})
	}
}

func TestProdex04374NativeTerminalScannerRejectsForeignAndUntypedRecords(t *testing.T) {
	const session = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	for _, fragment := range []string{
		`{"type":"response_item","payload":{"role":"user"}}` + "\n" +
			`{"type":"event_msg","payload":{"type":"task_complete","thread_id":"00000000-0000-4000-8000-000000000002","error":{"codex_error_info":"usage_limit_exceeded"}}}` + "\n",
		`{"type":"response_item","payload":{"role":"user"}}` + "\n" +
			`{"type":"event_msg","payload":{"type":"task_complete","error":{"message":"Selected model is at capacity."}}}` + "\n" +
			`{"type":"error","error":{"codex_error_info":"usage_limit_exceeded"}}` + "\n",
	} {
		if got := scanRecoveryClassFragment04360(context.Background(), session, []byte(fragment)); got != "" {
			t.Fatalf("untrusted native completion left recovery armed: %q", got)
		}
	}
}

func TestProdex04377LiveRecoveryDrainsTailAndRequiresTerminalCompletion(t *testing.T) {
	const session = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	for _, tc := range []struct {
		name, fragment, want string
	}{
		{
			name: "intermediate retry error is ignored",
			fragment: `{"type":"response_item","payload":{"role":"user"}}
{"type":"event_msg","payload":{"type":"error","codex_error_info":"server_overloaded","will_retry":true}}
`,
		},
		{
			name: "later progress clears an old terminal failure",
			fragment: `{"type":"response_item","payload":{"role":"user"}}
{"type":"task_complete","error":{"codex_error_info":"server_overloaded"}}
{"type":"event_msg","payload":{"type":"agent_message","message":"retry succeeded"}}
`,
		},
		{
			name: "later cancellation clears an old terminal failure",
			fragment: `{"type":"response_item","payload":{"role":"user"}}
{"type":"event_msg","payload":{"type":"task_complete","error":{"codex_error_info":"server_overloaded"}}}
{"type":"event_msg","payload":{"type":"turn_cancelled"}}
`,
		},
		{
			name: "later accepted terminal failure supersedes an old one",
			fragment: `{"type":"response_item","payload":{"role":"user"}}
{"type":"event_msg","payload":{"type":"task_complete","error":{"codex_error_info":"server_overloaded"}}}
{"type":"event_msg","payload":{"type":"task_started"}}
{"type":"event_msg","payload":{"type":"user_message"}}
{"type":"turn_complete","error":{"codex_error_info":"usage_limit_exceeded"}}
`,
			want: "usage_limit",
		},
		{
			name: "successful completion clears an earlier terminal failure",
			fragment: `{"type":"response_item","payload":{"role":"user"}}
{"type":"task_complete","error":{"codex_error_info":"server_overloaded"}}
{"type":"event_msg","payload":{"type":"turn_complete","error":null}}
`,
		},
		{
			name: "foreign activity does not clear same-session recovery",
			fragment: `{"type":"response_item","payload":{"role":"user"}}
{"type":"task_complete","error":{"codex_error_info":"server_overloaded"}}
{"type":"event_msg","thread_id":"00000000-0000-4000-8000-000000000002","payload":{"type":"agent_message"}}
`,
			want: "overload",
		},
		{
			name: "bare error notification is not terminal",
			fragment: `{"type":"response_item","payload":{"role":"user"}}
{"type":"error","error":{"codex_error_info":"server_overloaded"}}
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, consumed, _ := scanRecoveryClassSince04360(
				context.Background(), session, []byte(tc.fragment), false,
			)
			if consumed != int64(len(tc.fragment)) {
				t.Fatalf("consumed=%d want %d", consumed, len(tc.fragment))
			}
			if got != tc.want {
				t.Fatalf("class=%q want %q", got, tc.want)
			}
		})
	}
}

func TestProdex04377LiveRecoveryRetainsIncompleteTailOffset(t *testing.T) {
	const session = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	user := []byte(`{"type":"response_item","payload":{"role":"user"}}` + "\n")
	terminal := `{"type":"task_complete","error":{"codex_error_info":"server_overloaded"}}`
	fragment := append(append([]byte(nil), user...), terminal...)
	got, consumed, accepted := scanRecoveryClassSince04360(context.Background(), session, fragment, false)
	if got != "" || consumed != int64(len(user)) || !accepted {
		t.Fatalf("partial tail: class=%q consumed=%d accepted=%t", got, consumed, accepted)
	}
	got, consumed, accepted = scanRecoveryClassSince04360(
		context.Background(), session, []byte(terminal+"\n"), accepted,
	)
	if got != "overload" || consumed != int64(len(terminal)+1) || accepted {
		t.Fatalf("completed tail: class=%q consumed=%d accepted=%t", got, consumed, accepted)
	}
}
