package deepseek

import (
	"errors"
	"io"
	"strings"
	"testing"
)

type deepSeekErrorAfterDataReader struct {
	data    *strings.Reader
	errored bool
}

func (reader *deepSeekErrorAfterDataReader) Read(buffer []byte) (int, error) {
	read, err := reader.data.Read(buffer)
	if read > 0 {
		return read, nil
	}
	if !reader.errored {
		reader.errored = true
		return 0, errors.New("error decoding response body")
	}
	return 0, err
}

func (*deepSeekErrorAfterDataReader) Close() error { return nil }

func TestProdex04355DeepSeekChatSSEPrematureEOFFails(t *testing.T) {
	stream := `data: {"id":"chatcmpl_eof","model":"deepseek-v4-pro","choices":[{"delta":{"content":"partial"}}]}

`
	translated := readDeepSeekChatSSEForTest(t, io.NopCloser(strings.NewReader(stream)))
	if !strings.Contains(translated, "event: response.failed") || strings.Contains(translated, "event: response.completed") {
		t.Fatalf("premature EOF stream = %s", translated)
	}
}

func TestProdex04355DeepSeekChatSSEReadErrorFailsAfterPriorDelta(t *testing.T) {
	stream := `data: {"id":"chatcmpl_err","choices":[{"delta":{"content":"hi"}}]}

`
	translated := readDeepSeekChatSSEForTest(t, &deepSeekErrorAfterDataReader{data: strings.NewReader(stream)})
	if !strings.Contains(translated, `"delta":"hi"`) ||
		!strings.Contains(translated, "event: response.failed") ||
		!strings.Contains(translated, "provider_stream_error") ||
		strings.Contains(translated, "event: response.completed") {
		t.Fatalf("read-error stream = %s", translated)
	}
}

func TestProdex04355DeepSeekChatSSEEmbeddedErrorFails(t *testing.T) {
	stream := `data: {"error":{"type":"rate_limit_error","message":"busy"}}

`
	translated := readDeepSeekChatSSEForTest(t, io.NopCloser(strings.NewReader(stream)))
	if !strings.Contains(translated, "event: response.failed") ||
		!strings.Contains(translated, "rate_limit_error") ||
		!strings.Contains(translated, "busy") ||
		strings.Contains(translated, "event: response.completed") {
		t.Fatalf("embedded-error stream = %s", translated)
	}
}

