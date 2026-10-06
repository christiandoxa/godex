package presidio

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04356PresidioConfigDefaultsAndValidation(t *testing.T) {
	config, enabled, err := LoadConfig(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if enabled || config.AnalyzerURL != DefaultAnalyzerURL || config.AnonymizerURL != DefaultAnonymizerURL ||
		len(config.Languages) != 1 || config.Languages[0] != "en" || config.LanguageMode != "fixed" ||
		config.FailClosed || config.Timeout != DefaultTimeout || config.MaxResponseBytes != DefaultMaxResponseBytes ||
		config.MaxConcurrency != DefaultMaxConcurrency {
		t.Fatalf("default config = %#v enabled=%t", config, enabled)
	}

	root := t.TempDir()
	body := strings.Join([]string{
		"enabled = true",
		"analyzer_url = \"http://127.0.0.1:5102\"",
		"anonymizer_url = \"http://127.0.0.1:5101\"",
		"languages = [\"en\", \"id\"]",
		"language_mode = \"multi\"",
		"fail_mode = \"closed\"",
		"trusted_hosts = [\"presidio.example.test\"]",
		"timeout_ms = 1500",
		"max_response_bytes = 4096",
		"max_concurrency = 3",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, ConfigFileName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	config, enabled, err = LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if !enabled || !config.FailClosed || config.LanguageMode != "multi" || len(config.Languages) != 2 ||
		config.Timeout != 1500*time.Millisecond || config.MaxResponseBytes != 4096 || config.MaxConcurrency != 3 {
		t.Fatalf("configured Presidio = %#v enabled=%t", config, enabled)
	}

	for name, invalid := range map[string]string{
		"credential URL": "analyzer_url = \"https://user:pass@example.test\"",
		"query URL":      "anonymizer_url = \"https://example.test?token=secret\"",
		"bad fail":       "fail_mode = \"maybe\"",
		"bad fixed":      "languages = [\"en\", \"id\"]\nlanguage_mode = \"fixed\"",
		"unknown":        "surprise = true",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, ConfigFileName), []byte(invalid), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := LoadConfig(root); err == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}

func TestProdex04356PresidioSchemaAwareRedactionPreservesProtocolAndTools(t *testing.T) {
	var analyzerSawEmail atomic.Bool
	fixture := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/analyze":
			var payload map[string]any
			_ = json.NewDecoder(request.Body).Decode(&payload)
			text, _ := payload["text"].(string)
			if strings.Contains(text, "user@example.com") {
				analyzerSawEmail.Store(true)
			}
			_ = json.NewEncoder(writer).Encode([]any{})
		case "/anonymize":
			_ = json.NewEncoder(writer).Encode(map[string]any{"text": "", "items": []any{}})
		case "/health":
			_, _ = writer.Write([]byte("ok"))
		}
	}))
	defer fixture.Close()

	redactor := newFixtureRedactor(t, fixture.URL, true)
	input := []byte(`{"model":"gpt-5.6","input":[{"role":"user","content":[{"type":"input_text","text":"contact user@example.com"}]}],"tools":[{"type":"function","name":"send","description":"tool user@example.com"}]}`)
	got, err := redactor.Redact(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if strings.Contains(text, "contact user@example.com") {
		t.Fatalf("content not redacted: %s", text)
	}
	if !strings.Contains(text, `"model":"gpt-5.6"`) || !strings.Contains(text, `"description":"tool user@example.com"`) {
		t.Fatalf("protocol/tool metadata changed: %s", text)
	}
	if analyzerSawEmail.Load() {
		t.Fatal("local inspection failed to mask email before external analyzer")
	}
}

func TestProdex04356PresidioFailClosedRejectsPartialCoverage(t *testing.T) {
	fixture := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/analyze" {
			_ = json.NewEncoder(writer).Encode([]any{})
			return
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"text": "", "items": []any{}})
	}))
	defer fixture.Close()
	body := []byte(`{"input":"hello","input_image":{"url":"https://example.test/a.png"}}`)
	closed := newFixtureRedactor(t, fixture.URL, true)
	if _, err := closed.Redact(t.Context(), body); err == nil || !strings.Contains(err.Error(), "presidio_redaction_failed") {
		t.Fatalf("partial coverage fail-closed = %v", err)
	}
	open := newFixtureRedactor(t, fixture.URL, false)
	got, err := open.Redact(t.Context(), body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "input_image") {
		t.Fatalf("fail-open lost modality: %s", got)
	}
}

