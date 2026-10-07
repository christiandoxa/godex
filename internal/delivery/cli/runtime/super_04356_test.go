package runtime

import (
	"bytes"
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestProdex04356SuperDefaultsMatchTaggedBehavior(t *testing.T) {
	options, err := parseSuperArguments([]string{"--dry-run"})
	if err != nil {
		t.Fatal(err)
	}
	if !options.fullAccess || !options.smartContext || !options.superMode {
		t.Fatalf("super defaults = full:%t smart:%t mode:%t", options.fullAccess, options.smartContext, options.superMode)
	}
	if options.noAutoRotate || options.skipQuota || options.presidio || options.subAgent.enabled {
		t.Fatalf("unexpected super defaults = %#v", options)
	}
	wantPrefix := []string{"-c", "features.apps=false"}
	if len(options.codexArgs) < len(wantPrefix) || !reflect.DeepEqual(options.codexArgs[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("parsed super codex args = %#v, want prefix %#v", options.codexArgs, wantPrefix)
	}
	prepared := superPreparedCodexArgs(options)
	if len(prepared) == 0 || prepared[0] != "--dangerously-bypass-approvals-and-sandbox" {
		t.Fatalf("prepared super codex args = %#v", prepared)
	}
	if !reflect.DeepEqual(superDefaultTools, []string{
		"caveman", "rtk", "codebase-memory-mcp", "playwright-mcp", "ponytail",
	}) {
		t.Fatalf("super default tools = %#v", superDefaultTools)
	}
}

func TestProdex04356SuperScansOverridesAfterSessionUntilLiteralBoundary(t *testing.T) {
	const session = "00000000-0000-7000-8000-000000000042"
	options, err := parseSuperArguments([]string{
		session,
		"--profile=first",
		"--profile", "second",
		"--provider=gemini",
		"--provider", "deepseek",
		"--model=first",
		"--local-model", "last",
		"--no-auto-rotate",
		"--auto-rotate",
		"--rollout-budget-tokens=64",
		"--rollout-budget-reminders=1,2",
		"--rollout-budget-reminders", "2,1",
		"--dry-run",
		"--",
		"--provider=gemini",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.profile != "first" || options.provider != "deepseek" || options.model != "last" {
		t.Fatalf("scanned options = profile:%q provider:%q model:%q", options.profile, options.provider, options.model)
	}
	if !options.autoRotate || options.noAutoRotate || !options.dryRun || !options.skipQuota {
		t.Fatalf("scanned booleans = auto:%t no-auto:%t dry:%t quota:%t", options.autoRotate, options.noAutoRotate, options.dryRun, options.skipQuota)
	}
	if !reflect.DeepEqual(options.features.rolloutReminders, []uint64{1, 2, 2, 1}) {
		t.Fatalf("rollout reminders = %#v", options.features.rolloutReminders)
	}
	if !slices.Contains(options.codexArgs, session) {
		t.Fatalf("session disappeared from codex args: %#v", options.codexArgs)
	}
	boundary := slices.Index(options.codexArgs, "--")
	if boundary < 0 || boundary+1 >= len(options.codexArgs) || options.codexArgs[boundary+1] != "--provider=gemini" {
		t.Fatalf("literal boundary tail = %#v", options.codexArgs)
	}
}

func TestProdex04356SuperDirectAutoRotateConflictButTailUsesLastValue(t *testing.T) {
	if _, err := parseSuperArguments([]string{"--auto-rotate", "--no-auto-rotate", "--dry-run"}); err == nil ||
		!strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("direct auto-rotate conflict = %v", err)
	}
	options, err := parseSuperArguments([]string{
		"00000000-0000-7000-8000-000000000042",
		"--no-auto-rotate",
		"--auto-rotate",
		"--dry-run",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !options.autoRotate || options.noAutoRotate {
		t.Fatalf("tail auto-rotate precedence = auto:%t no-auto:%t", options.autoRotate, options.noAutoRotate)
	}
}

func TestProdex04356SuperProviderAliasesAndProviderModeSkipQuota(t *testing.T) {
	for _, provider := range []string{"gemini", "deepseek"} {
		options, err := parseSuperArguments([]string{provider, "--dry-run"})
		if err != nil {
			t.Fatal(err)
		}
		if options.provider != provider || !options.skipQuota {
			t.Fatalf("%s alias = provider:%q skip:%t", provider, options.provider, options.skipQuota)
		}
		for index := 0; index+1 < len(options.codexArgs); index++ {
			if options.codexArgs[index] == "-c" && options.codexArgs[index+1] == "features.apps=false" {
				t.Fatalf("provider mode unexpectedly disabled Codex apps: %#v", options.codexArgs)
			}
		}
	}
}

func TestProdex04356SuperValidationMatchesTaggedConflicts(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"presidio", []string{"--presidio", "--no-presidio", "--dry-run"}, "conflicts"},
		{"required presidio", []string{"--no-presidio", "--require-tool", "presidio", "--dry-run"}, "conflicts"},
		{"provider url", []string{"--provider", "gemini", "--url", "http://127.0.0.1:1/v1", "--dry-run"}, "conflicts"},
		{"base url", []string{"--base-url", "https://example.test", "--url", "http://127.0.0.1:1/v1", "--dry-run"}, "conflicts"},
		{"api key", []string{"--api-key", "synthetic", "--dry-run"}, "requires --provider"},
		{"context", []string{"--context-window", "1000", "--dry-run"}, "require --provider or --url"},
		{"sub-agent conflict", []string{"--sub-agent", "--no-sub-agent", "--dry-run"}, "conflicts"},
		{"sub-agent detail", []string{"--sub-agent-provider", "openai", "--dry-run"}, "require explicit --sub-agent"},
		{"local agent url", []string{"--sub-agent", "--sub-agent-provider", "local", "--dry-run"}, "requires --sub-agent-url"},
		{"nonlocal agent url", []string{"--sub-agent", "--sub-agent-url", "http://127.0.0.1:1/v1", "--dry-run"}, "requires --sub-agent-provider local"},
		{"desktop agent", []string{"--sub-agent", "gui", "--dry-run"}, "unsupported with the Codex Desktop"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := parseSuperArguments(testCase.args)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error = %v, want %q", err, testCase.want)
			}
		})
	}
}

func TestProdex04356SuperToolAliasesAndRequiredFailure(t *testing.T) {
	options, err := parseSuperArguments([]string{
		"--tool", "cbm",
		"--tool=playwright",
		"--require-tool", "rtk",
		"--presidio",
		"--dry-run",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(options.tools, "codebase-memory-mcp") ||
		!slices.Contains(options.tools, "playwright-mcp") ||
		!slices.Contains(options.requiredTools, "rtk") ||
		!superPresidioEnabled(options) {
		t.Fatalf("tool selection = tools:%#v required:%#v presidio:%t", options.tools, options.requiredTools, superPresidioEnabled(options))
	}
	lookup := func(tool string) (string, bool) {
		if tool == "rtk" {
			return "", false
		}
		return "/tools/" + tool, true
	}
	if _, err := resolveSuperTools(options, lookup); err == nil || !strings.Contains(err.Error(), "required optional tool rtk is unavailable") {
		t.Fatalf("required tool error = %v", err)
	}
}

func TestProdex04356SuperDryRunIsSideEffectFreeAndRedactsAPIKey(t *testing.T) {
	var out bytes.Buffer
	lookup := func(tool string) (string, bool) { return "/tools/" + tool, tool != "ponytail" }
	err := superWithToolLookup(context.Background(), &out, []string{
		"--provider", "gemini",
		"--api-key", "super-secret-sentinel",
		"--model", "gemini-model",
		"--tool", "presidio",
		"--sub-agent",
		"--sub-agent-provider", "openai",
		"--sub-agent-model", "child-model",
		"--sub-agent-max-concurrency", "8",
		"--dry-run",
		"resume", "00000000-0000-7000-8000-000000000042",
	}, lookup)
	if err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{
		"Godex Super dry run",
		"Provider: gemini",
		"Quota preflight: skipped",
		"Full access: enabled",
		"Smart Context: enabled",
		"Presidio redaction: enabled",
		"Provider API key: configured (<redacted>)",
		"ponytail: skipped (not found)",
		"Sub-agent: enabled",
		"Sub-agent provider: OpenAI",
		"Sub-agent model: child-model",
		"Maximum active sub-agents: 8 (explicit preset)",
		"Sub-agent concurrency hard maximum: 64",
		"Sub-agent concurrency enforcement: cross-process exclusive slot leases",
		"Sub-agent recursion marker: GODEX_SUB_AGENT=1",
		"--dangerously-bypass-approvals-and-sandbox",
		"Dry run: overlays and services are not started.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "super-secret-sentinel") {
		t.Fatalf("dry-run leaked API key:\n%s", text)
	}
}

func TestProdex04356StandaloneSuperHelperRequiresRuntimeDependencies(t *testing.T) {
	err := superWithToolLookup(context.Background(), &bytes.Buffer{}, nil,
		func(tool string) (string, bool) { return "/tools/" + tool, true })
	if err == nil || !strings.Contains(err.Error(), "runtime dependencies are required") {
		t.Fatalf("standalone non-dry-run Super = %v", err)
	}
}
