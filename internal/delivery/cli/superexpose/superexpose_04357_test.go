package superexpose

import "testing"

func TestProdex04357ExposeAuditRouteLabelsMatchCanonicalSurface(t *testing.T) {
	for _, testCase := range []struct {
		method, tool string
		wantMethod   string
		wantTool     string
	}{
		{"server/discover", "", "server_discover", "unknown"},
		{"initialize", "", "initialize", "unknown"},
		{"ping", "", "ping", "unknown"},
		{"tools/list", "", "tools_list", "unknown"},
		{"notifications/initialized", "", "notification", "unknown"},
		{"notifications/cancelled", "", "notification", "unknown"},
		{"tools/call", godexStartToolName, "tools_call", "start"},
		{"tools/call", godexStatusToolName, "tools_call", "status"},
		{"tools/call", godexEventsToolName, "tools_call", "events"},
		{"tools/call", godexResultToolName, "tools_call", "result"},
		{"tools/call", godexCancelToolName, "tools_call", "cancel"},
		{"tools/call", godexListToolName, "tools_call", "list"},
		{"tools/call", godexExecToolName, "tools_call", "exec"},
		{"tools/call", godexSessionPromptWriteToolName, "tools_call", "session_prompt_write"},
		{"tools/call", godexSessionPreemptToolName, "tools_call", "session_preempt"},
		{"tools/call", godexSessionOutputReadToolName, "tools_call", "session_output_read"},
		{"other", "", "unknown", "unknown"},
	} {
		gotMethod, gotTool := exposeAuditRoute(testCase.method, testCase.tool)
		if gotMethod != testCase.wantMethod || gotTool != testCase.wantTool {
			t.Fatalf("route %q/%q = %q/%q, want %q/%q",
				testCase.method, testCase.tool, gotMethod, gotTool, testCase.wantMethod, testCase.wantTool)
		}
	}
}
