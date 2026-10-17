package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	prodexCommit = "decfed45f884fe2bef20911303beab7b198698b7"
	prodexSHA    = "3f2fcd776fee0335dc49910d0651adc43a55c8627913556c307755b1575dac3f"
)

type attempt struct{ account, mode, body string }
type backend struct {
	server       *httptest.Server
	mu           sync.Mutex
	attempts     []attempt
	cancelled    int
	wsCounts     map[string]int
	wsHandshakes int
}

func newBackend() *backend {
	b := &backend{wsCounts: make(map[string]int)}
	b.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			b.serveWebSocket(w, r)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		var value map[string]any
		_ = json.Unmarshal(body, &value)
		mode, _ := value["mode"].(string)
		if strings.HasSuffix(r.URL.Path, "/wham/usage") {
			mode = "usage"
		}
		account := r.Header.Get("ChatGPT-Account-Id")
		b.mu.Lock()
		b.attempts = append(b.attempts, attempt{account: account, mode: mode, body: string(body)})
		b.mu.Unlock()
		if mode == "usage" {
			writeJSON(w, http.StatusOK, `{"plan_type":"plus","rate_limit":{"primary_window":{"used_percent":10,"reset_at":4102444800,"limit_window_seconds":18000},"secondary_window":{"used_percent":10,"reset_at":4102444800,"limit_window_seconds":604800}}}`)
			return
		}
		if mode == "cancel" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher, _ := w.(http.Flusher)
			_, _ = io.WriteString(w, "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp-cancel\"}}\n\n")
			_, _ = io.WriteString(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"cancel\"}\n\n")
			if flusher != nil {
				flusher.Flush()
			}
			ticker := time.NewTicker(20 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-r.Context().Done():
					b.mu.Lock()
					b.cancelled++
					b.mu.Unlock()
					return
				case <-ticker.C:
					if _, err := io.WriteString(w, ": keep-alive\n\n"); err != nil {
						return
					}
					if flusher != nil {
						flusher.Flush()
					}
				}
			}
		}
		if mode == "transport" && account == "account-a" {
			hijacker, ok := w.(http.Hijacker)
			if ok {
				conn, _, _ := hijacker.Hijack()
				_ = conn.Close()
			}
			return
		}
		if mode == "overload" && account == "account-a" {
			writeJSON(w, http.StatusServiceUnavailable, `{"error":{"code":"server_is_overloaded","message":"synthetic busy"}}`)
			return
		}
		if mode == "quota" && account == "account-a" {
			writeJSON(w, http.StatusTooManyRequests, `{"error":{"code":"insufficient_quota","message":"synthetic quota"}}`)
			return
		}
		if mode == "commit" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp-committed\"}}\n\n")
			_, _ = io.WriteString(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"visible\"}\n\n")
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			_, _ = io.WriteString(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-committed\"}}\n\n")
			return
		}
		if mode == "seed" {
			w.Header().Set("x-codex-turn-state", "turn-a")
		}
		id := "resp-main"
		if account == "account-b" {
			id = "resp-second"
		}
		writeJSON(w, http.StatusOK, fmt.Sprintf(`{"id":%q,"object":"response","output":[]}`, id))
	}))
	return b
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func (b *backend) close() { b.server.Close() }
func (b *backend) snapshot() ([]attempt, int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]attempt(nil), b.attempts...), b.cancelled
}

type product struct {
	name, binary, root, user, state string
	env                             []string
	cmd                             *exec.Cmd
	endpoint                        string
}

