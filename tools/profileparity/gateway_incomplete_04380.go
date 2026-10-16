package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// Prodex 0.438.0 / Codex 0.163.0: incomplete responses keep their native
// identity, usage, unknown fields, and terminal SSE framing. They are not
// quota errors, successful completions, or permission to replay a model turn.
const incompleteResponse04380 = `{"id":"resp_incomplete_native","object":"response","model":"gpt-6","status":"incomplete","incomplete_details":{"reason":"max_output_tokens","extension":"still-there"},"usage":{"input_tokens":13,"output_tokens":7,"total_tokens":20,"input_tokens_details":{"cached_tokens":3}},"output":[],"native_future":{"flag":true}}`
const incompleteSSE04380 = "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_incomplete_native\"}}\n\n" +
	"event: response.incomplete\ndata: {\"type\":\"response.incomplete\",\"response\":" + incompleteResponse04380 + "}\n\n"

type incompleteGatewayResult04380 struct {
	calls                 int
	statuses              []int
	contentTypes          []string
	bodies                []string
	upstreamAuthenticated bool
	exited                bool
}

func checkGatewayIncomplete04380(_ string, opts cliOptions) ([]stepResult, error) {
	// The managed capability-secret path in Prodex is length-limited.
	root, err := os.MkdirTemp("", "gdx-inc-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(root)
	var outcomes []incompleteGatewayResult04380
	for _, item := range []struct{ name, binary string }{
		{name: "prodex", binary: opts.prodex}, {name: "godex", binary: opts.godex},
	} {
		outcome, runErr := runGatewayIncomplete04380(root, item.name, item.binary)
		if runErr != nil {
			return nil, fmt.Errorf("%s native incomplete wire: %w", item.name, runErr)
		}
		outcomes = append(outcomes, outcome)
	}
	if !sameGatewayIncomplete04380(outcomes[0], outcomes[1]) {
		return nil, fmt.Errorf("incomplete Responses wire diverged: Prodex=%+v Godex=%+v", outcomes[0], outcomes[1])
	}
	return []stepResult{{Name: "gateway_native_incomplete_04380", ProdexExit: 0, GodexExit: 0, Requests: 2}}, nil
}

func sameGatewayIncomplete04380(a, b incompleteGatewayResult04380) bool {
	valid := func(item incompleteGatewayResult04380) bool {
		if item.calls != 2 || !item.exited || !item.upstreamAuthenticated || len(item.statuses) != 2 ||
			len(item.bodies) != 2 || len(item.contentTypes) != 2 {
			return false
		}
		return item.statuses[0] == 200 && item.statuses[1] == 200 &&
			strings.HasPrefix(strings.ToLower(item.contentTypes[0]), "application/json") &&
			strings.HasPrefix(strings.ToLower(item.contentTypes[1]), "text/event-stream") &&
			item.bodies[0] == incompleteResponse04380 && item.bodies[1] == incompleteSSE04380
	}
	return valid(a) && valid(b)
}

func runGatewayIncomplete04380(root, product, binary string) (incompleteGatewayResult04380, error) {
	var outcome incompleteGatewayResult04380
	directory := filepath.Join(root, product)
	userHome := filepath.Join(directory, "user")
	codexHome := filepath.Join(userHome, ".codex")
	configHome := filepath.Join(directory, "state")
	if err := os.MkdirAll(codexHome, 0700); err != nil {
		return outcome, err
	}
	auth := []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"synthetic-incomplete","account_id":"synthetic-owner"}}`)
	if err := os.WriteFile(filepath.Join(codexHome, "auth.json"), auth, 0600); err != nil {
		return outcome, err
	}
	fixture := profileFixture{name: product, binary: binary, home: userHome, configHome: configHome,
		env: startupEnvironment(userHome, codexHome, configHome)}
	if code, output, err := invokeProfileCommand(fixture, []string{"profile", "import-current", "main", "--insecure"}); err != nil || code != 0 {
		return outcome, fmt.Errorf("synthetic profile import: code=%d err=%v output=%q", code, err, output)
	}
	var sent atomic.Int32
	var invalid atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/backend-api/wham/usage" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"plan_type":"plus","rate_limit":{"primary_window":{"used_percent":12}}}`)
			return
		}
		payload, err := io.ReadAll(io.LimitReader(r.Body, 32<<10))
		var request struct {
			Model  string `json:"model"`
			Input  string `json:"input"`
			Stream bool   `json:"stream"`
		}
		if err != nil || json.Unmarshal(payload, &request) != nil || r.Method != http.MethodPost ||
			r.URL.Path != "/backend-api/codex/responses" ||
			r.Header.Get("Authorization") != "Bearer synthetic-incomplete" ||
			r.Header.Get("ChatGPT-Account-Id") != "synthetic-owner" ||
			request.Model != "gpt-6" || request.Input != "synthetic incomplete" {
			invalid.Store(true)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		sent.Add(1)
		body, mime := incompleteResponse04380, "application/json"
		if request.Stream {
			body, mime = incompleteSSE04380, "text/event-stream"
		}
		w.Header().Set("Content-Type", mime)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, body)
	}))
	defer upstream.Close()
	command := exec.Command(binary, "gateway", "--listen", "127.0.0.1:0", "--base-url", upstream.URL+"/backend-api")
	command.Env = fixture.env
	command.Dir = userHome
	stdout, err := command.StdoutPipe()
	if err != nil {
		return outcome, err
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return outcome, err
	}
	done := false
	defer func() {
		if !done {
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
		return outcome, fmt.Errorf("incomplete gateway did not start: %s", stderr.String())
	}
	parsed, err := url.Parse(address)
	if err != nil || parsed.Hostname() != "127.0.0.1" {
		return outcome, fmt.Errorf("invalid isolated endpoint %q: %w", address, err)
	}
	client := &http.Client{Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}, Timeout: 6 * time.Second}
	for _, stream := range []bool{false, true} {
		payload, encodeErr := json.Marshal(map[string]any{"model": "gpt-6", "input": "synthetic incomplete", "stream": stream})
		if encodeErr != nil {
			return outcome, encodeErr
		}
		response, requestErr := client.Post("http://"+parsed.Host+"/backend-api/codex/responses", "application/json", bytes.NewReader(payload))
		if requestErr != nil {
			return outcome, requestErr
		}
		data, readErr := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		_ = response.Body.Close()
		if readErr != nil {
			return outcome, readErr
		}
		outcome.statuses = append(outcome.statuses, response.StatusCode)
		outcome.contentTypes = append(outcome.contentTypes, response.Header.Get("Content-Type"))
		outcome.bodies = append(outcome.bodies, string(data))
	}
	outcome.calls = int(sent.Load())
	outcome.upstreamAuthenticated = !invalid.Load()
	stopErr := stopStartupGateway(command)
	done = true
	outcome.exited = stopErr == nil || command.ProcessState != nil && command.ProcessState.ExitCode() < 0
	if !outcome.exited {
		return outcome, fmt.Errorf("incomplete gateway exited abnormally: %v stderr=%s", stopErr, stderr.String())
	}
	if !outcome.upstreamAuthenticated {
		return outcome, errors.New("upstream received wrong route, token, model or input")
	}
	return outcome, nil
}
