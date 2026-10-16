package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"time"
)

// Tagged Prodex 0.437.11 uses the OpenAI-compatible Chat Completions wire
// format for Gemini API-key Super. Keep its executable differential separate
// so the existing DeepSeek --scenario all remains unchanged.
type geminiCapturedRequest struct {
	Method string         `json:"method"`
	Path   string         `json:"path"`
	Auth   bool           `json:"synthetic_auth_present"`
	Slot   string         `json:"credential_slot,omitempty"`
	Body   map[string]any `json:"body"`
}

type geminiCandidateResult struct {
	Name     string                  `json:"name"`
	Version  string                  `json:"version"`
	ExitCode int                     `json:"exit_code"`
	Upstream []geminiCapturedRequest `json:"upstream"`
	Client   exchange                `json:"client_exchange"`
}

type geminiCaseResult struct {
	Name        string                  `json:"name"`
	Runs        []geminiCandidateResult `json:"runs"`
	Differences []string                `json:"comparison_differences"`
}

type geminiDifferentialReport struct {
	Status          string             `json:"status"`
	ProdexSource    string             `json:"prodex_source"`
	GodexSource     string             `json:"godex_source"`
	Cases           []geminiCaseResult `json:"cases"`
	Differences     []string           `json:"comparison_differences"`
	NegativeControl string             `json:"negative_control"`
}

type geminiMock struct {
	server   *httptest.Server
	scenario string
	mu       sync.Mutex
	requests []geminiCapturedRequest
}

func newGeminiMock(scenario string) *geminiMock {
	mock := &geminiMock{scenario: scenario}
	mock.server = httptest.NewServer(http.HandlerFunc(mock.serve))
	return mock
}

