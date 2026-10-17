package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

const (
	queueAdmissionLimit04381 = "1"
	queueRequestPath04381    = "/backend-api/codex/models"
	queueUsagePath04381      = "/backend-api/wham/usage"
)

type queueGatewayResult04381 struct {
	status        []int
	calls         int
	queued        bool
	fresh         bool
	authenticated bool
	exited        bool
}

type queueGatewayServer04381 struct {
	server        *httptest.Server
	firstRelease  chan struct{}
	firstEntered  chan struct{}
	secondEntered chan struct{}
	calls         atomic.Int32
	mu            sync.Mutex
	authenticated bool
}

func checkGatewayQueue04381(_ string, opts cliOptions) ([]stepResult, error) {
	root, err := os.MkdirTemp("", "gdx-q-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(root)
	results := make([]queueGatewayResult04381, 0, 2)
	for _, item := range []struct{ name, binary string }{
		{name: "prodex", binary: opts.prodex}, {name: "godex", binary: opts.godex},
	} {
		result, err := runGatewayQueue04381(root, item.name, item.binary)
		if err != nil {
			return nil, fmt.Errorf("%s admission queue wake: %w", item.name, err)
		}
		results = append(results, result)
	}
	if len(results) != 2 || !sameGatewayQueue04381(results[0], results[1]) {
		return nil, fmt.Errorf("admission queue wake diverged: Prodex=%+v Godex=%+v", results[0], results[1])
	}
	return []stepResult{{
		Name: "gateway_admission_queue_wake_04381", ProdexExit: 0, GodexExit: 0,
		Requests: results[0].calls,
	}}, nil
}

func sameGatewayQueue04381(left, right queueGatewayResult04381) bool {
	return left.calls == 3 && right.calls == 3 && left.queued && right.queued &&
		left.fresh && right.fresh && left.authenticated && right.authenticated &&
		left.exited && right.exited &&
		len(left.status) == 3 && len(right.status) == 3 &&
		left.status[0] == http.StatusOK && left.status[1] == http.StatusOK && left.status[2] == http.StatusOK &&
		left.status[0] == right.status[0] && left.status[1] == right.status[1] && left.status[2] == right.status[2]
}

func runGatewayQueue04381(root, product, binary string) (queueGatewayResult04381, error) {
	var result queueGatewayResult04381
	directory := filepath.Join(root, "queue-04381", product)
	userHome := filepath.Join(directory, "user")
	codexHome := filepath.Join(userHome, ".codex")
	configHome := filepath.Join(directory, product)
	for _, path := range []string{directory, userHome, codexHome, configHome} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return result, err
		}
		if err := os.Chmod(path, 0o700); err != nil {
			return result, err
		}
	}
	auth := []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"synthetic-queue","account_id":"synthetic-owner"}}`)
	if err := os.WriteFile(filepath.Join(codexHome, "auth.json"), auth, 0o600); err != nil {
		return result, err
	}
	fixture := profileFixture{
		name: product, binary: binary, home: userHome, configHome: configHome,
		env: queueEnvironment04381(userHome, codexHome, configHome),
	}
	if code, output, err := invokeProfileCommand(fixture, []string{"profile", "import-current", "main", "--insecure"}); err != nil || code != 0 {
		return result, fmt.Errorf("synthetic profile import: code=%d err=%v output=%q", code, err, output)
	}
	upstream := newQueueGatewayServer04381()
	defer upstream.server.Close()
	command := exec.Command(binary, "gateway", "--listen", "127.0.0.1:0", "--base-url", upstream.server.URL+"/backend-api")
	command.Dir = userHome
	command.Env = fixture.env
	stdout, err := command.StdoutPipe()
	if err != nil {
		return result, err
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return result, err
	}
	finished := false
	defer func() {
		if !finished {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()
	endpoint := make(chan string, 1)
	go scanStartupEndpoint(stdout, endpoint)
	var address string
	select {
	case address = <-endpoint:
	case <-time.After(10 * time.Second):
		return result, fmt.Errorf("gateway did not start: %s", stderr.String())
	}
	parsed, err := url.Parse(address)
	if err != nil || parsed.Hostname() != "127.0.0.1" {
		return result, fmt.Errorf("invalid isolated endpoint %q: %w", address, err)
	}
	client := &http.Client{Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}, Timeout: 8 * time.Second}
	request := func() (int, error) {
		response, requestErr := client.Get("http://" + parsed.Host + queueRequestPath04381)
		if requestErr != nil {
			return 0, requestErr
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 4<<10))
		closeErr := response.Body.Close()
		if readErr != nil {
			return 0, readErr
		}
		if response.StatusCode != http.StatusOK {
			return response.StatusCode, fmt.Errorf("unexpected status %d body=%q", response.StatusCode, body)
		}
		return response.StatusCode, closeErr
	}
	first := make(chan struct {
		status int
		err    error
	}, 1)
	go func() {
		status, requestErr := request()
		first <- struct {
			status int
			err    error
		}{status: status, err: requestErr}
	}()
	select {
	case <-upstream.firstEntered:
	case <-time.After(5 * time.Second):
		return result, errors.New("first request did not occupy upstream")
	}
	second := make(chan struct {
		status int
		err    error
	}, 1)
	go func() {
		status, requestErr := request()
		second <- struct {
			status int
			err    error
		}{status: status, err: requestErr}
	}()
	select {
	case <-upstream.secondEntered:
		return result, errors.New("queued request dispatched before permit release")
	case <-time.After(250 * time.Millisecond):
		result.queued = true
	}
	close(upstream.firstRelease)
	firstResult := <-first
	secondResult := <-second
	if firstResult.err != nil || secondResult.err != nil {
		return result, fmt.Errorf("queued requests failed: first=%v second=%v", firstResult.err, secondResult.err)
	}
	result.status = []int{firstResult.status, secondResult.status}
	status, err := request()
	if err != nil {
		return result, err
	}
	result.status = append(result.status, status)
	result.fresh = status == http.StatusOK
	result.calls = int(upstream.calls.Load())
	upstream.mu.Lock()
	result.authenticated = upstream.authenticated
	upstream.mu.Unlock()
	stopErr := stopStartupGateway(command)
	finished = true
	result.exited = stopErr == nil || command.ProcessState != nil && command.ProcessState.ExitCode() < 0
	if !result.exited {
		return result, fmt.Errorf("gateway exited abnormally: %v stderr=%s", stopErr, stderr.String())
	}
	return result, nil
}

func queueEnvironment04381(userHome, codexHome, configHome string) []string {
	env := startupEnvironment(userHome, codexHome, configHome)
	for _, value := range []string{
		"GODEX_RUNTIME_PROXY_ACTIVE_REQUEST_LIMIT=" + queueAdmissionLimit04381,
		"GODEX_RUNTIME_PROXY_RESPONSES_ACTIVE_LIMIT=" + queueAdmissionLimit04381,
		"GODEX_RUNTIME_PROXY_ADMISSION_WAIT_BUDGET_MS=2000",
		"GODEX_RUNTIME_PROXY_PRESSURE_ADMISSION_WAIT_BUDGET_MS=2000",
		"PRODEX_RUNTIME_PROXY_ACTIVE_REQUEST_LIMIT=" + queueAdmissionLimit04381,
		"PRODEX_RUNTIME_PROXY_RESPONSES_ACTIVE_LIMIT=" + queueAdmissionLimit04381,
		"PRODEX_RUNTIME_PROXY_ADMISSION_WAIT_BUDGET_MS=2000",
		"PRODEX_RUNTIME_PROXY_PRESSURE_ADMISSION_WAIT_BUDGET_MS=2000",
	} {
		env = append(env, value)
	}
	return env
}

func newQueueGatewayServer04381() *queueGatewayServer04381 {
	upstream := &queueGatewayServer04381{
		firstRelease: make(chan struct{}), firstEntered: make(chan struct{}), secondEntered: make(chan struct{}),
	}
	upstream.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet && request.URL.Path == queueUsagePath04381 {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"plan_type":"plus","rate_limit":{"primary_window":{"used_percent":12}}}`)
			return
		}
		valid := request.Method == http.MethodGet && request.URL.Path == queueRequestPath04381 &&
			request.Header.Get("Authorization") == "Bearer synthetic-queue" &&
			request.Header.Get("ChatGPT-Account-Id") == "synthetic-owner"
		if !valid {
			upstream.mu.Lock()
			upstream.authenticated = false
			upstream.mu.Unlock()
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		call := int(upstream.calls.Add(1))
		if call == 1 {
			close(upstream.firstEntered)
			<-upstream.firstRelease
		} else if call == 2 {
			close(upstream.secondEntered)
		}
		upstream.mu.Lock()
		upstream.authenticated = true
		upstream.mu.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, `{"object":"list","data":[]}`)
	}))
	return upstream
}
