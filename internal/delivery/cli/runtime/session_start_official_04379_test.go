package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Opt-in native Codex boundary qualification. The provider is a synthetic
// loopback Responses server; no user HOME, credentials or live model calls.
func TestProdex04379OfficialCodex01621NativeSessionStartCursor(t *testing.T) {
	binary := os.Getenv("GODEX_TEST_CODEX_01621_BIN")
	if binary == "" {
		t.Skip("set GODEX_TEST_CODEX_01621_BIN to native Codex 0.162.1")
	}
	if runtime.GOOS != "linux" {
		t.Skip("native end-to-end fixture is currently qualified only on Linux")
	}
	version, err := exec.Command(binary, "--version").CombinedOutput()
	if err != nil || strings.TrimSpace(string(version)) != "codex-cli 0.162.1" {
		t.Fatalf("unexpected native Codex version: %q: %v", version, err)
	}

	home := t.TempDir()
	codexHome := filepath.Join(home, "codex")
	if err := os.Mkdir(codexHome, 0o700); err != nil {
		t.Fatal(err)
	}
	marker, err := newSessionStartMarker04360()
	if err != nil {
		t.Fatal(err)
	}
	defer marker.Close()
	goRoot, err := filepath.Abs("../../../../")
	if err != nil {
		t.Fatal(err)
	}
	godexBinary := filepath.Join(home, "godex")
	build := exec.Command("go", "build", "-trimpath", "-o", godexBinary, "./cmd/godex")
	build.Dir = goRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build hook receiver: %v: %s", err, output)
	}
	// Capture only this isolated synthetic SessionStart and relay it to the
	// actual Godex hidden CLI, rather than calling the Go handler directly.
	captured := filepath.Join(home, "session-hook.json")
	relay := filepath.Join(home, "session-hook-relay.sh")
	script := fmt.Sprintf("#!/bin/sh\numask 077\ntee %q | %q \"$@\"\n", captured, godexBinary)
	if err := os.WriteFile(relay, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w,
			"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp-offline\"}}\n\n"+
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-offline\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\n",
		)
	}))
	defer backend.Close()
	args := []string{
		"--strict-config", "--disable", "plugins",
		"-c", "model_provider=\"synthetic\"",
		"-c", "model_providers.synthetic.name=\"Synthetic\"",
		"-c", "model_providers.synthetic.base_url=" + fmt.Sprintf("%q", backend.URL+"/v1"),
		"-c", "model_providers.synthetic.wire_api=\"responses\"",
		"-c", "model_providers.synthetic.requires_openai_auth=false",
	}
	args = append(args, marker.codexHookArgumentsForOS04360(nil, relay, runtime.GOOS)...)
	args = append(args, "exec", "--skip-git-repo-check", "--sandbox", "read-only", "-m", "gpt-5", "synthetic offline hook probe")
	ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = home
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + home, "CODEX_HOME=" + codexHome,
		"OPENAI_API_KEY=synthetic-offline", "NO_COLOR=1", "NO_PROXY=*",
		"HTTP_PROXY=", "HTTPS_PROXY=", "ALL_PROXY=",
	}
	cmd.Stdin = strings.NewReader("")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for marker.ID() == "" {
		select {
		case <-ticker.C:
		case err := <-done:
			t.Fatalf("Codex exited before SessionStart: %v: %s", err, stderr.String())
		case <-ctx.Done():
			<-done
			t.Fatalf("Codex SessionStart exceeded bounded timeout: %v: %s", ctx.Err(), stderr.String())
		}
	}
	id := marker.ID()
	raw, err := os.ReadFile(captured)
	if err != nil {
		t.Fatal(err)
	}
	var event struct {
		Kind           string `json:"hook_event_name"`
		SessionID      string `json:"session_id"`
		TranscriptPath string `json:"transcript_path"`
	}
	if err := json.Unmarshal(raw, &event); err != nil ||
		event.Kind != "SessionStart" || event.SessionID != id ||
		!sessionStartRolloutPath04379(event.TranscriptPath, id) {
		t.Fatalf("native SessionStart identity/schema differs: kind=%q id=%q callback=%q err=%v",
			event.Kind, id, event.SessionID, err)
	}
	offset, ok := sessionStartReadOffset04379(ctx, marker.path, id)
	maxOffset := sessionStartTranscriptOffset04379(map[string]any{"transcript_path": event.TranscriptPath}, id)
	if !ok || maxOffset < 0 || offset > maxOffset {
		t.Fatalf("native Codex rollout cursor: id=%q offset=%d valid=%t current=%d",
			id, offset, ok, maxOffset)
	}
	cancel()
	<-done

	// This is still the isolated native rollout created by Codex 0.162.1.
	// Inject the synthetic terminal failure only after the child is reaped,
	// but before Godex's first monitor poll, to qualify that recovery boundary.
	if strings.HasSuffix(event.TranscriptPath, ".zst") {
		t.Skip("native compressed rollout is covered by the decoded fixture")
	}
	file, err := os.OpenFile(event.TranscriptPath, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	tail := `{"type":"event_msg","payload":{"type":"user_message"}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"task_complete","error":{"codex_error_info":"server_overloaded"}}}` + "\n"
	_, writeErr := io.WriteString(file, tail)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatalf("append isolated native recovery evidence: write=%v close=%v", writeErr, closeErr)
	}
	monitor := &liveGoalRecovery04361{
		marker: marker,
		read: func(context.Context, string) (runtimemodel.GoalRecoveryState, bool) {
			return runtimemodel.GoalRecoveryState{SessionID: id, Status: "active"}, true
		},
		resolve: func(context.Context, string) (sessionmodel.Report, error) {
			return sessionmodel.Report{ID: id, Path: event.TranscriptPath}, nil
		},
	}
	state, available := monitor.readState(t.Context())
	if !available || state.Status != "usage_limited" {
		t.Fatalf("official native SessionStart cursor skipped synthetic pre-poll failure: state=%+v valid=%t",
			state, available)
	}
}
