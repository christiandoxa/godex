package openai

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWebSocketInvalidPreviousResponseIDMatchesProdex04354(t *testing.T) {
	fixtures := []struct {
		name string
		body any
		want bool
	}{
		{
			name: "top-level invalid request",
			body: map[string]any{
				"type": "error",
				"error": map[string]any{
					"type":    "invalid_request_error",
					"message": "Invalid `previous_response_id`.",
				},
			},
			want: true,
		},
		{
			name: "nested response invalid request",
			body: map[string]any{
				"type": "response.failed",
				"response": map[string]any{
					"error": map[string]any{
						"type":   "INVALID_REQUEST_ERROR",
						"detail": "  INVALID `previous_response_id`.  ",
					},
				},
			},
			want: true,
		},
		{
			name: "explicit not-found code has precedence",
			body: map[string]any{
				"error": map[string]any{
					"type":    "invalid_request_error",
					"code":    "previous_response_not_found",
					"message": "Invalid `previous_response_id`.",
				},
			},
		},
		{
			name: "other invalid request",
			body: map[string]any{
				"error": map[string]any{
					"type":    "invalid_request_error",
					"message": "invalid model",
				},
			},
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			payload, err := json.Marshal(fixture.body)
			if err != nil {
				t.Fatal(err)
			}
			if got := websocketInvalidPreviousResponseID(payload); got != fixture.want {
				t.Fatalf("invalid previous=%t want=%t payload=%s", got, fixture.want, payload)
			}
		})
	}
}

func TestWebSocketInvalidPreviousResponseIDHonorsReferenceScanBound(t *testing.T) {
	value := any(map[string]any{
		"type":    "invalid_request_error",
		"message": "Invalid `previous_response_id`.",
	})
	for range websocketPreviousErrorScanLimit {
		value = []any{value}
	}
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if websocketInvalidPreviousResponseID(payload) {
		t.Fatal("classifier scanned beyond the tagged 2048-node bound")
	}

	near := map[string]any{
		"type":    "invalid_request_error",
		"message": "Invalid `previous_response_id`.",
	}
	payload, err = json.Marshal([]any{strings.Repeat("x", 8), near})
	if err != nil {
		t.Fatal(err)
	}
	if !websocketInvalidPreviousResponseID(payload) {
		t.Fatal("bounded classifier missed a nearby invalid previous-response error")
	}
}
