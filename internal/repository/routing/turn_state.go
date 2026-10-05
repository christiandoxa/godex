package routing

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/christiandoxa/godex/internal/helper/fileutil"
	"github.com/christiandoxa/godex/internal/helper/lockfile"
)

const (
	responseTurnStateDirectory = ".godex-turn-state"
	responseTurnStateFiles     = 2048
	responseTurnStateMaxBytes  = 32 << 10
)

type responseTurnStateFile struct {
	Version   int    `json:"version"`
	ExpiresAt int64  `json:"expires_unix"`
	TurnState string `json:"turn_state"`
}

// SaveResponseTurnState keeps opaque continuation state inside its owning private CODEX_HOME.
func (store *Store) SaveResponseTurnState(
	ctx context.Context,
	profileHome, responseKey, turnState string,
	expiresAt time.Time,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validResponseTurnStateKey(responseKey) || turnState == "" || len(turnState) > 4<<10 ||
		strings.ContainsAny(turnState, "\r\n") || expiresAt.IsZero() || expiresAt.Unix() <= 0 {
		return errors.New("invalid response turn state")
	}
	directory, err := prepareResponseTurnStateDirectory(profileHome)
	if err != nil {
		return fmt.Errorf("prepare response turn state: %w", err)
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(directory, "guard"))
	if err != nil {
		return fmt.Errorf("lock response turn state: %w", err)
	}
	defer release()

	name := responseKey + ".json"
	path := filepath.Join(directory, name)
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		if err := makeResponseTurnStateRoom(directory); err != nil {
			return fmt.Errorf("make response turn state room: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("inspect response turn state: %w", err)
	}
	content, err := json.Marshal(responseTurnStateFile{
		Version: 1, ExpiresAt: expiresAt.Unix(), TurnState: turnState,
	})
	if err != nil {
		return err
	}
	if len(content) > responseTurnStateMaxBytes {
		return errors.New("response turn state is too large")
	}
	_, err = fileutil.AtomicWrite(path, content)
	if err != nil {
		return fmt.Errorf("write response turn state: %w", err)
	}
	return nil
}

// LoadResponseTurnState only returns state that is still within its recorded lifetime.
func (store *Store) LoadResponseTurnState(
	ctx context.Context,
	profileHome, responseKey string,
	now time.Time,
) (string, time.Time, error) {
	if err := ctx.Err(); err != nil {
		return "", time.Time{}, err
	}
	if !validResponseTurnStateKey(responseKey) {
		return "", time.Time{}, errors.New("invalid response turn state key")
	}
	directory, err := prepareResponseTurnStateDirectory(profileHome)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("prepare response turn state: %w", err)
	}
	path := filepath.Join(directory, responseKey+".json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", time.Time{}, nil
	}
	if err != nil {
		return "", time.Time{}, fmt.Errorf("inspect response turn state: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > responseTurnStateMaxBytes ||
		(runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0) {
		return "", time.Time{}, errors.New("response turn state file is unsafe")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("open response turn state: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, responseTurnStateMaxBytes+1))
	decoder.DisallowUnknownFields()
	var value responseTurnStateFile
	if err := decoder.Decode(&value); err != nil {
		return "", time.Time{}, fmt.Errorf("decode response turn state: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return "", time.Time{}, errors.New("response turn state file has trailing data")
	}
	if value.Version != 1 || value.TurnState == "" || len(value.TurnState) > 4<<10 ||
		strings.ContainsAny(value.TurnState, "\r\n") || value.ExpiresAt <= 0 {
		return "", time.Time{}, errors.New("invalid response turn state file")
	}
	expiresAt := time.Unix(value.ExpiresAt, 0)
	if !expiresAt.After(now) {
		return "", time.Time{}, nil
	}
	return value.TurnState, expiresAt, nil
}

// RemoveResponseTurnState deletes opaque continuation state for a response id.
func (store *Store) RemoveResponseTurnState(ctx context.Context, profileHome, responseKey string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validResponseTurnStateKey(responseKey) {
		return errors.New("invalid response turn state key")
	}
	root := filepath.Clean(profileHome)
	if !filepath.IsAbs(profileHome) || root == filepath.Dir(root) {
		return errors.New("invalid profile home for response turn state")
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() ||
		(runtime.GOOS != "windows" && rootInfo.Mode().Perm()&0o077 != 0) {
		return errors.New("profile home for response turn state must be a private directory")
	}
	directory := filepath.Join(root, responseTurnStateDirectory)
	directoryInfo, err := os.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if directoryInfo.Mode()&os.ModeSymlink != 0 || !directoryInfo.IsDir() ||
		(runtime.GOOS != "windows" && directoryInfo.Mode().Perm()&0o077 != 0) {
		return errors.New("response turn state directory is unsafe")
	}
	release, err := lockfile.Acquire(ctx, filepath.Join(directory, "guard"))
	if err != nil {
		return fmt.Errorf("lock response turn state: %w", err)
	}
	defer release()
	path := filepath.Join(directory, responseKey+".json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() ||
		(runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0) {
		return errors.New("response turn state file is unsafe")
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove response turn state: %w", err)
	}
	return nil
}

func prepareResponseTurnStateDirectory(profileHome string) (string, error) {
	if !filepath.IsAbs(profileHome) || filepath.Clean(profileHome) == filepath.Dir(filepath.Clean(profileHome)) {
		return "", errors.New("invalid profile home for response turn state")
	}
	info, err := os.Lstat(profileHome)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("profile home for response turn state must be a real directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("profile home for response turn state must be private")
	}
	directory := filepath.Join(profileHome, responseTurnStateDirectory)
	if err := os.Mkdir(directory, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	info, err = os.Lstat(directory)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("response turn state directory must be a real directory")
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return "", err
	}
	return directory, nil
}

func makeResponseTurnStateRoom(directory string) error {
	root, err := os.Open(directory)
	if err != nil {
		return err
	}
	entries, err := root.Readdirnames(responseTurnStateFiles + 2)
	closeErr := root.Close()
	if err != nil && err != io.EOF {
		return errors.Join(err, closeErr)
	}
	if closeErr != nil {
		return closeErr
	}
	if len(entries) > responseTurnStateFiles+1 {
		return errors.New("response turn state directory is over limit")
	}
	type file struct {
		name    string
		modTime time.Time
	}
	files := make([]file, 0, len(entries))
	for _, name := range entries {
		if !strings.HasSuffix(name, ".json") || !validResponseTurnStateKey(strings.TrimSuffix(name, ".json")) {
			continue
		}
		info, err := os.Lstat(filepath.Join(directory, name))
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			files = append(files, file{name: name, modTime: info.ModTime()})
		}
	}
	if len(files) < responseTurnStateFiles {
		return nil
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].modTime.Equal(files[j].modTime) {
			return files[i].name < files[j].name
		}
		return files[i].modTime.Before(files[j].modTime)
	})
	for _, oldest := range files[:len(files)-responseTurnStateFiles+1] {
		if err := os.Remove(filepath.Join(directory, oldest.name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func validResponseTurnStateKey(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
