package gemini

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProdex04384GeminiAPIKeyIdentityUsesOriginalStringIDPresence(t *testing.T) {
	for _, tc := range []struct {
		member string
		keep   bool
	}{
		{`"id":"source-id",`, true}, {`"id":"",`, true},
		{`"id":null,`, false}, {`"id":7,`, false}, {``, false},
	} {
		wire := `{` + tc.member + `"responseId":"native-preserved","modelVersion":"gemini-3.1-pro-preview","candidates":[{"content":{"parts":[{"text":"identity-control"}]}}]}`
		for _, wrapped := range []bool{false, true} {
			input := wire
			if wrapped {
				input = `{` + tc.member + `"response":` + wire + `}`
			}
			body, err := geminiEnvelopeResponse04384(input, true)
			if err != nil {
				t.Fatal(err)
			}
			var response map[string]any
			if json.Unmarshal(body, &response) != nil {
				t.Fatalf("invalid response: %s", body)
			}
			id, _ := response["id"].(string)
			if tc.keep && id != "native-preserved" || !tc.keep && !strings.HasPrefix(id, "resp_deepseek_") {
				t.Errorf("source=%s changed typed fallback identity: %q", tc.member, id)
			}
		}
	}
}
