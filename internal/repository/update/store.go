package update

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/christiandoxa/godex/internal/helper/fileutil"
	"github.com/christiandoxa/godex/internal/helper/lockfile"
)

const (
	cacheTTL      = 5 * time.Minute
	cacheMaxBytes = 64 << 10
)

type Store struct {
	root string
}

type cacheFile struct {
	LatestVersion string `json:"latest_version"`
	CheckedAt     int64  `json:"checked_at"`
}

func NewStore(root string) *Store { return &Store{root: filepath.Clean(root)} }

func (store *Store) CachedLatest(now time.Time) (string, bool) {
	cache, err := store.readCache()
	if err != nil || cache.LatestVersion == "" {
		return "", false
	}
	checked := time.Unix(cache.CheckedAt, 0)
	if now.Before(checked) || now.Sub(checked) >= cacheTTL {
		return "", false
	}
	return cache.LatestVersion, true
}

func (store *Store) SaveLatest(version string, now time.Time) error {
	if err := store.prepare(); err != nil {
		return err
	}
	content, err := json.MarshalIndent(cacheFile{LatestVersion: version, CheckedAt: now.Unix()}, "", "  ")
	if err != nil {
		return errors.New("serialize update check cache")
	}
	content = append(content, '\n')
	_, err = fileutil.AtomicWrite(store.cachePath(), content)
	return err
}

func (store *Store) AcquireCheck(ctx context.Context) (func() error, error) {
	if err := store.prepare(); err != nil {
		return nil, err
	}
	return lockfile.Acquire(ctx, filepath.Join(store.root, "update-check.lock"))
}

func (store *Store) AcquireInstall(ctx context.Context) (func() error, error) {
	if err := store.prepare(); err != nil {
		return nil, err
	}
	return lockfile.Acquire(ctx, filepath.Join(store.root, "update-install.lock"))
}

func (store *Store) prepare() error {
	info, err := os.Lstat(store.root)
	if errors.Is(err, os.ErrNotExist) {
		return os.MkdirAll(store.root, 0o700)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("Godex update state root must be a real directory")
	}
	return os.Chmod(store.root, 0o700)
}

func (store *Store) readCache() (cacheFile, error) {
	path := store.cachePath()
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return cacheFile{}, nil
	}
	if err != nil {
		return cacheFile{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > cacheMaxBytes {
		return cacheFile{}, errors.New("update check cache exceeds read limit")
	}
	file, err := os.Open(path)
	if err != nil {
		return cacheFile{}, errors.New("failed to open update check cache")
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, cacheMaxBytes+1))
	if err != nil || len(content) > cacheMaxBytes {
		return cacheFile{}, errors.New("update check cache exceeds read limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var cache cacheFile
	if err := decoder.Decode(&cache); err != nil {
		return cacheFile{}, errors.New("failed to parse update check cache")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return cacheFile{}, errors.New("failed to parse update check cache")
	}
	return cache, nil
}

func (store *Store) cachePath() string { return filepath.Join(store.root, "update-check.json") }
