package codex

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPrepareCodexHomeWritesPrivateFileStoreConfig(t *testing.T) {
	home := filepath.Join(t.TempDir(), "codex")
	if err := prepareCodexHome(home); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(home, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != codexFileCredentialConfig {
		t.Fatalf("config = %q", content)
	}
	if runtime.GOOS != "windows" {
		if mode := fileMode(t, home); mode.Perm() != 0o700 {
			t.Fatalf("home mode = %o", mode.Perm())
		}
		if mode := fileMode(t, filepath.Join(home, "config.toml")); mode.Perm() != 0o600 {
			t.Fatalf("config mode = %o", mode.Perm())
		}
	}
}

func TestManagedArgumentsValidateURLs(t *testing.T) {
	args, err := proxyArguments("http://127.0.0.1:1234", nil)
	if err != nil || !strings.Contains(strings.Join(args, " "), `base_url="http://127.0.0.1:1234/backend-api/prodex"`) {
		t.Fatalf("managed URL arguments invalid")
	}
	for _, url := range []string{"https://user:pass@example.test", "file:///tmp/path", "https://example.test?secret=value"} {
		if _, err := proxyArguments(url, nil); err == nil {
			t.Fatal("invalid runtime URL accepted")
		}
	}
}

func TestProxyArgumentsRejectBaseURLOverridesWithSpacing(t *testing.T) {
	if _, err := proxyArguments("http://127.0.0.1:1234", []string{"-c", `chatgpt_base_url = "https://example.test"`}); err == nil {
		t.Fatal("expected proxy override rejection")
	}
}

func TestCodexProcessRejectsMissingBinary(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "codex-missing")
	process := NewCodexProcess(missing, Terminal{})
	if _, err := process.resolveBinary(); err == nil {
		t.Fatal("expected missing Codex executable error")
	}
}

func TestCodexProcessRunPreservesChildExitStatus(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX shell")
	}
	process := NewCodexProcess("/bin/sh", Terminal{
		Stdin:  strings.NewReader(""),
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})
	err := process.Run(context.Background(), t.TempDir(), []string{"-c", "exit 23"})
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) || exitError.ExitCode() != 23 {
		t.Fatalf("error = %v", err)
	}
}

func TestCodexProcessRunHonorsCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX shell")
	}
	process := NewCodexProcess("sleep", Terminal{
		Stdin:  strings.NewReader(""),
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := process.Run(ctx, t.TempDir(), []string{"5"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v", err)
	}
}

func TestPrepareCodexHomeRejectsRelativePath(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := prepareCodexHome("relative-home"); err == nil {
		t.Fatal("expected relative Codex home to be rejected")
	}
}

func TestCodexProcessRejectsUnsafeHomes(t *testing.T) {
	if err := prepareCodexHome("/"); err == nil {
		t.Fatal("filesystem root unexpectedly accepted")
	}
	if err := secureCodexHome("/"); err == nil {
		t.Fatal("filesystem root unexpectedly secured")
	}
	if err := NewCodexProcess("/bin/sh", Terminal{}).Run(context.Background(), "/", nil); err == nil {
		t.Fatal("run with filesystem root unexpectedly accepted")
	}
}

func TestPrepareCodexHomeRejectsConfigSymlink(t *testing.T) {
	home := t.TempDir()
	if err := os.Symlink(filepath.Join(t.TempDir(), "target"), filepath.Join(home, "config.toml")); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	if err := prepareCodexHome(home); err == nil {
		t.Fatal("config symlink unexpectedly accepted")
	}
}

func TestSecureCodexHomeRejectsNonRegularCredentialEntry(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, "auth.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := secureCodexHome(home); err == nil {
		t.Fatal("credential directory unexpectedly accepted")
	}
}

func TestCodexProcessRejectsDirectoryBinaryAndEmptyProxyEndpoint(t *testing.T) {
	directory := t.TempDir()
	if _, err := NewCodexProcess(directory, Terminal{}).resolveBinary(); err == nil {
		t.Fatal("directory executable unexpectedly accepted")
	}
	if runtime.GOOS == "windows" {
		t.Skip("helper uses a POSIX executable")
	}
	script := writeProxyHelper(t, "exit 0")
	if err := NewCodexProcess(script, Terminal{}).RunThroughProxy(context.Background(), t.TempDir(), "", nil); err == nil {
		t.Fatal("empty proxy endpoint unexpectedly accepted")
	}
}

func TestCodexProcessVersionAndLoginReportChildFailures(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper uses a POSIX executable")
	}
	script := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	process := NewCodexProcess(script, Terminal{})
	if _, err := process.Version(context.Background()); err == nil {
		t.Fatal("version failure unexpectedly ignored")
	}
	if _, err := process.Login(context.Background(), filepath.Join(t.TempDir(), "home"), false); err == nil {
		t.Fatal("login failure unexpectedly accepted")
	}
}

