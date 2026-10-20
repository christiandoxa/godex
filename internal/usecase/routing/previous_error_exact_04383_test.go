package routing

import (
	"net/http"
	"strings"
	"testing"
)

// Mojo 0.438.3 compares structured type, param and code against ASCII
// literals without trimming or Unicode case folding. Only the invalid-ID
// message itself allows ASCII-case folding and ASCII whitespace trimming.
func TestProdex04383StructuredPreviousErrorUsesExactASCIIIdentifiers(t *testing.T) {
	for _, tc := range []struct {
		name, body                string
		wantInvalid, wantPrevious bool
	}{
		{"canonical-invalid", `{"type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}`, true, true},
		{"type-trailing-space", `{"type":"invalid_request_error ","message":"Invalid ` + "`previous_response_id`" + `."}`, false, false},
		{"type-uppercase", `{"type":"INVALID_REQUEST_ERROR","message":"Invalid ` + "`previous_response_id`" + `."}`, false, false},
		{"type-long-s", `{"type":"invalid_requeſt_error","message":"Invalid ` + "`previous_response_id`" + `."}`, false, false},
		{"code-upper-not-found", `{"code":"PREVIOUS_RESPONSE_NOT_FOUND"}`, false, false},
		{"code-padded-not-found", `{"code":"previous_response_not_found "}`, false, false},
		{"code-not-found", `{"code":"previous_response_not_found"}`, false, true},
		{"padded-code-invalid-type", `{"code":"previous_response_not_found ","type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}`, true, true},
		{"uppercase-code-invalid-type", `{"code":"PREVIOUS_RESPONSE_NOT_FOUND","type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}`, true, true},
		{"code-not-found-before-invalid-type", `{"code":"previous_response_not_found","type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}`, false, true},
		{"message-ascii-trim", `{"type":"invalid_request_error","message":"\t INVALID ` + "`previous_response_id`" + `. \n"}`, true, true},
		{"message-unicode-space", `{"type":"invalid_request_error","message":"\u00a0Invalid ` + "`previous_response_id`" + `."}`, false, false},
		{"message-unicode-long-s", `{"type":"invalid_request_error","message":"Invalid ` + "`previous_reſponse_id`" + `."}`, false, false},
		{"tool-input", `{"message":"no tool call found","param":"input"}`, false, true},
		{"tool-input-upper", `{"message":"no tool call found","param":"INPUT"}`, false, false},
		{"tool-invalid-request", `{"message":"no tool call found","type":"invalid_request_error"}`, false, true},
		{"tool-type-space", `{"message":"no tool call found","type":"invalid_request_error "}`, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := invalidPreviousResponseID([]byte(tc.body)); got != tc.wantInvalid {
				t.Fatalf("invalid-id class %q got=%t want=%t", tc.name, got, tc.wantInvalid)
			}
			if got := previousResponseNotFound([]byte(tc.body)); got != tc.wantPrevious {
				t.Fatalf("previous-response class %q got=%t want=%t", tc.name, got, tc.wantPrevious)
			}
		})
	}
}

func TestProdex04383PreviousResponseScanStopsAt2048Nodes(t *testing.T) {
	const invalidID = `{"type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}`
	const notFound = `{"code":"previous_response_not_found"}`
	for _, tc := range []struct {
		name, terminal string
		kind           string
	}{
		{"invalid-id", invalidID, "invalid"},
		{"not-found", notFound, "previous"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// serde_json's lexical key order visits a huge 'a' array before
			// reaching 'z'. 2048 scanned nodes is the tagged hard cap.
			body := `{"a":[` + strings.Repeat(`0,`, 2100) + `0],"z":` + tc.terminal + `}`
			if tc.kind == "invalid" {
				if invalidPreviousResponseID([]byte(body)) {
					t.Fatal("unbounded invalid-ID traversal escaped tagged node cap")
				}
			} else if previousResponseNotFound([]byte(body)) {
				t.Fatal("unbounded previous-response scan escaped node cap")
			}
			control := `{"a":[0],"z":` + tc.terminal + `}`
			if tc.kind == "invalid" {
				if !invalidPreviousResponseID([]byte(control)) {
					t.Fatal("valid shallow invalid-ID was not recognized")
				}
			} else if !previousResponseNotFound([]byte(control)) {
				t.Fatal("valid shallow previous-response not-found was lost")
			}
		})
	}
}

// Exact structured tags decide whether a verified HTTP previous-response
// chain gets a single owner-local full-history recovery, a stale 409, or the
// untouched original 400. Unicode/casefolding is not permission to replay.
func TestProdex04383StructuredErrorTagBytesControlBoundRecovery(t *testing.T) {
	for _, tc := range []struct {
		name, body               string
		wantAttempts, wantStatus int
	}{
		{"exact-invalid-type", `{"error":{"type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}}`, 2, 200},
		{"padded-type", `{"error":{"type":"invalid_request_error ","message":"Invalid ` + "`previous_response_id`" + `."}}`, 1, 400},
		{"uppercase-type", `{"error":{"type":"INVALID_REQUEST_ERROR","message":"Invalid ` + "`previous_response_id`" + `."}}`, 1, 400},
		{"padded-code-with-exact-type", `{"error":{"code":"previous_response_not_found ","type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}}`, 2, 200},
		{"uppercase-code-with-exact-type", `{"error":{"code":"PREVIOUS_RESPONSE_NOT_FOUND","type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}}`, 2, 200},
		{"exact-notfound-priority", `{"error":{"code":"previous_response_not_found","type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}}`, 1, 409},
		{"unicode-structured-type", `{"error":{"type":"invalid_requeſt_error","message":"Invalid ` + "`previous_response_id`" + `."}}`, 1, 400},
		{"unicode-code-with-exact-type", `{"error":{"code":"previous_reſponse_not_found","type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}}`, 2, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gateway := &responseRecoveryGateway{replies: []responseRecoveryReply{
				{http.StatusBadRequest, "application/json", tc.body},
				{http.StatusOK, "application/json", `{"id":"resp-valid-recovered","object":"response"}`},
			}}
			router := newResponseRecoveryRouter(t, gateway, true)
			defer router.Close()
			original := responsesRecoveryRequest(`{"model":"gpt-6","session_id":"valid-session","previous_response_id":"resp-dead","input":[{"role":"user"},{"role":"assistant"},{"role":"user"}]}`)
			result, err := router.Forward(t.Context(), original)
			if err != nil {
				t.Fatal(err)
			}
			defer result.Close()
			if len(gateway.requests) != tc.wantAttempts || result.Result.Response.StatusCode != tc.wantStatus {
				t.Fatalf("type/code bytes: attempts=%d status=%d want attempts=%d status=%d", len(gateway.requests), result.Result.Response.StatusCode, tc.wantAttempts, tc.wantStatus)
			}
			if string(gateway.requests[0].Body) != string(original.Body) {
				t.Fatal("initial invalid previous request changed")
			}
			if tc.wantAttempts == 2 && requestPreviousResponseID(gateway.requests[1].Body) != "" {
				t.Fatal("owner-local recovery retained prior response")
			}
		})
	}
}
