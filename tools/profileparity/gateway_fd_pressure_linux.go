//go:build linux

package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	gatewayPressureFdLimit04380     = 80
	gatewayPressureConnections04380 = 120
	gatewayPressureModelPath04380   = "/backend-api/codex/models"
	gatewayPressureModelBody04380   = `{"object":"list","data":[]}`
)

type gatewayPressureResult04380 struct {
	requests int
	maxFDs   int
	retained bool
	queued   bool
	fresh    bool
	exited   bool
}

// Compare real TCP resource pressure for both *binaries*, not just the Go
// listener. Every run uses separate synthetic credentials, application state,
// and a loopback upstream. A real RLIMIT is inherited by only its child.
func checkGatewayFDPressure(_ string, opts cliOptions) ([]stepResult, error) {
	// Prodex's private broker secret store applies a bounded path policy.
	// A nested or user-selected TempDir can exceed it despite correct HTTP
	// behavior, so always allocate a fresh short isolated scratch root.
	root, err := os.MkdirTemp("", "gdx-fd-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(root)
	var observed []gatewayPressureResult04380
	for _, product := range []struct{ name, binary string }{
		{name: "prodex", binary: opts.prodex}, {name: "godex", binary: opts.godex},
	} {
		outcome, err := runGatewayPressure04380(root, product.name, product.binary)
		if err != nil {
			return nil, fmt.Errorf("%s gateway descriptor pressure: %w", product.name, err)
		}
		observed = append(observed, outcome)
	}
	if !sameGatewayPressure04380(observed[0], observed[1]) {
		return nil, fmt.Errorf("gateway real descriptor-pressure oracle diverged: Prodex=%+v Godex=%+v", observed[0], observed[1])
	}
	return []stepResult{{Name: "gateway_fd_pressure_recovery", ProdexExit: 0, GodexExit: 0, Requests: observed[0].requests}}, nil
}

func sameGatewayPressure04380(a, b gatewayPressureResult04380) bool {
	valid := func(v gatewayPressureResult04380) bool {
		return v.requests == 3 && v.maxFDs == gatewayPressureFdLimit04380 &&
			v.retained && v.queued && v.fresh && v.exited
	}
	return valid(a) && valid(b)
}

func runGatewayPressure04380(root, product, binary string) (gatewayPressureResult04380, error) {
	var outcome gatewayPressureResult04380
	directory := filepath.Join(root, "fd-pressure", product)
	userHome := filepath.Join(directory, "user")
	codexHome := filepath.Join(userHome, ".codex")
	configHome := filepath.Join(directory, "state")
	if err := os.MkdirAll(codexHome, 0700); err != nil {
		return outcome, err
	}
	authPath := filepath.Join(codexHome, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"synthetic-fd-token","account_id":"synthetic-account"}}`), 0600); err != nil {
		return outcome, err
	}
	fixture := profileFixture{name: product, binary: binary, home: userHome, configHome: configHome,
		env: startupEnvironment(userHome, codexHome, configHome)}
	if code, output, err := invokeProfileCommand(fixture, []string{"profile", "import-current", "main", "--insecure"}); err != nil || code != 0 {
		return outcome, fmt.Errorf("import isolated profile failed: status=%d err=%v output=%q", code, err, output)
	}
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/backend-api/wham/usage" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"plan_type":"plus","rate_limit":{"primary_window":{"used_percent":12}}}`)
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != gatewayPressureModelPath04380 ||
			r.Header.Get("Authorization") != "Bearer synthetic-fd-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, gatewayPressureModelBody04380)
	}))
	defer upstream.Close()
	baseURL := upstream.URL + "/backend-api"
	// Shell only sets RLIMIT_NOFILE for this one child and execs the exact
	// product binary; neither process-wide Go limits nor HOME are changed.
	command := exec.Command("sh", "-c", fmt.Sprintf("ulimit -n %d; exec \"$0\" gateway --listen 127.0.0.1:0 --base-url \"$1\"", gatewayPressureFdLimit04380), binary, baseURL)
	command.Dir = userHome
	command.Env = fixture.env
	stdout, err := command.StdoutPipe()
	if err != nil {
		return outcome, err
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return outcome, err
	}
	childDone := false
	defer func() {
		if !childDone {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()
	endpoint := make(chan string, 1)
	go scanStartupEndpoint(stdout, endpoint)
	var address string
	select {
	case address = <-endpoint:
	case <-time.After(8 * time.Second):
		return outcome, fmt.Errorf("listener did not start: %s", stderr.String())
	}
	parsed, err := url.Parse(address)
	if err != nil || parsed.Hostname() != "127.0.0.1" {
		return outcome, fmt.Errorf("gateway emitted unsafe endpoint %q: %w", address, err)
	}
	origin := "http://" + parsed.Host
	client := &http.Client{Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}, Timeout: 7 * time.Second}
	request := func() (string, error) {
		response, err := client.Get(origin + gatewayPressureModelPath04380)
		if err != nil {
			return "", err
		}
		defer response.Body.Close()
		content, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		if err != nil {
			return "", err
		}
		if response.StatusCode != 200 {
			return "", fmt.Errorf("upstream HTTP %d: %s", response.StatusCode, content)
		}
		return string(content), nil
	}
	baseline, err := request()
	if err != nil || baseline != gatewayPressureModelBody04380 {
		return outcome, fmt.Errorf("baseline request: body=%q err=%v", baseline, err)
	}
	held := make([]net.Conn, 0, gatewayPressureConnections04380)
	defer func() {
		for _, connection := range held {
			_ = connection.Close()
		}
	}()
	for index := 0; index < gatewayPressureConnections04380; index++ {
		conn, dialErr := net.DialTimeout("tcp", parsed.Host, 500*time.Millisecond)
		if dialErr != nil {
			return outcome, fmt.Errorf("descriptor setup connection %d: %w", index, dialErr)
		}
		if _, err := io.WriteString(conn, "GET "+gatewayPressureModelPath04380+" HTTP/1.1\r\nHost: local\r\nX-Padding: "); err != nil {
			_ = conn.Close()
			return outcome, err
		}
		held = append(held, conn)
	}
	// Inspect the actual child /proc file table; opening 120 sockets on the
	// client side alone would not prove listener descriptor exhaustion.
	for index := 0; index < 30; index++ {
		entries, readErr := os.ReadDir(fmt.Sprintf("/proc/%d/fd", command.Process.Pid))
		if readErr != nil {
			return outcome, readErr
		}
		outcome.maxFDs = len(entries)
		if outcome.maxFDs == gatewayPressureFdLimit04380 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if outcome.maxFDs != gatewayPressureFdLimit04380 {
		return outcome, fmt.Errorf("child did not reach actual descriptor limit: fd=%d/%d", outcome.maxFDs, gatewayPressureFdLimit04380)
	}
	if err := command.Process.Signal(syscall.Signal(0)); err != nil {
		return outcome, fmt.Errorf("listener exited at capacity: %w", err)
	}
	outcome.retained = true
	queued, err := net.DialTimeout("tcp", parsed.Host, 1200*time.Millisecond)
	if err != nil {
		return outcome, fmt.Errorf("queued TCP connection: %w", err)
	}
	defer queued.Close()
	if _, err = io.WriteString(queued, "GET "+gatewayPressureModelPath04380+" HTTP/1.1\r\nHost: local\r\nConnection: close\r\n\r\n"); err != nil {
		return outcome, err
	}
	completed := make(chan struct {
		body string
		err  error
	}, 1)
	go func() {
		_ = queued.SetReadDeadline(time.Now().Add(6 * time.Second))
		response, readErr := http.ReadResponse(bufio.NewReader(queued), nil)
		if readErr != nil {
			completed <- struct {
				body string
				err  error
			}{err: readErr}
			return
		}
		defer response.Body.Close()
		content, readErr := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		if readErr != nil || response.StatusCode != 200 {
			completed <- struct {
				body string
				err  error
			}{err: fmt.Errorf("queued HTTP=%d body=%s read=%v", response.StatusCode, content, readErr)}
			return
		}
		completed <- struct {
			body string
			err  error
		}{body: string(content)}
	}()
	select {
	case value := <-completed:
		return outcome, fmt.Errorf("request processed despite full descriptor table: %+v", value)
	case <-time.After(220 * time.Millisecond):
	}
	if upstreamCalls.Load() != 1 {
		return outcome, fmt.Errorf("pressure request prematurely forwarded: upstream=%d", upstreamCalls.Load())
	}
	for _, connection := range held {
		_ = connection.Close()
	}
	held = nil
	select {
	case value := <-completed:
		if value.err != nil || value.body != gatewayPressureModelBody04380 {
			return outcome, fmt.Errorf("queued request not restored: %+v", value)
		}
		outcome.queued = true
	case <-time.After(6 * time.Second):
		return outcome, errors.New("queued request never completed after releasing FD pressure")
	}
	next, err := request()
	if err != nil || next != gatewayPressureModelBody04380 {
		return outcome, fmt.Errorf("new TCP request on retained port: body=%q err=%v", next, err)
	}
	outcome.fresh = true
	outcome.requests = int(upstreamCalls.Load())
	exitErr := stopStartupGateway(command)
	childDone = true
	outcome.exited = exitErr == nil || command.ProcessState != nil && command.ProcessState.ExitCode() < 0
	if !outcome.exited {
		return outcome, fmt.Errorf("gateway refused shutdown: %w stderr=%s", exitErr, stderr.String())
	}
	return outcome, nil
}
