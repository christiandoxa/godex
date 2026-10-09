package routing

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
)

const (
	restartProcessModeEnv = "GODEX_04361_RESTART_PROCESS_MODE"
	restartProcessRootEnv = "GODEX_04361_RESTART_STATE_ROOT"
	restartProcessHomeEnv = "GODEX_04361_RESTART_PROFILE_HOME"
	deadRestartModeEnv    = "GODEX_04361_DEAD_RESTART_PROCESS_MODE"
	deadRestartRootEnv    = "GODEX_04361_DEAD_RESTART_STATE_ROOT"
	deadRestartHomeEnv    = "GODEX_04361_DEAD_RESTART_PROFILE_HOME"
	restartProcessOwner   = "11111111111111111111111111111111"
	restartProcessOther   = "22222222222222222222222222222222"
	restartProcessID      = "resp_process_restart_04361"
	restartProcessTurn    = "opaque-process-restart-turn"
)

func TestProdex04361RouterContinuationRestoresAcrossProcesses(t *testing.T) {
	stateHome := filepath.Join(t.TempDir(), "state")
	profileHome := t.TempDir()
	if err := os.Chmod(profileHome, 0o700); err != nil {
		t.Fatal(err)
	}

	runRestartRouterProcess(t, "write", stateHome, profileHome)
	if _, err := os.Stat(filepath.Join(stateHome, "routing.json")); err != nil {
		t.Fatalf("writer process did not persist routing bindings: %v", err)
	}
	if _, err := os.Stat(filepath.Join(profileHome, ".godex-turn-state", affinityDigest("previous", restartProcessID)+".json")); err != nil {
		t.Fatalf("writer process did not persist the profile turn state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateHome, "routing.json"), []byte(`{"version":1,"bindings":[`), 0o600); err != nil {
		t.Fatal(err)
	}
	runRestartRouterProcess(t, "read", stateHome, profileHome)
	primary, err := os.ReadFile(filepath.Join(stateHome, "routing.json"))
	if err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(filepath.Join(stateHome, "routing.json.last-good"))
	if err != nil || string(primary) != string(backup) {
		t.Fatalf("reader process did not repair the primary from last-good: %v", err)
	}

	// Unknown opaque continuations must stay closed after a fresh process starts.
	runRestartRouterProcess(t, "unknown", stateHome, profileHome)
}

func TestProdex04361DeadTurnStateSurvivesProcessRestart(t *testing.T) {
	stateHome := filepath.Join(t.TempDir(), "state")
	profileHome := t.TempDir()
	if err := os.Chmod(profileHome, 0o700); err != nil {
		t.Fatal(err)
	}

	runDeadRestartRouterProcess(t, "write", stateHome, profileHome)
	if _, err := os.Stat(filepath.Join(stateHome, "continuation-status.json")); err != nil {
		t.Fatalf("writer process did not persist dead continuation status: %v", err)
	}
	runDeadRestartRouterProcess(t, "read", stateHome, profileHome)
}

func TestProdex04361RestartRouterProcess(t *testing.T) {
	mode := os.Getenv(restartProcessModeEnv)
	if mode == "" {
		return
	}
	router, gateway := newRestartProcessRouter(t, mode)
	switch mode {
	case "write":
		writeRestartProcessBinding(t, router)
	case "read":
		readRestartProcessBinding(t, router, gateway)
	case "unknown":
		rejectUnknownRestartProcessBinding(t, router, gateway)
	default:
		t.Fatalf("unexpected restart helper mode %q", mode)
	}
}

func TestProdex04361DeadRestartRouterProcess(t *testing.T) {
	mode := os.Getenv(deadRestartModeEnv)
	if mode == "" {
		return
	}
	router, gateway := newDeadRestartRouter(t, mode)
	switch mode {
	case "write":
		writeDeadRestartBinding(t, router)
	case "read":
		readDeadRestartBinding(t, router, gateway)
	default:
		t.Fatalf("unexpected dead restart helper mode %q", mode)
	}
}

func newRestartProcessRouter(t *testing.T, mode string) (*Router, *websocketMessageRoutingGateway) {
	stateHome, profileHome := os.Getenv(restartProcessRootEnv), os.Getenv(restartProcessHomeEnv)
	accounts := []proxymodel.Account{
		{ID: restartProcessOwner, Home: profileHome, Enabled: true},
		{ID: restartProcessOther, Home: filepath.Join(profileHome, "other"), Enabled: true},
	}
	preferred := restartProcessOther
	if mode == "write" {
		preferred = restartProcessOwner
	}
	gateway := &websocketMessageRoutingGateway{responses: []*proxymodel.Response{restartProcessResponse(mode)}}
	router, err := NewRouter(Config{
		Gateway: gateway, Bindings: routingrepo.NewStore(stateHome), PreferredAccount: preferred,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return accounts, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return router, gateway
}

func newDeadRestartRouter(t *testing.T, mode string) (*Router, *prodex04356Gateway) {
	stateHome, profileHome := os.Getenv(deadRestartRootEnv), os.Getenv(deadRestartHomeEnv)
	accounts := []proxymodel.Account{
		{ID: restartProcessOwner, Home: profileHome, Enabled: true},
		{ID: restartProcessOther, Home: filepath.Join(profileHome, "other"), Enabled: true},
	}
	preferred := restartProcessOther
	steps := []prodex04356Step{{account: restartProcessOther, status: http.StatusOK, body: `{"id":"dead-restart-next"}`}}
	if mode == "write" {
		preferred = restartProcessOwner
		steps = []prodex04356Step{
			{account: restartProcessOwner, status: http.StatusForbidden, body: `{"error":{"code":"insufficient_quota"}}`},
			{account: restartProcessOther, status: http.StatusOK, body: `{"id":"dead-restart-fallback"}`},
		}
	}
	gateway := &prodex04356Gateway{steps: steps}
	router, err := NewRouter(Config{
		Gateway: gateway, Bindings: routingrepo.NewStore(stateHome), PreferredAccount: preferred,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return accounts, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return router, gateway
}

func restartProcessResponse(mode string) *proxymodel.Response {
	response := &proxymodel.Response{
		StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("next-frame")),
		WebSocketFrames: true, FirstEventCommitted: true,
		WebSocketResponseID: "resp_process_restart_next",
	}
	if mode == "write" {
		response.Body = io.NopCloser(strings.NewReader("frame"))
		response.WebSocketResponseID = restartProcessID
		response.WebSocketTurnState = restartProcessTurn
	}
	return response
}

func writeRestartProcessBinding(t *testing.T, router *Router) {
	t.Helper()
	exchange, err := router.Forward(t.Context(), websocketMessageRequest(`{"type":"response.create","response":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.AccountID != restartProcessOwner {
		t.Fatalf("initial response owner = %q", exchange.Result.AccountID)
	}
}

func writeDeadRestartBinding(t *testing.T, router *Router) {
	t.Helper()
	if err := router.affinity.remember(t.Context(), restartProcessOwner, affinityKeys{turn: restartProcessTurn}, router.now()); err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(t.Context(), deadRestartRequest())
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.AccountID != restartProcessOther || exchange.Result.Failed {
		t.Fatalf("dead continuation fallback = owner:%q failed:%t", exchange.Result.AccountID, exchange.Result.Failed)
	}
}

func readRestartProcessBinding(t *testing.T, router *Router, gateway *websocketMessageRoutingGateway) {
	t.Helper()
	exchange, err := router.Forward(t.Context(), websocketMessageRequest(
		`{"type":"response.create","response":{"previous_response_id":"`+restartProcessID+`"}}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.AccountID != restartProcessOwner || len(gateway.requests) != 1 ||
		gateway.requests[0].Header.Get("x-codex-turn-state") != restartProcessTurn {
		t.Fatalf("restarted route owner/header = %q/%q", exchange.Result.AccountID, gateway.requests[0].Header.Get("x-codex-turn-state"))
	}
}

func readDeadRestartBinding(t *testing.T, router *Router, gateway *prodex04356Gateway) {
	t.Helper()
	exchange, err := router.Forward(t.Context(), deadRestartRequest())
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.AccountID != restartProcessOther || len(gateway.requests) != 1 {
		t.Fatalf("dead continuation restart route = owner:%q requests:%d", exchange.Result.AccountID, len(gateway.requests))
	}
	request := gateway.requests[0]
	if got := request.Header.Get("x-codex-turn-state"); got != "" {
		t.Fatalf("restarted request forwarded dead turn-state header %q", got)
	}
	var body map[string]any
	if err := json.Unmarshal(request.Body, &body); err != nil {
		t.Fatalf("decode restarted request: %v", err)
	}
	if metadata, ok := body["client_metadata"].(map[string]any); ok {
		if _, found := metadata["x-codex-turn-state"]; found {
			t.Fatal("restarted request forwarded dead turn-state body metadata")
		}
	}
}

func deadRestartRequest() proxymodel.Request {
	request := prodex04357ResponsesRequest(prodex04357FullHistoryBody())
	request.Header.Set("X-Codex-Turn-State", restartProcessTurn)
	return request
}

func rejectUnknownRestartProcessBinding(t *testing.T, router *Router, gateway *websocketMessageRoutingGateway) {
	t.Helper()
	_, err := router.Forward(t.Context(), websocketMessageRequest(
		`{"type":"response.create","response":{"previous_response_id":"resp_unknown_after_restart"}}`,
	))
	if err == nil || len(gateway.requests) != 0 {
		t.Fatalf("unknown continuation error/calls = %v/%d", err, len(gateway.requests))
	}
}

func runRestartRouterProcess(t *testing.T, mode, stateHome, profileHome string) {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestProdex04361RestartRouterProcess$")
	command.Env = restartRouterProcessEnv(mode, stateHome, profileHome)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("restart process %q failed: %v\n%s", mode, err, output)
	}
}

func runDeadRestartRouterProcess(t *testing.T, mode, stateHome, profileHome string) {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestProdex04361DeadRestartRouterProcess$")
	command.Env = deadRestartProcessEnv(mode, stateHome, profileHome)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("dead restart process %q failed: %v\n%s", mode, err, output)
	}
}

func restartRouterProcessEnv(mode, stateHome, profileHome string) []string {
	env := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if key != restartProcessModeEnv && key != restartProcessRootEnv && key != restartProcessHomeEnv {
			env = append(env, entry)
		}
	}
	return append(env,
		restartProcessModeEnv+"="+mode,
		restartProcessRootEnv+"="+stateHome,
		restartProcessHomeEnv+"="+profileHome,
	)
}

func deadRestartProcessEnv(mode, stateHome, profileHome string) []string {
	env := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if key != deadRestartModeEnv && key != deadRestartRootEnv && key != deadRestartHomeEnv {
			env = append(env, entry)
		}
	}
	return append(env,
		deadRestartModeEnv+"="+mode,
		deadRestartRootEnv+"="+stateHome,
		deadRestartHomeEnv+"="+profileHome,
	)
}
