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

const manifestSHA04382 = "ba2f1f8d3430bd3fe64addd886ebee1207bf59b9db0d0e34b834b0dc7a0008f8"

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
		"linux/arm64":   {"prodex-aarch64-unknown-linux-gnu", "4d9a3ccb61c9f779083ef7dee3a0d363dc0c027824e37acabeb66e469e5ed25f"},
		"darwin/amd64":  {"prodex-x86_64-apple-darwin", "6dc28e85b458cc29dfa164ca6ab00a1da4e5b00a01ba35957283dec5f82c3a4e"},
		"darwin/arm64":  {"prodex-aarch64-apple-darwin", "c5be320524fb436f159026f7907a3daedfcd397fb795f07d7cbc51ff5061a276"},
		"windows/amd64": {"prodex-x86_64-pc-windows-msvc.exe", "e75aa83ce901fbdd593b729038e738ff7c30c152b4373075129af36e84de9a85"},
		"windows/arm64": {"prodex-aarch64-pc-windows-msvc.exe", "804c0bf905cdecee2eb029efa0e492d6f2fea5b177f3c6302cc66f07895b9656"},
	}
	if pin, ok := pins[goos+"/"+arch]; ok {
		return pin[0], pin[1], nil
	}
	return "", "", fmt.Errorf("official Prodex 0.439.0 oracle unavailable for %s/%s", goos, arch)
}

func verifyProdexManifest04382(path, asset string) error {
	if sha256File(path) != manifestSHA04382 {
		return fmt.Errorf("Prodex release manifest digest mismatch for %s", asset)
	}
	return nil
}
