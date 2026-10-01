package runtime

import (
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

func TestParseRunArgumentsPassesCodexFlagsWithoutSeparator(t *testing.T) {
	selector, arguments, err := parseRunArguments([]string{"--model", "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	if !selector.Empty() || len(arguments) != 2 {
		t.Fatalf("selection=%+v arguments=%#v", selector, arguments)
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

func TestParseRunArgumentsLeavesURLToCodex(t *testing.T) {
	selection, arguments, err := parseRunArguments([]string{
		"--provider", "anthropic", "--url=https://local.example.test/v1", "exec",
	})
	if err != nil {
		t.Fatal(err)
	}
	if selection.Provider != "anthropic" || selection.BaseURL != "" ||
		len(arguments) != 2 || arguments[0] != "--url=https://local.example.test/v1" {
		t.Fatalf("selection/arguments = %#v / %#v", selection, arguments)
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
