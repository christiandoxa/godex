package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

type quotaRestartProfile04380 struct {
	name    string
	token   string
	account string
}

func runQuotaRestart04380(root, product, binary string) (quotaRestartResult04380, error) {
	if runtime.GOOS == "windows" {
		return quotaRestartResult04380{}, errors.New("fixture requires a POSIX child script")
	}
	directory := filepath.Join(root, "quota-restart", product)
	userHome := filepath.Join(directory, "user")
	configHome := filepath.Join(directory, product)
	currentHome := filepath.Join(userHome, ".codex")
	for _, path := range []string{root, directory, userHome, currentHome} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return quotaRestartResult04380{}, err
		}
		if err := os.Chmod(path, 0o700); err != nil {
			return quotaRestartResult04380{}, err
		}
	}
	fixture := profileFixture{
		name: product, binary: binary, home: userHome, configHome: configHome,
		env: startupEnvironment(userHome, currentHome, configHome),
	}
	profiles := []quotaRestartProfile04380{
		{name: "main", token: "token-main", account: quotaRestartMainAccount04380},
		{name: "second", token: "token-second", account: quotaRestartSecondAccount04380},
		{name: "third", token: "token-third", account: quotaRestartThirdAccount04380},
	}
	for _, profile := range profiles {
		if err := writeQuotaRestartAuth04380(filepath.Join(currentHome, "auth.json"), profile.token, profile.account); err != nil {
			return quotaRestartResult04380{}, err
		}
		code, output, err := invokeProfileCommand(fixture, []string{"profile", "import-current", profile.name})
		if err != nil {
			return quotaRestartResult04380{}, err
		}
		if code != 0 {
			return quotaRestartResult04380{}, fmt.Errorf("import-current failed for %s: %s", profile.name, strings.ReplaceAll(output, profile.token, "<redacted>"))
		}
	}
	code, listing, err := invokeProfileCommand(fixture, []string{"profile", "list"})
	if err != nil || code != 0 || !strings.Contains(listing, "main") || !strings.Contains(listing, "second") || !strings.Contains(listing, "third") {
		return quotaRestartResult04380{}, errors.New("synthetic three-profile catalog was not persisted")
	}
	if product == "prodex" {
		for _, profile := range profiles {
			if err := quotaRestartAuthMatches04380(configHome, profile); err != nil {
				return quotaRestartResult04380{}, err
			}
		}
		code, diagnostic, err := invokeProfileCommand(fixture, []string{
			"run", "--dry-run", "--base-url", "http://127.0.0.1:1/backend-api",
			"exec", "synthetic quota restart probe",
		})
		if err != nil || code != 0 || !strings.Contains(diagnostic, "Runtime proxy: would be enabled") {
			var state []string
			for _, line := range strings.Split(diagnostic, "\n") {
				if strings.HasPrefix(line, "Provider:") || strings.HasPrefix(line, "Runtime proxy:") {
					state = append(state, line)
				}
			}
			return quotaRestartResult04380{}, fmt.Errorf("Prodex did not enable the multi-profile runtime proxy: %s; catalog=%q", strings.Join(state, "; "), strings.TrimSpace(listing))
		}
	}
	activeCommand := []string{"profile", "use", "--profile", "main"}
	if product == "godex" {
		activeCommand = []string{"account", "use", "main"}
	}
	if code, _, err := invokeProfileCommand(fixture, activeCommand); err != nil || code != 0 {
		return quotaRestartResult04380{}, errors.New("select synthetic main profile")
	}
	child, err := writeQuotaRestartChild04380(directory)
	if err != nil {
		return quotaRestartResult04380{}, err
	}
	fixture.env = quotaRestartSetEnv04380(fixture.env, "PRODEX_CODEX_BIN", child)
	fixture.env = quotaRestartSetEnv04380(fixture.env, "GODEX_CODEX_BIN", child)

	firstCredentials := map[string]string{
		quotaRestartMainAccount04380:   "token-main",
		quotaRestartSecondAccount04380: "token-second",
		quotaRestartThirdAccount04380:  "token-third",
	}
	firstServer := newQuotaRestartServer04380(firstCredentials, true)
	firstExit, runErr := runQuotaRestartPhase04380(fixture, firstServer.server.URL+"/backend-api", filepath.Join(directory, "first-child"))
	if runErr != nil {
		firstServer.close()
		_ = stopQuotaRuntimeBrokers04380(configHome, binary)
		return quotaRestartResult04380{}, runErr
	}
	if err := stopQuotaRuntimeBrokers04380(configHome, binary); err != nil {
		firstServer.close()
		return quotaRestartResult04380{}, err
	}
	first := firstServer.result(quotaSnapshotPresent04380(configHome))
	firstServer.close()

	if err := rotateQuotaRestartCredential04380(configHome); err != nil {
		return quotaRestartResult04380{}, err
	}
	restartedCredentials := map[string]string{
		quotaRestartMainAccount04380:   "token-main-rotated",
		quotaRestartSecondAccount04380: "token-second",
		quotaRestartThirdAccount04380:  "token-third",
	}
	restartServer := newQuotaRestartServer04380(restartedCredentials, false)
	restartExit, runErr := runQuotaRestartPhase04380(fixture, restartServer.server.URL+"/backend-api", filepath.Join(directory, "restart-child"))
	if runErr != nil {
		restartServer.close()
		_ = stopQuotaRuntimeBrokers04380(configHome, binary)
		return quotaRestartResult04380{}, runErr
	}
	if err := stopQuotaRuntimeBrokers04380(configHome, binary); err != nil {
		restartServer.close()
		return quotaRestartResult04380{}, err
	}
	restart := restartServer.result(quotaSnapshotPresent04380(configHome))
	restartServer.close()

	result := quotaRestartResult04380{first: first, restart: restart, firstExit: firstExit, restartExit: restartExit}
	if !quotaRestartResultValid04380(result) {
		return quotaRestartResult04380{}, fmt.Errorf("synthetic three-profile owner/auth sequence did not match the bounded contract: %+v", result)
	}
	return result, nil
}

