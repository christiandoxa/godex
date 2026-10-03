package profile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/christiandoxa/godex/internal/helper/fileutil"
)

type importRollbackManifest struct {
	Version int                          `json:"version"`
	Files   []importRollbackManifestFile `json:"files"`
}

type importRollbackManifestFile struct {
	Path    string `json:"path"`
	Backup  string `json:"backup,omitempty"`
	Existed bool   `json:"existed"`
}

func prepareImportRollback(home, id string, paths []string) error {
	backupRoot := filepath.Join(home, bundleImportBackupDir+id)
	if err := os.Mkdir(backupRoot, 0o700); err != nil {
		return fmt.Errorf("create profile import rollback directory: %w", err)
	}
	manifest := importRollbackManifest{Version: bundleImportVersion, Files: make([]importRollbackManifestFile, 0, len(paths))}
	for _, name := range paths {
		file, err := prepareImportRollbackFile(home, backupRoot, name, manifest.Files)
		if err != nil {
			return err
		}
		manifest.Files = append(manifest.Files, file)
	}
	return writeImportRollbackManifest(backupRoot, manifest)
}

func prepareImportRollbackFile(
	home, backupRoot, name string,
	previous []importRollbackManifestFile,
) (importRollbackManifestFile, error) {
	if err := validateImportRollbackFile(name); err != nil {
		return importRollbackManifestFile{}, err
	}
	for _, file := range previous {
		if file.Path == name {
			return importRollbackManifestFile{}, fmt.Errorf("duplicate profile import rollback file %q", name)
		}
	}
	entry := importRollbackManifestFile{Path: name, Backup: fmt.Sprintf("%d", len(previous))}
	content, existed, err := readImportRollbackSource(filepath.Join(home, name), name)
	if err != nil {
		return importRollbackManifestFile{}, err
	}
	if !existed {
		entry.Backup = ""
		return entry, nil
	}
	defer clearBytes(content)
	if _, err := fileutil.AtomicWrite(filepath.Join(backupRoot, entry.Backup), content); err != nil {
		return importRollbackManifestFile{}, fmt.Errorf("save profile import rollback backup: %w", err)
	}
	entry.Existed = true
	return entry, nil
}

func readImportRollbackSource(path, name string) ([]byte, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxProfileAuthBytes || info.Mode()&os.ModeSymlink != 0 {
		return nil, false, fmt.Errorf("profile import rollback source %q is unavailable", name)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, false, fmt.Errorf("profile import rollback source %q is not private", name)
	}
	content, err := os.ReadFile(path)
	if err != nil || len(content) > maxProfileAuthBytes {
		clearBytes(content)
		return nil, false, fmt.Errorf("profile import rollback source %q is unavailable", name)
	}
	return content, true, nil
}

func writeImportRollbackManifest(root string, manifest importRollbackManifest) error {
	content, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	if _, err := fileutil.AtomicWrite(filepath.Join(root, "manifest.json"), content); err != nil {
		return fmt.Errorf("save profile import rollback manifest: %w", err)
	}
	return fileutil.SyncDirectory(root)
}

func restoreImportRollback(home, id string) error {
	backupRoot := filepath.Join(home, bundleImportBackupDir+id)
	manifest, err := readImportRollbackManifest(backupRoot)
	if err != nil {
		return err
	}
	for _, file := range manifest.Files {
		if err := validateImportRollbackFile(file.Path); err != nil {
			return err
		}
		path := filepath.Join(home, file.Path)
		if !file.Existed {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove imported profile file: %w", err)
			}
			continue
		}
		backup, err := readImportBackup(filepath.Join(backupRoot, file.Backup))
		if err != nil {
			return err
		}
		_, writeErr := fileutil.AtomicWrite(path, backup)
		clearBytes(backup)
		if writeErr != nil {
			return fmt.Errorf("restore imported profile file: %w", writeErr)
		}
	}
	return nil
}

func readImportRollbackManifest(root string) (importRollbackManifest, error) {
	if err := validateImportRollbackRoot(root); err != nil {
		return importRollbackManifest{}, err
	}
	content, err := readImportRollbackManifestBytes(filepath.Join(root, "manifest.json"))
	if err != nil {
		return importRollbackManifest{}, err
	}
	manifest, err := decodeImportRollbackManifest(content)
	if err != nil {
		return importRollbackManifest{}, err
	}
	if err := validateImportRollbackManifestFiles(manifest); err != nil {
		return importRollbackManifest{}, err
	}
	return manifest, nil
}

func validateImportRollbackRoot(root string) error {
	info, err := os.Lstat(root)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("profile import rollback backup is unavailable")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return errors.New("profile import rollback backup is not private")
	}
	return nil
}

func readImportRollbackManifestBytes(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 64<<10 || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("profile import rollback manifest is unavailable")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("profile import rollback manifest is not private")
	}
	content, err := os.ReadFile(path)
	if err != nil || len(content) > 64<<10 {
		return nil, errors.New("profile import rollback manifest is unavailable")
	}
	return content, nil
}

func decodeImportRollbackManifest(content []byte) (importRollbackManifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var manifest importRollbackManifest
	if err := decoder.Decode(&manifest); err != nil ||
		requireBundleJournalEOF(decoder) != nil ||
		manifest.Version != bundleImportVersion ||
		len(manifest.Files) > 16 {
		return importRollbackManifest{}, errors.New("invalid profile import rollback manifest")
	}
	return manifest, nil
}

func validateImportRollbackManifestFiles(manifest importRollbackManifest) error {
	seen := make(map[string]bool, len(manifest.Files))
	for index, file := range manifest.Files {
		if err := validateImportRollbackManifestFile(index, file, seen); err != nil {
			return err
		}
	}
	return nil
}

func validateImportRollbackManifestFile(index int, file importRollbackManifestFile, seen map[string]bool) error {
	if err := validateImportRollbackFile(file.Path); err != nil || seen[file.Path] {
		return errors.New("invalid profile import rollback manifest file")
	}
	seen[file.Path] = true
	switch {
	case file.Existed && file.Backup != fmt.Sprintf("%d", index):
		return errors.New("invalid profile import rollback backup name")
	case !file.Existed && file.Backup != "":
		return errors.New("invalid profile import rollback backup name")
	default:
		return nil
	}
}

func readImportBackup(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxProfileAuthBytes || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("profile import rollback file is unavailable")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("profile import rollback file is not private")
	}
	content, err := os.ReadFile(path)
	if err != nil || len(content) > maxProfileAuthBytes {
		clearBytes(content)
		return nil, errors.New("profile import rollback file is unavailable")
	}
	return content, nil
}

func removeImportRollback(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove profile import rollback backup: %w", err)
	}
	return nil
}

func removeManagedImportHome(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("imported profile home is unavailable")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return errors.New("imported profile home is not private")
	}
	return os.RemoveAll(path)
}

func validateImportRollbackFile(name string) error {
	if name == profileAuthFileName {
		return nil
	}
	return validateSecretName(name)
}
