package superexpose

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	queueCommandTimeout     = 15 * time.Second
	queueCommandOutputLimit = 64 * 1024
)

type systemSessionQueueControl struct{}

func (systemSessionQueueControl) checkCapability(target resolvedSessionTarget) error {
	if strings.HasPrefix(target.remoteEndpoint, "unix://") {
		return nil
	}
	output, err := runCodexQueueCommand(target, []string{"queue", "--help"})
	if err != nil {
		return sessionQueueUnsupported
	}
	text := string(output)
	if strings.Contains(text, "--thread") && strings.Contains(text, "--message") {
		return nil
	}
	return sessionQueueUnsupported
}

func (systemSessionQueueControl) persistedThread(stateDB, threadID string) (bool, error) {
	database, err := openSessionReadOnlyDatabase(stateDB)
	if err != nil {
		return false, sessionVerificationInconclusive
	}
	defer database.Close()
	var found string
	err = database.QueryRow(
		"SELECT id FROM threads WHERE id = ?1 OR id = ?2 LIMIT 1",
		threadID, "thread_"+threadID,
	).Scan(&found)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	default:
		return false, sessionNotQueueAddressable
	}
}

func (systemSessionQueueControl) rolloutPath(stateDB, threadID string) (string, error) {
	database, err := openSessionReadOnlyDatabase(stateDB)
	if err != nil {
		return "", sessionOutputSourceUnavailable
	}
	defer database.Close()
	var path sql.NullString
	err = database.QueryRow(
		"SELECT rollout_path FROM threads WHERE id = ?1 OR id = ?2 LIMIT 1",
		threadID, "thread_"+threadID,
	).Scan(&path)
	switch {
	case err == nil && path.Valid:
		return path.String, nil
	case err == nil:
		return "", nil
	case errors.Is(err, sql.ErrNoRows):
		return "", nil
	default:
		return "", sessionOutputSourceUnavailable
	}
}

func (systemSessionQueueControl) queueOnce(target resolvedSessionTarget, message string) queueInvocation {
	if strings.HasPrefix(target.remoteEndpoint, "unix://") {
		return appServerQueueAddOnce(target, message)
	}
	output, err := runCodexQueueCommand(target, []string{
		"queue", "--thread", target.threadID, "--message", message,
	})
	if err != nil {
		return queueInvocation{outcome: queueAmbiguous}
	}
	messageID := parseQueuedMessageID(output)
	if messageID == "" {
		return queueInvocation{outcome: queueAmbiguous}
	}
	code := 0
	return queueInvocation{
		outcome: queueAccepted, exitCode: &code, messageID: messageID, queued: true,
	}
}

func (systemSessionQueueControl) preempt(target resolvedSessionTarget) (queuePreemptResult, error) {
	if !strings.HasPrefix(target.remoteEndpoint, "unix://") {
		return queuePreemptResult{}, sessionQueueUnsupported
	}
	return appServerPreempt(target)
}

func (systemSessionQueueControl) loadedThreadAddressable(target resolvedSessionTarget) (bool, error) {
	if !strings.HasPrefix(target.remoteEndpoint, "unix://") {
		return false, nil
	}
	activity, ok, err := appServerThreadActivity(target, false)
	return ok && activity != nil, err
}

func openSessionReadOnlyDatabase(path string) (*sql.DB, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("database path is empty")
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("database path is unavailable")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	uri := url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}
	query := uri.Query()
	query.Set("mode", "ro")
	query.Add("_pragma", "query_only(1)")
	query.Add("_pragma", "busy_timeout(2000)")
	uri.RawQuery = query.Encode()
	database, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return nil, err
	}
	return database, nil
}

type boundedQueueOutput struct {
	buffer    bytes.Buffer
	truncated bool
}

func (output *boundedQueueOutput) Write(content []byte) (int, error) {
	if output.buffer.Len() < queueCommandOutputLimit {
		remaining := queueCommandOutputLimit - output.buffer.Len()
		keep := min(remaining, len(content))
		_, _ = output.buffer.Write(content[:keep])
		if keep < len(content) {
			output.truncated = true
		}
	} else if len(content) > 0 {
		output.truncated = true
	}
	return len(content), nil
}

func runCodexQueueCommand(target resolvedSessionTarget, arguments []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queueCommandTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, target.writer.executable, arguments...)
	command.Dir = target.environment.pwd
	command.Env = []string{
		"HOME=" + target.environment.home,
		"CODEX_HOME=" + target.environment.codexHome,
		"CODEX_SQLITE_HOME=" + target.environment.codexSQLiteHome,
		"PWD=" + target.environment.pwd,
	}
	var output boundedQueueOutput
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err != nil {
		return nil, err
	}
	if ctx.Err() != nil || output.truncated {
		return nil, errors.New("Codex queue output was incomplete")
	}
	return append([]byte(nil), output.buffer.Bytes()...), nil
}

func parseQueuedMessageID(output []byte) string {
	text := string(output)
	for _, line := range strings.Split(text, "\n") {
		suffix, ok := strings.CutPrefix(strings.TrimSpace(line), "Queued message ")
		if !ok {
			continue
		}
		candidate := strings.Fields(suffix)
		if len(candidate) == 0 {
			continue
		}
		value := strings.Trim(candidate[0], ".,:;()[]{}")
		if canonicalUUID(value) {
			return strings.ToLower(value)
		}
	}
	return ""
}

func queueInvocationAccepted(messageID, submissionID string, queued bool) queueInvocation {
	code := 0
	return queueInvocation{
		outcome: queueAccepted, exitCode: &code,
		messageID: messageID, submissionID: submissionID, queued: queued,
	}
}

func queueCommandError(detail string) error {
	if detail == "" {
		return sessionQueueFailed
	}
	return fmt.Errorf("%w: %s", sessionQueueFailed, detail)
}
