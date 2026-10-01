package runtime

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDoctorBundleStoreWritesPrivateAtomicFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "doctor.json")
	written, err := NewDoctorBundleStore().Write(path, []byte(`{"bundle":"synthetic"}`))
	if err != nil || written != path {
		t.Fatalf("written=%q err=%v", written, err)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "{\"bundle\":\"synthetic\"}\n" {
		t.Fatalf("content=%q err=%v", content, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("mode=%v err=%v", info.Mode().Perm(), err)
		}
	}
}

func TestDoctorBundleStoreRejectsUnsafeTargets(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "directory.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := NewDoctorBundleStore().Write(filepath.Join(root, "directory.json"), []byte("{}")); err == nil {
		t.Fatal("directory target unexpectedly accepted")
	}
	if runtime.GOOS != "windows" {
		target := filepath.Join(root, "target.json")
		if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(root, "link.json")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if _, err := NewDoctorBundleStore().Write(link, []byte("{}")); err == nil {
			t.Fatal("symlink target unexpectedly accepted")
		}
	}
}
