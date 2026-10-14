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
	Body   map[string]any `json:"body"`
}

type geminiCandidateResult struct {
	Name     string                  `json:"name"`
	Version  string                  `json:"version"`
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
	record := geminiCapturedRequest{
		Method: r.Method, Path: r.URL.RequestURI(), Body: decoded,
		Auth: r.Header.Get("X-Goog-Api-Key") == apiKey ||
			r.Header.Get("Authorization") == "Bearer "+apiKey ||
			r.URL.Query().Get("key") == apiKey,
	}
	mock.mu.Lock()
	mock.requests = append(mock.requests, record)
	mock.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
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
	for _, scenario := range []string{"chat-request", "orphan-tool", "chat-response"} {
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
		NegativeControl: "PASS: route, authentication, model, client status, response payload and error mutations are detected",
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
	args := []string{
		"super", "--provider", "gemini", "--api-key", apiKey,
		"--model", "gemini-3.1-pro-preview", "--base-url", mock.server.URL + "/v1beta",
		"--no-presidio", "--no-sub-agent", "exec", "synthetic Gemini differential",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	env := productEnv(name, stateHome, codexHome, userHome, shim, childResult, runOptions{})
	env = append(env, "DIFFERENTIAL_PROVIDER=gemini")
	if scenario == "orphan-tool" {
		env = append(env, "DIFFERENTIAL_GEMINI_TOOL=1")
	}
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return geminiCandidateResult{}, fmt.Errorf("child failed: %w stderr=%q upstream=%v",
			err, redact(stderr.String()), mock.recorded())
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
		Name: name, Version: version, Upstream: mock.recorded(), Client: result,
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
	left, leftErr := geminiResponseProjection(prodex.Client.Body)
	right, rightErr := geminiResponseProjection(godex.Client.Body)
	if leftErr != nil || rightErr != nil || !geminiSemanticEqual043711(left, right) {
		differences = append(differences, "client semantic Responses body differs")
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
	if attempt.Body["model"] != "gemini-3.1-pro-preview" || attempt.Body["stream"] != false {
		return fmt.Errorf("Gemini model or stream flag rewritten: %+v", attempt.Body)
	}
	messages, _ := attempt.Body["messages"].([]any)
	if len(messages) != 1 {
		return fmt.Errorf("canonical chat request expected one user message: %+v", attempt.Body)
	}
	first, _ := messages[0].(map[string]any)
	if first["role"] != "user" || first["content"] != "same request" {
		return fmt.Errorf("user conversation was dropped or changed: %+v", messages)
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
		if body["status"] != "failed" || failure["code"] != "gemini_empty_response" ||
			failure["message"] != "Gemini returned no visible response content." ||
			len(output) != 0 || body["model"] != "gemini-2.5-pro" {
			return fmt.Errorf("Chat Completion envelope incorrectly treated as valid native Gemini output: %+v", body)
		}
		return nil
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
