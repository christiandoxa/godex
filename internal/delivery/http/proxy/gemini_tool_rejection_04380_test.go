package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProdex04380GeminiInvalidFunctionToolRejectionWireMatchesTaggedJSON(t *testing.T) {
	const diagnostic = "invalid_tool_declaration: Gemini request field `tools[0].function.parameters` must be an object"
	canonical := `{"error":{"code":"invalid_request_error","message":"` + diagnostic + `"}}`
	response := httptest.NewRecorder()
	writeProxyError(response, "/backend-api/godex/responses", http.StatusBadRequest, diagnostic)
	if response.Code != 400 || response.Header().Get("Content-Type") != "application/json" ||
		response.Header().Get("Cache-Control") != "no-store" ||
		response.Header().Get("X-Content-Type-Options") != "nosniff" ||
		response.Body.String() != canonical {
		t.Fatalf("bad Gemini tool request did not match Prodex 0.438.0 error: status=%d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
	}
	for _, tc := range []struct {
		path    string
		code    int
		message string
	}{
		{"/backend-api/godex/chat/completions", 400, diagnostic},
		{"/backend-api/godex/responses", 500, diagnostic},
		{"/backend-api/godex/responses", 400, "ordinary upstream error"},
	} {
		recorder := httptest.NewRecorder()
		writeProxyError(recorder, tc.path, tc.code, tc.message)
		if recorder.Code != tc.code || recorder.Body.String() != tc.message ||
			recorder.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
			t.Fatalf("non-Gemini validation error was mutated: path=%s code=%d headers=%v body=%s", tc.path, recorder.Code, recorder.Header(), recorder.Body.String())
		}
	}
}
