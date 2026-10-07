package codex

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	pingmodel "github.com/christiandoxa/godex/internal/model/ping"
)

func TestPingArgumentsMatchCanonicalProdexPath(t *testing.T) {
	got := pingArguments(pingmodel.Options{Model: "gpt-test", Effort: "max", BaseURL: "https://example.test/backend-api", NoProxy: true})
	want := []string{
		"exec", "--sandbox", "read-only", "--ephemeral", "--ignore-user-config",
		"--ignore-rules", "--skip-git-repo-check", "-c", `model_provider="openai"`,
		"-c", `chatgpt_base_url="https://example.test/backend-api"`,
		"--model", "gpt-test", "-c", "model_reasoning_effort=max", "--json", "--color", "never", "hello",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("arguments = %#v\nwant %#v", got, want)
	}
}

func TestPingEnvironmentRemovesProviderSecretsAndOptionalProxy(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "synthetic")
	t.Setenv("HTTPS_PROXY", "http://proxy.invalid")
	t.Setenv("LD_PRELOAD", "/synthetic/preload")
	home := t.TempDir()
	environment := pingEnvironment(home, true)
	joined := strings.Join(environment, "\n")
	for _, key := range []string{"OPENAI_API_KEY=", "HTTPS_PROXY=", "LD_PRELOAD="} {
		if strings.Contains(joined, key) {
			t.Fatalf("removed environment %q is present", key)
		}
	}
	if !strings.Contains(joined, "CODEX_HOME="+home) {
		t.Fatalf("CODEX_HOME missing from %q", joined)
	}

	environment = pingEnvironment(home, false)
	joined = strings.Join(environment, "\n")
	if !strings.Contains(joined, "HTTPS_PROXY=http://proxy.invalid") {
		t.Fatalf("proxy unexpectedly removed: %q", joined)
	}
	if strings.Contains(joined, "OPENAI_API_KEY=synthetic") {
		t.Fatalf("provider secret leaked into child environment")
	}
}

func TestPingCaptureFindsFirstAgentMessageAcrossChunks(t *testing.T) {
	capture := newPingCapture(time.Now().Add(-10*time.Millisecond), true)
	_, _ = capture.Write([]byte(`{"type":"item.completed","item":{"type":"agent_`))
	_, _ = capture.Write([]byte("message\",\"text\":\"Hello\"}}\n"))
	if capture.FirstResponseMS() == nil {
		t.Fatal("first agent response was not detected")
	}
	if len(capture.Bytes()) == 0 {
		t.Fatal("capture lost stdout")
	}
}

func TestPingEnvironmentHasNoDuplicateCodexHome(t *testing.T) {
	t.Setenv("CODEX_HOME", "/old/home")
	home := t.TempDir()
	count := 0
	for _, entry := range pingEnvironment(home, false) {
		if strings.HasPrefix(entry, "CODEX_HOME=") {
			count++
			if entry != "CODEX_HOME="+home {
				t.Fatalf("CODEX_HOME = %q", entry)
			}
		}
	}
	if count != 1 {
		t.Fatalf("CODEX_HOME count = %d", count)
	}
	_ = os.Getenv("PATH")
}
