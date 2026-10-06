package runtimebroker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/christiandoxa/godex/internal/helper/lockfile"
)

const registryMaxBytes = 1 << 20

type Store struct {
	home string
}

func NewStore(home string) (*Store, error) {
	if home == "" || !filepath.IsAbs(home) {
		return nil, errors.New("runtime broker store home must be absolute")
	}
	home = filepath.Clean(home)
	if home == filepath.Dir(home) {
		return nil, errors.New("runtime broker store home must not be filesystem root")
	}
	return &Store{home: home}, nil
}

func (store *Store) RegistryPath(key string) string {
	return filepath.Join(store.home, "runtime-broker-"+key+".json")
}

func (store *Store) RegistryBackupPath(key string) string {
	return store.RegistryPath(key) + ".last-good"
}

func (store *Store) CapabilityPath(key string) string {
	return filepath.Join(store.home, "runtime-broker-"+key+".capability")
}

func (store *Store) LeaseDir(key string) string {
	return filepath.Join(store.home, "runtime-broker-"+key+"-leases")
}

func (store *Store) EnsureLockPath(key string) string {
	return filepath.Join(store.home, "runtime-broker-"+key+"-ensure")
}

func (store *Store) SaveArtifacts(
	ctx context.Context,
	key string,
	capability Capability,
	registry Registry,
) error {
	if !validID(key) || key != registryKey(registry, key) {
		return errors.New("runtime broker key is invalid")
	}
	if registry.InstanceID != capability.InstanceID || !validID(registry.InstanceID) {
		return errors.New("runtime broker registry/capability instance mismatch")
	}
	if err := store.ensureHome(); err != nil {
		return err
	}
	release, err := lockfile.Acquire(ctx, store.RegistryPath(key)+".lock")
	if err != nil {
		return err
	}
	defer release()

	payload, err := EncodeCapability(capability.InstanceID, capability.AdminToken)
	if err != nil {
		return err
	}
	if err := atomicPrivateWrite(store.CapabilityPath(key), payload, 0o600); err != nil {
		return fmt.Errorf("write runtime broker capability: %w", err)
	}
	if err := store.saveRegistryUnlocked(key, registry); err != nil {
		_ = store.removeCapabilityIfMatchesUnlocked(key, capability)
		return err
	}
	return nil
}

func registryKey(registry Registry, fallback string) string {
	if validID(fallback) {
		return fallback
	}
	return ""
}

func (store *Store) SaveRegistry(ctx context.Context, key string, registry Registry) error {
	if !validID(key) || !validID(registry.InstanceID) {
		return errors.New("runtime broker registry is invalid")
	}
	if err := store.ensureHome(); err != nil {
		return err
	}
	release, err := lockfile.Acquire(ctx, store.RegistryPath(key)+".lock")
	if err != nil {
		return err
	}
	defer release()
	return store.saveRegistryUnlocked(key, registry)
}

