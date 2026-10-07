package codex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/christiandoxa/godex/internal/helper/fileutil"
	authmodel "github.com/christiandoxa/godex/internal/model/auth"
)

// ImportCurrent stages a private copy of the current Codex home. The source
// remains in place, and staged credentials match the identity snapshot.
func (process *CodexProcess) ImportCurrent(
	ctx context.Context,
	sourceHome string,
	stagedHome string,
	insecure bool,
) (authmodel.ImportCurrentIdentity, error) {
	identity, err := process.StageImportCurrentAuth(ctx, sourceHome, stagedHome, insecure)
	if err != nil {
		return authmodel.ImportCurrentIdentity{}, err
	}
	if err := process.CompleteImportCurrentHome(ctx, sourceHome, stagedHome); err != nil {
		return authmodel.ImportCurrentIdentity{}, err
	}
	return identity, nil
}

// StageImportCurrentAuth captures the source identity and exact auth snapshot without
// copying unrelated native state. Duplicate identities can therefore refresh auth
// without touching or depending on the rest of the source home.
func (process *CodexProcess) StageImportCurrentAuth(
	ctx context.Context,
	sourceHome string,
	stagedHome string,
	insecure bool,
) (authmodel.ImportCurrentIdentity, error) {
	if err := ctx.Err(); err != nil {
		return authmodel.ImportCurrentIdentity{}, err
	}
	if err := validateImportSource(sourceHome, insecure); err != nil {
		return authmodel.ImportCurrentIdentity{}, err
	}
	if err := ensureImportCurrentStagedHome(stagedHome); err != nil {
		return authmodel.ImportCurrentIdentity{}, err
	}

	authPath := filepath.Join(sourceHome, "auth.json")
	content, err := readImportCurrentAuthFile(authPath, insecure)
	if err != nil {
		return authmodel.ImportCurrentIdentity{}, err
	}
	defer clear(content)
	identity, err := chatGPTIdentity(content)
	if err != nil {
		return authmodel.ImportCurrentIdentity{}, fmt.Errorf("read current Codex login: %w", err)
	}
	if err := writePrivateFile(filepath.Join(stagedHome, "auth.json"), content); err != nil {
		return authmodel.ImportCurrentIdentity{}, fmt.Errorf("stage current Codex login: %w", err)
	}
	return authmodel.ImportCurrentIdentity{
		Email: identity.Email, ChatGPTAccountID: identity.ChatGPTAccountID,
	}, nil
}

// CompleteImportCurrentHome copies native state for a genuinely new identity while
// preserving the auth snapshot captured by StageImportCurrentAuth.
func (process *CodexProcess) CompleteImportCurrentHome(
	ctx context.Context,
	sourceHome string,
	stagedHome string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	authPath := filepath.Join(stagedHome, "auth.json")
	snapshot, err := readPrivateAuthFile(authPath)
	if err != nil {
		return fmt.Errorf("read staged current Codex login: %w", err)
	}
	defer clear(snapshot)
	if err := os.Remove(authPath); err != nil {
		return fmt.Errorf("prepare current Codex home copy: %w", err)
	}
	if err := fileutil.CopyCodexHome(sourceHome, stagedHome); err != nil {
		return fmt.Errorf("copy current Codex home: %w", err)
	}
	if err := writePrivateFile(authPath, snapshot); err != nil {
		return fmt.Errorf("stage current Codex login: %w", err)
	}
	configPath := filepath.Join(stagedHome, "config.toml")
	if _, err := os.Lstat(configPath); errors.Is(err, os.ErrNotExist) {
		if err := writePrivateFile(configPath, []byte(codexFileCredentialConfig)); err != nil {
			return fmt.Errorf("stage Codex config: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("inspect staged Codex config: %w", err)
	}
	return secureCodexHome(stagedHome)
}

func ensureImportCurrentStagedHome(path string) error {
	if err := validateCodexHomePath(path); err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create staged Codex home: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect staged Codex home: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("staged Codex home must be a real directory")
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("secure staged Codex home: %w", err)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("read staged Codex home: %w", err)
	}
	if len(entries) != 0 {
		return errors.New("staged Codex home must be empty")
	}
	return nil
}

func validateImportSource(path string, insecure bool) error {
	if err := validateCodexHomePath(path); err != nil {
		return fmt.Errorf("invalid current Codex home: %w", err)
	}
	clean := filepath.Clean(path)
	for current := clean; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect current Codex home path %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if !importCurrentSymlinkTrusted(info) {
				return fmt.Errorf("current Codex home path contains symbolic link or reparse point %s", current)
			}
		} else if !importCurrentEntryIsSafe(info) {
			return fmt.Errorf("current Codex home path contains symbolic link or reparse point %s", current)
		}
		if current == clean && !info.IsDir() {
			return errors.New("current Codex home must be a real directory")
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		if !insecure && info.IsDir() && !importCurrentDirectoryTrusted(current, info) {
			return fmt.Errorf("CODEX_HOME path %s is not trusted by the current user; pass --insecure to bypass this check", current)
		}
	}
	return nil
}

func readImportCurrentAuthFile(path string, insecure bool) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("read Codex auth profile: %w", err)
	}
	if !info.Mode().IsRegular() || !importCurrentEntryIsSafe(info) {
		return nil, errors.New("codex auth profile is not a non-reparse regular file")
	}
	if info.Size() > maxAuthFileSize {
		return nil, errors.New("codex auth profile is too large")
	}
	if !insecure && !importCurrentPrivateFileTrusted(path, info) {
		return nil, errors.New("codex auth profile is not private to the current user; pass --insecure to bypass this check")
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read Codex auth profile: %w", err)
	}
	openedInfo, statErr := file.Stat()
	if statErr != nil {
		_ = file.Close()
		return nil, fmt.Errorf("read Codex auth profile metadata: %w", statErr)
	}
	currentInfo, statErr := os.Lstat(path)
	if statErr != nil {
		_ = file.Close()
		return nil, fmt.Errorf("read Codex auth profile metadata: %w", statErr)
	}
	if !currentInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) || !os.SameFile(info, currentInfo) {
		_ = file.Close()
		return nil, errors.New("codex auth profile changed while opening")
	}

	content, readErr := io.ReadAll(io.LimitReader(file, maxAuthFileSize+1))
	closeErr := file.Close()
	if readErr != nil {
		clear(content)
		return nil, fmt.Errorf("read Codex auth profile: %w", readErr)
	}
	if closeErr != nil {
		clear(content)
		return nil, fmt.Errorf("close Codex auth profile: %w", closeErr)
	}
	if len(content) > maxAuthFileSize {
		clear(content)
		return nil, errors.New("codex auth profile is too large")
	}
	return content, nil
}
