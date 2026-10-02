package kiro

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestACPRequestShapesMatchProdex(t *testing.T) {
	initialize := acpInitializeRequest(1)
	if initialize["method"] != "initialize" || initialize["jsonrpc"] != "2.0" || initialize["id"] != uint64(1) {
		t.Fatalf("initialize = %#v", initialize)
	}
	params := initialize["params"].(map[string]any)
	client := params["clientInfo"].(map[string]any)
	if params["protocolVersion"] != 1 || client["name"] != "prodex" || client["title"] != "Prodex" || client["version"] != "0.435.1" {
		t.Fatalf("initialize params = %#v", params)
	}
	newSession := acpSessionNewRequest(2, "/tmp/prodex")
	newParams := newSession["params"].(map[string]any)
	if newSession["method"] != "session/new" || newParams["cwd"] != "/tmp/prodex" || len(newParams["mcpServers"].([]any)) != 0 {
		t.Fatalf("session/new = %#v", newSession)
	}
	prompt := acpSessionPromptRequest(3, "sess_1", "hello")
	promptParams := prompt["params"].(map[string]any)
	items := promptParams["prompt"].([]any)
	if prompt["method"] != "session/prompt" || promptParams["sessionId"] != "sess_1" || items[0].(map[string]any)["text"] != "hello" {
		t.Fatalf("session/prompt = %#v", prompt)
	}
}

func TestRunACPTurnSpeaksJSONRPCAndUsesRuntimeDataDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX ACP fixture")
	}
	home := writeKiroRuntimeHome(t)
	script := filepath.Join(t.TempDir(), "kiro-fixture.sh")
	content := `#!/bin/sh
set -eu
[ "$1" = "acp" ]
[ "$2" = "--model" ]
[ "$3" = "gpt-5.6-luna" ]
[ "$4" = "--effort" ]
[ "$5" = "high" ]
[ -n "${KIRO_DATA_DIR:-}" ]
[ -n "${Q_CLI_DATA_DIR:-}" ]
[ -f "${KIRO_TEST_DB_PATH:-missing}" ]
IFS= read -r init
IFS= read -r new_session
echo '{"jsonrpc":"2.0","id":0,"result":{"protocolVersion":1}}'
echo '{"jsonrpc":"2.0","id":9,"method":"session/request_permission","params":{"options":[{"optionId":"once","kind":"allow_once"},{"optionId":"always","kind":"allow_always"}]}}'
IFS= read -r permission
echo "$permission" | grep '"optionId":"once"' >/dev/null
echo '{"jsonrpc":"2.0","id":1,"result":{"sessionId":"sess_fixture","models":{"currentModelId":"gpt-5.6-luna","availableModels":[{"modelId":"gpt-5.6-luna","name":"GPT-5.6 Luna"}]}}}'
IFS= read -r prompt
echo "$prompt" | grep '"method":"session/prompt"' >/dev/null
echo '{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"sess_fixture","update":{"sessionUpdate":"agent_thought_chunk","content":{"type":"text","text":"reasoning"}}}}'
echo '{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"sess_fixture","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"fixture answer"}}}}'
echo '{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"sess_fixture","update":{"sessionUpdate":"usage_update","used":12,"size":20,"cost":{"amount":0.25,"currency":"USD"}}}}'
echo '{"jsonrpc":"2.0","id":2,"result":{"stopReason":"end_turn"}}'
`
	if err := os.WriteFile(script, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRODEX_SUB_AGENT", "1")
	source := NewSource()
	source.getenv = func(key string) string {
		if key == "PRODEX_KIRO_BIN" {
			return script
		}
		return os.Getenv(key)
	}
	turn, err := source.runACPTurn(context.Background(), home, "gpt-5.6-luna", "high", "hello fixture")
	if err != nil {
		t.Fatal(err)
	}
	if turn.Session.SessionID != "sess_fixture" || turn.Session.Models == nil || turn.Session.Models.CurrentModelID != "gpt-5.6-luna" {
		t.Fatalf("turn session = %#v", turn.Session)
	}
	response := kiroResponseFromTurn(turn, 7, "gpt-5.6-luna", "kiro-work")
	if kiroResponseText(response) != "fixture answer" || kiroResponseReasoning(response) != "reasoning" {
		t.Fatalf("turn response = %#v", response)
	}
	metadata := response["metadata"].(map[string]any)["kiro"].(map[string]any)
	usage := metadata["usage_update"].(map[string]any)
	if usage["used"] != uint64(12) || usage["size"] != uint64(20) || usage["remaining"] != uint64(8) {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestACPPermissionOptionPrefersAllowOnce(t *testing.T) {
	params := json.RawMessage(`{"options":[{"optionId":"always","kind":"allow_always"},{"optionId":"once","kind":"allow_once"}]}`)
	if got := acpPermissionOption(params); got != "once" {
		t.Fatalf("permission option = %q", got)
	}
	if got := acpPermissionOption(json.RawMessage(`{"options":[]}`)); strings.TrimSpace(got) != "" {
		t.Fatalf("empty permission option = %q", got)
	}
}