func (mock *geminiMock) serve(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, bodyLimit+1))
	if err != nil || len(body) > bodyLimit {
		http.Error(w, "fixture request too large", http.StatusRequestEntityTooLarge)
		return
	}
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "{\"models\":[{\"name\":\"models/gemini-3.1-pro-preview\",\"supportedGenerationMethods\":[\"generateContent\"]}]}")
		return
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		http.Error(w, "fixture request invalid JSON", http.StatusBadRequest)
		return
	}
	slot := geminiCredentialSlot043711(r)
	record := geminiCapturedRequest{
		Method: r.Method, Path: r.URL.RequestURI(), Body: decoded,
		Auth: slot != "",
		Slot: slot,
	}
	mock.mu.Lock()
	mock.requests = append(mock.requests, record)
	mock.mu.Unlock()
	if geminiQuotaScenario043711(mock.scenario) && slot == "primary" {
		w.Header().Set("X-Synthetic-Quota", "primary")
		if mock.scenario == "quota-plain-429" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, "Quota exhausted for this account\n")
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":{"code":"rate_limit_exceeded","message":"synthetic quota exceeded"}}`)
		}
		return
	}
	if mock.scenario == "stream-chat" {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: "+`{"id":"chat-synthetic","model":"gemini-3.1-pro-preview","choices":[{"delta":{"content":"synthetic-"},"finish_reason":null}]}`+"\n\n")
		_, _ = io.WriteString(w, "data: "+`{"id":"chat-synthetic","model":"gemini-3.1-pro-preview","choices":[{"delta":{"content":"ok"},"finish_reason":null}]}`+"\n\n")
		_, _ = io.WriteString(w, "data: "+`{"id":"chat-synthetic","model":"gemini-3.1-pro-preview","choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`+"\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		return
	}
	if mock.scenario == "stream-native" {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: "+`{"candidates":[{"content":{"role":"model","parts":[{"text":"synthetic-"}]}}],"modelVersion":"gemini-3.1-pro-preview"}`+"\n\n")
		_, _ = io.WriteString(w, "data: "+`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"totalTokenCount":3}}`+"\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if mock.scenario == "buffered-null-tool-text" {
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"hidden before"},{"functionCall":null},{"text":"hidden after"}]},"finishReason":"STOP"}],"modelVersion":"gemini-3.1-pro-preview","usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"totalTokenCount":3}}`)
		return
	}
	if mock.scenario == "chat-response" {
		response := map[string]any{
			"id": "chatcmpl-gemini-fixture", "object": "chat.completion",
			"created": 1, "model": "gemini-3.1-pro-preview",
			"choices": []any{map[string]any{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": "synthetic-ok"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 2, "completion_tokens": 1, "total_tokens": 3},
		}
		_ = json.NewEncoder(w).Encode(response)
		return
	}
	response := map[string]any{
		"candidates": []any{map[string]any{
			"content":      map[string]any{"role": "model", "parts": []any{map[string]any{"text": "synthetic-ok"}}},
			"finishReason": "STOP",
		}},
		"modelVersion": "gemini-3.1-pro-preview",
		"usageMetadata": map[string]any{
			"promptTokenCount": 2, "candidatesTokenCount": 1, "totalTokenCount": 3,
		},
	}
	if mock.scenario == "wrapped-response-trace" || mock.scenario == "wrapped-response-no-trace" {
		// Prodex 0.438.0 unwraps the nested GenerateContent response even
		// if the optional outer traceId was absent. A present string traceId
		// has precedence over any stale nested responseId.
		response["responseId"] = "stale-nested-id"
		envelope := map[string]any{"response": response, "outerDecoy": "must-not-leak"}
		if mock.scenario == "wrapped-response-trace" {
			envelope["traceId"] = "synthetic-trace-id"
		}
		_ = json.NewEncoder(w).Encode(envelope)
		return
	}
	_ = json.NewEncoder(w).Encode(response)
}

func (mock *geminiMock) recorded() []geminiCapturedRequest {
	mock.mu.Lock()
	defer mock.mu.Unlock()
	return append([]geminiCapturedRequest(nil), mock.requests...)
}

func runGeminiDifferential(prodexBin, godexBin, prodexCommit, godexCommit string) error {
	root, err := os.MkdirTemp("", "godex-gemini-parity-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	var results []geminiCaseResult
	for _, scenario := range []string{"chat-request", "orphan-tool", "chat-response", "wrapped-response-no-trace", "wrapped-response-trace", "stream-native", "stream-chat", "multi-turn-tool", "quota-plain-429", "quota-structured-429", "invalid-wrapped-tool-parameters", "invalid-flat-tool-parameters", "buffered-null-tool-text"} {
		prodex, err := runGeminiProduct(root, scenario, "prodex", prodexBin)
		if err != nil {
			return fmt.Errorf("Prodex Gemini %s: %w", scenario, err)
		}
		godex, err := runGeminiProduct(root, scenario, "godex", godexBin)
		if err != nil {
			return fmt.Errorf("Godex Gemini %s: %w", scenario, err)
		}
		differences := geminiCompare(scenario, prodex, godex)
		results = append(results, geminiCaseResult{
			Name: scenario, Runs: []geminiCandidateResult{prodex, godex},
			Differences: differences,
		})
	}
	var differences []string
	for _, result := range results {
		differences = append(differences, result.Differences...)
	}
	differences = uniqueStrings(differences)
	if !geminiNegativeControl() {
		return errors.New("Gemini differential negative controls failed: semantic status, request routing or response mutations were not detected")
	}
	status := "PASS"
	if len(differences) != 0 {
		status = "FAIL"
	}
	report := geminiDifferentialReport{
		Status: status, ProdexSource: prodexCommit, GodexSource: godexCommit,
		Cases: results, Differences: differences,
		NegativeControl: "PASS: route, auth, model fallback, credential rotation, child exit, native/chat SSE, multi-turn tool adjacency, output, status, usage and error mutations detected",
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	if status != "PASS" {
		return fmt.Errorf("Gemini executable differential mismatched: %v", differences)
	}
	return nil
}

func runGeminiProduct(root, scenario, name, binary string) (geminiCandidateResult, error) {
	mock := newGeminiMock(scenario)
	defer mock.server.Close()
	home := filepath.Join(root, scenario, name)
	stateHome := filepath.Join(home, "state")
	codexHome := filepath.Join(home, "codex")
	userHome := filepath.Join(home, "user")
	for _, dir := range []string{stateHome, codexHome, userHome, filepath.Join(userHome, ".config")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return geminiCandidateResult{}, err
		}
	}
	shim, err := os.Executable()
	if err != nil {
		return geminiCandidateResult{}, err
	}
	childResult := filepath.Join(home, "gemini-child-exchange.json")
	args := []string{"super", "--provider", "gemini"}
	if !geminiQuotaScenario043711(scenario) {
		args = append(args, "--api-key", apiKey)
	}
	args = append(args,
		"--model", "gemini-3.1-pro-preview", "--base-url", mock.server.URL+"/v1beta",
		"--no-presidio", "--no-sub-agent", "exec", "synthetic Gemini differential",
	)
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	env := productEnv(name, stateHome, codexHome, userHome, shim, childResult, runOptions{})
	env = append(env, "DIFFERENTIAL_PROVIDER=gemini")
	if geminiQuotaScenario043711(scenario) {
		env = append(env, "GEMINI_API_KEYS="+rotationPrimaryKey+","+rotationSecondaryKey)
	}
	if scenario == "invalid-wrapped-tool-parameters" {
		env = append(env, "DIFFERENTIAL_GEMINI_INVALID_TOOL=1")
	}
	if scenario == "invalid-flat-tool-parameters" {
		env = append(env, "DIFFERENTIAL_GEMINI_INVALID_TOOL=2")
	}
	if scenario == "orphan-tool" {
		env = append(env, "DIFFERENTIAL_GEMINI_TOOL=1")
	}
	if scenario == "multi-turn-tool" {
		env = append(env, "DIFFERENTIAL_GEMINI_MULTITURN=1")
	}
	if scenario == "stream-native" || scenario == "stream-chat" {
		env = append(env, "DIFFERENTIAL_STREAM=1")
	}
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	exitCode := 0
	if err := cmd.Run(); err != nil {
		var childExit *exec.ExitError
		if !geminiQuotaScenario043711(scenario) && !geminiInvalidToolScenario04380(scenario) ||
			!errors.As(err, &childExit) || ctx.Err() != nil {
			return geminiCandidateResult{}, fmt.Errorf("child failed: %w stderr=%q upstream=%v",
				err, redact(stderr.String()), mock.recorded())
		}
		exitCode = childExit.ExitCode()
	}
	result, err := readExchange(childResult)
	if err != nil {
		return geminiCandidateResult{}, fmt.Errorf("child response missing: %w stderr=%q upstream=%v",
			err, redact(stderr.String()), mock.recorded())
	}
	version, err := productVersion(binary)
	if err != nil {
		return geminiCandidateResult{}, err
	}
	return geminiCandidateResult{
		Name: name, Version: version, ExitCode: exitCode, Upstream: mock.recorded(), Client: result,
	}, nil
}

func geminiCompare(scenario string, prodex, godex geminiCandidateResult) []string {
	var differences []string
	for _, result := range []geminiCandidateResult{prodex, godex} {
		if err := geminiFixtureInvariant(scenario, result); err != nil {
			differences = append(differences, result.Name+": "+err.Error())
		}
	}
	if prodex.Client.Status != godex.Client.Status {
		differences = append(differences, "client HTTP status differs")
	}
	if prodex.ExitCode != godex.ExitCode {
		differences = append(differences, "product child process exit status differs")
	}
	if geminiQuotaScenario043711(scenario) {
		return geminiQuotaCompare043711(scenario, prodex, godex, differences)
	}
	if scenario == "buffered-null-tool-text" {
		left, leftErr := geminiBufferedNullToolProjection04380(prodex.Client.Body)
		right, rightErr := geminiBufferedNullToolProjection04380(godex.Client.Body)
		if leftErr != nil || rightErr != nil || !reflect.DeepEqual(left, right) {
			differences = append(differences, "buffered null functionCall tool/text suppression or metadata differs")
		}
		if len(prodex.Upstream) != 1 || len(godex.Upstream) != 1 ||
			len(prodex.Upstream) == 1 && len(godex.Upstream) == 1 && !reflect.DeepEqual(prodex.Upstream[0].Body, godex.Upstream[0].Body) {
			differences = append(differences, "null functionCall upstream request count/body differs")
		}
		return differences
	}
	if geminiInvalidToolScenario04380(scenario) {
		if !geminiInvalidToolWireEqual04380(prodex.Client.Body, godex.Client.Body) {
			differences = append(differences, "Gemini invalid-tool rejection error envelope or reason differs")
		}
		if len(prodex.Upstream) != 0 || len(godex.Upstream) != 0 {
			differences = append(differences, "invalid Gemini tool was forwarded to provider")
		}
		return differences
	}
	if scenario == "stream-chat" {
		left, leftErr := geminiChatSSEProjection043711(prodex.Client.Body)
		right, rightErr := geminiChatSSEProjection043711(godex.Client.Body)
		if leftErr != nil || rightErr != nil || !reflect.DeepEqual(left, right) {
			differences = append(differences, "client Gemini Chat SSE item lifecycle, usage or metadata differs")
		}
	} else if scenario == "stream-native" {
		left, leftErr := geminiNativeSSEProjection043711(prodex.Client.Body)
		right, rightErr := geminiNativeSSEProjection043711(godex.Client.Body)
		if leftErr != nil || rightErr != nil || !reflect.DeepEqual(left, right) {
			differences = append(differences, "client Gemini Chat-compatible SSE event sequence differs")
		}
	} else {
		left, leftErr := geminiResponseProjection(prodex.Client.Body)
		right, rightErr := geminiResponseProjection(godex.Client.Body)
		if leftErr != nil || rightErr != nil || !geminiSemanticEqual043711(left, right) {
			differences = append(differences, "client semantic Responses body differs")
		}
	}
	if len(prodex.Upstream) != len(godex.Upstream) {
		differences = append(differences, "upstream attempt count differs")
	} else if len(prodex.Upstream) != 0 &&
		!reflect.DeepEqual(prodex.Upstream[0].Body, godex.Upstream[0].Body) {
		differences = append(differences, "provider Chat Completions request differs")
	}
	return differences
}

// Stable client semantics are compared independently of generated response
// UUIDs and wall-clock timestamps. Everything else, including error codes,
// output, usage and metadata, remains material.
func geminiResponseProjection(body string) (map[string]any, error) {
	var decoded map[string]any
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		return nil, err
	}
	delete(decoded, "id")
	delete(decoded, "created_at")
	return decoded, nil
}

func geminiSemanticEqual043711(left, right map[string]any) bool {
	return reflect.DeepEqual(left, right)
}

func geminiFixtureInvariant(scenario string, result geminiCandidateResult) error {
	if scenario == "buffered-null-tool-text" {
		if result.Client.Status != http.StatusOK || result.ExitCode != 0 || len(result.Upstream) != 1 {
			return fmt.Errorf("buffered tool/visible text request status/exit/upstream incorrect: %d/%d/%d", result.Client.Status, result.ExitCode, len(result.Upstream))
		}
		_, err := geminiBufferedNullToolProjection04380(result.Client.Body)
		return err
	}
	if geminiInvalidToolScenario04380(scenario) {
		return geminiInvalidToolInvariant04380(scenario, result)
	}
	if geminiQuotaScenario043711(scenario) {
		return geminiQuotaInvariant043711(scenario, result)
	}
	if result.Client.Status != http.StatusOK {
		return fmt.Errorf("client HTTP status=%d, want 200", result.Client.Status)
	}
	if len(result.Upstream) != 1 {
		return fmt.Errorf("upstream attempted %d times, want one", len(result.Upstream))
	}
	attempt := result.Upstream[0]
	if attempt.Method != http.MethodPost || attempt.Path != "/v1beta/openai/chat/completions" {
		return fmt.Errorf("unexpected provider route %s %s", attempt.Method, attempt.Path)
	}
	if !attempt.Auth {
		return errors.New("Gemini API-key authentication missing from synthetic upstream request")
	}
	if attempt.Body["model"] != "gemini-3.1-pro-preview" ||
		attempt.Body["stream"] != (scenario == "stream-native" || scenario == "stream-chat") {
		return fmt.Errorf("Gemini model or stream flag rewritten: %+v", attempt.Body)
	}
	messages, _ := attempt.Body["messages"].([]any)
	if scenario == "multi-turn-tool" {
		if err := geminiMultiTurnToolInvariant043711(messages); err != nil {
			return err
		}
	} else if len(messages) != 1 {
		return fmt.Errorf("canonical chat request expected one user message: %+v", attempt.Body)
	}
	first, _ := messages[0].(map[string]any)
	if first["role"] != "user" || first["content"] != "same request" {
		return fmt.Errorf("user conversation was dropped or changed: %+v", messages)
	}
	if scenario == "stream-chat" {
		if options, ok := attempt.Body["stream_options"].(map[string]any); !ok || options["include_usage"] != true {
			return fmt.Errorf("Gemini valid Chat SSE request omitted usage: %+v", attempt.Body)
		}
		if _, err := geminiChatSSEProjection043711(result.Client.Body); err != nil {
			return fmt.Errorf("Gemini Chat SSE client lifecycle failed canonical oracle: %w", err)
		}
		return nil
	}
	if scenario == "stream-native" {
		if options, ok := attempt.Body["stream_options"].(map[string]any); !ok || options["include_usage"] != true {
			return fmt.Errorf("Gemini Chat-compatible SSE request omitted include_usage=true: %+v", attempt.Body)
		}
		if _, err := geminiNativeSSEProjection043711(result.Client.Body); err != nil {
			return fmt.Errorf("native Gemini SSE produced invalid Chat-compatible client stream: %w", err)
		}
		return nil
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(result.Client.Body), &body); err != nil {
		return fmt.Errorf("client Responses body is not valid JSON: %w", err)
	}
	if body["object"] != "response" {
		return fmt.Errorf("client Responses object mismatched: %+v", body)
	}
	output, _ := body["output"].([]any)
	if scenario == "chat-response" {
		failure, _ := body["error"].(map[string]any)
		metadata, metadataOK := body["metadata"].(map[string]any)
		usage, usageOK := body["usage"].(map[string]any)
		_, timestampPresent := body["created_at"]
		if body["status"] != "failed" || failure["code"] != "gemini_empty_response" ||
			failure["message"] != "Gemini returned no visible response content." ||
			len(output) != 0 || body["model"] != "gemini-2.5-pro" ||
			body["id"] != "gemini_resp_prodex" || timestampPresent ||
			!metadataOK || len(metadata) != 0 || !usageOK || len(usage) != 0 {
			return fmt.Errorf("Chat Completion envelope incorrectly treated as valid native Gemini output: %+v", body)
		}
		return nil
	}
	if err := geminiBufferedIdentityInvariant043711(body); err != nil {
		return err
	}
	if body["model"] != "gemini-3.1-pro-preview" || len(output) == 0 {
		return fmt.Errorf("native Gemini model output mismatched: %+v", body)
	}
	message, _ := output[0].(map[string]any)
	content, _ := message["content"].([]any)
	if len(content) == 0 {
		return errors.New("client output has no visible text parts")
	}
	firstPart, _ := content[0].(map[string]any)
	if firstPart["text"] != "synthetic-ok" {
		return fmt.Errorf("client lost synthetic provider content: %+v", content)
	}
	usage, _ := body["usage"].(map[string]any)
	if usage["input_tokens"] != float64(2) || usage["output_tokens"] != float64(1) ||
		usage["total_tokens"] != float64(3) {
		return fmt.Errorf("client lost synthetic usage accounting: %+v", usage)
	}
	return nil
}