func (store *Store) saveRegistryUnlocked(key string, registry Registry) error {
	payload, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return fmt.Errorf("serialize runtime broker registry: %w", err)
	}
	if len(payload) > registryMaxBytes {
		return errors.New("runtime broker registry exceeds safe size limit")
	}
	if containsLegacyRegistrySecrets(payload) {
		return errors.New("runtime broker registry must not contain secrets")
	}
	primary := store.RegistryPath(key)
	backup := store.RegistryBackupPath(key)
	if current, err := readRegularFileBounded(primary, registryMaxBytes); err == nil {
		var existing Registry
		if json.Unmarshal(current, &existing) == nil && !containsLegacyRegistrySecrets(current) {
			if err := atomicPrivateWrite(backup, current, 0o600); err != nil {
				return fmt.Errorf("write runtime broker registry backup: %w", err)
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := atomicPrivateWrite(primary, payload, 0o600); err != nil {
		return fmt.Errorf("write runtime broker registry: %w", err)
	}
	return nil
}

func (store *Store) LoadRegistry(ctx context.Context, key string) (Registry, bool, error) {
	if !validID(key) {
		return Registry{}, false, errors.New("runtime broker key is invalid")
	}
	if err := store.ensureHome(); err != nil {
		return Registry{}, false, err
	}
	release, err := lockfile.Acquire(ctx, store.RegistryPath(key)+".lock")
	if err != nil {
		return Registry{}, false, err
	}
	defer release()
	for _, path := range []string{store.RegistryPath(key), store.RegistryBackupPath(key)} {
		payload, err := readRegularFileBounded(path, registryMaxBytes)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return Registry{}, false, err
		}
		if containsLegacyRegistrySecrets(payload) {
			store.removeArtifactsUnlocked(key)
			return Registry{}, false, nil
		}
		var registry Registry
		if err := json.Unmarshal(payload, &registry); err != nil {
			continue
		}
		if !validRegistry(registry) {
			continue
		}
		return registry, true, nil
	}
	return Registry{}, false, nil
}

func (store *Store) LoadCapability(ctx context.Context, key, expectedInstance string) (Capability, error) {
	if !validID(key) || !validID(expectedInstance) {
		return Capability{}, errors.New("runtime broker capability lookup is invalid")
	}
	if err := store.ensureHome(); err != nil {
		return Capability{}, err
	}
	release, err := lockfile.Acquire(ctx, store.RegistryPath(key)+".lock")
	if err != nil {
		return Capability{}, err
	}
	defer release()
	payload, err := readRegularFileBounded(store.CapabilityPath(key), CapabilityMaxBytes)
	if err != nil {
		return Capability{}, err
	}
	capability, err := DecodeCapability(payload)
	if err != nil {
		return Capability{}, err
	}
	if capability.InstanceID != expectedInstance {
		return Capability{}, errors.New("runtime broker capability belongs to another instance")
	}
	return capability, nil
}

func (store *Store) RemoveIfMatches(
	ctx context.Context,
	key, instanceID string,
	secret Secret,
) {
	if !validID(key) || !validID(instanceID) {
		return
	}
	release, err := lockfile.Acquire(ctx, store.RegistryPath(key)+".lock")
	if err != nil {
		return
	}
	defer release()
	registry, found := store.loadRegistryUnlocked(key)
	if !found || registry.InstanceID != instanceID {
		return
	}
	payload, err := readRegularFileBounded(store.CapabilityPath(key), CapabilityMaxBytes)
	if err != nil {
		return
	}
	capability, err := DecodeCapability(payload)
	if err != nil || capability.InstanceID != instanceID || !capability.AdminToken.Matches(secret.Expose()) {
		return
	}
	store.removeArtifactsUnlocked(key)
}

func (store *Store) loadRegistryUnlocked(key string) (Registry, bool) {
	for _, path := range []string{store.RegistryPath(key), store.RegistryBackupPath(key)} {
		payload, err := readRegularFileBounded(path, registryMaxBytes)
		if err != nil || containsLegacyRegistrySecrets(payload) {
			continue
		}
		var registry Registry
		if json.Unmarshal(payload, &registry) == nil && validRegistry(registry) {
			return registry, true
		}
	}
	return Registry{}, false
}

func (store *Store) removeCapabilityIfMatchesUnlocked(key string, expected Capability) error {
	payload, err := readRegularFileBounded(store.CapabilityPath(key), CapabilityMaxBytes)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	actual, err := DecodeCapability(payload)
	if err != nil {
		return nil
	}
	if actual.InstanceID == expected.InstanceID && actual.AdminToken.Matches(expected.AdminToken.Expose()) {
		return removeRegularFile(store.CapabilityPath(key))
	}
	return nil
}

func (store *Store) removeArtifactsUnlocked(key string) {
	_ = removeRegularFile(store.RegistryPath(key))
	_ = removeRegularFile(store.RegistryBackupPath(key))
	_ = removeRegularFile(store.CapabilityPath(key))
}

func (store *Store) ensureHome() error {
	info, err := os.Lstat(store.home)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(store.home, 0o700); err != nil {
			return err
		}
		info, err = os.Lstat(store.home)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("runtime broker store home must be a regular directory")
	}
	return nil
}

func validRegistry(registry Registry) bool {
	return registry.PID != 0 &&
		validID(registry.InstanceID) &&
		registry.ListenAddr != "" && listenAddrIsLoopback(registry.ListenAddr) &&
		registry.CurrentProfile != "" && len(registry.CurrentProfile) <= 256 &&
		registry.UpstreamBaseURL != "" && len(registry.UpstreamBaseURL) <= 4096
}

func containsLegacyRegistrySecrets(payload []byte) bool {
	var top map[string]json.RawMessage
	if json.Unmarshal(payload, &top) != nil {
		return false
	}
	_, admin := top["admin_token"]
	_, instance := top["instance_token"]
	return admin || instance
}

func readRegularFileBounded(path string, limit int) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("runtime broker artifact must be a regular file")
	}
	if info.Size() > int64(limit) {
		return nil, errors.New("runtime broker artifact exceeds safe size limit")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	payload, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(payload) > limit {
		return nil, errors.New("runtime broker artifact exceeds safe size limit")
	}
	return payload, nil
}

func atomicPrivateWrite(path string, payload []byte, mode os.FileMode) error {
	parent := filepath.Dir(path)
	info, err := os.Lstat(parent)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("runtime broker artifact parent must be a regular directory")
	}
	if existing, err := os.Lstat(path); err == nil {
		if existing.Mode()&os.ModeSymlink != 0 || !existing.Mode().IsRegular() {
			return errors.New("runtime broker artifact path must be a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.CreateTemp(parent, ".godex-runtime-broker-*")
	if err != nil {
		return err
	}
	temp := file.Name()
	defer os.Remove(temp)
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(payload); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

func removeRegularFile(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("refusing to remove non-regular runtime broker artifact")
	}
	return os.Remove(path)
}
