package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/christiandoxa/godex/internal/helper/redact"
)

const manifestSHA04382 = "28b7a894cd10fb69c528245e25dc01b9f204cfcc78b5ee575e22b4967c72c650"

func createFixtureRoot() (string, error) {
	root, err := os.MkdirTemp("", "godex-continuation-parity-")
	if err != nil {
		return "", err
	}
	// macOS's default /var temporary path is a symlink rejected for secrets.
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		_ = os.RemoveAll(root)
		return "", fmt.Errorf("canonicalize fixture root: %w", err)
	}
	return canonical, nil
}

func writeFixtureSecurityMetadata04382(output io.Writer, root string) {
	printPath := func(path string) {
		info, err := os.Lstat(path)
		if err == nil {
			fmt.Fprintf(output, "fixture_security path=%q mode=%s metadata=%+v acl=%s\n", path, info.Mode(), info.Sys(), fixtureSecurityMetadata(path))
		}
	}
	for path, count := root, 0; count < 12; count++ {
		printPath(path)
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
		path = parent
	}
	remaining := 32
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fs.SkipDir
		}
		if entry.IsDir() || entry.Name() == "auth.json" || entry.Name() == "runtime-usage-snapshots.json" {
			printPath(path)
			remaining--
		}
		if remaining == 0 {
			return fs.SkipAll
		}
		return nil
	})
}

func writeProdexFailureLogs04382(output io.Writer, root string) {
	for _, pattern := range []string{
		filepath.Join(root, "prodex", "state", "logs", "prodex-runtime-*.log"),
		filepath.Join(root, "*", "prodex", "state", "logs", "prodex-runtime-*.log"),
	} {
		paths, _ := filepath.Glob(pattern)
		for _, path := range paths[:min(len(paths), 4)] {
			file, err := os.Open(path)
			if err != nil {
				continue
			}
			data, _ := io.ReadAll(io.LimitReader(file, 16<<10))
			_ = file.Close()
			fmt.Fprintln(output, redact.Secrets(string(data)))
		}
	}
}

func nativeProdexPin() (string, string, error) {
	return prodexPlatformPin04382(runtime.GOOS, runtime.GOARCH)
}

func prodexPlatformPin04382(goos, arch string) (string, string, error) {
	pins := map[string][2]string{
		"linux/amd64":   {"prodex-x86_64-unknown-linux-gnu", prodexSHA},
		"linux/arm64":   {"prodex-aarch64-unknown-linux-gnu", "d9ca7888a62409c4be1bd5923b9ea52e3a9b4f9b1af3fee2b1a5e852558f1097"},
		"darwin/amd64":  {"prodex-x86_64-apple-darwin", "514c066af8c78e2e45a14be5db6a9aa7cf5a45c387db53796f2b10823d5515d0"},
		"darwin/arm64":  {"prodex-aarch64-apple-darwin", "1dc6957b2659faece91d5c8234f4e4f7ea9a8f7ddbea68c2d9041042157538a6"},
		"windows/amd64": {"prodex-x86_64-pc-windows-msvc.exe", "6f852a4c523bf301c11e88d83ed2ee17467a175942ffc00e6b656d4d50e6c7c3"},
		"windows/arm64": {"prodex-aarch64-pc-windows-msvc.exe", "f57893b66fb7ae620f65ef11a0026f04cd0518d566adf21efe2fa668f7dc451c"},
	}
	if pin, ok := pins[goos+"/"+arch]; ok {
		return pin[0], pin[1], nil
	}
	return "", "", fmt.Errorf("official Prodex 0.438.4 oracle unavailable for %s/%s", goos, arch)
}

func verifyProdexManifest04382(path, asset string) error {
	if sha256File(path) != manifestSHA04382 {
		return fmt.Errorf("Prodex release manifest digest mismatch for %s", asset)
	}
	return nil
}
