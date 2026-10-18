package main

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestProdex04382ReferenceFixtureSealsManagedAuthAfterImport(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native Windows ACL regression covers the platform permission contract")
	}
	root := t.TempDir()
	path := filepath.Join(root, "profiles", "main", "auth.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"auth_mode":"chatgpt"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if err := seedProductState(product{name: "prodex", state: root}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("reference runtime fixture retained insecure imported auth permissions")
	}
}

func TestProdex04382WebSocketBackendSeparatesQuotaProbesFromModelTurns(t *testing.T) {
	b := newBackend()
	defer b.close()
	b.mu.Lock()
	b.separateUsage = true
	b.mu.Unlock()
	for range 2 {
		response, err := http.Get(b.server.URL + "/backend-api/wham/usage")
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
	}
	conn, reader, err := wsConnect(b.server.URL + "/backend-api/")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := capacityWebSocketTurn04382(conn, reader); err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.usageProbes != 2 || b.wsHandshakes != 1 || len(b.attempts) != 1 || b.attempts[0].mode != capacityMode04382 {
		t.Fatal("quota traffic changed the exact inference or handshake observation")
	}
}
