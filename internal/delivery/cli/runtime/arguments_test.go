package runtime

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseRunArguments(t *testing.T) {
	selector, arguments, err := parseRunArguments([]string{"--account", "work", "--", "--model", "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	if selector.Account != "work" || selector.Profile != "" || len(arguments) != 2 || arguments[0] != "--model" {
		t.Fatalf("selection=%+v arguments=%#v", selector, arguments)
	}
}

func TestParseRunArgumentsPassesModelThroughWhileTrackingWrapperOverride(t *testing.T) {
	selection, arguments, err := parseRunArguments([]string{"--model", "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	if selection.Model != "synthetic" || selection.Provider != "" || selection.URL != "" ||
		len(arguments) != 2 || arguments[0] != "--model" || arguments[1] != "synthetic" {
		t.Fatalf("selection=%+v arguments=%#v", selection, arguments)
	}
}

func TestParseRunArgumentsRequiresAccountSelector(t *testing.T) {
	if _, _, err := parseRunArguments([]string{"--account"}); err == nil {
		t.Fatal("missing account selector unexpectedly accepted")
	}
}

func TestParseRunArgumentsSupportsProfileSelectors(t *testing.T) {
	for _, arguments := range [][]string{{"--profile", "work", "exec"}, {"-p=work", "exec"}} {
		selection, codexArguments, err := parseRunArguments(arguments)
		if err != nil {
			t.Fatal(err)
		}
		if selection.Profile != "work" || selection.Account != "" || len(codexArguments) != 1 {
			t.Fatalf("selection/arguments = %+v / %#v", selection, codexArguments)
		}
	}
	if _, _, err := parseRunArguments([]string{"--account", "one", "--profile", "two"}); err == nil {
		t.Fatal("account/profile conflict unexpectedly accepted")
	}
}

func TestParseRunArgumentsSupportsProfileSelection(t *testing.T) {
	selection, arguments, err := parseRunArguments([]string{"--profile", "work", "--", "exec", "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if selection.Profile != "work" || selection.Account != "" || len(arguments) != 2 || arguments[0] != "exec" {
		t.Fatalf("selection=%#v arguments=%#v", selection, arguments)
	}
	if _, _, err := parseRunArguments([]string{"--account", "one", "--profile", "two"}); err == nil {
		t.Fatal("account and profile selectors unexpectedly combined")
	}
}

func TestParseRunArgumentsSupportsExternalProviderShortcut(t *testing.T) {
	selection, arguments, err := parseRunArguments([]string{
		"--provider", " CLAUDE ",
		"--api-key", "fixture-secret",
		"--base-url=https://api.example.test/v1",
		"--", "exec", "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if selection.Provider != "anthropic" ||
		selection.APIKey != "fixture-secret" ||
		selection.BaseURL != "https://api.example.test/v1" ||
		selection.Account != "" ||
		len(arguments) != 2 || arguments[0] != "exec" {
		t.Fatalf("selection/arguments = %#v / %#v", selection, arguments)
	}
}

func TestParseRunArgumentsProviderValidationAndLiteralSeparator(t *testing.T) {
	for _, arguments := range [][]string{
		{"--provider", "unknown"},
		{"--api-key", "secret"},
		{"--base-url", "https://example.test"},
		{"--provider", "anthropic", "--account", "work"},
	} {
		if _, _, err := parseRunArguments(arguments); err == nil {
			t.Fatalf("arguments %#v unexpectedly accepted", arguments)
		}
	}
	selection, arguments, err := parseRunArguments([]string{
		"--provider=anthropic", "--", "--api-key", "literal-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if selection.Provider != "anthropic" || selection.APIKey != "" ||
		len(arguments) != 2 || arguments[0] != "--api-key" {
		t.Fatalf("selection/arguments = %#v / %#v", selection, arguments)
	}
}

func TestParseRunArgumentsProviderAliases(t *testing.T) {
	fixtures := map[string]string{
		"anthropic": "anthropic", "claude": "anthropic",
		"copilot": "copilot", "github-copilot": "copilot", "github_copilot": "copilot",
		"deepseek": "deepseek", "gemini": "gemini", "kiro": "kiro",
	}
	for input, want := range fixtures {
		selection, _, err := parseRunArguments([]string{"--provider", input})
		if err != nil || selection.Provider != want {
			t.Fatalf("provider %q = %#v, err=%v", input, selection, err)
		}
	}
}

func TestParseRunArgumentsSupportsNativeAntigravityAndRejectsUnsupportedOptions(t *testing.T) {
	selection, arguments, err := parseRunArguments([]string{
		"--provider", "gemini", "--cli", "agy", "--model", "gemini-3.1-pro", "--", "exec", "review",
	})
	if err != nil {
		t.Fatal(err)
	}
	if selection.Provider != "gemini" || selection.CLI != "agy" || selection.Model != "gemini-3.1-pro" ||
		!reflect.DeepEqual(arguments, []string{"exec", "review"}) {
		t.Fatalf("native Antigravity selection = %#v, args = %#v", selection, arguments)
	}
	for _, args := range [][]string{
		{"--cli", "unknown"},
		{"--provider", "gemini", "--cli", "AGY"},
		{"--provider", "gemini", "--cli", " agy "},
		{"--cli", "agy"},
		{"--provider", "anthropic", "--cli", "agy"},
		{"--provider", "gemini", "--cli", "agy", "--account", "work"},
		{"--provider", "gemini", "--cli", "agy", "--profile", "work"},
		{"--provider", "gemini", "--cli", "agy", "--api-key", "synthetic"},
		{"--provider", "gemini", "--cli", "agy", "--base-url", "https://example.test"},
		{"--provider", "gemini", "--cli", "agy", "--auto-redeem"},
		{"--provider", "gemini", "--cli", "agy", "--web-search", "live"},
	} {
		if _, _, err := parseRunArguments(args); err == nil {
			t.Fatalf("native Antigravity accepted unsupported arguments %#v", args)
		}
	}
	for _, args := range [][]string{
		{"--provider", "gemini", "--cli", "agy", "resume", "thread"},
		{"--provider", "gemini", "--cli", "agy", "exec", "resume", "thread"},
	} {
		if _, _, err := parseRunArguments(args); err == nil || !strings.Contains(err.Error(), "resume is unsupported") {
			t.Fatalf("native Antigravity accepted resume arguments %#v: %v", args, err)
		}
	}
	if _, _, err := parseRunArguments([]string{
		"--provider", "gemini", "--cli", "agy", "--", "--model", "resume", "exec", "review",
	}); err != nil {
		t.Fatalf("native Antigravity mistook a model value for resume: %v", err)
	}
	selection, arguments, err = parseRunArguments([]string{
		"--provider", "gemini", "--cli", "agy", "--dry-run", "--", "exec", "review",
	})
	if err != nil || !selection.DryRun || !reflect.DeepEqual(arguments, []string{"exec", "review"}) {
		t.Fatalf("native Antigravity dry-run = %#v / %#v, err=%v", selection, arguments, err)
	}
	selection, arguments, err = parseRunArguments([]string{"--provider", "gemini", "--dry-run"})
	if err != nil || !selection.DryRun || selection.Provider != "gemini" || len(arguments) != 0 {
		t.Fatalf("generic provider dry-run = %#v / %#v, err=%v", selection, arguments, err)
	}
	selection, arguments, err = parseRunArguments([]string{
		"--provider", "gemini", "--cli", "agy", "--", "--dry-run",
	})
	if err != nil || selection.DryRun || !reflect.DeepEqual(arguments, []string{"--dry-run"}) {
		t.Fatalf("Codex passthrough dry-run = %#v / %#v, err=%v", selection, arguments, err)
	}
	secret := "antigravity-api-key-sentinel"
	_, _, err = parseRunArguments([]string{"--provider", "gemini", "--cli", "agy", "--api-key", secret})
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("native Antigravity API-key error = %v", err)
	}
}

func TestProdex04356RunDryRunAcceptedWithoutNativeCLI(t *testing.T) {
	selection, arguments, err := parseRunArguments([]string{
		"--dry-run", "--", "--model", "gpt-test", "exec", "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !selection.DryRun || !reflect.DeepEqual(arguments, []string{"--model", "gpt-test", "exec", "hello"}) {
		t.Fatalf("generic dry-run = %#v / %#v", selection, arguments)
	}
}

func TestParseRunArgumentsSupportsLocalProviderShortcut(t *testing.T) {
	selection, arguments, err := parseRunArguments([]string{
		"--url=http://127.0.0.1:8131",
		"--local-model", "qwen3-coder",
		"--local-context-window=8192",
		"--local-auto-compact-token-limit", "7000",
		"exec", "review",
	})
	if err != nil {
		t.Fatal(err)
	}
	if selection.URL != "http://127.0.0.1:8131" ||
		selection.Model != "qwen3-coder" ||
		selection.ContextWindow == nil || *selection.ContextWindow != 8192 ||
		selection.AutoCompactTokenLimit == nil || *selection.AutoCompactTokenLimit != 7000 ||
		selection.Provider != "" {
		t.Fatalf("selection = %#v", selection)
	}
	if len(arguments) != 2 || arguments[0] != "exec" || arguments[1] != "review" {
		t.Fatalf("arguments = %#v", arguments)
	}
}

func TestParseRunArgumentsRejectsLocalProviderConflicts(t *testing.T) {
	for _, arguments := range [][]string{
		{"--provider", "anthropic", "--url", "http://127.0.0.1:8131"},
		{"--base-url", "https://api.example.test", "--url", "http://127.0.0.1:8131"},
		{"--context-window", "8192"},
		{"--auto-compact-token-limit", "7000"},
	} {
		if _, _, err := parseRunArguments(arguments); err == nil {
			t.Fatalf("arguments %#v unexpectedly accepted", arguments)
		}
	}
}

func TestParseRunArgumentsValidatesLocalProviderURLLikeProdex(t *testing.T) {
	for _, value := range []string{
		"ftp://example.test",
		"https:///v1",
		"https://user:pass@example.test/v1",
		"https://example.test/v1?key=value",
		"https://example.test/v1#frag",
		" https://example.test/v1",
	} {
		if _, _, err := parseRunArguments([]string{"--url=" + value}); err == nil ||
			!strings.Contains(err.Error(), "invalid --url") {
			t.Fatalf("URL %q error = %v", value, err)
		}
	}
}

func TestParseRunArgumentsPreservesAPIKeyWhitespaceForValidation(t *testing.T) {
	selection, _, err := parseRunArguments([]string{
		"--provider=anthropic", "--api-key= padded-secret ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if selection.APIKey != " padded-secret " {
		t.Fatalf("API key was normalized before validation: %q", selection.APIKey)
	}
	if _, _, err := parseRunArguments([]string{"--provider=anthropic", "--api-key="}); err == nil ||
		!strings.Contains(err.Error(), "cannot be empty") {
		t.Fatalf("empty explicit API key error = %v", err)
	}
}

func TestParseRunArgumentsValidatesProviderBaseURLLikeProdex(t *testing.T) {
	for _, value := range []string{
		"ftp://example.test/v1",
		"https:///v1",
		"https://user:pass@example.test/v1",
		"https://example.test/v1?key=value",
		"https://example.test/v1?",
		"https://example.test/v1#frag",
		" https://example.test/v1",
	} {
		if _, _, err := parseRunArguments([]string{"--provider=anthropic", "--base-url=" + value}); err == nil ||
			!strings.Contains(err.Error(), "invalid --base-url") {
			t.Fatalf("base URL %q error = %v", value, err)
		}
	}
	for _, value := range []string{
		"https://example.test",
		"http://127.0.0.1:8080/v1",
		"https://example.test/custom/path/",
	} {
		selection, _, err := parseRunArguments([]string{"--provider", "anthropic", "--base-url", value})
		if err != nil || selection.BaseURL != value {
			t.Fatalf("valid base URL %q = %#v, err=%v", value, selection, err)
		}
	}
}

func TestParseRunArgumentsSupportsAutoRedeem(t *testing.T) {
	selection, arguments, err := parseRunArguments([]string{"--auto-redeem", "exec", "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if !selection.AutoRedeem || len(arguments) != 2 || arguments[0] != "exec" || arguments[1] != "hello" {
		t.Fatalf("selection/arguments = %#v / %#v", selection, arguments)
	}
	if selection.Empty() {
		t.Fatal("auto-redeem selection unexpectedly reported empty")
	}
}

func TestParseRunArgumentsKeepsAutoRedeemLiteralAfterSeparator(t *testing.T) {
	selection, arguments, err := parseRunArguments([]string{"--", "--auto-redeem"})
	if err != nil {
		t.Fatal(err)
	}
	if selection.AutoRedeem || len(arguments) != 1 || arguments[0] != "--auto-redeem" {
		t.Fatalf("selection/arguments = %#v / %#v", selection, arguments)
	}
}