func TestProdex04355DeepSeekChatSSEPreservesCompletionMetadata(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"id":"chatcmpl_meta","model":"deepseek-v4-pro","created":1782740700,"system_fingerprint":"fp_deepseek_stream","choices":[{"delta":{"content":"done","refusal":"I cannot","annotations":[{"type":"url_citation","url":"https://example.com/ref"}]},"logprobs":{"content":[{"token":"done","logprob":-0.2,"top_logprobs":[]}]}}]}`,
		``,
		`data: {"id":"chatcmpl_meta","choices":[{"delta":{"refusal":" help with that."},"finish_reason":"stop"}]}`,
		``,
		`data: {"id":"chatcmpl_meta","choices":[],"usage":{"prompt_tokens":20,"completion_tokens":5,"total_tokens":25,"completion_tokens_details":{"reasoning_tokens":3},"prompt_cache_hit_tokens":12,"prompt_cache_miss_tokens":8}}`,
		``,
		`data: [DONE]`,
		``,
		``,
	}, "\n")
	translated := readDeepSeekChatSSEForTest(t, io.NopCloser(strings.NewReader(stream)))
	for _, want := range []string{
		`"created_at":1782740700`,
		`"input_tokens_details":{"cached_tokens":12}`,
		`"output_tokens_details":{"reasoning_tokens":3}`,
		`"prompt_cache_miss_tokens":8`,
		`"system_fingerprint":"fp_deepseek_stream"`,
		`"finish_reason":"stop"`,
		`"refusal":"I cannot help with that."`,
		`"annotations":[{"type":"url_citation","url":"https://example.com/ref"}]`,
		`"logprobs":{"content"`,
	} {
		if !strings.Contains(translated, want) {
			t.Fatalf("completion metadata missing %q: %s", want, translated)
		}
	}
}

func TestProdex04355DeepSeekChatSSECompletesMessageItemBeforeResponse(t *testing.T) {
	stream := "data: {\"id\":\"chatcmpl_text\",\"choices\":[{\"delta\":{\"content\":\"done\"}}]}\n\n" +
		"data: [DONE]\n\n"
	translated := readDeepSeekChatSSEForTest(t, io.NopCloser(strings.NewReader(stream)))
	added := strings.Index(translated, `"type":"response.output_item.added"`)
	delta := strings.Index(translated, `"type":"response.output_text.delta"`)
	done := strings.Index(translated, `"type":"response.output_item.done"`)
	completed := strings.Index(translated, `"type":"response.completed"`)
	if added < 0 || delta < 0 || done < 0 || completed < 0 || !(added < delta && delta < done && done < completed) ||
		!strings.Contains(translated, `"text":"done"`) {
		t.Fatalf("message event order = %s", translated)
	}
}

func TestProdex04355DeepSeekChatSSEMapsSpecialTools(t *testing.T) {
	for _, test := range []struct {
		name      string
		toolName  string
		arguments string
		want      []string
		reject    []string
	}{
		{
			name: "tool search", toolName: "tool_search", arguments: `{"query":"sqz tools"}`,
			want:   []string{`"type":"tool_search_call"`, `"execution":"client"`, `"query":"sqz tools"`},
			reject: []string{"event: response.output_item.added", `"type":"response.function_call_arguments.delta"`},
		},
		{
			name: "apply patch", toolName: "apply_patch", arguments: `{"input":"*** Begin Patch\n*** End Patch"}`,
			want:   []string{`"type":"custom_tool_call"`, `"name":"apply_patch"`, `*** Begin Patch`},
			reject: []string{`"type":"function_call"`},
		},
		{
			name: "mcp namespace", toolName: "mcp__prodex_sqz__compress", arguments: `{"text":"hello"}`,
			want: []string{`"type":"function_call"`, `"namespace":"mcp__prodex_sqz"`, `"name":"compress"`},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			stream := "data: {\"id\":\"chatcmpl_tool\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"function\":{\"name\":\"" + test.toolName + "\",\"arguments\":" + quotedJSONString(test.arguments) + "}}]},\"finish_reason\":\"tool_calls\"}]}\n\n" +
				"data: [DONE]\n\n"
			translated := readDeepSeekChatSSEForTest(t, io.NopCloser(strings.NewReader(stream)))
			for _, want := range test.want {
				if !strings.Contains(translated, want) {
					t.Fatalf("special tool missing %q: %s", want, translated)
				}
			}
			for _, reject := range test.reject {
				if strings.Contains(translated, reject) {
					t.Fatalf("special tool unexpectedly contains %q: %s", reject, translated)
				}
			}
		})
	}
}

func TestProdex04355DeepSeekChatSSEInvalidToolArgumentsFail(t *testing.T) {
	for _, stream := range []string{
		"data: {\"id\":\"bad_args\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"function\":{\"name\":\"shell\",\"arguments\":\"{\\\"cmd\\\":\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n",
		"data: {\"id\":\"bad_function\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\"}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n",
		"data: {\"id\":\"bad_name\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"function\":{\"arguments\":\"{}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n",
	} {
		translated := readDeepSeekChatSSEForTest(t, io.NopCloser(strings.NewReader(stream)))
		if !strings.Contains(translated, "event: response.failed") ||
			!strings.Contains(translated, "invalid_tool_call_arguments") ||
			strings.Contains(translated, "event: response.completed") {
			t.Fatalf("invalid tool stream = %s", translated)
		}
	}
}

func readDeepSeekChatSSEForTest(t *testing.T, source io.ReadCloser) string {
	t.Helper()
	body := deepSeekChatSSE(source)
	defer body.Close()
	content, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func quotedJSONString(value string) string {
	var builder strings.Builder
	builder.WriteByte('"')
	for _, current := range value {
		switch current {
		case '\\', '"':
			builder.WriteByte('\\')
			builder.WriteRune(current)
		case '\n':
			builder.WriteString(`\n`)
		default:
			builder.WriteRune(current)
		}
	}
	builder.WriteByte('"')
	return builder.String()
}
