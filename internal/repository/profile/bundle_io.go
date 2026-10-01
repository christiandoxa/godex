package profile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/christiandoxa/godex/internal/helper/fileutil"
)

const bundleReadErrorFormat = "failed to read %s"

func WriteProfileBundle(path string, content []byte) error {
	if len(content) > profileExportBundleMaxBytes {
		return fmt.Errorf("profile export bundle %s exceeds safe size limit (%d bytes)", path, profileExportBundleMaxBytes)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := ensurePrivateBundleParent(filepath.Dir(absolute)); err != nil {
		return err
	}
	if info, err := os.Lstat(absolute); err == nil && info.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(absolute); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(absolute), ".profile-export-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(content); err != nil {
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
	if err := fileutil.Replace(temporary, absolute); err != nil {
		return err
	}
	return fileutil.SyncDirectory(filepath.Dir(absolute))
}

func ReadProfileBundle(path string) ([]byte, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := ensurePrivateBundleParent(filepath.Dir(absolute)); err != nil {
		return nil, err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, fmt.Errorf(bundleReadErrorFormat, absolute)
	}
	if !info.Mode().IsRegular() || info.Size() > profileExportBundleMaxBytes {
		return nil, fmt.Errorf("profile export bundle %s exceeds safe size limit (%d bytes)", absolute, profileExportBundleMaxBytes)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o022 != 0 {
		return nil, fmt.Errorf("profile export bundle %s must be private", absolute)
	}
	file, err := os.Open(absolute)
	if err != nil {
		return nil, fmt.Errorf(bundleReadErrorFormat, absolute)
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, profileExportBundleMaxBytes+1))
	if err != nil || len(content) > profileExportBundleMaxBytes {
		return nil, fmt.Errorf(bundleReadErrorFormat, absolute)
	}
	return content, nil
}

func ensurePrivateBundleParent(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("profile export directory must be a real directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o022 != 0 {
		return errors.New("profile export directory must not be writable by group or others")
	}
	return nil
}
