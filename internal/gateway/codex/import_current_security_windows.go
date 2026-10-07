//go:build windows

package codex

import (
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func importCurrentEntryIsSafe(info os.FileInfo) bool {
	attributes, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return !ok || attributes.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT == 0
}

func importCurrentSymlinkTrusted(os.FileInfo) bool { return false }

func importCurrentDirectoryTrusted(path string, _ os.FileInfo) bool {
	return importCurrentWindowsACLTrusted(path, false)
}

func importCurrentPrivateFileTrusted(path string, _ os.FileInfo) bool {
	return importCurrentWindowsACLTrusted(path, true)
}

const (
	importCurrentFileAddFile         = 0x0002
	importCurrentFileAddSubdirectory = 0x0004
	importCurrentFileDeleteChild     = 0x0040
)

func importCurrentWindowsACLTrusted(path string, privateFile bool) bool {
	descriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil || descriptor == nil {
		return false
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil {
		return false
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		return false
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil {
		return false
	}
	if privateFile {
		if !owner.Equals(user.User.Sid) {
			return false
		}
		control, _, err := descriptor.Control()
		if err != nil || control&windows.SE_DACL_PRESENT == 0 || control&windows.SE_DACL_PROTECTED == 0 {
			return false
		}
	} else if !importCurrentWindowsPrincipalTrusted(owner, user.User.Sid) {
		return false
	}

	sensitive := uint32(importCurrentFileAddFile | importCurrentFileAddSubdirectory | importCurrentFileDeleteChild |
		windows.DELETE | windows.WRITE_DAC | windows.WRITE_OWNER | windows.GENERIC_ALL | windows.GENERIC_WRITE)
	if privateFile {
		sensitive = uint32(windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE | windows.DELETE |
			windows.WRITE_DAC | windows.WRITE_OWNER | windows.GENERIC_ALL | windows.GENERIC_READ | windows.GENERIC_WRITE)
	}
	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil || ace == nil {
			return false
		}
		header := (*windows.ACE_HEADER)(unsafe.Pointer(ace))
		if uint32(header.AceFlags)&windows.INHERIT_ONLY_ACE != 0 || header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return false
		}
		if uint32(ace.Mask)&sensitive == 0 {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !importCurrentWindowsPrincipalTrusted(sid, user.User.Sid) {
			return false
		}
	}
	return true
}

func importCurrentWindowsPrincipalTrusted(candidate, user *windows.SID) bool {
	return candidate != nil && user != nil && (candidate.Equals(user) ||
		candidate.IsWellKnown(windows.WinLocalSystemSid) ||
		candidate.IsWellKnown(windows.WinBuiltinAdministratorsSid))
}
