package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

const sessionStartNotifyCommand04360 = "__runtime-goal-session-notify"
const sessionStartHookPayloadLimit04360 int64 = 64 << 10
const sessionStartHookTimeout04360 = 5

type sessionStartMarker04360 struct {
	directory string
	path      string
}

func newSessionStartMarker04360() (*sessionStartMarker04360, error) {
	dir, err := os.MkdirTemp("", "godex-session-hook-")
	if err != nil {
		return nil, err
	}
	marker := &sessionStartMarker04360{directory: dir, path: filepath.Join(dir, "session.id")}
	return marker, nil
}
func (marker *sessionStartMarker04360) Close() error {
	if marker == nil || marker.directory == "" {
		return nil
	}
	return os.RemoveAll(marker.directory)
}

func (marker *sessionStartMarker04360) ID() string {
	if marker == nil {
		return ""
	}
	info, err := os.Lstat(marker.path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 64 || info.Size() < 36 {
		return ""
	}
	payload, err := os.ReadFile(marker.path)
	if err != nil {
		return ""
	}
	id := strings.TrimSpace(string(payload))
	if !sessionentity.ValidID(id) {
		return ""
	}
	return id
}

func sessionStartHookCommand04360(args []string, goos string) string {
	pieces := make([]string, 0, len(args))
	for _, argument := range args {
		if goos == "windows" {
			pieces = append(pieces, `"`+strings.ReplaceAll(argument, `"`, `""`)+`"`)
		} else {
			pieces = append(pieces, "'"+strings.ReplaceAll(argument, "'", "'\"'\"'")+"'")
		}
	}
	return strings.Join(pieces, " ")
}

func sessionStartHookHash04360(command string) string {
	literal, _ := json.Marshal(command)
	identity := `{"event_name":"session_start","hooks":[{"async":false,"command":` +
		string(literal) + `,"timeout":5,"type":"command"}]}`
	sum := sha256.Sum256([]byte(identity))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func sessionStartHookKey04360(goos string) string {
	if goos == "windows" {
		return `C:\<session-flags>\config.toml:session_start:0:0`
	}
	return "/<session-flags>/config.toml:session_start:0:0"
}

func (marker *sessionStartMarker04360) codexHookArgumentsForOS04360(
	original []string, exe, goos string,
) []string {
	if marker == nil || strings.TrimSpace(exe) == "" {
		return original
	}
	// Leave user-configured hooks untouched. This command alone has no
	// permission to replace a user's existing SessionStart handlers.
	if customSessionStartOverride04360(original) {
		return original
	}
	cmd := sessionStartHookCommand04360([]string{exe, sessionStartNotifyCommand04360, marker.path}, goos)
	literal, _ := json.Marshal(cmd)
	key := strconv.Quote(sessionStartHookKey04360(goos))
	hash := strconv.Quote(sessionStartHookHash04360(cmd))
	injected := []string{
		"-c", fmt.Sprintf(`hooks.SessionStart=[{hooks=[{type="command",command=%s,timeout=%d}]}]`, string(literal), sessionStartHookTimeout04360),
		"-c", fmt.Sprintf("hooks.state={%s={trusted_hash=%s}}", key, hash),
	}
	return append(injected, original...)
}

// Codex -c hooks.SessionStart=[...] is a TOML array, not a string.
// The generic dry-run scalar lookup does not recognize it, so check
// this exact option key before injecting any new handler.
func customSessionStartOverride04360(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		assignment := ""
		switch {
		case arg == "-c" || arg == "--config":
			if i+1 < len(args) {
				i++
				assignment = args[i]
			}
		case strings.HasPrefix(arg, "--config="):
			assignment = strings.TrimPrefix(arg, "--config=")
		case strings.HasPrefix(arg, "-c="):
			assignment = strings.TrimPrefix(arg, "-c=")
		}
		key, _, ok := strings.Cut(assignment, "=")
		if ok && strings.TrimSpace(key) == "hooks.SessionStart" {
			return true
		}
	}
	return false
}

// HandleSessionStartNotify04360 is an intentionally hidden native child
// callback. It only writes a newly created UUID marker in an ephemeral
// user-private directory, never sends network traffic and cannot mutate
// arbitrary files or follow a symlink.
func HandleSessionStartNotify04360(args []string, input io.Reader) (bool, error) {
	if len(args) == 0 || args[0] != sessionStartNotifyCommand04360 {
		return false, nil
	}
	if len(args) < 2 || len(args) > 3 {
		return true, errors.New("session start callback requires marker and optional JSON payload")
	}
	path, err := filepath.Abs(args[1])
	if err != nil {
		return true, err
	}
	parent := filepath.Dir(path)
	if filepath.Base(path) != "session.id" || !strings.HasPrefix(filepath.Base(parent), "godex-session-hook-") {
		return true, errors.New("invalid session start callback marker path")
	}
	info, err := os.Lstat(parent)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return true, errors.New("session callback directory is unavailable")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return true, errors.New("session callback directory is not private")
	}
	var payload []byte
	if len(args) == 3 {
		payload = []byte(args[2])
	} else {
		if input == nil {
			return true, errors.New("missing callback payload")
		}
		payload, err = io.ReadAll(io.LimitReader(input, sessionStartHookPayloadLimit04360+1))
		if err != nil {
			return true, err
		}
	}
	if len(payload) > int(sessionStartHookPayloadLimit04360) {
		return true, errors.New("session callback payload exceeds 64 KiB")
	}
	var value map[string]any
	if err := json.Unmarshal(payload, &value); err != nil {
		return true, err
	}
	id, _ := value["thread-id"].(string)
	session, _ := value["session_id"].(string)
	if id != "" && session != "" && id != session {
		return true, errors.New("session callback contains conflicting UUIDs")
	}
	if id == "" {
		id = session
	}
	if !sessionentity.ValidID(id) {
		return true, errors.New("invalid callback session identity")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return true, err
	}
	if _, err = file.WriteString(id + "\n"); err != nil {
		_ = file.Close()
		return true, err
	}
	return true, file.Close()
}

// If a verified native hook identifies exactly one new UUID from this
// launch, prefer it over ambiguous concurrent session listings. Unknown,
// historical or duplicate IDs never earn continuation priority.
func newSessionAfterMarker04360(before, after []sessionmodel.Report, markerID string) (sessionmodel.Report, bool) {
	if markerID == "" {
		return newSessionAfter04360(before, after)
	}
	if !sessionentity.ValidID(markerID) {
		return sessionmodel.Report{}, false
	}
	for _, report := range before {
		if report.ID == markerID {
			return sessionmodel.Report{}, false
		}
	}
	var matched sessionmodel.Report
	found := false
	for _, report := range after {
		if report.ID != markerID {
			continue
		}
		if found || report.Path == "" ||
			(report.Source != "" && report.Source != "exec") {
			return sessionmodel.Report{}, false
		}
		matched = report
		found = true
	}
	return matched, found
}
