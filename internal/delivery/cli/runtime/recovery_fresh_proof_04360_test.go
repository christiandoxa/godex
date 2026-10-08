package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestProdex04360FreshSessionProofRejectsUntrustedAndAcceptsStructuredLimit(t *testing.T) {
	const id = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	meta := `{"type":"session_meta","payload":{"id":"` + id + `","source":"exec"}}` + "\n"
	user := `{"type":"response_item","payload":{"role":"user"}}` + "\n"
	errLimit := `{"type":"error","error":{"code":"usage_limit_reached"}}` + "\n"
	for _, tc := range []struct {
		name, body       string
		compressed, want bool
	}{
		{"fresh rollout accepted", meta + user + errLimit, false, true},
		{"fresh compressed accepted", meta + user + errLimit, true, true},
		{"unknown session id", `{"type":"session_meta","payload":{"id":"other"}}` + "\n" + user + errLimit, false, false},
		{"only err without user", meta + errLimit, false, false},
		{"only text limit", meta + user + `{"type":"event_msg","payload":{"message":"usage limit reached"}}` + "\n", false, false},
		{"incomplete file", meta + user + `{"type":"error","error":{"code":"usage_limit_reached"}}`, false, false},
		{"missing session metadata", user + errLimit, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			suffix := ".jsonl"
			payload := []byte(tc.body)
			if tc.compressed {
				suffix += ".zst"
				encoder, err := zstd.NewWriter(nil)
				if err != nil {
					t.Fatal(err)
				}
				payload = encoder.EncodeAll(payload, nil)
				encoder.Close()
			}
			path := filepath.Join(t.TempDir(), "rollout-2026-10-08T01-00-00-"+id+suffix)
			if err := os.WriteFile(path, payload, 0600); err != nil {
				t.Fatal(err)
			}
			got := freshSessionUsageLimitProof04360(t.Context(), path, id)
			if got != tc.want {
				t.Fatalf("fresh session evidence=%t want %t", got, tc.want)
			}
		})
	}
}
