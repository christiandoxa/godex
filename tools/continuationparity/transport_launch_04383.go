package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// A real managed launch admits multiple profiles. The public gateway command
// deliberately fixes its pool to the active profile in both products.
func startTransportLaunch04383(p product, upstream string) (string, func(), error) {
	endpoint := make(chan string, 1)
	release := make(chan struct{})
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
		if err != nil || r.Method != http.MethodPost || !strings.HasPrefix(string(body), "http://127.0.0.1:") {
			http.Error(w, "invalid child endpoint", 400)
			return
		}
		select {
		case endpoint <- string(body):
		case <-r.Context().Done():
			return
		}
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	command := exec.Command(p.binary, "run", "--base-url", upstream,
		"exec", "synthetic transport fixture")
	command.Dir = p.user
	command.Env = append(p.env, "GODEX_CONTINUATION_TRANSPORT_CONTROL="+control.URL,
		"CODEX_CHATGPT_BASE_URL="+upstream)
	configureGatewayProcess(command)
	command.Stdout, command.Stderr = io.Discard, io.Discard
	if err := command.Start(); err != nil {
		close(release)
		control.Close()
		return "", nil, err
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	stop := func() {
		close(release)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = command.Process.Kill()
			<-done
		}
		control.Close()
	}
	select {
	case address := <-endpoint:
		return address, stop, nil
	case err := <-done:
		close(release)
		control.Close()
		return "", nil, fmt.Errorf("managed launch exited before child endpoint: %v", err)
	case <-time.After(15 * time.Second):
		stop()
		return "", nil, errors.New("managed launch did not publish child endpoint")
	}
}

func transportChild04383(control string, arguments []string) error {
	for _, argument := range arguments {
		if argument == "--version" || argument == "exec-server" || argument == "app-server" {
			fmt.Println("codex-cli 0.163.0")
			return nil
		}
	}
	for _, argument := range arguments {
		key, value, ok := strings.Cut(argument, "=")
		if !ok || !strings.HasPrefix(key, "model_providers.") || !strings.HasSuffix(key, ".base_url") {
			continue
		}
		endpoint, err := strconv.Unquote(value)
		if err != nil || !strings.HasPrefix(endpoint, "http://127.0.0.1:") {
			return errors.New("child received invalid loopback endpoint")
		}
		client := &http.Client{Timeout: 90 * time.Second}
		response, err := client.Post(control, "text/plain", strings.NewReader(endpoint))
		if err != nil {
			return err
		}
		defer response.Body.Close()
		_, err = io.Copy(io.Discard, response.Body)
		return err
	}
	return errors.New("managed child did not receive proxy configuration")
}
