package routing

import (
	"context"
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
