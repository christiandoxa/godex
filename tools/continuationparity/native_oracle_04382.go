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

const manifestSHA04382 = "300a8f046041a95f9757e4cbdb72917d37927683b6e1af442492254e2f1a37da"

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
		"linux/arm64":   {"prodex-aarch64-unknown-linux-gnu", "4f743322e3b5653f47c52423263c769dfea543e24c1dfbddd1050942bfa2276d"},
		"darwin/amd64":  {"prodex-x86_64-apple-darwin", "14e13b57ac15bcc0ff826ffd471c74735ce8802a2f72a0e28a40516030015c7a"},
		"darwin/arm64":  {"prodex-aarch64-apple-darwin", "a7b8d4af5ba9454ea22df64f36a69d45b3de8b54a65ba7e38cfded0df7f7ced8"},
		"windows/amd64": {"prodex-x86_64-pc-windows-msvc.exe", "1f5ba38c456219e64044d4bc7b84fb16ca63b6385f6f2f493c357e693496417f"},
		"windows/arm64": {"prodex-aarch64-pc-windows-msvc.exe", "27da114337ce9cf799615e2aeddad332947263b2eb0f2cb1fec6447481fc2403"},
	}
	if pin, ok := pins[goos+"/"+arch]; ok {
		return pin[0], pin[1], nil
	}
	return "", "", fmt.Errorf("official Prodex 0.438.3 oracle unavailable for %s/%s", goos, arch)
}

func verifyProdexManifest04382(path, asset string) error {
	if sha256File(path) != manifestSHA04382 {
		return fmt.Errorf("Prodex release manifest digest mismatch for %s", asset)
	}
	return nil
}
