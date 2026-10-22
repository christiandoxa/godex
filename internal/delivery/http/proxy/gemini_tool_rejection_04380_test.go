package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProdex04380GeminiInvalidFunctionToolRejectionWireMatchesTaggedJSON(t *testing.T) {
	const diagnostic = "invalid_tool_declaration: Gemini request field `tools[0].function.parameters` must be an object"
	for _, diagnostic := range []string{
		diagnostic,
		"invalid_candidate_count: Gemini request field `candidate_count` must be omitted, null, or 1",
		"invalid_candidate_count: Gemini request field `candidateCount` must be omitted, null, or 1",
		"invalid_candidate_count: Gemini request fields `candidate_count` and `candidateCount` conflict",
	} {
		canonical := `{"error":{"code":"invalid_request_error","message":"` + diagnostic + `"}}`
		response := httptest.NewRecorder()
		writeProxyError(response, "/backend-api/godex/responses", http.StatusBadRequest, diagnostic)
		if response.Code != 400 || response.Header().Get("Content-Type") != "application/json" ||
			response.Header().Get("Cache-Control") != "no-store" ||
			response.Header().Get("X-Content-Type-Options") != "nosniff" ||
			response.Body.String() != canonical {
			t.Fatalf("bad Gemini tool request did not match Prodex 0.438.0 error: status=%d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
		}
	}
	for _, tc := range []struct {
		path    string
		code    int
		message string
	}{
		{"/backend-api/godex/chat/completions", 400, diagnostic},
		{"/backend-api/godex/responses", 500, diagnostic},
		{"/backend-api/godex/responses", 400, "ordinary upstream error"},
		{"/backend-api/godex/responses", 400, "invalid_candidate_count: unrelated upstream error"},
		{"/backend-api/godex/responses", 400, "invalid_candidate_count: Gemini request field `other` must be omitted, null, or 1"},
		{"/backend-api/godex/chat/completions", 400, "invalid_candidate_count: Gemini request field `candidate_count` must be omitted, null, or 1"},
		{"/backend-api/godex/responses", 502, "invalid_candidate_count: Gemini request field `candidate_count` must be omitted, null, or 1"},
	} {
		recorder := httptest.NewRecorder()
		writeProxyError(recorder, tc.path, tc.code, tc.message)
		if recorder.Code != tc.code || recorder.Body.String() != tc.message ||
			recorder.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
			t.Fatalf("non-Gemini validation error was mutated: path=%s code=%d headers=%v body=%s", tc.path, recorder.Code, recorder.Header(), recorder.Body.String())
		}
	}
}