func main() {
	prodex := flag.String("prodex", "", "official Prodex 0.438.2 binary")
	godex := flag.String("godex", "", "Godex binary")
	prodexSource := flag.String("prodex-source", "", "tagged Prodex 0.438.2 source")
	websocket := flag.Bool("websocket", false, "run the isolated WebSocket request/recovery differential")
	websocketCapacity := flag.Bool("websocket-capacity", false, "check 0.438.2 completed WebSocket profile capacity and cached reuse")
	godexSource := flag.String("godex-source", "", "clean Godex commit source (required with either WebSocket fixture)")
	godexCommit := flag.String("godex-commit", "", "exact 40-character Godex commit (required with either WebSocket fixture)")
	flag.Parse()
	if *websocket && *websocketCapacity {
		fatal("select at most one WebSocket fixture")
	}
	if *prodex == "" || *godex == "" || *prodexSource == "" {
		fatal("usage: continuationparity --prodex BIN --godex BIN --prodex-source DIR")
	}
	if err := verifyProdex(*prodex, *prodexSource); err != nil {
		fatal(err.Error())
	}
	if *websocket || *websocketCapacity {
		if err := verifyWebSocketCandidate(*godex, *godexSource, *godexCommit); err != nil {
			fatal(err.Error())
		}
	}
	root, err := os.MkdirTemp("", "godex-continuation-parity-")
	if err != nil {
		fatal(err.Error())
	}
	if os.Getenv("KEEP_CONTINUATION_FIXTURE") == "" {
		defer os.RemoveAll(root)
	}
	b := newBackend()
	defer b.close()
	for _, spec := range []struct{ name, binary string }{{"prodex", *prodex}, {"godex", *godex}} {
		p := product{name: spec.name, binary: spec.binary}
		var err error
		if *websocketCapacity {
			err = runWebSocketCapacityProduct04382(root, b, p)
		} else if *websocket {
			err = runWebSocketProduct(root, b, p)
		} else {
			err = runProduct(root, b, p)
		}
		if err != nil {
			fatal(spec.name + ": " + err.Error())
		}
	}
	if *websocketCapacity {
		fmt.Printf("PASS Prodex 0.438.2 completed-idle WebSocket capacity/reuse: %d total upstream attempts across both binaries\n", len(mustAttempts(b)))
		return
	}
	if *websocket {
		fmt.Printf("PASS WebSocket continuation loopback: %d upstream attempts, strict precommit-retry, hard-owner and no-replay checks\n", len(mustAttempts(b)))
		return
	}
	fmt.Printf("PASS continuation loopback: %d attempts; cancellation negative control recorded\n", len(mustAttempts(b)))
}

func verifyProdex(binary, source string) error {
	if got := sha256File(binary); got != prodexSHA {
		return fmt.Errorf("Prodex binary digest %s, want %s", got, prodexSHA)
	}
	output, err := exec.Command(binary, "--version").CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != "prodex 0.438.2" {
		return fmt.Errorf("unexpected Prodex version %q: %v", output, err)
	}
	if err := verifyProdexSource(source); err != nil {
		return err
	}
	return nil
}

func verifyProdexSource(source string) error {
	output, err := exec.Command("git", "-C", source, "rev-parse", "HEAD").Output()
	if err != nil || strings.TrimSpace(string(output)) != prodexCommit {
		return fmt.Errorf("Prodex source is not tag 0.438.2")
	}
	return verifyProdexSourceClean(source)
}

func verifyProdexSourceClean(source string) error {
	output, err := exec.Command("git", "-C", source, "status", "--porcelain=v1", "--untracked-files=all").Output()
	if err != nil {
		return fmt.Errorf("inspect Prodex source status: %w", err)
	}
	if strings.TrimSpace(string(output)) != "" {
		return errors.New("Prodex source is modified or has untracked artifacts")
	}
	return nil
}

func sha256File(path string) string {
	h := sha256.New()
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	_, _ = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil))
}

func prepareProduct(root string, p *product) error {
	p.root = filepath.Join(root, p.name)
	p.user, p.state = filepath.Join(p.root, "user"), filepath.Join(p.root, "state")
	p.env = productEnv(*p)
	if err := os.MkdirAll(filepath.Join(p.user, ".codex"), 0700); err != nil {
		return err
	}
	if err := os.Chmod(p.root, 0700); err != nil {
		return err
	}
	if err := writeAuth(filepath.Join(p.user, ".codex", "auth.json"), "account-a", "token-a"); err != nil {
		return err
	}
	if _, err := invoke(p.binary, p.env, "profile", "import-current", "main", "--insecure"); err != nil {
		return err
	}
	if err := writeAuth(filepath.Join(p.user, ".codex", "auth.json"), "account-b", "token-b"); err != nil {
		return err
	}
	if _, err := invoke(p.binary, p.env, "profile", "import-current", "second", "--insecure"); err != nil {
		return err
	}
	if _, err := invoke(p.binary, p.env, "profile", "use", "--profile", "main"); err != nil {
		return err
	}
	return seedProductState(*p)
}

