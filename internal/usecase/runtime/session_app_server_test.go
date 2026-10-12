package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
)

type sessionProxyProcess struct {
	fakeProxyProcess
	used bool
}

type sessionRecoveryProxyProcess struct {
	sessionProxyProcess
	release func(context.Context, string) error
}

func (process *sessionRecoveryProxyProcess) RunThroughProxyWithSessionServerAndRecovery(
	_ context.Context,
	_ string,
	_ string,
	_ []string,
	_ string,
	release func(context.Context, string) error,
) error {
	process.used = true
	process.release = release
	return nil
}

type sessionAffinityProxy struct {
	fakeProxy
	released string
}

func (proxy *sessionAffinityProxy) ReleaseSessionAffinity(_ context.Context, sessionID string) error {
	proxy.released = sessionID
	return nil
}

func (process *sessionProxyProcess) RunThroughProxyWithSessionServer(
	context.Context, string, string, []string, string,
) error {
	process.used = true
	return nil
}

func TestSuperOverlayUsesOptionalSessionServerProcessPort(t *testing.T) {
	base := filepath.Join(t.TempDir(), "base")
	if err := createRuntimeHome(base); err != nil {
		t.Fatal(err)
	}
	process := &sessionProxyProcess{}
	runner := NewRunner(nil, process, func(proxyconfig.Config) (Proxy, error) {
		return &fakeProxy{}, nil
	})
	runner.SetManagedProfilesRoot(filepath.Join(t.TempDir(), "profiles"))
	if err := runner.launchHomeWithOptions(
		context.Background(), base, "profile", proxyconfig.Provider{}, nil,
		[]proxyconfig.Account{{ID: "profile", Home: base, Enabled: true}}, nil,
		RuntimeLaunchOptions{SuperOverlay: true},
	); err != nil {
		t.Fatal(err)
	}
	if !process.used {
		t.Fatal("Super overlay bypassed the session-server process port")
	}
}

func TestNativeRecoveryReceivesRuntimeProxyAffinityRelease(t *testing.T) {
	base := filepath.Join(t.TempDir(), "base")
	if err := createRuntimeHome(base); err != nil {
		t.Fatal(err)
	}
	process := &sessionRecoveryProxyProcess{}
	proxy := &sessionAffinityProxy{}
	runner := NewRunner(nil, process, func(proxyconfig.Config) (Proxy, error) {
		return proxy, nil
	})
	runner.SetManagedProfilesRoot(filepath.Join(t.TempDir(), "profiles"))
	if err := runner.launchHomeWithOptions(
		context.Background(), base, "profile", proxyconfig.Provider{}, nil,
		[]proxyconfig.Account{{ID: "profile", Home: base, Enabled: true}}, nil,
		RuntimeLaunchOptions{SuperOverlay: true, nativeSessionID: "native-session"},
	); err != nil {
		t.Fatal(err)
	}
	if !process.used || process.release == nil {
		t.Fatal("native recovery process did not receive affinity release callback")
	}
	if err := process.release(context.Background(), "native-session"); err != nil {
		t.Fatal(err)
	}
	if proxy.released != "native-session" {
		t.Fatalf("released session = %q", proxy.released)
	}
}

func createRuntimeHome(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(path, "config.toml"), []byte("model = \"synthetic\"\n"), 0o600)
}
