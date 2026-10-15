package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const fakeSessionID = "01900000-0000-7000-8000-000000000712"

// runFakeCodex is the credential-free child used by both release binaries. It
// implements only the capability probes, app-server readiness, and hook call
// needed to exercise the launch-owned footer contract.
func runFakeCodex() int {
	arguments := os.Args[1:]
	if len(arguments) == 1 && arguments[0] == "--version" {
		fmt.Println("codex-cli 0.162.0")
		return 0
	}
	if appServerHelp(arguments) {
		fmt.Println("Codex app-server\n  --listen <stdio|unix://PATH>")
		return 0
	}
	if contains(arguments, "exec-server") || contains(arguments, "--stdio") {
		return runFakeStdio()
	}
	if appServerIndex(arguments) >= 0 {
		if path := listenPath(arguments); path != "" {
			return runFakeUnix(path)
		}
		return 2
	}
	if os.Getenv("SESSION_FOOTER_NATIVE_NORMAL_SCREEN") == "1" {
		// Codex has already left the alternate screen. A second 1049 reset
		// by the parent would restore an obsolete saved cursor on VTE/xterm.
		fmt.Fprint(os.Stdout, "\x1b[?1049h\x1b[?1049l")
	}
	fmt.Fprint(os.Stdout, "Native usage summary\r\nNative session summary\r\n")
	if err := invokeSessionHook(arguments); err != nil && os.Getenv("SESSION_FOOTER_DEBUG") == "1" {
		fmt.Fprintf(os.Stderr, "fake hook: %v\r\n", err)
	}
	fmt.Fprint(os.Stderr, "native stderr\r\n")
	if value, err := strconv.Atoi(os.Getenv("SESSION_FOOTER_EXIT")); err == nil && value > 0 && value < 256 {
		return value
	}
	return 0
}

func runFakeStdio() int {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request map[string]any
		if json.Unmarshal(scanner.Bytes(), &request) != nil {
			continue
		}
		id, ok := request["id"]
		if !ok {
			continue
		}
		response := map[string]any{
			"id":      id,
			"jsonrpc": "2.0",
			"result":  map[string]any{"data": []any{}, "nextCursor": nil},
		}
		encoded, _ := json.Marshal(response)
		_, _ = fmt.Fprintln(os.Stdout, string(encoded))
	}
	return 0
}

func runFakeUnix(path string) int {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return 2
	}
	_ = os.Remove(path)
	listener, err := net.Listen("unix", path)
	if err != nil {
		return 2
	}
	defer listener.Close()
	for {
		connection, err := listener.Accept()
		if err != nil {
			return 0
		}
		_ = connection.Close()
	}
}

func invokeSessionHook(arguments []string) error {
	if os.Getenv("SESSION_FOOTER_EMIT_ID") == "0" {
		return nil
	}
	id := strings.TrimSpace(os.Getenv("SESSION_FOOTER_ID"))
	if id == "" {
		id = fakeSessionID
	}
	config, _ := os.ReadFile(filepath.Join(os.Getenv("CODEX_HOME"), "config.toml"))
	source := strings.Join(arguments, "\n") + "\n" + string(config)
	start := strings.Index(source, "hooks.SessionStart")
	if start < 0 {
		debugFake("SessionStart missing")
		return nil
	}
	command := source[start:]
	value, ok := hookCommandValue(command)
	if !ok {
		debugFake("command value missing in %q", command)
		return nil
	}
	decoded, err := strconv.Unquote(value)
	if err != nil {
		return err
	}
	argv := splitShellCommand(decoded)
	debugFake("hook argv=%#v", argv)
	if len(argv) < 3 || argv[1] != "__runtime-goal-session-notify" {
		return nil
	}
	payload, _ := json.Marshal(map[string]string{"session_id": id})
	callback := exec.Command(argv[0], argv[1:]...)
	callback.Stdin = strings.NewReader(string(payload))
	return callback.Run()
}

func hookCommandValue(source string) (string, bool) {
	patterns := []string{",command=", "command =", "command="}
	best := -1
	patternLength := 0
	for _, pattern := range patterns {
		index := strings.Index(source, pattern)
		if index >= 0 && (best < 0 || index < best) {
			best, patternLength = index, len(pattern)
		}
	}
	if best < 0 {
		return "", false
	}
	value := strings.TrimSpace(source[best+patternLength:])
	if !strings.HasPrefix(value, "\"") {
		return "", false
	}
	end := quotedStringEnd(value)
	if end < 0 {
		return "", false
	}
	return value[:end], true
}

func quotedStringEnd(value string) int {
	for index := 1; index < len(value); index++ {
		if value[index] == '\\' {
			index++
			continue
		}
		if value[index] == '"' {
			return index + 1
		}
	}
	return -1
}

func splitShellCommand(value string) []string {
	var result []string
	var token strings.Builder
	inSingle := false
	for index := 0; index < len(value); index++ {
		switch value[index] {
		case '\'':
			inSingle = !inSingle
		case ' ', '\t', '\n':
			if !inSingle && token.Len() > 0 {
				result = append(result, token.String())
				token.Reset()
			} else if inSingle {
				token.WriteByte(value[index])
			}
		default:
			token.WriteByte(value[index])
		}
	}
	if token.Len() > 0 {
		result = append(result, token.String())
	}
	return result
}

func listenPath(arguments []string) string {
	for index, argument := range arguments {
		if argument != "--listen" || index+1 >= len(arguments) {
			continue
		}
		value := arguments[index+1]
		return strings.TrimPrefix(value, "unix://")
	}
	return ""
}

func appServerIndex(arguments []string) int {
	for index, argument := range arguments {
		if argument == "app-server" {
			return index
		}
	}
	return -1
}

func appServerHelp(arguments []string) bool {
	index := appServerIndex(arguments)
	return index >= 0 && index+1 < len(arguments) && arguments[index+1] == "--help"
}

func contains(arguments []string, wanted string) bool {
	for _, argument := range arguments {
		if argument == wanted {
			return true
		}
	}
	return false
}

func debugFake(format string, arguments ...any) {
	if os.Getenv("SESSION_FOOTER_DEBUG") == "1" {
		fmt.Fprintf(os.Stderr, "fake: "+format+"\r\n", arguments...)
	}
}
