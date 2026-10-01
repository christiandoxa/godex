package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBoundedUpdateOutputRedactsSecretsAndControls(t *testing.T) {
	secret := "secret-sentinel-value"
	input := "Authorization: Bearer " + secret + "\x1b OPENAI_API_KEY=" + secret + " " + strings.Repeat("x", updateRenderedMaxRunes+100)
	output := boundedUpdateOutput(input)
	if strings.Contains(output, secret) || strings.ContainsRune(output, '\x1b') || !strings.Contains(output, "output truncated") {
		t.Fatalf("unsafe update output = %q", output)
	}
}

func TestUpdateEnvironmentPinsRunningExecutableDirectory(t *testing.T) {
	running := filepath.Join(t.TempDir(), "bin", "godex")
	if runtime.GOOS == "windows" {
		running += ".exe"
	}
	environment := updateEnvironment(running, "1.2.3")
	joined := strings.Join(environment, "\n")
	for _, want := range []string{"GODEX_VERSION=1.2.3", "GODEX_INSTALL_DIR=" + filepath.Dir(running), "GODEX_RUNNING_EXE=" + running, "GODEX_MIGRATE=1", "GODEX_NON_INTERACTIVE=1"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("environment missing %q", want)
		}
	}
}

func TestInstallerProbesGodexVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	path := filepath.Join(t.TempDir(), "godex")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf 'godex 1.2.3 (commit fixture, built now)\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	version, err := NewInstaller().ProbeVersion(context.Background(), path)
	if err != nil || version != "1.2.3" {
		t.Fatalf("version = %q, err = %v", version, err)
	}
}

func TestInstallerUsesVerifiedReleaseArchive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix installer fixture")
	}
	archiveName := releaseArchiveName(t, "1.2.3")
	archive := fakeGodexArchive(t, "1.2.3")
	digest := sha256.Sum256(archive)
	checksums := fmt.Sprintf("%x  %s\n", digest, archiveName)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1.2.3/" + archiveName:
			_, _ = writer.Write(archive)
		case "/v1.2.3/checksums.txt":
			_, _ = io.WriteString(writer, checksums)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("GODEX_RELEASE_BASE_URL", server.URL)

	directory := t.TempDir()
	running := filepath.Join(directory, "godex")
	if err := os.WriteFile(running, []byte("#!/bin/sh\nprintf 'godex 1.2.2 (commit old, built now)\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	result, err := NewInstaller().Install(context.Background(), running, "1.2.3")
	if err != nil {
		t.Fatalf("install failed: %v; stdout=%q stderr=%q", err, result.Stdout, result.Stderr)
	}
	output, err := exec.Command(running, "--version").CombinedOutput()
	if err != nil || !strings.Contains(string(output), "godex 1.2.3") {
		t.Fatalf("updated binary = %q, err = %v", output, err)
	}
}

func releaseArchiveName(t *testing.T, version string) string {
	t.Helper()
	osName := runtime.GOOS
	arch := runtime.GOARCH
	if osName != "linux" && osName != "darwin" {
		t.Skip("Unix installer fixture")
	}
	if arch != "amd64" && arch != "arm64" {
		t.Skip("release fixture supports amd64/arm64")
	}
	return fmt.Sprintf("godex_%s_%s_%s.tar.gz", version, osName, arch)
}

func fakeGodexArchive(t *testing.T, version string) []byte {
	t.Helper()
	var compressed bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressed)
	tarWriter := tar.NewWriter(gzipWriter)
	content := []byte("#!/bin/sh\nprintf 'godex " + version + " (commit fixture, built now)\\n'\n")
	if err := tarWriter.WriteHeader(&tar.Header{Name: "godex", Mode: 0o755, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return compressed.Bytes()
}
