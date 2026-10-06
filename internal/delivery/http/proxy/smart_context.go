package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"unicode/utf8"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	"github.com/tiktoken-go/tokenizer"
)

const (
	smartContextDuplicateTextMinBytes = 1024
	smartContextAdmissionMinBodyBytes = 512
	smartContextHTTPRewriteMaxBytes   = 256 * 1024
	smartContextJSONMaxDepth          = 64
	smartContextJSONMaxNodes          = 50_000
	smartContextDuplicatePlanMax      = 50_000
	smartContextTokenSavingsFloor     = 128
	smartContextTokenSavingsPercent   = 3
)

const smartContextInlineReferenceProtocol = "Godex context reference protocol v1: a 'godex-context-ref' value is byte-for-byte identical to the referenced 'original-input[N]' value in this request. Resolve it only from that earlier input item and verify its SHA-256 'sc2:' digest and byte length. No external retrieval is available."

type smartContextRewrite struct {
	Body      []byte
	Rewritten bool
}

type smartContextCandidate struct {
	inputIndex int
	text       string
}

var (
	smartContextTokenizerOnce sync.Once
	smartContextTokenizer     tokenizer.Codec
	smartContextTokenizerErr  error
)

func prepareSmartContextHTTPBody(enabled bool, path string, headers http.Header, body []byte) smartContextRewrite {
	original := smartContextRewrite{Body: body}
	if !enabled ||
		len(body) < smartContextAdmissionMinBodyBytes ||
		len(body) > smartContextHTTPRewriteMaxBytes ||
		!utf8.Valid(body) ||
		quotaSelection(path, false, body).RouteKind != quotamodel.RouteKindResponses ||
		smartContextExactRequested(headers) ||
		!smartContextJSONContentType(headers.Get("Content-Type")) {
		return original
	}

	value, ok := smartContextParseJSON(body)
	if !ok || smartContextUnsupportedShape(value) || smartContextRequiresExact(value, headers) {
		return original
	}
	object, ok := value.(map[string]any)
	if !ok {
		return original
	}
	model, _ := object["model"].(string)
	if !smartContextO200kModel(strings.TrimSpace(model)) {
		return original
	}
	input, ok := object["input"].([]any)
	if !ok || len(input) == 0 {
		return original
	}
	candidates, ok := smartContextCandidates(input)
	if !ok || len(candidates) == 0 {
		return original
	}

	rewritten := deepCloneJSON(value)
	rewrittenObject := rewritten.(map[string]any)
	rewrittenInput := rewrittenObject["input"].([]any)
	sources := make(map[string]smartContextSource)
	replacements := 0
	for _, candidate := range candidates {
		source, exists := sources[candidate.text]
		if !exists {
			sources[candidate.text] = smartContextSource{
				inputIndex: candidate.inputIndex,
				digest:     smartContextArtifactID(candidate.text),
				byteLen:    len(candidate.text),
			}
			continue
		}
		if source.inputIndex == candidate.inputIndex {
			continue
		}
		if smartContextReplaceText(
			rewrittenInput[candidate.inputIndex],
			candidate.text,
			smartContextInlineReference(source),
		) {
			replacements++
		}
	}
	if replacements == 0 {
		return original
	}
	rewrittenObject["input"] = append(rewrittenInput, map[string]any{
		"type":    "message",
		"role":    "developer",
		"content": smartContextInlineReferenceProtocol,
	})

	expanded, ok := smartContextExpandInlineReferences(value, rewritten)
	if !ok || !smartContextRoundTripExact(value, expanded) {
		return original
	}
	candidateBody, err := json.Marshal(rewritten)
	if err != nil || len(candidateBody) >= len(body) {
		return original
	}
	if !smartContextCriticalSignalsPreserved(body, candidateBody) {
		return original
	}
	beforeTokens, ok := smartContextTokenCount(body)
	if !ok {
		return original
	}
	afterTokens, ok := smartContextTokenCount(candidateBody)
	if !ok || afterTokens >= beforeTokens {
		return original
	}
	saved := beforeTokens - afterTokens
	required := max(
		smartContextTokenSavingsFloor,
		int(math.Ceil(float64(beforeTokens*smartContextTokenSavingsPercent)/100.0)),
	)
	if saved < required {
		return original
	}
	return smartContextRewrite{Body: candidateBody, Rewritten: true}
}

func smartContextParseJSON(body []byte) (any, bool) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return nil, false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, false
	}
	return value, true
}

func smartContextUnsupportedShape(value any) bool {
	depth, nodes, ok := smartContextShape(value, 1)
	return !ok || depth > smartContextJSONMaxDepth || nodes > smartContextJSONMaxNodes
}

func smartContextShape(value any, depth int) (int, int, bool) {
	if depth > smartContextJSONMaxDepth+1 {
		return depth, 1, false
	}
	maxDepth, nodes := depth, 1
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			childDepth, childNodes, ok := smartContextShape(item, depth+1)
			if !ok {
				return childDepth, nodes + childNodes, false
			}
			maxDepth = max(maxDepth, childDepth)
			nodes += childNodes
			if nodes > smartContextJSONMaxNodes {
				return maxDepth, nodes, false
			}
		}
	case map[string]any:
		for _, item := range typed {
			childDepth, childNodes, ok := smartContextShape(item, depth+1)
			if !ok {
				return childDepth, nodes + childNodes, false
			}
			maxDepth = max(maxDepth, childDepth)
			nodes += childNodes
			if nodes > smartContextJSONMaxNodes {
				return maxDepth, nodes, false
			}
		}
	}
	return maxDepth, nodes, true
}