func TestCodexProcessLoginRejectsMissingAuthFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper uses a POSIX executable")
	}
	process := NewCodexProcess(writeProxyHelper(t, "exit 0"), Terminal{})
	if _, err := process.Login(context.Background(), filepath.Join(t.TempDir(), "home"), false); err == nil {
		t.Fatal("login without auth profile unexpectedly succeeded")
	}
}

func TestCodexProcessVersionUsesIsolatedHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper script uses Unix shell")
	}
	record := filepath.Join(t.TempDir(), "home")
	globalHome := filepath.Join(t.TempDir(), "global")
	script := filepath.Join(t.TempDir(), "codex")
	content := "#!/bin/sh\nprintf '%s' \"$CODEX_HOME\" > \"$GODEX_VERSION_RECORD\"\nprintf '%s\\n' 'codex 1.2.3'\n"
	if err := os.WriteFile(script, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", globalHome)
	t.Setenv("GODEX_VERSION_RECORD", record)
	process := NewCodexProcess(script, Terminal{})

	version, err := process.Version(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if version != "codex 1.2.3" {
		t.Fatalf("version = %q", version)
	}
	childHome, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if string(childHome) == globalHome || string(childHome) == "" {
		t.Fatalf("child CODEX_HOME = %q, want private home", childHome)
	}
	if _, err := os.Stat(string(childHome)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("version home cleanup error = %v", err)
	}
}

func TestCodexProcessLoginUsesIsolatedHomeAndDeviceAuth(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper wrapper uses a POSIX shell")
	}
	process := newCodexLoginTestProcess(t)
	for _, test := range []struct {
		name       string
		deviceAuth bool
		wantArgs   string
	}{
		{name: "browser", wantArgs: "login"},
		{name: "device", deviceAuth: true, wantArgs: "login --device-auth"},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertCodexLogin(t, process, test.deviceAuth, test.wantArgs)
		})
	}
}

func newCodexLoginTestProcess(t *testing.T) *CodexProcess {
	t.Helper()
	t.Setenv("GODEX_TEST_HELPER", "1")
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), "global"))
	t.Setenv("GODEX_TEST_BINARY", os.Args[0])
	wrapper := filepath.Join(t.TempDir(), "codex-wrapper")
	if err := os.WriteFile(wrapper, []byte("#!/bin/sh\nexec \"$GODEX_TEST_BINARY\" -test.run=TestCodexHelperProcess -- \"$@\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return NewCodexProcess(wrapper, Terminal{
		Stdin:  strings.NewReader(""),
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})
}

func assertCodexLogin(t *testing.T, process *CodexProcess, deviceAuth bool, wantArgs string) {
	t.Helper()
	record := filepath.Join(t.TempDir(), "record")
	t.Setenv("GODEX_TEST_RECORD", record)
	home := filepath.Join(t.TempDir(), "staged-codex-home")

	identity, err := process.Login(context.Background(), home, deviceAuth)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Email != "person@example.com" || identity.ChatGPTAccountID != "account-123" {
		t.Fatalf("identity = %+v", identity)
	}
	childHome, err := os.ReadFile(record + ".home")
	if err != nil {
		t.Fatal(err)
	}
	if string(childHome) != home {
		t.Fatalf("child CODEX_HOME = %q, want %q", childHome, home)
	}
	arguments, err := os.ReadFile(record + ".args")
	if err != nil {
		t.Fatal(err)
	}
	if string(arguments) != wantArgs {
		t.Fatalf("child arguments = %q, want %q", arguments, wantArgs)
	}
	if got := string(mustReadFile(t, filepath.Join(home, "config.toml"))); got != codexFileCredentialConfig {
		t.Fatalf("config = %q", got)
	}
}

func TestCodexHelperProcess(t *testing.T) {
	if os.Getenv("GODEX_TEST_HELPER") != "1" {
		return
	}
	record := os.Getenv("GODEX_TEST_RECORD")
	home := os.Getenv("CODEX_HOME")
	if record == "" || home == "" {
		os.Exit(2)
	}
	if err := os.WriteFile(record+".home", []byte(home), 0o600); err != nil {
		os.Exit(3)
	}
	arguments := os.Args[1:]
	for index, argument := range arguments {
		if argument == "--" {
			arguments = arguments[index+1:]
			break
		}
	}
	if err := os.WriteFile(record+".args", []byte(strings.Join(arguments, " ")), 0o600); err != nil {
		os.Exit(4)
	}
	if _, err := os.Stat(filepath.Join(home, "config.toml")); err != nil {
		os.Exit(5)
	}
	auth := map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]any{
			"access_token": jwtForClaims(map[string]any{
				"email": "person@example.com",
				"https://api.openai.com/auth": map[string]any{
					"chatgpt_account_id": "account-123",
				},
			}),
		},
	}
	content, err := json.Marshal(auth)
	if err != nil || os.WriteFile(filepath.Join(home, "auth.json"), content, 0o600) != nil {
		os.Exit(6)
	}
	os.Exit(0)
}

func jwtForClaims(claims map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload, _ := json.Marshal(claims)
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".synthetic"
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode()
}
