package compact

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const (
	SummaryPrefix   = "Another language model started to solve this problem and produced a summary of its thinking process. You also have access to the state of the tools that were used by that language model. Use this to build on the work that has already been done and avoid duplicating work. Here is the summary produced by the other language model, use the information in this summary to assist with your own analysis:"
	maxSnippets     = 24
	maxSnippetBytes = 768
	maxSummaryBytes = 24 * 1024
)

func Semantic(summary, provider string) (*proxymodel.Response, error) {
	content, err := json.Marshal(map[string]any{
		"output": []any{map[string]any{
			"type": "message", "role": "user",
			"content": []any{map[string]any{
				"type": "input_text", "text": SummaryPrefix + "\n\n" + strings.TrimSpace(summary),
			}},
		}},
	})
	if err != nil {
		return nil, errors.New("serialize semantic compact response")
	}
	header := make(http.Header)
	header.Set("Content-Type", "application/json; charset=utf-8")
	header.Set("X-Godex-Compact-Mode", "semantic")
	header.Set("X-Godex-Compact-Provider", provider)
	return &proxymodel.Response{
		StatusCode: http.StatusOK, Header: header,
		Body: io.NopCloser(bytes.NewReader(content)), Trailer: make(http.Header),
	}, nil
}

func LocalFallback(body []byte, provider, reason string) (*proxymodel.Response, error) {
	summary := LocalSummary(body)
	content, err := compactResponseBody(summary)
	if err != nil {
		content = []byte(`{"output":[]}`)
	}
	header := make(http.Header)
	header.Set("Content-Type", "application/json; charset=utf-8")
	header.Set("X-Godex-Compact-Mode", "local-fallback")
	header.Set("X-Godex-Compact-Provider", provider)
	header.Set("X-Godex-Compact-Degraded", "true")
	if strings.TrimSpace(reason) == "" {
		reason = "local-policy"
	}
	header.Set("X-Godex-Compact-Reason", reason)
	return &proxymodel.Response{
		StatusCode: http.StatusOK,
		Header:     header,
		Body:       io.NopCloser(bytes.NewReader(content)),
		Trailer:    make(http.Header),
	}, nil
}

func compactResponseBody(summary string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"output": []any{map[string]any{
			"type": "message", "role": "user",
			"content": []any{map[string]any{
				"type": "input_text", "text": SummaryPrefix + "\n\n" + strings.TrimSpace(summary),
			}},
		}},
	})
}

func LocalSummary(body []byte) string {
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return "Local Godex compact fallback could not parse the compact request body."
	}
	model := stringField(value, "model", "unknown")
	input, _ := value["input"].([]any)
	snippets := compactSnippets(input)
	var summary strings.Builder
	summary.WriteString("Local Godex compact fallback summary.\n\n")
	fmt.Fprintf(&summary, "Model: %s\n", model)
	fmt.Fprintf(&summary, "Original input items: %d\n", len(input))
	fmt.Fprintf(&summary, "Retained recent items: %d\n\n", len(snippets))
	summary.WriteString("Recent conversation and tool state:\n")
	writeSnippets(&summary, snippets)
	return truncateUTF8(summary.String(), maxSummaryBytes)
}

func compactSnippets(input []any) []string {
	snippets := make([]string, 0, min(len(input), maxSnippets))
	for _, item := range input {
		if snippet, ok := localSnippet(item); ok {
			snippets = append(snippets, snippet)
		}
	}
	if len(snippets) > maxSnippets {
		return append([]string(nil), snippets[len(snippets)-maxSnippets:]...)
	}
	return snippets
}

func writeSnippets(summary *strings.Builder, snippets []string) {
	if len(snippets) == 0 {
		summary.WriteString("- No parseable recent message or tool content was found.\n")
		return
	}
	for _, snippet := range snippets {
		summary.WriteString("- ")
		summary.WriteString(strings.ReplaceAll(snippet, "\n", "\n  "))
		summary.WriteByte('\n')
	}
}
