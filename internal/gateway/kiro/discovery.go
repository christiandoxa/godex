package kiro

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const kiroDatabaseFileName = "data.sqlite3"

func (source *Source) discoverDatabasePath() (string, error) {
	if path := source.explicitDatabasePath(); path != "" {
		return path, nil
	}
	for _, candidate := range source.defaultDatabaseCandidates() {
		if regularSourceFile(candidate) {
			return filepath.Clean(candidate), nil
		}
	}
	return "", errors.New("failed to find Kiro auth database; expected ~/.local/share/kiro-cli/data.sqlite3 or ~/.local/share/amazon-q/data.sqlite3")
}

func (source *Source) explicitDatabasePath() string {
	if path := strings.TrimSpace(source.getenv("KIRO_TEST_DB_PATH")); path != "" && regularSourceFile(path) {
		return filepath.Clean(path)
	}
	for _, variable := range []string{"KIRO_DATA_DIR", "Q_CLI_DATA_DIR"} {
		root := strings.TrimSpace(source.getenv(variable))
		if root == "" {
			continue
		}
		candidate := filepath.Join(root, kiroDatabaseFileName)
		if regularSourceFile(candidate) {
			return filepath.Clean(candidate)
		}
	}
	return ""
}

func (source *Source) defaultDatabaseCandidates() []string {
	candidates := make([]string, 0, 6)
	if local := strings.TrimSpace(source.dataLocalDir()); local != "" {
		candidates = append(candidates,
			filepath.Join(local, "kiro-cli", kiroDatabaseFileName),
			filepath.Join(local, "amazon-q", kiroDatabaseFileName),
		)
	}
	home, err := source.homeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return candidates
	}
	for _, name := range []string{"kiro-cli", "amazon-q"} {
		candidates = append(candidates, filepath.Join(home, ".local", "share", name, kiroDatabaseFileName))
	}
	return candidates
}

func regularSourceFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink == 0 && info.Mode().IsRegular()
}

func ensureRegularDatabase(path string) error {
	if !regularSourceFile(path) {
		return fmt.Errorf("Kiro auth database %s must be a regular file", path)
	}
	return nil
}
