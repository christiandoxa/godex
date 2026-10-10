package runtimebroker

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Lease struct {
	path string
}

func (lease *Lease) Path() string {
	if lease == nil {
		return ""
	}
	return lease.path
}

func (lease *Lease) Close() error {
	if lease == nil || lease.path == "" {
		return nil
	}
	err := removeRegularFile(lease.path)
	lease.path = ""
	return err
}

func (store *Store) CreateLease(key string, pid uint32) (*Lease, error) {
	if !validID(key) || pid == 0 {
		return nil, errors.New("runtime broker lease request is invalid")
	}
	dir := store.LeaseDir(key)
	if err := ensureRegularPrivateDir(dir); err != nil {
		return nil, err
	}
	token, err := randomToken("lease")
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, fmt.Sprintf("%d-%s.lease", pid, token))
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	if _, err := fmt.Fprintf(file, "pid=%d\n", pid); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return &Lease{path: path}, nil
}

func (store *Store) CleanupStaleLeases(key string) int {
	if !validID(key) {
		return 0
	}
	dir := store.LeaseDir(key)
	info, err := os.Lstat(dir)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return 0
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	live := 0
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			continue
		}
		pid := leasePID(entry.Name())
		if pid == 0 {
			continue
		}
		if processAlive(pid) {
			live++
			continue
		}
		_ = os.Remove(path)
	}
	if live == 0 {
		_ = os.Remove(dir)
	}
	return live
}

// CleanupStaleLeasesAll removes dead broker leases left by a crashed child.
// It scans only owned lease directories and leaves unrelated files untouched.
func (store *Store) CleanupStaleLeasesAll() {
	if store == nil {
		return
	}
	if err := store.ensureHome(); err != nil {
		return
	}
	entries, err := os.ReadDir(store.home)
	if err != nil {
		return
	}
	const prefix = "runtime-broker-"
	const suffix = "-leases"
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
			continue
		}
		key := strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix)
		if !validID(key) {
			continue
		}
		info, err := os.Lstat(filepath.Join(store.home, name))
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			continue
		}
		store.CleanupStaleLeases(key)
	}
}

func leasePID(name string) int {
	first, _, ok := strings.Cut(name, "-")
	if !ok {
		return 0
	}
	pid, err := strconv.Atoi(first)
	if err != nil || pid <= 0 {
		return 0
	}
	return pid
}

func randomToken(prefix string) (string, error) {
	payload := make([]byte, 32)
	if _, err := rand.Read(payload); err != nil {
		return "", err
	}
	return prefix + "-" + base64.RawURLEncoding.EncodeToString(payload), nil
}

func ensureRegularPrivateDir(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return err
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("refusing to use symlinked runtime broker lease dir %s", path)
	}
	return nil
}
