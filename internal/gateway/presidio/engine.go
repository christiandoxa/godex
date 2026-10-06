package presidio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/christiandoxa/godex/internal/helper/redact"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type Redactor struct {
	config proxymodel.PresidioConfig
	client *http.Client
	slots  chan struct{}
}
type Health struct {
	OK      bool
	Message string
}

type analyzerResult struct {
	Start, End           int
	Score                float64
	EntityType, Language string
}
type inspectMode int

const (
	inspectSchema inspectMode = iota
	inspectDirect
	inspectAll
)

type fieldPlan struct {
	mode                                 inspectMode
	known, unsupported, tools, sensitive bool
}
type target struct {
	text      string
	sensitive bool
	object    map[string]any
	key       string
	array     []any
	index     int
}

func (t target) set(value string) {
	if t.object != nil {
		t.object[t.key] = value
	} else if t.array != nil {
		t.array[t.index] = value
	}
}

type contentState struct {
	targets         []target
	partial, opaque bool
}

var emailPattern = regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`)
var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var digitGroupPattern = regexp.MustCompile(`[0-9][0-9 -]{11,25}[0-9]`)

func NewRedactor(config proxymodel.PresidioConfig) (*Redactor, error) {
	if config.Timeout <= 0 {
		config.Timeout = DefaultTimeout
	}
	if config.MaxResponseBytes <= 0 {
		config.MaxResponseBytes = DefaultMaxResponseBytes
	}
	if config.MaxConcurrency <= 0 {
		config.MaxConcurrency = DefaultMaxConcurrency
	}
	transport, ok := http.DefaultTransport.(*http.Transport)
	var roundTripper http.RoundTripper = http.DefaultTransport
	if ok {
		clone := transport.Clone()
		clone.Proxy = nil
		roundTripper = clone
	}
	client := &http.Client{Transport: roundTripper, Timeout: config.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &Redactor{config: config, client: client, slots: make(chan struct{}, config.MaxConcurrency)}, nil
}

func (r *Redactor) Probe(ctx context.Context, base string) Health {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint(base, "health"), nil)
	if err != nil {
		return Health{Message: "invalid health request"}
	}
	response, err := r.client.Do(request)
	if err != nil {
		return Health{Message: redact.Secrets(err.Error())}
	}
	defer response.Body.Close()
	body, readErr := readBounded(response.Body, r.config.MaxResponseBytes)
	if readErr != nil {
		return Health{Message: redact.Secrets(readErr.Error())}
	}
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = response.Status
	}
	return Health{OK: response.StatusCode >= 200 && response.StatusCode < 300, Message: redact.Secrets(response.Status + " " + message)}
}

func (r *Redactor) Redact(ctx context.Context, body []byte) ([]byte, error) {
	select {
	case r.slots <- struct{}{}:
		defer func() { <-r.slots }()
	default:
		return body, errors.New("presidio_concurrency_limit_reached")
	}
	value, isJSON := parseJSON(body)
	if !isJSON {
		return r.redactTextBody(ctx, body)
	}
	state := contentState{}
	walkJSON(value, inspectSchema, true, &state)
	if len(state.targets) == 0 {
		if r.config.FailClosed {
			return body, errors.New("presidio_redaction_failed")
		}
		return body, nil
	}
	for index := range state.targets {
		masked := localMask(state.targets[index].text, state.targets[index].sensitive)
		state.targets[index].text = masked
		state.targets[index].set(masked)
	}
	coverageFull := !state.partial && !state.opaque
	if r.config.FailClosed && !coverageFull {
		return body, errors.New("presidio_redaction_failed")
	}
	if err := r.externalRedactTargets(ctx, state.targets); err != nil {
		if r.config.FailClosed {
			return body, errors.New("presidio_redaction_failed")
		}
		encoded, _ := json.Marshal(value)
		return encoded, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		if r.config.FailClosed {
			return body, errors.New("presidio_redaction_failed")
		}
		return body, nil
	}
	return encoded, nil
}

func (r *Redactor) redactTextBody(ctx context.Context, body []byte) ([]byte, error) {
	if !utf8.Valid(body) {
		if r.config.FailClosed {
			return body, errors.New("presidio_redaction_failed")
		}
		return body, nil
	}
	masked := localMask(string(body), false)
	targets := []target{{text: masked}}
	if err := r.externalRedactTargets(ctx, targets); err != nil {
		if r.config.FailClosed {
			return body, errors.New("presidio_redaction_failed")
		}
		return []byte(masked), nil
	}
	return []byte(targets[0].text), nil
}

func (r *Redactor) externalRedactTargets(ctx context.Context, targets []target) error {
	values := make([]string, len(targets))
	for i := range targets {
		values[i] = targets[i].text
	}
	separator := presidioSeparator(values)
	combined := strings.Join(values, separator)
	results, err := r.analyze(ctx, combined)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		return nil
	}
	anonymized, err := r.anonymize(ctx, combined, results)
	if err != nil {
		return err
	}
	if anonymized == combined {
		return errors.New("Presidio Anonymizer returned unchanged text for findings")
	}
	split := strings.Split(anonymized, separator)
	if len(split) != len(targets) {
		return errors.New("Presidio changed JSON value separator count")
	}
	for i := range targets {
		targets[i].text = split[i]
		targets[i].set(split[i])
	}
	return nil
}

func (r *Redactor) analyze(ctx context.Context, text string) ([]analyzerResult, error) {
	languages := r.languagesFor(text)
	all := make([]analyzerResult, 0)
	for _, language := range languages {
		results, err := r.analyzeLanguage(ctx, text, language)
		if err != nil {
			return nil, err
		}
		all = append(all, results...)
	}
	return mergeResults(all), nil
}
func (r *Redactor) languagesFor(text string) []string {
	if len(r.config.Languages) <= 1 || r.config.LanguageMode == "fixed" {
		return append([]string(nil), r.config.Languages...)
	}
	if r.config.LanguageMode == "multi" {
		return append([]string(nil), r.config.Languages...)
	}
	return []string{detectLanguage(text, r.config.Languages)}
}
func detectLanguage(text string, candidates []string) string {
	lower := strings.ToLower(text)
	idWords := []string{"yang", "dan", "di", "ke", "dari", "saya", "kami", "anda", "nomor", "nama", "alamat", "tanggal", "lahir", "dengan", "untuk"}
	enWords := []string{"the", "and", "to", "from", "my", "name", "phone", "email", "address", "with", "for", "birth"}
	score := func(words []string) int {
		n := 0
		for _, w := range words {
			if strings.Contains(lower, w) {
				n++
			}
		}
		return n
	}
	id, en := score(idWords), score(enWords)
	if id > en && contains(candidates, "id") {
		return "id"
	}
	if en > id && contains(candidates, "en") {
		return "en"
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return DefaultLanguage
}

func (r *Redactor) analyzeLanguage(ctx context.Context, text, language string) ([]analyzerResult, error) {
	payload, _ := json.Marshal(map[string]any{"text": text, "language": language})
	body, status, err := r.post(ctx, r.config.AnalyzerURL, "analyze", payload)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("Presidio Analyzer returned %d: %s", status, redact.Secrets(strings.TrimSpace(string(body))))
	}
	var raw []map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, errors.New("failed to parse Presidio Analyzer response")
	}
	out := make([]analyzerResult, 0, len(raw))
	for _, item := range raw {
		start, ok1 := numberInt(item["start"])
		end, ok2 := numberInt(item["end"])
		score, ok3 := numberFloat(item["score"])
		kind, ok4 := item["entity_type"].(string)
		if !ok1 || !ok2 || !ok3 || !ok4 || start > end {
			return nil, errors.New("Presidio Analyzer returned an invalid finding range")
		}
		lang, _ := item["language"].(string)
		if lang == "" {
			lang = language
		}
		out = append(out, analyzerResult{Start: start, End: end, Score: score, EntityType: kind, Language: lang})
	}
	return out, nil
}
func (r *Redactor) anonymize(ctx context.Context, text string, results []analyzerResult) (string, error) {
	raw := make([]map[string]any, 0, len(results))
	for _, v := range results {
		raw = append(raw, map[string]any{"start": v.Start, "end": v.End, "score": v.Score, "entity_type": v.EntityType, "language": v.Language})
	}
	payload, _ := json.Marshal(map[string]any{"text": text, "analyzer_results": raw})
	body, status, err := r.post(ctx, r.config.AnonymizerURL, "anonymize", payload)
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("Presidio Anonymizer returned %d: %s", status, redact.Secrets(strings.TrimSpace(string(body))))
	}
	var value map[string]any
	if json.Unmarshal(body, &value) != nil {
		return "", errors.New("failed to parse Presidio Anonymizer response")
	}
	textOut, ok := value["text"].(string)
	if !ok {
		return "", errors.New("failed to parse Presidio Anonymizer response")
	}
	return textOut, nil
}
func (r *Redactor) post(ctx context.Context, base, path string, payload []byte) ([]byte, int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint(base, path), bytes.NewReader(payload))
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := r.client.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	body, err := readBounded(response.Body, r.config.MaxResponseBytes)
	return body, response.StatusCode, err
}
func readBounded(reader io.Reader, max int) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, int64(max)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > max {
		return nil, fmt.Errorf("Presidio response exceeded safe size limit (%d)", max)
	}
	return body, nil
}
func endpoint(base, path string) string { return strings.TrimRight(base, "/") + "/" + path }

func parseJSON(body []byte) (any, bool) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return nil, false
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, false
	}
	return value, true
}
func walkJSON(value any, mode inspectMode, root bool, state *contentState) {
	switch typed := value.(type) {
	case []any:
		for index, item := range typed {
			if text, ok := item.(string); ok && mode != inspectSchema {
				state.targets = append(state.targets, target{text: text, array: typed, index: index})
			} else {
				walkJSON(item, mode, false, state)
			}
		}
	case map[string]any:
		skipTools := root
		if role, _ := typed["role"].(string); role == "developer" {
			skipTools = true
		}
		for key, item := range typed {
			plan := planField(key)
			if skipTools && plan.tools {
				continue
			}
			if plan.unsupported && item != nil {
				state.partial = true
				continue
			}
			childMode := plan.mode
			if mode == inspectAll {
				childMode = inspectAll
			}
			if mode != inspectAll && plan.mode == inspectSchema && item != nil && !plan.known {
				state.opaque = true
			}
			if text, ok := item.(string); ok && childMode != inspectSchema {
				state.targets = append(state.targets, target{text: text, sensitive: plan.sensitive, object: typed, key: key})
			} else {
				walkJSON(item, childMode, false, state)
			}
		}
	}
}
func planField(key string) fieldPlan {
	p := fieldPlan{mode: inspectSchema}
	switch key {
	case "arguments", "output":
		p.mode = inspectAll
	case "content", "input", "instructions", "prompt", "text":
		p.mode = inspectDirect
	}
	p.known = knownMetadata(key)
	p.unsupported = unsupportedModality(key)
	p.tools = key == "tools"
	p.sensitive = sensitiveKey(key)
	return p
}
func knownMetadata(key string) bool {
	switch key {
	case "background", "call_id", "conversation", "id", "include", "max_completion_tokens", "max_output_tokens", "model", "name", "parallel_tool_calls", "previous_response_id", "prompt_cache_key", "reasoning", "response_format", "role", "server_label", "service_tier", "store", "stream", "temperature", "top_k", "top_p", "truncation", "type", "user", "verbosity":
		return true
	}
	return false
}
func unsupportedModality(key string) bool {
	switch key {
	case "audio", "audio_url", "file", "image", "image_url", "input_audio", "input_file", "input_image", "video":
		return true
	}
	return false
}
func sensitiveKey(key string) bool {
	n := normalizeKey(key)
	exact := map[string]bool{"authorization": true, "apikey": true, "xapikey": true, "authkey": true, "cookie": true, "setcookie": true, "token": true, "accesstoken": true, "refreshtoken": true, "idtoken": true, "secret": true, "password": true, "credential": true, "credentials": true, "email": true, "githublogin": true, "profilearn": true, "profilenameupstream": true, "starturl": true, "accountid": true, "chatgptaccountid": true, "proxyauthorization": true}
	if exact[n] || strings.HasSuffix(n, "token") {
		return true
	}
	for _, part := range []string{"apikey", "privatekey", "secret", "password", "cookie", "credential"} {
		if strings.Contains(n, part) {
			return true
		}
	}
	return false
}
func normalizeKey(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func localMask(text string, sensitive bool) string {
	if text == "" {
		return text
	}
	if sensitive {
		return "<redacted>"
	}
	text = redact.Secrets(text)
	text = emailPattern.ReplaceAllString(text, "<redacted>")
	text = digitGroupPattern.ReplaceAllStringFunc(text, func(candidate string) string {
		trim := strings.TrimSpace(candidate)
		if uuidPattern.MatchString(trim) {
			return candidate
		}
		digits := 0
		for _, r := range candidate {
			if r >= '0' && r <= '9' {
				digits++
			}
		}
		if digits >= 13 && digits <= 19 {
			return "<redacted>"
		}
		return candidate
	})
	return text
}
func presidioSeparator(values []string) string {
	sep := "\ue000GODEX_PRESIDIO_VALUE\ue001"
	for {
		collision := false
		for _, v := range values {
			if strings.Contains(v, sep) {
				collision = true
				break
			}
		}
		if !collision {
			return sep
		}
		sep += "\ue002"
	}
}
func mergeResults(values []analyzerResult) []analyzerResult {
	sort.Slice(values, func(i, j int) bool {
		if values[i].Start != values[j].Start {
			return values[i].Start < values[j].Start
		}
		if values[i].End != values[j].End {
			return values[i].End < values[j].End
		}
		if values[i].Score != values[j].Score {
			return values[i].Score > values[j].Score
		}
		return values[i].EntityType > values[j].EntityType
	})
	merged := make([]analyzerResult, 0, len(values))
	for _, v := range values {
		if len(merged) == 0 {
			merged = append(merged, v)
			continue
		}
		last := &merged[len(merged)-1]
		if last.Start == v.Start && last.End == v.End && last.EntityType == v.EntityType {
			if v.Score > last.Score {
				*last = v
			}
			continue
		}
		overlap := v.Start < last.End && v.End > last.Start
		stronger := v.Score > last.Score || (v.Score == last.Score && (v.End-v.Start) > (last.End-last.Start))
		if overlap && stronger {
			contained := (v.Start >= last.Start && v.End <= last.End) || (last.Start >= v.Start && last.End <= v.End)
			if contained {
				if v.Score > last.Score {
					*last = v
				}
				continue
			}
			if v.Score > last.Score {
				last.Start = min(last.Start, v.Start)
				last.End = max(last.End, v.End)
				last.Score = v.Score
				last.EntityType = v.EntityType
				last.Language = v.Language
				continue
			}
		}
		merged = append(merged, v)
	}
	return merged
}
func numberInt(value any) (int, bool) {
	switch v := value.(type) {
	case json.Number:
		n, e := v.Int64()
		return int(n), e == nil
	case float64:
		return int(v), v == float64(int(v))
	}
	return 0, false
}
func numberFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case json.Number:
		n, e := v.Float64()
		return n, e == nil
	case float64:
		return v, true
	}
	return 0, false
}
func contains(values []string, value string) bool {
	for _, current := range values {
		if current == value {
			return true
		}
	}
	return false
}