func smartContextExactRequested(headers http.Header) bool {
	for _, name := range []string{"X-Godex-Smart-Context", "X-Prodex-Smart-Context"} {
		if strings.EqualFold(strings.TrimSpace(headers.Get(name)), "exact") {
			return true
		}
	}
	return false
}

func smartContextJSONContentType(contentType string) bool {
	if strings.TrimSpace(contentType) == "" {
		return true
	}
	media := strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])
	return strings.EqualFold(media, "application/json")
}

func smartContextRequiresExact(value any, headers http.Header) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return true
	}
	if smartContextNonblankString(object["previous_response_id"]) ||
		smartContextNonblankString(object["session_id"]) ||
		smartContextNonblankString(object["x-codex-turn-state"]) {
		return true
	}
	if metadata, ok := object["client_metadata"].(map[string]any); ok {
		if smartContextNonblankString(metadata["session_id"]) ||
			smartContextNonblankString(metadata["x-codex-turn-state"]) {
			return true
		}
	}
	if strings.TrimSpace(headers.Get("X-Codex-Turn-State")) != "" {
		return true
	}
	for _, name := range []string{"Session_Id", "Session-Id", "X-Session-Id"} {
		if strings.TrimSpace(headers.Get(name)) != "" {
			return true
		}
	}
	if raw := strings.TrimSpace(headers.Get("X-Codex-Turn-Metadata")); raw != "" {
		var metadata map[string]any
		if json.Unmarshal([]byte(raw), &metadata) == nil &&
			smartContextNonblankString(metadata["session_id"]) {
			return true
		}
	}
	return false
}

func smartContextNonblankString(value any) bool {
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) != ""
}

func smartContextCandidates(input []any) ([]smartContextCandidate, bool) {
	result := make([]smartContextCandidate, 0, 16)
	for index, item := range input {
		if object, ok := item.(map[string]any); ok {
			role, _ := object["role"].(string)
			if strings.EqualFold(strings.TrimSpace(role), "system") ||
				strings.EqualFold(strings.TrimSpace(role), "developer") {
				continue
			}
		}
		if !smartContextCollectCandidateText(item, index, &result) {
			return nil, false
		}
	}
	return result, true
}

func smartContextCollectCandidateText(value any, inputIndex int, output *[]smartContextCandidate) bool {
	switch typed := value.(type) {
	case string:
		if len(typed) >= smartContextDuplicateTextMinBytes {
			if len(*output) >= smartContextDuplicatePlanMax {
				return false
			}
			*output = append(*output, smartContextCandidate{inputIndex: inputIndex, text: typed})
		}
	case []any:
		for _, item := range typed {
			if !smartContextCollectCandidateText(item, inputIndex, output) {
				return false
			}
		}
	case map[string]any:
		for _, item := range typed {
			if !smartContextCollectCandidateText(item, inputIndex, output) {
				return false
			}
		}
	}
	return true
}

func smartContextReplaceText(value any, expected, replacement string) bool {
	switch typed := value.(type) {
	case []any:
		for index, item := range typed {
			if text, ok := item.(string); ok && text == expected {
				typed[index] = replacement
				return true
			}
			if smartContextReplaceText(item, expected, replacement) {
				return true
			}
		}
	case map[string]any:
		for key, item := range typed {
			if text, ok := item.(string); ok && text == expected {
				typed[key] = replacement
				return true
			}
			if smartContextReplaceText(item, expected, replacement) {
				return true
			}
		}
	}
	return false
}

func deepCloneJSON(value any) any {
	switch typed := value.(type) {
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = deepCloneJSON(item)
		}
		return result
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			result[key] = deepCloneJSON(item)
		}
		return result
	default:
		return typed
	}
}

func smartContextO200kModel(model string) bool {
	model = strings.TrimSpace(model)
	if strings.HasPrefix(model, "ft:") {
		base := strings.TrimPrefix(model, "ft:")
		if separator := strings.IndexByte(base, ':'); separator >= 0 {
			base = base[:separator]
		}
		return smartContextO200kModel(base)
	}
	for _, exact := range []string{"o1", "o3", "o4-mini", "gpt-5", "gpt-4.1", "gpt-4o"} {
		if model == exact {
			return true
		}
	}
	for _, prefix := range []string{
		"o1-", "o3-", "o4-mini-", "gpt-5", "gpt-4.5-", "gpt-4.1-",
		"chatgpt-4o-", "gpt-4o-", "codex-mini",
	} {
		if strings.HasPrefix(model, prefix) {
			return true
		}
	}
	return false
}

func smartContextTokenCount(body []byte) (int, bool) {
	smartContextTokenizerOnce.Do(func() {
		smartContextTokenizer, smartContextTokenizerErr = tokenizer.Get(tokenizer.O200kBase)
	})
	if smartContextTokenizerErr != nil || smartContextTokenizer == nil {
		return 0, false
	}
	count, err := smartContextTokenizer.Count(string(body))
	return count, err == nil
}
