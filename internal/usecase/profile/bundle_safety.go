package profile

import "path/filepath"

func clearBundleBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}

func cleanBundlePath(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return filepath.Clean(absolute)
}