func runProduct(root string, b *backend, p product) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	startIndex := len(mustAttempts(b))
	endpoint, cmd, err := startGateway(p, b.server.URL+"/backend-api")
	if err != nil {
		return err
	}
	p.cmd, p.endpoint = cmd, endpoint
	defer stopGateway(cmd)
	if p.name == "prodex" {
		if err := waitForFile(filepath.Join(p.state, "runtime-usage-snapshots.json"), 5*time.Second); err != nil {
			return err
		}
	}
	client := &http.Client{Timeout: 35 * time.Second}
	conflict, err := post(client, endpoint+"/responses", `{"mode":"route","session_id":"sess-conflict","previous_response_id":"resp-main","input":[]}`)
	if err != nil || conflict.status != http.StatusOK || !strings.Contains(conflict.body, "resp-main") {
		return fmt.Errorf("hard owner versus soft session: %v status=%d body=%s", err, conflict.status, conflict.body)
	}
	turn, err := postContextHeader(client, endpoint+"/responses", `{"mode":"route","session_id":"sess-conflict","input":[]}`, "x-codex-turn-state", "turn-a")
	if err != nil || turn.status != http.StatusOK || !strings.Contains(turn.body, "resp-main") {
		return fmt.Errorf("turn-state owner versus soft session: %v status=%d body=%s", err, turn.status, turn.body)
	}
	committedBefore := postCount(mustAttempts(b))
	committed, err := post(client, endpoint+"/responses", `{"mode":"commit","stream":true,"input":[]}`)
	if err != nil || committed.status != http.StatusOK || !strings.Contains(committed.body, "visible") {
		return fmt.Errorf("committed stream: %v status=%d body=%s", err, committed.status, committed.body)
	}
	if postCount(mustAttempts(b)) != committedBefore+1 {
		return errors.New("committed output was replayed on another account")
	}
	cancellationBefore := mustCancelled(b)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	_, _ = postContext(ctx, client, endpoint+"/responses", `{"mode":"cancel","stream":true,"input":[]}`)
	cancel()
	cancellationObserved := waitForCancellation(b, cancellationBefore, time.Second)
	if !cancellationObserved {
		attempts, cancelled := b.snapshot()
		return fmt.Errorf("cancellation did not reach upstream: before=%d after=%d attempts=%s", cancellationBefore, cancelled, formatAttempts(attempts))
	}
	continuation, err := post(client, endpoint+"/responses", `{"mode":"overload","session_id":"sess-hard","previous_response_id":"resp-main","input":[{"type":"function_call_output","call_id":"call","output":"done"}]}`)
	continuationSafe := continuation.status == http.StatusBadRequest && strings.Contains(continuation.body, "previous_response_not_found")
	continuationPassthrough := continuation.status >= http.StatusInternalServerError && strings.Contains(continuation.body, "server_is_overloaded")
	if err != nil || (!continuationSafe && !continuationPassthrough) {
		attempts, _ := b.snapshot()
		return fmt.Errorf("hard quota continuation: %v status=%d body=%s attempts=%v", err, continuation.status, continuation.body, attempts)
	}
	replay, err := post(client, endpoint+"/responses", `{"mode":"route","session_id":"sess-hard","input":[{"role":"user","content":"one"},{"role":"assistant","content":"answer"},{"role":"user","content":"two"}]}`)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("full replay: %v status=%d body=%s", err, replay.status, replay.body)
	}
	if err == nil && replay.status != http.StatusOK && replay.status < http.StatusInternalServerError {
		return fmt.Errorf("full replay: status=%d body=%s", replay.status, replay.body)
	}
	quota, err := post(client, endpoint+"/responses", `{"mode":"quota","session_id":"sess-conflict","input":[]}`)
	if err != nil || (quota.status != http.StatusOK && quota.status != http.StatusTooManyRequests && quota.status != http.StatusServiceUnavailable) {
		return fmt.Errorf("quota-positive alternate: %v status=%d body=%s", err, quota.status, quota.body)
	}
	transport, err := post(client, endpoint+"/responses", `{"mode":"transport","session_id":"sess-transport","input":[]}`)
	if err != nil || (transport.status != http.StatusOK && transport.status != http.StatusServiceUnavailable) {
		return fmt.Errorf("soft session transport recovery: %v status=%d body=%s", err, transport.status, transport.body)
	}
	stopGateway(cmd)
	endpoint, cmd, err = startGateway(p, b.server.URL+"/backend-api")
	if err != nil {
		return err
	}
	defer stopGateway(cmd)
	resume, err := post(client, endpoint+"/responses", `{"mode":"route","session_id":"sess-transport","input":[]}`)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("restart rebind: %v status=%d body=%s", err, resume.status, resume.body)
	}
	attempts, _ := b.snapshot()
	attempts = attempts[startIndex:]
	if len(attempts) == 0 {
		return errors.New("loopback saw no upstream attempts")
	}
	fmt.Printf("%s: hard_failure=%s quota=%s replay=%s committed=%s attempts=%d modes=%s cancellation_upstream=%t trace=%s\n", p.name, summarizeExchange(continuation), summarizeExchange(quota), summarizeExchange(replay), summarizeExchange(committed), len(attempts), formatAttemptCounts(attempts), cancellationObserved, formatAttempts(attempts))
	return nil
}

