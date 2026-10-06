package codex

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestThreadIndexProtocolParityScansActiveAndArchivedPages(t *testing.T) {
	responses := strings.Join([]string{
		`{"id":1,"result":{}}`,
		`{"method":"remoteControl/status/changed","params":{}}`,
		`{"id":2,"result":{"data":[],"nextCursor":"active-next"}}`,
		`{"id":3,"result":{"data":[],"nextCursor":null}}`,
		`{"id":4,"result":{"data":[],"nextCursor":null}}`,
	}, "\n") + "\n"
	var written strings.Builder
	if err := reconcileCodexThreadIndexProtocol(strings.NewReader(responses), &written); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(written.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("request count = %d, want 5", len(lines))
	}
	requests := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		var request map[string]any
		if err := json.Unmarshal([]byte(line), &request); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, request)
	}
	client := requests[0]["params"].(map[string]any)["clientInfo"].(map[string]any)
	if client["name"] != "prodex-thread-index-reconciliation" || client["version"] != "0.435.6" {
		t.Fatalf("clientInfo = %#v", client)
	}
	active := requests[2]["params"].(map[string]any)
	if active["archived"] != false || active["cursor"] != nil || active["limit"] != float64(100) ||
		active["useStateDbOnly"] != false {
		t.Fatalf("active request = %#v", active)
	}
	if requests[3]["params"].(map[string]any)["cursor"] != "active-next" ||
		requests[4]["params"].(map[string]any)["archived"] != true {
		t.Fatalf("pagination requests = %#v", requests[2:])
	}
}

func TestThreadIndexProtocolParityIgnoresForeignStringID(t *testing.T) {
	responses := strings.Join([]string{
		`{"id":"foreign","result":{"ignored":true}}`,
		`{"id":1,"result":{}}`,
		`{"id":2,"result":{"nextCursor":null}}`,
		`{"id":3,"result":{"nextCursor":null}}`,
	}, "\n") + "\n"
	var written strings.Builder
	if err := reconcileCodexThreadIndexProtocol(strings.NewReader(responses), &written); err != nil {
		t.Fatal(err)
	}
}

func TestThreadIndexProtocolParityPreservesServerErrorDetail(t *testing.T) {
	responses := `{"id":1,"error":{"message":"synthetic app-server detail"}}` + "\n"
	var written strings.Builder
	err := reconcileCodexThreadIndexProtocol(strings.NewReader(responses), &written)
	if err == nil || err.Error() != "Codex thread index reconciliation failed: synthetic app-server detail" {
		t.Fatalf("error = %v", err)
	}
}

func TestThreadIndexProtocolParityRejectsRepeatedCursor(t *testing.T) {
	responses := strings.Join([]string{
		`{"id":1,"result":{}}`,
		`{"id":2,"result":{"nextCursor":"same"}}`,
		`{"id":3,"result":{"nextCursor":"same"}}`,
	}, "\n") + "\n"
	var written strings.Builder
	err := reconcileCodexThreadIndexProtocol(strings.NewReader(responses), &written)
	if err == nil || !strings.Contains(err.Error(), "repeated") {
		t.Fatalf("error = %v", err)
	}
}
