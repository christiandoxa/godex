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
	"slices"
	"strings"
	"sync"
	"time"
)

// Tagged Prodex 0.437.11 uses the OpenAI-compatible Chat Completions wire
// format for Gemini API-key Super. Keep its executable differential separate
// so the existing DeepSeek --scenario all remains unchanged.
type geminiCapturedRequest struct {
	Method  string         `json:"method"`
	Path    string         `json:"path"`
	Auth    bool           `json:"synthetic_auth_present"`
	Slot    string         `json:"credential_slot,omitempty"`
	Body    map[string]any `json:"body"`
	RawBody string         `json:"raw_body,omitempty"`
}

type geminiCandidateResult struct {
	Name                  string                  `json:"name"`
	Version               string                  `json:"version"`
	ExitCode              int                     `json:"exit_code"`
	Upstream              []geminiCapturedRequest `json:"upstream"`
	Client                exchange                `json:"client_exchange"`
	Stdout                string                  `json:"stdout,omitempty"`
	StateIntegrity        []string                `json:"state_integrity_errors,omitempty"`
	ClientError           string                  `json:"client_error,omitempty"`
	Seed                  *geminiCandidateResult  `json:"continuation_seed,omitempty"`
	ContinuationPersisted bool                    `json:"continuation_persisted,omitempty"`
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
	_, userID := geminiUserIDCase04384(mock.scenario)
	_, _, ignored := geminiIgnoredControl04384(mock.scenario)
	_, _, _, controlTypes := geminiControlTypeCase04384(mock.scenario)
	_, _, jsonAdmission := geminiJSONAdmission04384(mock.scenario)
	primitiveFixture, primitive := requestPrimitiveCase04384(mock.scenario)
	if geminiRequestNumberCase04384(mock.scenario) || userID || ignored || controlTypes || jsonAdmission || primitive || geminiCompactCase04384(mock.scenario) {
		record.RawBody = string(body)
	}
	mock.mu.Lock()
	mock.requests = append(mock.requests, record)
	compactSeed := len(mock.requests) == 1 && geminiCompactOwnershipSeed04384(mock.scenario)
	mock.mu.Unlock()
	streamSeed := compactSeed && geminiCompactStreamSeed04384(mock.scenario)
	if mock.scenario == "compact-control-responses" || compactSeed && !streamSeed {
		w.Header().Set("Content-Type", "application/json")
		body := geminiContinuationResponse04384()
		if mock.scenario == "compact-owner-assistant" || mock.scenario == "compact-owner-numeric" {
			extra := ""
			if mock.scenario == "compact-owner-numeric" {
				extra = `,"tool_calls":[{"id":"call-synthetic","type":"function","priority":9007199254740993,"function":{"name":"lookup","arguments":"{}"}}]`
			}
			content := `"synthetic cached assistant"`
			if mock.scenario == "compact-owner-numeric" {
				content = `null`
			}
			body = strings.TrimSuffix(body, "}") + `,"choices":[{"message":{"role":"user","content":` + content + extra + `}}]}`
		}
		_, _ = io.WriteString(w, body)
		return
	}
	if geminiCompactCase04384(mock.scenario) && mock.scenario != "compact-control-responses" && !streamSeed {
		status, payload := geminiCompactUpstream04384(mock.scenario)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Upstream-Decoy", "must-not-leak")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, payload)
		return
	}
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
	if geminiChatStreamScenario04384(mock.scenario) || primitiveFixture.geminiChatStream || streamSeed {
		w.Header().Set("Content-Type", "text/event-stream")
		if geminiProviderBOMScenario04384(mock.scenario) {
			_, _ = io.WriteString(w, providerBOMUpstream04384(mock.scenario == "stream-chat-bom-repeat"))
			return
		}
		if mock.scenario == "stream-chat-error" {
			_, _ = io.WriteString(w, "data: "+`{"error":{"type":null,"code":"must-not-borrow","message":null,"detail":"must-not-borrow"},"choices":[{"delta":{"content":"must-not-emit"}}]}`+"\n\ndata: [DONE]\n\n")
			return
		}
		separator := "\n\n"
		if mock.scenario == "stream-chat-data-lines" {
			separator = "\n"
		}
		first := `{"id":"chat-synthetic","model":"gemini-3.1-pro-preview","choices":[{"delta":{"content":"synthetic-"},"finish_reason":null}]}`
		if mock.scenario == "stream-chat-json-fragments" {
			first = strings.Replace(first, "synthetic-", "synthe\n\ndata: tic-", 1)
		}
		_, _ = io.WriteString(w, "data: "+first+separator)
		_, _ = io.WriteString(w, "data: "+`{"id":"chat-synthetic","model":"gemini-3.1-pro-preview","choices":[{"delta":{"content":"ok<>&/🌍\u2028\u2029"},"finish_reason":null}]}`+"\n\n")
		_, _ = io.WriteString(w, "data: "+`{"id":"chat-synthetic","model":"gemini-3.1-pro-preview","choices":[{"delta":{},"finish_reason":"stop"}]}`+"\n\n")
		_, _ = io.WriteString(w, "data: "+`{"choices":[],"usage":{"prompt_tokens":9007199254740993,"completion_tokens":1,"total_tokens":9007199254740994}}`+"\n\n")
		if mock.scenario != "stream-chat-eof" && mock.scenario != "compact-owner-stream-eof" {
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
		}
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
	if primitiveFixture.geminiChatFallback {
		_, _ = io.WriteString(w, `{"id":"ignored-id","model":"ignored-model","choices":[{"message":{"role":"assistant","content":"must-not-emit"}}]}`)
		return
	}
	if response, ok := geminiPatchResponse04384(mock.scenario); ok {
		_, _ = io.WriteString(w, response)
		return
	}
	if response, ok := geminiToolIdentityResponse04384(mock.scenario); ok {
		_, _ = io.WriteString(w, response)
		return
	}
	if response, ok := geminiSourceIDResponse04384(mock.scenario); ok {
		_, _ = io.WriteString(w, response)
		return
	}
	if geminiContinuationCase04384(mock.scenario) || geminiRequestNumberCase04384(mock.scenario) || userID || ignored || controlTypes || jsonAdmission || primitive {
		_, _ = io.WriteString(w, geminiContinuationResponse04384())
		return
	}
	if response, ok := geminiEnvelopeFixture04384(mock.scenario); ok {
		_, _ = io.WriteString(w, response)
		return
	}
	if response, ok := geminiBufferedArgsResponse04384(mock.scenario); ok {
		_, _ = io.WriteString(w, response)
		return
	}
	if response, ok := geminiBufferedBodyResponse04384(mock.scenario); ok {
		_, _ = io.WriteString(w, response)
		return
	}
	if mock.scenario == "buffered-null-tool-text" {
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"hidden before"},{"functionCall":null},{"text":"hidden after"}]},"finishReason":"STOP"}],"modelVersion":"gemini-3.1-pro-preview","usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"totalTokenCount":3}}`)
		return
	}
	if geminiMediaScenario04384(mock.scenario) {
		_, _ = io.WriteString(w, geminiMediaResponse04384(mock.scenario))
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

func runGeminiDifferential(prodexBin, godexBin, prodexCommit, godexCommit, selected string) error {
	cases := []string{
		"request-json-valid-unicode", "request-json-root-null", "request-json-root-null-rotation", "request-json-root-array", "request-json-root-number", "request-json-root-boolean", "request-json-root-string", "request-json-syntax",
		"request-json-trailing", "request-json-surrogate", "request-json-utf8", "request-json-overflow", "request-json-depth",
		"request-user-id-absent", "request-user-id-canonical", "request-user-id-user-alias", "request-user-id-safety-alias", "request-user-id-blank",
		"request-user-id-trim-limit", "request-user-id-null", "request-user-id-number", "request-user-id-over-limit", "request-user-id-invalid-character",
		"request-user-id-number-rotation",
		"request-tool-numbers-buffered",
		"request-tool-numbers-stream",
		geminiRestartResponseCase04384,
		geminiRestartKeyCase04384,
		geminiRestartEndpointCase04384,
		geminiBoundResponseCase04384,
		geminiUnboundResponseCase04384,
		"buffered-body-under",
		"buffered-body-limit",
		"buffered-body-over",
		"buffered-body-over-rotation",
		"raw-key-session-limit",
		"raw-key-session-over-limit",
		"raw-key-session-unicode-limit",
		"raw-key-session-unicode-over",
		"raw-key-session-control",
		"chat-request",
		"orphan-tool",
		"chat-response",
		"wrapped-response-no-trace",
		"wrapped-response-trace",
		"stream-native",
		"stream-chat",
		"stream-chat-eof",
		"stream-chat-error",
		"stream-chat-bom",
		"stream-chat-bom-repeat",
		"stream-chat-data-lines",
		"stream-chat-json-fragments",
		"multi-turn-tool",
		"quota-plain-429",
		"quota-structured-429",
		"invalid-wrapped-tool-parameters",
		"invalid-flat-tool-parameters",
		"buffered-null-tool-text",
		"buffered-media-invalid-inline",
		"buffered-media-invalid-code",
		"buffered-media-suppressed-code",
		"buffered-grounding-null-status",
		"buffered-args-null",
		"buffered-args-absent",
		"buffered-args-empty",
		"buffered-args-wrapped-null",
		"buffered-args-canonical-wire",
		"buffered-args-patch-fallback",
		"buffered-envelope-null",
		"buffered-envelope-array",
		"buffered-envelope-scalar",
		"buffered-value-array",
		"buffered-value-number",
		"buffered-source-id-string",
		"buffered-source-id-empty",
		"buffered-source-id-wrapped",
		"buffered-source-id-null",
		"buffered-tool-dotted",
		"buffered-tool-padded",
		"buffered-tool-empty",
		"buffered-tool-mcp-empty-prefix",
		"buffered-tool-trailing-separator",
		"buffered-patch-unified-update",
		"buffered-patch-unified-files",
		"buffered-patch-literal-crlf",
	}
	for _, suffix := range append(requestPrimitiveSuffixes04384(), geminiPreparationSuffixes04384()...) {
		cases = append(cases, "request-primitive-"+suffix)
	}
	cases = append(cases, geminiCompactCases04384()...)
	cases = append(cases, geminiIgnoredControlCases04384()...)
	for _, suffix := range geminiControlTypeSuffixes04384() {
		cases = append(cases, "request-control-"+suffix)
	}
	if selected != "all" {
		if !slices.Contains(cases, selected) {
			return fmt.Errorf("unknown Gemini scenario %q", selected)
		}
		cases = []string{selected}
	}
	return runGeminiDifferentialCases04384(prodexBin, godexBin, prodexCommit, godexCommit, cases)
}

func runGeminiDifferentialCases04384(prodexBin, godexBin, prodexCommit, godexCommit string, cases []string) error {
	root, err := os.MkdirTemp("", "godex-gemini-parity-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	var results []geminiCaseResult
	for _, scenario := range cases {
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
		NegativeControl: "PASS: route, auth, model fallback, credential rotation, child exit, native/chat SSE, multi-turn tool adjacency, output, status, usage, raw-key identity/state and error mutations detected",
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
	_, _, rawIdentity := geminiRawKeyIdentity04384(scenario)
	_, bodyBoundary := geminiBufferedBodyCase04384(scenario)
	continuation := geminiContinuationCase04384(scenario)
	requestNumbers := geminiRequestNumberCase04384(scenario)
	_, userID := geminiUserIDCase04384(scenario)
	_, _, ignored := geminiIgnoredControl04384(scenario)
	_, _, _, controlTypes := geminiControlTypeCase04384(scenario)
	_, _, jsonAdmission := geminiJSONAdmission04384(scenario)
	_, primitive := requestPrimitiveCase04384(scenario)
	auditState := rawIdentity || bodyBoundary || continuation || requestNumbers || userID || ignored || controlTypes || jsonAdmission || primitive || geminiCompactCase04384(scenario)
	if auditState {
		childResult = filepath.Join(home, "child-exchange.json")
	}
	args := []string{"super", "--provider", "gemini"}
	if !geminiQuotaScenario043711(scenario) && scenario != "buffered-body-over-rotation" && scenario != "request-user-id-number-rotation" && scenario != "request-json-root-null-rotation" {
		args = append(args, "--api-key", apiKey)
	}
	args = append(args,
		"--model", "gemini-3.1-pro-preview", "--base-url", mock.server.URL+"/v1beta",
		"--no-presidio", "--no-sub-agent", "exec", "synthetic Gemini differential",
	)
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	env := productEnv(name, stateHome, codexHome, userHome, shim, childResult, runOptions{})
	env = append(env, "DIFFERENTIAL_PROVIDER=gemini")
	if geminiCompactCase04384(scenario) {
		env = append(env, "DIFFERENTIAL_GEMINI_COMPACT="+scenario)
	}
	if geminiCompactOwnershipSeed04384(scenario) {
		env = append(env, "DIFFERENTIAL_CONTINUATION_PHASE=same-process")
	}
	if primitive {
		env = append(env, "DIFFERENTIAL_REQUEST_PRIMITIVE="+scenario)
	}
	if jsonAdmission {
		env = append(env, "DIFFERENTIAL_GEMINI_JSON_ADMISSION="+scenario)
	}
	if controlTypes {
		env = append(env, "DIFFERENTIAL_GEMINI_CONTROL_TYPES="+scenario)
	}
	if ignored {
		env = append(env, "DIFFERENTIAL_GEMINI_IGNORED_CONTROL="+scenario)
	}
	if userID {
		env = append(env, "DIFFERENTIAL_GEMINI_USER_ID="+scenario)
	}
	if requestNumbers {
		env = append(env, "DIFFERENTIAL_GEMINI_REQUEST_NUMBERS="+scenario)
	}
	if scenario == geminiBoundResponseCase04384 {
		env = append(env, "DIFFERENTIAL_CONTINUATION_PHASE=same-process")
	} else if scenario == geminiUnboundResponseCase04384 {
		env = append(env, "DIFFERENTIAL_CONTINUATION_PHASE=unknown")
	}
	if rawIdentity {
		env = append(env, "DIFFERENTIAL_GEMINI_RAW_KEY_IDENTITY="+scenario)
	}
	if geminiQuotaScenario043711(scenario) || scenario == "buffered-body-over-rotation" || scenario == "request-user-id-number-rotation" || scenario == "request-json-root-null-rotation" {
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
	if fixture, yes := geminiModelCase04384For(scenario); yes {
		env = append(env, "DIFFERENTIAL_GEMINI_MODEL_QUERY="+fixture.query)
	}
	if scenario == "stream-native" || geminiChatStreamScenario04384(scenario) {
		env = append(env, "DIFFERENTIAL_STREAM=1")
	}
	var seed *geminiCandidateResult
	if geminiRestartCase04384(scenario) {
		seed, err = runGeminiRestartSeed04384(ctx, binary, args, env, home, childResult, mock)
		if err != nil {
			return geminiCandidateResult{}, err
		}
		if err := os.Remove(childResult); err != nil {
			return geminiCandidateResult{}, fmt.Errorf("remove Gemini seed exchange: %w", err)
		}
		args = geminiRestartArguments04384(scenario, args)
		env = append(env, "DIFFERENTIAL_CONTINUATION_PHASE=resume")
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	exitCode := 0
	if err := cmd.Run(); err != nil {
		var childExit *exec.ExitError
		_, envelopeScenario := geminiEnvelopeFixture04384(scenario)
		if !auditState && !geminiQuotaScenario043711(scenario) && !geminiInvalidToolScenario04380(scenario) && !geminiModelScenario04384(scenario) && !envelopeScenario ||
			!errors.As(err, &childExit) || ctx.Err() != nil {
			return geminiCandidateResult{}, fmt.Errorf("child failed: %w stderr=%q upstream=%v",
				err, redact(stderr.String()), mock.recorded())
		}
		exitCode = childExit.ExitCode()
	}
	result, readErr := readExchange(childResult)
	if readErr != nil && !continuation && !userID && !ignored && !controlTypes && !jsonAdmission && !primitive {
		return geminiCandidateResult{}, fmt.Errorf("child response missing: %w stderr=%q upstream=%v",
			readErr, redact(stderr.String()), mock.recorded())
	}
	version, err := productVersion(binary)
	if err != nil {
		return geminiCandidateResult{}, err
	}
	candidate := geminiCandidateResult{
		Name: name, Version: version, ExitCode: exitCode, Upstream: mock.recorded(), Client: result, Seed: seed,
	}
	if readErr != nil {
		candidate.ClientError = "missing client exchange: " + redact(stderr.String())
	}
	if auditState {
		candidate.Stdout = stdout.String()
		owners := geminiContinuationOwners04384(scenario, candidate, mock.server.URL+"/v1beta")
		candidate.StateIntegrity = auditFixtureDurableStateWithBindings(home, owners)
	}
	return candidate, nil
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
	if geminiCompactCase04384(scenario) {
		return differences
	}
	if geminiContinuationCase04384(scenario) {
		return differences
	}
	if geminiRequestNumberCase04384(scenario) {
		return differences
	}
	if _, known := geminiUserIDCase04384(scenario); known {
		return differences
	}
	if _, _, known := geminiIgnoredControl04384(scenario); known {
		return differences
	}
	if _, _, _, known := geminiControlTypeCase04384(scenario); known {
		return differences
	}
	if _, _, known := geminiJSONAdmission04384(scenario); known {
		return differences
	}
	if _, known := requestPrimitiveCase04384(scenario); known {
		return differences
	}
	if _, known := geminiBufferedBodyCase04384(scenario); known {
		return differences
	}
	if _, reject, known := geminiRawKeyIdentity04384(scenario); known && reject {
		if prodex.Client.Body != godex.Client.Body {
			differences = append(differences, "Gemini raw-key identity rejection bytes differ")
		}
		return differences
	}
	if scenario == "buffered-args-canonical-wire" || scenario == "buffered-args-patch-fallback" {
		left, leftErr := geminiBufferedWireProjection04384(prodex.Client.Body)
		right, rightErr := geminiBufferedWireProjection04384(godex.Client.Body)
		if leftErr != nil || rightErr != nil || left != right {
			differences = append(differences, "buffered Gemini response bytes differ after UUID normalization")
		}
	}
	if geminiModelScenario04384(scenario) {
		return geminiModelCompare04384(scenario, prodex, godex, differences)
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
	if geminiProviderBOMScenario04384(scenario) {
		if !equivalentProviderBOMSSE04384(prodex.Client.Body, godex.Client.Body) {
			differences = append(differences, "client Gemini provider payload BOM failure differs")
		}
	} else if scenario == "stream-chat-error" {
		if !geminiChatErrorWireValid04384(prodex.Client.Body) || !geminiChatErrorWireValid04384(godex.Client.Body) {
			differences = append(differences, "client Gemini embedded error precedence or failure bytes differ")
		}
	} else if scenario == "stream-chat-eof" {
		if !geminiChatEOFWireValid04384(prodex.Client.Body) || !geminiChatEOFWireValid04384(godex.Client.Body) {
			differences = append(differences, "client Gemini Chat EOF failure bytes differ")
		}
	} else if scenario == "stream-chat" || scenario == "stream-chat-data-lines" || scenario == "stream-chat-json-fragments" {
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
	if geminiCompactCase04384(scenario) {
		return geminiCompactInvariant04384(scenario, result)
	}
	if _, known := requestPrimitiveCase04384(scenario); known {
		return geminiPrimitiveInvariant04384(scenario, result)
	}
	if _, _, known := geminiJSONAdmission04384(scenario); known {
		return geminiJSONAdmissionInvariant04384(scenario, result)
	}
	if _, _, _, known := geminiControlTypeCase04384(scenario); known {
		if err := geminiUserIDInvariant04384(geminiControlTypeExpectation04384(scenario), result); err != nil {
			return errors.New("Gemini control type lost exact prepared failure or accepted original request")
		}
		return nil
	}
	if _, _, known := geminiIgnoredControl04384(scenario); known {
		return geminiIgnoredInvariant04384(result)
	}
	if _, known := geminiUserIDCase04384(scenario); known {
		return geminiUserIDInvariant04384(scenario, result)
	}
	if geminiRequestNumberCase04384(scenario) {
		return geminiRequestNumberInvariant04384(scenario, result)
	}
	if geminiContinuationCase04384(scenario) {
		return geminiContinuationInvariant04384(scenario, result)
	}
	if _, known := geminiBufferedBodyCase04384(scenario); known {
		return geminiBufferedBodyInvariant04384(scenario, result)
	}
	if _, _, known := geminiRawKeyIdentity04384(scenario); known {
		return geminiRawKeyIdentityInvariant04384(scenario, result)
	}
	if geminiModelScenario04384(scenario) {
		return geminiModelFixtureInvariant04384(scenario, result)
	}
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
		attempt.Body["stream"] != (scenario == "stream-native" || geminiChatStreamScenario04384(scenario)) {
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
	if geminiChatStreamScenario04384(scenario) {
		if options, ok := attempt.Body["stream_options"].(map[string]any); !ok || options["include_usage"] != true {
			return fmt.Errorf("Gemini valid Chat SSE request omitted usage: %+v", attempt.Body)
		}
		if geminiProviderBOMScenario04384(scenario) {
			if !validProviderBOMWire04384(result.Client.Body) {
				return errors.New("Gemini provider payload BOM was accepted as JSON whitespace")
			}
		} else if scenario == "stream-chat-error" {
			if !geminiChatErrorWireValid04384(result.Client.Body) {
				return errors.New("Gemini embedded error leaked creation/content or incorrect error fields")
			}
		} else if scenario == "stream-chat-eof" {
			if !geminiChatEOFWireValid04384(result.Client.Body) {
				return errors.New("Gemini Chat EOF client trace lost canonical failure bytes")
			}
		} else if _, err := geminiChatSSEProjection043711(result.Client.Body); err != nil {
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
	if _, ok := geminiEnvelopeFixture04384(scenario); ok {
		if result.ExitCode != 0 {
			return errors.New("Gemini valid non-object response produced a child error exit")
		}
		return geminiEnvelopeInvariant04384(body)
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
	_, _, sourceIDFixture := geminiSourceIDFixture04384(scenario)
	if sourceIDFixture {
		if err := geminiSourceIDInvariant04384(scenario, body); err != nil {
			return err
		}
	} else if err := geminiBufferedIdentityInvariant043711(body); err != nil {
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
	if geminiMediaScenario04384(scenario) {
		return geminiMediaFixtureInvariant04384(scenario, body)
	}
	if _, _, ok := geminiBufferedArgsFixture04384(scenario); ok {
		return geminiBufferedArgsInvariant04384(scenario, body)
	}
	if _, ok := geminiToolIdentityCase04384(scenario); ok {
		return geminiToolIdentityInvariant04384(scenario, body)
	}
	if _, _, ok := geminiPatchFixture04384(scenario); ok {
		return geminiPatchInvariant04384(scenario, body)
	}
	return nil
}