type exchange struct {
	status int
	body   string
}

func summarizeExchange(value exchange) string {
	marker := responseMarker(value.body)
	if marker == "" {
		return strconv.Itoa(value.status)
	}
	return strconv.Itoa(value.status) + "/" + marker
}

func responseMarker(body string) string {
	if strings.Contains(body, "visible") {
		return "visible"
	}
	var value any
	if json.Unmarshal([]byte(body), &value) != nil {
		return ""
	}
	var visit func(any) string
	visit = func(current any) string {
		switch typed := current.(type) {
		case map[string]any:
			if code, ok := typed["code"].(string); ok && strings.TrimSpace(code) != "" {
				return code
			}
			if id, ok := typed["id"].(string); ok && strings.TrimSpace(id) != "" {
				return id
			}
			for _, child := range typed {
				if marker := visit(child); marker != "" {
					return marker
				}
			}
		case []any:
			for _, child := range typed {
				if marker := visit(child); marker != "" {
					return marker
				}
			}
		}
		return ""
	}
	return visit(value)
}

func post(client *http.Client, url, body string) (exchange, error) {
	return postContext(context.Background(), client, url, body)
}
func postContext(ctx context.Context, client *http.Client, url, body string) (exchange, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return exchange{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return exchange{}, err
	}
	defer resp.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return exchange{status: resp.StatusCode, body: string(data)}, readErr
}