func TestProdex04356PresidioFailureModesAndNoopAnonymizer(t *testing.T) {
	config := proxymodel.PresidioConfig{AnalyzerURL: "http://127.0.0.1:1", AnonymizerURL: "http://127.0.0.1:1", Languages: []string{"en"}, LanguageMode: "fixed", Timeout: 100 * time.Millisecond, MaxResponseBytes: 4096, MaxConcurrency: 2}
	open, _ := NewRedactor(config)
	got, err := open.Redact(t.Context(), []byte(`{"input":"email user@example.com and sk-1234567890abcdef"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "user@example.com") || strings.Contains(string(got), "sk-1234567890abcdef") {
		t.Fatalf("fail-open lost local masking: %s", got)
	}
	config.FailClosed = true
	closed, _ := NewRedactor(config)
	if _, err := closed.Redact(t.Context(), []byte(`{"input":"ordinary text"}`)); err == nil {
		t.Fatal("fail-closed unavailable Presidio allowed request")
	}

	fixture := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/analyze":
			_ = json.NewEncoder(writer).Encode([]map[string]any{{"start": 0, "end": 4, "score": 0.99, "entity_type": "PERSON"}})
		case "/anonymize":
			var payload map[string]any
			_ = json.NewDecoder(request.Body).Decode(&payload)
			_ = json.NewEncoder(writer).Encode(map[string]any{"text": payload["text"], "items": []any{}})
		}
	}))
	defer fixture.Close()
	redactor := newFixtureRedactor(t, fixture.URL, true)
	if _, err := redactor.Redact(t.Context(), []byte(`{"input":"name John"}`)); err == nil {
		t.Fatal("no-op anonymizer did not fail closed")
	}
}

func TestProdex04356PresidioAutoAndMultiLanguage(t *testing.T) {
	var languages []string
	fixture := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/analyze" {
			var payload map[string]any
			_ = json.NewDecoder(request.Body).Decode(&payload)
			languages = append(languages, payload["language"].(string))
			_ = json.NewEncoder(writer).Encode([]any{})
			return
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"text": "", "items": []any{}})
	}))
	defer fixture.Close()
	config := fixtureConfig(fixture.URL, false)
	config.LanguageMode = "auto"
	config.Languages = []string{"en", "id"}
	redactor, _ := NewRedactor(config)
	if _, err := redactor.Redact(t.Context(), []byte(`{"input":"nama dan alamat saya"}`)); err != nil {
		t.Fatal(err)
	}
	if len(languages) != 1 || languages[0] != "id" {
		t.Fatalf("auto calls = %#v", languages)
	}
	languages = nil
	config.LanguageMode = "multi"
	redactor, _ = NewRedactor(config)
	if _, err := redactor.Redact(t.Context(), []byte(`{"input":"ordinary hello"}`)); err != nil {
		t.Fatal(err)
	}
	if len(languages) != 2 || languages[0] != "en" || languages[1] != "id" {
		t.Fatalf("multi calls = %#v", languages)
	}
}

func TestProdex04356PresidioRedirectBoundAndConcurrency(t *testing.T) {
	targetHit := atomic.Bool{}
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { targetHit.Store(true) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/leak", http.StatusFound)
	}))
	defer redirect.Close()
	redactor := newFixtureRedactor(t, redirect.URL, true)
	if _, err := redactor.Redact(t.Context(), []byte(`{"input":"ordinary"}`)); err == nil {
		t.Fatal("redirect analyzer allowed")
	}
	if targetHit.Load() {
		t.Fatal("redirect followed")
	}

	large := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(strings.Repeat("x", 2048))) }))
	defer large.Close()
	cfg := fixtureConfig(large.URL, true)
	cfg.MaxResponseBytes = 1024
	redactor, _ = NewRedactor(cfg)
	if _, err := redactor.Redact(t.Context(), []byte(`{"input":"ordinary"}`)); err == nil {
		t.Fatal("oversized response allowed")
	}

	block := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-block; _ = json.NewEncoder(w).Encode([]any{}) }))
	defer slow.Close()
	cfg = fixtureConfig(slow.URL, true)
	cfg.MaxConcurrency = 1
	cfg.Timeout = 2 * time.Second
	redactor, _ = NewRedactor(cfg)
	first := make(chan error, 1)
	go func() { _, err := redactor.Redact(context.Background(), []byte(`{"input":"one"}`)); first <- err }()
	time.Sleep(50 * time.Millisecond)
	if _, err := redactor.Redact(t.Context(), []byte(`{"input":"two"}`)); err == nil || !strings.Contains(err.Error(), "concurrency") {
		t.Fatalf("cap = %v", err)
	}
	close(block)
	<-first
}

func fixtureConfig(base string, closed bool) proxymodel.PresidioConfig {
	return proxymodel.PresidioConfig{AnalyzerURL: base, AnonymizerURL: base, Languages: []string{"en"}, LanguageMode: "fixed", FailClosed: closed, Timeout: time.Second, MaxResponseBytes: 4096, MaxConcurrency: 8}
}
func newFixtureRedactor(t *testing.T, base string, closed bool) *Redactor {
	t.Helper()
	r, err := NewRedactor(fixtureConfig(base, closed))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestProdex04356PresidioResolveForLaunchRequiredAndOptionalPolicy(t *testing.T) {
	t.Setenv("GODEX_PRESIDIO_AUTO_START", "0")

	root := t.TempDir()
	openConfig := strings.Join([]string{
		"fail_mode = \"open\"",
		"analyzer_url = \"http://127.0.0.1:1\"",
		"anonymizer_url = \"http://127.0.0.1:1\"",
		"timeout_ms = 100",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, ConfigFileName), []byte(openConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveForLaunch(t.Context(), root, true); err == nil ||
		!strings.Contains(err.Error(), "fail_mode") {
		t.Fatalf("required fail-open config = %v", err)
	}
	resolved, err := ResolveForLaunch(t.Context(), root, false)
	if err != nil {
		t.Fatal(err)
	}
	if resolved == nil || resolved.FailClosed {
		t.Fatalf("optional fail-open config = %#v", resolved)
	}

	healthy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/health" {
			http.NotFound(writer, request)
			return
		}
		_, _ = writer.Write([]byte("ok"))
	}))
	defer healthy.Close()
	root = t.TempDir()
	closedHealthy := strings.Join([]string{
		"fail_mode = \"closed\"",
		"analyzer_url = \"" + healthy.URL + "\"",
		"anonymizer_url = \"" + healthy.URL + "\"",
		"timeout_ms = 500",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, ConfigFileName), []byte(closedHealthy), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err = ResolveForLaunch(t.Context(), root, true)
	if err != nil {
		t.Fatal(err)
	}
	if resolved == nil || !resolved.FailClosed {
		t.Fatalf("required healthy config = %#v", resolved)
	}

	root = t.TempDir()
	closedUnhealthy := strings.Join([]string{
		"fail_mode = \"closed\"",
		"analyzer_url = \"http://127.0.0.1:1\"",
		"anonymizer_url = \"http://127.0.0.1:1\"",
		"timeout_ms = 100",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, ConfigFileName), []byte(closedUnhealthy), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveForLaunch(t.Context(), root, true); err == nil ||
		!strings.Contains(err.Error(), "required Presidio services are not ready") {
		t.Fatalf("required unhealthy custom endpoints = %v", err)
	}
}

func TestProdex04356PresidioStartupAndImagePolicyUseGodexCanonicalControls(t *testing.T) {
	for _, value := range []string{"0", "FALSE", " no ", "OFF"} {
		t.Run("disabled-"+strings.TrimSpace(value), func(t *testing.T) {
			t.Setenv("GODEX_PRESIDIO_AUTO_START", value)
			if !startupDisabled() {
				t.Fatalf("auto-start value %q did not disable startup", value)
			}
		})
	}
	t.Setenv("GODEX_PRESIDIO_AUTO_START", "true")
	if startupDisabled() {
		t.Fatal("true unexpectedly disabled startup")
	}
	if _, err := resolveImage("", "", "analyzer", presidioAnalyzerImage); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveImage("", "", "analyzer", "ghcr.io/data-privacy-stack/presidio-analyzer:2.2.363"); err == nil {
		t.Fatal("old Presidio image accepted")
	}
	if _, err := resolveImage("", "", "analyzer", "example.com/presidio-analyzer:3.0.0"); err == nil {
		t.Fatal("foreign Presidio image accepted")
	}
}