const (
	httpStatusOK04380           = 200
	httpStatusUnauthorized04380 = 401
)

func runQuotaRestartPhase04380(fixture profileFixture, baseURL, record string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, fixture.binary, "run", "--base-url", baseURL, "exec", "synthetic quota restart probe")
	command.Dir = fixture.home
	command.Env = quotaRestartSetEnv04380(fixture.env, "GODEX_QUOTA_CHILD_RECORD", record)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if ctx.Err() != nil {
		return -1, ctx.Err()
	}
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		args, _ := os.ReadFile(record + ".args")
		return exitErr.ExitCode(), fmt.Errorf("runtime phase exited with status %d (child args %q)", exitErr.ExitCode(), strings.TrimSpace(string(args)))
	}
	return -1, errors.New("runtime phase failed to start")
}

func writeQuotaRestartChild04380(directory string) (string, error) {
	path := filepath.Join(directory, "synthetic-codex-child.sh")
	const script = `#!/bin/sh
printf '%s\n' 'CALL' "$@" >> "$GODEX_QUOTA_CHILD_RECORD.args"
case "$1" in
  --version|app-server) printf '%s\n' 'codex 0.163.0'; exit 0 ;;
esac
for argument in "$@"; do
  if [ "$argument" = "exec-server" ]; then printf '%s\n' 'codex 0.163.0'; exit 0; fi
done
endpoint=$(printf '%s\n' "$@" | sed -n 's/.*model_providers\.[^ ]*\.base_url="\(http[^" ]*\)".*/\1/p' | head -n 1)
if [ -z "$endpoint" ]; then exit 17; fi
sleep 2
body='{"model":"gpt-6","session_id":"quota-restart-session","input":[{"role":"user","content":"synthetic quota restart"}],"stream":false}'
curl --noproxy '*' --silent --show-error --max-time 10 \
  -H 'Content-Type: application/json' \
  -d "$body" \
  "$endpoint/responses" > "$GODEX_QUOTA_CHILD_RECORD.body"
status=$?
printf '%s\n' "$status" > "$GODEX_QUOTA_CHILD_RECORD.status"
exit "$status"
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		return "", err
	}
	return path, nil
}

func writeQuotaRestartAuth04380(path, token, account string) error {
	content := fmt.Sprintf(`{"auth_mode":"chatgpt","tokens":{"access_token":%q,"account_id":%q}}`, token, account)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func quotaRestartAuthMatches04380(configHome string, profile quotaRestartProfile04380) error {
	path := filepath.Join(configHome, "profiles", profile.name, "auth.json")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 64<<10 || info.Mode().Perm()&0o077 != 0 {
		return errors.New("synthetic Prodex profile auth file is unsafe")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var auth struct {
		AuthMode string `json:"auth_mode"`
		Tokens   struct {
			AccessToken string `json:"access_token"`
			AccountID   string `json:"account_id"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(raw, &auth); err != nil || auth.AuthMode != "chatgpt" ||
		auth.Tokens.AccessToken != profile.token || auth.Tokens.AccountID != profile.account {
		return errors.New("synthetic Prodex profile credentials do not match the quota oracle")
	}
	return nil
}

