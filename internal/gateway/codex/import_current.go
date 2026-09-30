package codex

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	entity "github.com/christiandoxa/godex/internal/entity/account"
)

// ImportCurrent stages the current Codex ChatGPT login without taking ownership
// of the source profile. Sessions, history, and other Codex state remain in the
// source home; Godex imports only the authentication needed for an isolated
// managed profile.
func (process *CodexProcess) ImportCurrent(
	ctx context.Context,
	sourceHome string,
	stagedHome string,
) (entity.Identity, error) {
	if err := ctx.Err(); err != nil {
		return entity.Identity{}, err
	}
	if err := validateImportSource(sourceHome); err != nil {
		return entity.Identity{}, err
	}

	authPath := filepath.Join(sourceHome, "auth.json")
	content, err := readPrivateAuthFile(authPath)
	if err != nil {
		return entity.Identity{}, err
	}
	defer clear(content)
	// Metadata and staged credentials must describe the same native snapshot.
	identity, err := chatGPTIdentity(content)
	if err != nil {
		return entity.Identity{}, fmt.Errorf("read current Codex login: %w", err)
	}

	if err := prepareCodexHome(stagedHome); err != nil {
		return entity.Identity{}, err
	}
	if err := writePrivateFile(filepath.Join(stagedHome, "auth.json"), content); err != nil {
		return entity.Identity{}, fmt.Errorf("stage current Codex login: %w", err)
	}
	if err := secureCodexHome(stagedHome); err != nil {
		return entity.Identity{}, err
	}
	return identity, nil
}

func validateImportSource(path string) error {
	if err := validateCodexHomePath(path); err != nil {
		return fmt.Errorf("invalid current Codex home: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect current Codex home: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("current Codex home must be a real directory")
	}
	return nil
}
