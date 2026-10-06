package runtime

import (
	"bytes"
	"context"
	"strings"
	"testing"

	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

func TestProdex04356RunHelpIsWrapperOwnedAndSideEffectFree(t *testing.T) {
	accounts := &fakeRunnerAccounts{selected: "unchanged"}
	process := &fakeRunnerProcess{}
	runner := runtimeusecase.NewRunner(accounts, process, nil)
	var output bytes.Buffer

	if err := Run(t.Context(), runner, nil, []string{"--help"}, &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, want := range []string{
		"Usage: godex run",
		"--profile",
		"--auto-rotate",
		"--no-auto-rotate",
		"--auto-redeem",
		"--skip-quota-check",
		"--full-access",
		"--base-url",
		"--no-proxy",
		"--dry-run",
		"--web-search",
		"--current-time-reminder",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("run help missing %q: %s", want, text)
		}
	}
	if accounts.selected != "unchanged" || process.home != "" || len(process.arguments) != 0 {
		t.Fatalf("run help caused runtime side effects: selected=%q home=%q args=%#v",
			accounts.selected, process.home, process.arguments)
	}
}

func TestProdex04356SuperHelpSkipsToolResolutionAndRuntime(t *testing.T) {
	var output bytes.Buffer
	lookups := 0
	lookup := func(string) (string, bool) {
		lookups++
		return "", false
	}

	err := superProfilesWithToolLookup(
		context.Background(), nil, nil, nil, &output, []string{"--help"}, lookup,
	)
	if err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, want := range []string{
		"Usage: godex super",
		"--profile",
		"--full-access",
		"--presidio",
		"--no-presidio",
		"--sub-agent",
		"--tool",
		"--require-tool",
		"--url",
		"--provider",
		"--cli",
		"--api-key",
		"--model",
		"--context-window",
		"--auto-compact-token-limit",
		"--web-search",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("super help missing %q: %s", want, text)
		}
	}
	if lookups != 0 {
		t.Fatalf("super help probed optional tools %d time(s)", lookups)
	}
}
