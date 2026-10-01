package runtime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/christiandoxa/godex/internal/helper/fileutil"
)

const doctorBundleMaxBytes = 4 << 20

type DoctorBundleStore struct{}

func NewDoctorBundleStore() *DoctorBundleStore { return &DoctorBundleStore{} }

func (*DoctorBundleStore) Write(path string, content []byte) (string, error) {
	if len(content) > doctorBundleMaxBytes {
		return "", fmt.Errorf("doctor bundle exceeds safe size limit (%d bytes)", doctorBundleMaxBytes)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", errors.New("resolve doctor bundle path")
	}
	parent := filepath.Dir(absolute)
	parentInfo, err := os.Lstat(parent)
	if err != nil {
		return "", errors.New("inspect doctor bundle directory")
	}
	if !parentInfo.IsDir() || parentInfo.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("doctor bundle directory must be a real directory")
	}
	if info, err := os.Lstat(absolute); err == nil {
		if !info.Mode().IsRegular() {
			return "", errors.New("doctor bundle target must be a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", errors.New("inspect doctor bundle target")
	}
	payload := append(append([]byte(nil), content...), '\n')
	if _, err := fileutil.AtomicWrite(absolute, payload); err != nil {
		return "", fmt.Errorf("write doctor bundle: %w", err)
	}
	if err := os.Chmod(absolute, 0o600); err != nil {
		return "", fmt.Errorf("secure doctor bundle: %w", err)
	}
	return absolute, nil
}