func rotateQuotaRestartCredential04380(root string) error {
	count := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "auth.json" {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("synthetic auth fixture contains a symlink")
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() > 64<<10 {
			return errors.New("synthetic auth fixture is unsafe")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var auth struct {
			Tokens struct {
				AccessToken string `json:"access_token"`
				AccountID   string `json:"account_id"`
			} `json:"tokens"`
		}
		if err := json.Unmarshal(raw, &auth); err != nil {
			return err
		}
		if auth.Tokens.AccountID != quotaRestartMainAccount04380 {
			return nil
		}
		if auth.Tokens.AccessToken == "" {
			return errors.New("synthetic main credential is missing")
		}
		auth.Tokens.AccessToken = "token-main-rotated"
		updated, err := json.Marshal(auth)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, updated, 0o600); err != nil {
			return err
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return err
		}
		count++
		return nil
	})
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("synthetic main credential was not rotated exactly once")
	}
	return nil
}

func quotaRestartSetEnv04380(environment []string, key, value string) []string {
	result := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		name, _, found := strings.Cut(entry, "=")
		if found && name == key {
			continue
		}
		result = append(result, entry)
	}
	return append(result, key+"="+value)
}

func stopQuotaRuntimeBrokers04380(configHome, binary string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	paths, err := filepath.Glob(filepath.Join(configHome, "runtime-broker-*.json"))
	if err != nil {
		return err
	}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		var registry struct {
			PID            int    `json:"pid"`
			ExecutablePath string `json:"executable_path"`
		}
		if json.Unmarshal(raw, &registry) != nil || registry.PID <= 0 {
			continue
		}
		candidate, _ := filepath.Abs(binary)
		registered, _ := filepath.Abs(registry.ExecutablePath)
		if registered == "" || candidate == "" || registered != candidate {
			continue
		}
		if quotaRestartWaitPathGone04380(path, 12*time.Second) {
			continue
		}
		process, err := os.FindProcess(registry.PID)
		if err != nil {
			return err
		}
		_ = process.Signal(os.Interrupt)
		if quotaRestartWaitPathGone04380(path, 8*time.Second) {
			continue
		}
		_ = process.Kill()
		if quotaRestartWaitPathGone04380(path, 2*time.Second) {
			continue
		}
		if !quotaRestartProcessGone04380(process) {
			return errors.New("runtime broker did not stop")
		}
		if err := cleanupQuotaRestartBrokerArtifacts04380(configHome, path); err != nil {
			return err
		}
	}
	return nil
}

func quotaRestartProcessGone04380(process *os.Process) bool {
	err := process.Signal(syscall.Signal(0))
	return errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH)
}

func cleanupQuotaRestartBrokerArtifacts04380(configHome, registryPath string) error {
	name := strings.TrimSuffix(filepath.Base(registryPath), ".json")
	paths := []string{
		registryPath,
		registryPath + ".last-good",
		registryPath + ".lock",
		filepath.Join(configHome, name+".capability"),
		filepath.Join(configHome, name+"-ensure.json.lock"),
		filepath.Join(configHome, name+"-leases"),
	}
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove stale runtime broker artifact: %w", err)
		}
	}
	return nil
}

func quotaRestartWaitPathGone04380(path string, timeout time.Duration) bool {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return true
		}
		select {
		case <-deadline.C:
			return false
		case <-ticker.C:
		}
	}
}