func postContextHeader(client *http.Client, url, body, name, value string) (exchange, error) {
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return exchange{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(name, value)
	resp, err := client.Do(req)
	if err != nil {
		return exchange{}, err
	}
	defer resp.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return exchange{status: resp.StatusCode, body: string(data)}, readErr
}

func seedProductState(p product) error {
	now := time.Now().Unix()
	if p.name == "prodex" {
		binding := func(profile string) map[string]any {
			return map[string]any{"profile_name": profile, "bound_at": now}
		}
		status := func() map[string]any {
			return map[string]any{"state": "Verified", "confidence": 4, "last_touched_at": now, "last_verified_at": now, "last_verified_route": "responses", "success_count": 1}
		}
		continuations := map[string]any{
			"generation": 1,
			"value": map[string]any{
				"response_profile_bindings": map[string]any{"resp-main": binding("main")},
				"session_profile_bindings": map[string]any{
					"sess-hard": binding("main"), "sess-conflict": binding("second"), "sess-transport": binding("main"),
				},
				"turn_state_bindings": map[string]any{"turn-a": binding("main")},
				"session_id_bindings": map[string]any{
					"sess-hard": binding("main"), "sess-conflict": binding("second"), "sess-transport": binding("main"),
				},
				"statuses": map[string]any{
					"response":   map[string]any{"resp-main": status()},
					"turn_state": map[string]any{"turn-a": status()},
					"session_id": map[string]any{"sess-hard": status(), "sess-conflict": status(), "sess-transport": status()},
				},
			},
		}
		if err := writeJSONFile(filepath.Join(p.state, "runtime-continuations.json"), continuations); err != nil {
			return err
		}
		usage := map[string]any{"generation": 1, "value": map[string]any{
			"main":   map[string]any{"checked_at": now, "plan_type": "plus", "five_hour_status": "Ready", "five_hour_remaining_percent": 90, "five_hour_reset_at": now + 3600, "weekly_status": "Ready", "weekly_remaining_percent": 90, "weekly_reset_at": now + 86400},
			"second": map[string]any{"checked_at": now, "plan_type": "plus", "five_hour_status": "Ready", "five_hour_remaining_percent": 90, "five_hour_reset_at": now + 3600, "weekly_status": "Ready", "weekly_remaining_percent": 90, "weekly_reset_at": now + 86400},
		}}
		return writeJSONFile(filepath.Join(p.state, "runtime-usage-snapshots.json"), usage)
	}
	var state struct {
		Accounts []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"accounts"`
	}
	data, err := os.ReadFile(filepath.Join(p.state, "state.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	ids := map[string]string{}
	for _, account := range state.Accounts {
		ids[account.Name] = account.ID
	}
	usage := map[string]any{"generation": 1, "value": map[string]any{}}
	usageValue := usage["value"].(map[string]any)
	for _, account := range state.Accounts {
		usageValue[account.ID] = map[string]any{
			"checked_at": now, "plan_type": "plus",
			"five_hour_status": "Ready", "five_hour_remaining_percent": 90, "five_hour_reset_at": now + 3600,
			"weekly_status": "Ready", "weekly_remaining_percent": 90, "weekly_reset_at": now + 86400,
		}
	}
	if err := writeJSONFile(filepath.Join(p.state, "runtime-usage-snapshots.json"), usage); err != nil {
		return err
	}
	bindings := make([]map[string]any, 0, 6)
	add := func(kind, value, account string) {
		digest := sha256.Sum256([]byte(kind + ":" + value))
		bindings = append(bindings, map[string]any{"kind": kind, "key": hex.EncodeToString(digest[:]), "account_id": ids[account], "updated_unix": now})
	}
	add("previous", "resp-main", "main")
	add("session", "sess-hard", "main")
	add("session", "sess-conflict", "second")
	add("session", "sess-transport", "main")
	add("turn", "turn-a", "main")
	return writeJSONFile(filepath.Join(p.state, "routing.json"), map[string]any{"version": 1, "bindings": bindings})
}

func formatAttempts(attempts []attempt) string {
	parts := make([]string, 0, len(attempts))
	for _, item := range attempts {
		mode := item.mode
		if mode == "" {
			mode = "unclassified"
		}
		parts = append(parts, mode+"/"+item.account)
	}
	return strings.Join(parts, ",")
}

func formatAttemptCounts(attempts []attempt) string {
	counts := make(map[string]int)
	for _, item := range attempts {
		mode := item.mode
		if mode == "" {
			mode = "unclassified"
		}
		counts[mode]++
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+strconv.Itoa(counts[key]))
	}
	return strings.Join(parts, ",")
}

func writeJSONFile(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func writeAuth(path, account, token string) error {
	data := []byte(fmt.Sprintf(`{"auth_mode":"chatgpt","tokens":{"access_token":%q,"account_id":%q}}`, token, account))
	return os.WriteFile(path, data, 0600)
}

func productEnv(p product) []string {
	base := make([]string, 0, len(os.Environ())+16)
	for _, item := range os.Environ() {
		key := strings.SplitN(item, "=", 2)[0]
		switch key {
		case "HOME", "CODEX_HOME", "GODEX_HOME", "PRODEX_HOME", "PRODEX_CODEX_BIN", "GODEX_CODEX_BIN", "PRODEX_SECRET_BACKEND":
			continue
		}
		base = append(base, item)
	}
	base = append(base, "HOME="+p.user, "CODEX_HOME="+filepath.Join(p.user, ".codex"), "PRODEX_HOME="+p.state, "GODEX_HOME="+p.state, "PRODEX_CODEX_BIN=/bin/true", "GODEX_CODEX_BIN=/bin/true", "PRODEX_SECRET_BACKEND=file", "PRODEX_NO_UPDATE_CHECK=1", "GODEX_NO_UPDATE_CHECK=1", "PRODEX_PRESIDIO_AUTO_START=0", "PRODEX_RUNTIME_LOG_DIR="+filepath.Join(p.state, "logs"), "GODEX_RUNTIME_LOG_DIR="+filepath.Join(p.state, "logs"), "NO_PROXY=127.0.0.1,localhost", "no_proxy=127.0.0.1,localhost")
	return base
}

func invoke(binary string, env []string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%s: %w: %s", strings.Join(args, " "), err, out)
	}
	return out, nil
}

func startGateway(p product, baseURL string) (string, *exec.Cmd, error) {
	args := []string{"gateway", "--listen", "127.0.0.1:0", "--base-url", baseURL, "--no-presidio"}
	cmd := exec.Command(p.binary, args...)
	cmd.Env = p.env
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", nil, err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return "", nil, err
	}
	lines := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.Contains(line, "http://127.0.0.1:") && strings.Contains(line, "/backend-api/") {
				lines <- line
				return
			}
		}
	}()
	select {
	case line := <-lines:
		start := strings.Index(line, "http://127.0.0.1:")
		return line[start:], cmd, nil
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return "", nil, errors.New("gateway did not announce a loopback endpoint")
	}
}

func stopGateway(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(os.Interrupt)
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
}

func waitForFile(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s", path)
}

func mustAttempts(b *backend) []attempt { attempts, _ := b.snapshot(); return attempts }
func mustCancelled(b *backend) int      { _, cancelled := b.snapshot(); return cancelled }

func waitForCancellation(b *backend, before int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if mustCancelled(b) > before {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func postCount(attempts []attempt) int {
	count := 0
	for _, item := range attempts {
		if item.mode != "" && item.mode != "usage" {
			count++
		}
	}
	return count
}
func fatal(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
