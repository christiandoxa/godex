package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExactCodex160SharedSessionPickerListsAllManagedProfiles(t *testing.T) {
	binary := strings.TrimSpace(os.Getenv("GODEX_TEST_CODEX_BIN"))
	if binary == "" {
		t.Skip("set GODEX_TEST_CODEX_BIN to exact Codex 0.160.1 for native picker integration")
	}
	versionCommand := exec.Command(binary, "--version")
	version, err := versionCommand.Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(version), "0.160.1") {
		t.Fatalf("integration requires Codex 0.160.1, got %q", strings.TrimSpace(string(version)))
	}

	root := t.TempDir()
	shared := filepath.Join(root, "shared")
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	firstID := "01900000-0000-7000-8000-000000000101"
	secondID := "01900000-0000-7000-8000-000000000202"
	writeExactCodex160Rollout(t, first, firstID, "first project")
	writeExactCodex160Rollout(t, second, secondID, "second project")

	process := NewCodexProcess(binary, Terminal{})
	if err := process.PrepareSharedSessionHome(first, shared); err != nil {
		t.Fatal(err)
	}
	if err := process.PrepareSharedSessionHome(second, shared); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(binary, "app-server")
	command.Env = codexThreadIndexEnvironment(first, shared)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	configureCodexAppServerProcess(command)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		terminateCodexAppServerProcess(command)
		_ = stdin.Close()
		_ = command.Wait()
	}()

	reader := bufio.NewReader(stdout)
	if err := writeCodexAppServerMessage(stdin, map[string]any{
		"id": 1, "method": "initialize",
		"params": map[string]any{"clientInfo": map[string]string{"name": "godex-parity-test", "version": "0.435.6"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := readCodexAppServerResponse(reader, 1); err != nil {
		t.Fatal(err)
	}
	if err := writeCodexAppServerMessage(stdin, map[string]any{"method": "initialized"}); err != nil {
		t.Fatal(err)
	}
	if err := writeCodexAppServerMessage(stdin, map[string]any{
		"id": 2, "method": "thread/list",
		"params": map[string]any{
			"archived": false, "limit": 100, "sortKey": "updated_at",
			"sourceKinds": []string{"cli", "vscode"}, "useStateDbOnly": false,
		},
	}); err != nil {
		t.Fatal(err)
	}
	result, err := readCodexAppServerResponse(reader, 2)
	if err != nil {
		t.Fatal(err)
	}
	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(result, &list); err != nil {
		t.Fatalf("decode thread/list: %v body=%s", err, result)
	}
	seen := map[string]bool{}
	for _, thread := range list.Data {
		seen[thread.ID] = true
	}
	if !seen[firstID] || !seen[secondID] {
		var sharedFiles []string
		_ = filepath.WalkDir(shared, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr == nil && !entry.IsDir() {
				relative, _ := filepath.Rel(shared, path)
				sharedFiles = append(sharedFiles, relative)
			}
			return nil
		})
		t.Fatalf("native thread/list did not see both managed profiles: ids=%v result=%s shared=%v stderr=%s", seen, result, sharedFiles, stderr.String())
	}
}

func writeExactCodex160Rollout(t *testing.T, home, id, cwd string) {
	t.Helper()
	directory := filepath.Join(home, "sessions", "2026", "10", "05")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "rollout-2026-10-05T00-00-00-"+id+".jsonl")
	line := map[string]any{
		"timestamp": "2026-10-05T00:00:00Z",
		"type":      "session_meta",
		"payload": map[string]any{
			"session_id":        id,
			"id":                id,
			"timestamp":         "2026-10-05T00:00:00Z",
			"cwd":               cwd,
			"originator":        "codex_cli_rs",
			"cli_version":       "0.160.1",
			"source":            "cli",
			"model_provider":    "openai",
			"base_instructions": nil,
			"dynamic_tools":     nil,
		},
	}
	encoded, err := json.Marshal(line)
	if err != nil {
		t.Fatal(err)
	}
	userLine, err := json.Marshal(map[string]any{
		"timestamp": "2026-10-05T00:00:01Z",
		"type":      "event_msg",
		"payload":   map[string]any{"type": "user_message", "message": "resume " + cwd, "kind": "plain"},
	})
	if err != nil {
		t.Fatal(err)
	}
	content := append(append(append([]byte(nil), encoded...), '\n'), userLine...)
	content = append(content, '\n')
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, time.Unix(1_759_622_400, 0), time.Unix(1_759_622_400, 0)); err != nil {
		t.Fatal(err)
	}
}
