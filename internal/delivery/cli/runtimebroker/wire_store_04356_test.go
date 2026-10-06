package runtimebroker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProdex04356RuntimeBrokerBootstrapWireContract(t *testing.T) {
	payload := []byte(`{"version":1,"current_profile":"primary","upstream_base_url":"https://chatgpt.com/backend-api","include_code_review":true,"upstream_no_proxy":false,"smart_context_enabled":true,"model_context_window_tokens":1048576,"broker_key":"broker_key-1","instance_id":"instance_1","admin_token":"admin-secret","listen_addr":"127.0.0.1:4321"}`)
	bootstrap, err := ReadBootstrap(bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap.CurrentProfile != "primary" || bootstrap.UpstreamBaseURL != "https://chatgpt.com/backend-api" ||
		!bootstrap.IncludeCodeReview || !bootstrap.SmartContextEnabled ||
		bootstrap.BrokerKey != "broker_key-1" || bootstrap.InstanceID != "instance_1" ||
		bootstrap.ListenAddr != "127.0.0.1:4321" ||
		bootstrap.ModelContextWindowTokens == nil || *bootstrap.ModelContextWindowTokens != 1_048_576 ||
		!bootstrap.AdminToken.Matches("admin-secret") {
		t.Fatalf("bootstrap = %#v", bootstrap)
	}
	if bootstrap.AdminToken.Matches("admin-secret-x") || bootstrap.AdminToken.Matches("admin-secreu") {
		t.Fatal("runtime broker secret comparison accepted mismatch")
	}

	for name, value := range map[string][]byte{
		"unknown field":       []byte(`{"version":1,"current_profile":"p","upstream_base_url":"u","broker_key":"k","instance_id":"i","admin_token":"a","unknown":true}`),
		"bad version":         []byte(`{"version":2,"current_profile":"p","upstream_base_url":"u","broker_key":"k","instance_id":"i","admin_token":"a"}`),
		"hostname listen":     []byte(`{"version":1,"current_profile":"p","upstream_base_url":"u","broker_key":"k","instance_id":"i","admin_token":"a","listen_addr":"localhost:1234"}`),
		"non-loopback listen": []byte(`{"version":1,"current_profile":"p","upstream_base_url":"u","broker_key":"k","instance_id":"i","admin_token":"a","listen_addr":"203.0.113.7:1234"}`),
		"bad broker key":      []byte(`{"version":1,"current_profile":"p","upstream_base_url":"u","broker_key":"bad/key","instance_id":"i","admin_token":"a"}`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ReadBootstrap(bytes.NewReader(value)); err == nil {
				t.Fatalf("invalid bootstrap accepted: %s", value)
			}
		})
	}

	oversized := bytes.Repeat([]byte{'x'}, BootstrapMaxBytes+1)
	if _, err := ReadBootstrap(bytes.NewReader(oversized)); err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("oversized bootstrap = %v", err)
	}
}

