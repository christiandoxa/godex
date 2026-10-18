package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/christiandoxa/godex/internal/helper/redact"
)

const manifestSHA04382 = "93b5b5f90d59b23f800e47d17127ee7671d8a16ef72c6cabf30edd1093617725"

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
		"linux/arm64":   {"prodex-aarch64-unknown-linux-gnu", "d1c18604695bd5b457872103e3671f2c3ab47fdf01d383a23d3d02df18254106"},
		"darwin/amd64":  {"prodex-x86_64-apple-darwin", "36ac2ab9f12652503d0c96369bd5cf21f3fe9275677af883c18ad4df108cf412"},
		"darwin/arm64":  {"prodex-aarch64-apple-darwin", "e7d878c1570bc62113b3683bdd21d9685c2050814f0467e5bd499da82051a406"},
		"windows/amd64": {"prodex-x86_64-pc-windows-msvc.exe", "61de5108f1e3f97a32cc132ade3847199042b438acfd61a2c6f5d0a30ebb9448"},
		"windows/arm64": {"prodex-aarch64-pc-windows-msvc.exe", "7a1690400b3b0a32dcf571f347a2871a5bb66fc0c084a95a3b72097576ec8554"},
	}
	if pin, ok := pins[goos+"/"+arch]; ok {
		return pin[0], pin[1], nil
	}
	return "", "", fmt.Errorf("official Prodex 0.438.2 oracle unavailable for %s/%s", goos, arch)
}

func verifyProdexManifest04382(path, asset string) error {
	if sha256File(path) != manifestSHA04382 {
		return fmt.Errorf("Prodex release manifest digest mismatch for %s", asset)
	}
	return nil
}
