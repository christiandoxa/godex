package codex

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
	"github.com/pelletier/go-toml/v2"
)

const (
	sessionIdentityNotifyCommand = "__runtime-goal-session-notify"
	sessionIdentityMarkerLimit   = 64
)

// prepareSessionIdentity allocates a private marker for one interactive Super
// launch. The hidden Godex callback owns the marker contents; this package only
// supplies the launch-local hook and reads the result after the child exits.
func prepareSessionIdentity(codexHome string, arguments []string) (string, func(), []string) {
	if !sessionAppServerEligible(arguments) {
		return "", func() {}, arguments
	}
	directory, err := os.MkdirTemp("", "godex-session-hook-")
	if err != nil {
		return "", func() {}, arguments
	}
	cleanup := func() { _ = os.RemoveAll(directory) }
	marker := filepath.Join(directory, "session.id")
	executable, err := os.Executable()
	if err != nil || strings.TrimSpace(executable) == "" {
		cleanup()
		return "", func() {}, arguments
	}
	return marker, cleanup, sessionIdentityArguments(codexHome, arguments, executable, marker)
}

func sessionIdentityArguments(codexHome string, original []string, executable, marker string) []string {
	argv := []string{executable, sessionIdentityNotifyCommand, marker}
	injected := make([]string, 0, 6)
	if !configOverridePresent(original, "hooks.SessionStart") {
		command := sessionIdentityHookCommand(argv)
		literal, err := json.Marshal(command)
		if err == nil {
			hash := sessionIdentityHookHash(command)
			key, _ := json.Marshal(sessionIdentityHookKey())
			hashLiteral, _ := json.Marshal(hash)
			injected = append(injected,
				"-c", fmt.Sprintf(`hooks.SessionStart=[{hooks=[{type="command",command=%s,timeout=5}]}]`, literal),
				"-c", fmt.Sprintf("hooks.state={%s={trusted_hash=%s}}", key, hashLiteral),
			)
		}
	}
	if !configOverridePresent(original, "notify") && !codexNotifyConfigured(codexHome) {
		literal, err := json.Marshal(argv)
		if err == nil {
			injected = append(injected, "-c", "notify="+string(literal))
		}
	}
	return append(injected, original...)
}

func sessionIdentityHookCommand(argv []string) string {
	if runtime.GOOS == "windows" {
		parts := make([]string, 0, len(argv))
		for _, arg := range argv {
			parts = append(parts, `"`+strings.ReplaceAll(arg, `"`, `""`)+`"`)
		}
		return strings.Join(parts, " ")
	}
	parts := make([]string, 0, len(argv))
	for _, arg := range argv {
		parts = append(parts, "'"+strings.ReplaceAll(arg, "'", "'\"'\"'")+"'")
	}
	return strings.Join(parts, " ")
}

func sessionIdentityHookHash(command string) string {
	literal, _ := json.Marshal(command)
	identity := `{"event_name":"session_start","hooks":[{"async":false,"command":` +
		string(literal) + `,"timeout":5,"type":"command"}]}`
	sum := sha256.Sum256([]byte(identity))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func sessionIdentityHookKey() string {
	if runtime.GOOS == "windows" {
		return `C:\<session-flags>\config.toml:session_start:0:0`
	}
	return "/<session-flags>/config.toml:session_start:0:0"
}

func readSessionIdentityMarker(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 36 || info.Size() > sessionIdentityMarkerLimit {
		return ""
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	id := strings.TrimSpace(string(body))
	if !sessionentity.ValidID(id) {
		return ""
	}
	return id
}

func configOverridePresent(arguments []string, wanted string) bool {
	for index := 0; index < len(arguments); index++ {
		assignment := ""
		switch arguments[index] {
		case "-c", "--config":
			if index+1 < len(arguments) {
				index++
				assignment = arguments[index]
			}
		default:
			assignment = strings.TrimPrefix(arguments[index], "--config=")
			assignment = strings.TrimPrefix(assignment, "-c=")
		}
		key, _, ok := strings.Cut(assignment, "=")
		if ok && strings.TrimSpace(key) == wanted {
			return true
		}
	}
	return false
}

func codexNotifyConfigured(codexHome string) bool {
	if strings.TrimSpace(codexHome) == "" {
		return false
	}
	path := filepath.Join(codexHome, "config.toml")
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return false
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var config map[string]any
	if toml.Unmarshal(body, &config) != nil {
		return false
	}
	_, configured := config["notify"]
	return configured
}