func TestProdex04356RuntimeBrokerCapabilityWireContract(t *testing.T) {
	secret, err := NewSecret("admin-token")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := EncodeCapability("instance-1", secret)
	if err != nil {
		t.Fatal(err)
	}
	capability, err := DecodeCapability(payload)
	if err != nil {
		t.Fatal(err)
	}
	if capability.InstanceID != "instance-1" || !capability.AdminToken.Matches("admin-token") {
		t.Fatalf("capability = %#v", capability)
	}
	var wire map[string]any
	if err := json.Unmarshal(payload, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["version"] != float64(1) || wire["instance_id"] != "instance-1" || wire["admin_token"] != "admin-token" {
		t.Fatalf("capability wire = %#v", wire)
	}
	if _, err := DecodeCapability(append(payload, []byte(` {"extra":1}`)...)); err == nil {
		t.Fatal("trailing capability JSON accepted")
	}
}

func TestProdex04356RuntimeBrokerStoreSeparatesCapabilityAndRecoversBackup(t *testing.T) {
	home := t.TempDir()
	store, err := NewStore(home)
	if err != nil {
		t.Fatal(err)
	}
	secret, _ := NewSecret("broker-secret")
	mount := OpenAIMountPath
	registry := Registry{
		PID: 12345, ListenAddr: "127.0.0.1:4321", StartedAt: 42,
		UpstreamBaseURL: "https://chatgpt.com/backend-api", CurrentProfile: "primary",
		InstanceID: "instance-1", OpenAIMountPath: &mount,
	}
	if err := store.SaveArtifacts(t.Context(), "broker-1", Capability{
		InstanceID: "instance-1", AdminToken: secret,
	}, registry); err != nil {
		t.Fatal(err)
	}
	registryBytes, err := os.ReadFile(store.RegistryPath("broker-1"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(registryBytes, []byte("broker-secret")) || bytes.Contains(registryBytes, []byte("admin_token")) {
		t.Fatalf("secret leaked into registry: %s", registryBytes)
	}
	capabilityBytes, err := os.ReadFile(store.CapabilityPath("broker-1"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(capabilityBytes, []byte("broker-secret")) {
		t.Fatal("capability did not contain expected secret")
	}
	if info, err := os.Stat(store.CapabilityPath("broker-1")); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("capability mode = %v / %v", info, err)
	}

	registry.CurrentProfile = "secondary"
	if err := store.SaveRegistry(t.Context(), "broker-1", registry); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.RegistryBackupPath("broker-1")); err != nil {
		t.Fatalf("registry backup missing: %v", err)
	}
	if err := os.WriteFile(store.RegistryPath("broker-1"), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	recovered, found, err := store.LoadRegistry(t.Context(), "broker-1")
	if err != nil || !found || recovered.CurrentProfile != "primary" {
		t.Fatalf("backup recovery = %#v found=%t err=%v", recovered, found, err)
	}
	capability, err := store.LoadCapability(t.Context(), "broker-1", "instance-1")
	if err != nil || !capability.AdminToken.Matches("broker-secret") {
		t.Fatalf("capability load = %#v err=%v", capability, err)
	}

	store.RemoveIfMatches(t.Context(), "broker-1", "instance-1", secret)
	for _, path := range []string{
		store.RegistryPath("broker-1"), store.RegistryBackupPath("broker-1"), store.CapabilityPath("broker-1"),
	} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("artifact not removed %s: %v", path, err)
		}
	}
}

func TestProdex04356RuntimeBrokerStoreRejectsLegacySecretRegistry(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := store.RegistryPath("broker")
	legacy := `{"pid":1,"listen_addr":"127.0.0.1:1234","started_at":1,"upstream_base_url":"u","include_code_review":false,"current_profile":"p","instance_id":"i","admin_token":"secret"}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.LoadRegistry(t.Context(), "broker"); err != nil || found {
		t.Fatalf("legacy secret registry = found:%t err:%v", found, err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy registry was not removed: %v", err)
	}
}

func TestProdex04356RuntimeBrokerLeasesArePrivateBoundedAndCleaned(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	live, err := store.CreateLease("broker", uint32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(live.Path()); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("lease mode = %v / %v", info, err)
	}
	if content, err := os.ReadFile(live.Path()); err != nil || string(content) != "pid="+itoa(os.Getpid())+"\n" {
		t.Fatalf("lease content = %q / %v", content, err)
	}
	stale, err := store.CreateLease("broker", 2_147_483_647)
	if err != nil {
		t.Fatal(err)
	}
	stalePath := stale.Path()
	if got := store.CleanupStaleLeases("broker"); got != 1 {
		t.Fatalf("live lease count = %d, want 1", got)
	}
	if _, err := os.Stat(stalePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale lease remains: %v", err)
	}
	if err := live.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestProdex04356RuntimeBrokerLeaseDirRejectsSymlink(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, store.LeaseDir("broker")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := store.CreateLease("broker", 1234); err == nil || !strings.Contains(err.Error(), "symlinked") {
		t.Fatalf("symlink lease dir = %v", err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("outside lease dir mutated: entries=%d err=%v", len(entries), err)
	}
}

func itoa(value int) string {
	return fmt.Sprintf("%d", value)
}

var _ = context.Background
var _ = filepath.Separator
