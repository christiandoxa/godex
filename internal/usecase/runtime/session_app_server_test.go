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

func createRuntimeHome(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(path, "config.toml"), []byte("model = \"synthetic\"\n"), 0o600)
}
