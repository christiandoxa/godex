package chatcompat

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestProdex04384ChatResponseRetainsPresentEmptyFinishReason(t *testing.T) {
	for _, raw := range []string{`""`, `"  stop  "`, `null`, `false`} {
		body, err := ChatResponseWithOptions([]byte(`{"choices":[{"finish_reason":`+raw+`,"message":{"content":"finish-control"}}]}`), time.Unix(1, 0), ResponseOptions{ProviderKey: "deepseek"})
		if err != nil {
			t.Fatal(err)
		}
		var response map[string]any
		if err := json.Unmarshal(body, &response); err != nil {
			t.Fatal(err)
		}
		metadata, _ := response["metadata"].(map[string]any)
		provider, _ := metadata["deepseek"].(map[string]any)
		_, present := provider["finish_reason"]
		if want := strings.HasPrefix(raw, `"`); present != want || want && !strings.Contains(string(body), `"finish_reason":`+raw) {
			t.Errorf("finish presence changed: input=%s response=%s", raw, body)
		}
	}
}
