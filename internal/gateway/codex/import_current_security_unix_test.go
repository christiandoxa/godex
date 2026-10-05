//go:build linux || darwin

package codex

import (
	"os"
	"syscall"
	"testing"
)

type importCurrentSecurityInfo struct {
	os.FileInfo
	stat syscall.Stat_t
}

func (info importCurrentSecurityInfo) Sys() any { return &info.stat }

func TestImportCurrentUnixOwnerPolicyMatchesProdex(t *testing.T) {
	path := t.TempDir()
	base, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	actual, ok := base.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatal("temp directory stat is not syscall.Stat_t")
	}
	euid := uint32(os.Geteuid())
	foreign := euid + 1
	if foreign == 0 {
		foreign = euid + 2
	}

	currentPrivate := importCurrentSecurityInfo{FileInfo: modeFileInfo{FileInfo: base, mode: os.ModeDir | 0o700}, stat: *actual}
	currentPrivate.stat.Uid = euid
	if !importCurrentDirectoryTrusted("/synthetic", currentPrivate) {
		t.Fatal("current-user private directory rejected")
	}

	foreignPrivate := currentPrivate
	foreignPrivate.stat.Uid = foreign
	if importCurrentDirectoryTrusted("/synthetic", foreignPrivate) {
		t.Fatal("foreign-owned private directory accepted")
	}

	rootSticky := importCurrentSecurityInfo{FileInfo: modeFileInfo{FileInfo: base, mode: os.ModeDir | os.ModeSticky | 0o777}, stat: *actual}
	rootSticky.stat.Uid = 0
	if !importCurrentDirectoryTrusted("/synthetic", rootSticky) {
		t.Fatal("root-owned sticky directory rejected")
	}
	rootWritable := rootSticky
	rootWritable.FileInfo = modeFileInfo{FileInfo: base, mode: os.ModeDir | 0o777}
	if importCurrentDirectoryTrusted("/synthetic", rootWritable) {
		t.Fatal("root-owned non-sticky writable directory accepted")
	}

	currentFile := importCurrentSecurityInfo{FileInfo: modeFileInfo{FileInfo: base, mode: 0o600}, stat: *actual}
	currentFile.stat.Uid = euid
	if !importCurrentPrivateFileTrusted("/synthetic/auth.json", currentFile) {
		t.Fatal("current-user private auth file rejected")
	}
	foreignFile := currentFile
	foreignFile.stat.Uid = foreign
	if importCurrentPrivateFileTrusted("/synthetic/auth.json", foreignFile) {
		t.Fatal("foreign-owned auth file accepted")
	}
	permissiveFile := currentFile
	permissiveFile.FileInfo = modeFileInfo{FileInfo: base, mode: 0o640}
	if importCurrentPrivateFileTrusted("/synthetic/auth.json", permissiveFile) {
		t.Fatal("group-readable auth file accepted")
	}
}

type modeFileInfo struct {
	os.FileInfo
	mode os.FileMode
}

func (info modeFileInfo) Mode() os.FileMode { return info.mode }
