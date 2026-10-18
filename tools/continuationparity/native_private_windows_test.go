package main

import (
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestProdex04382FixtureWindowsSecretsHaveProtectedOwnerACL(t *testing.T) {
	for _, kind := range []string{"auth", "state"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), kind+".json")
			var err error
			if kind == "auth" {
				err = writeAuth(path, "synthetic-account", "private-fixture-value")
			} else {
				err = writeJSONFile(path, map[string]any{"generation": 1})
			}
			if err != nil {
				t.Fatal(err)
			}
			descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
			if err != nil {
				t.Fatal(err)
			}
			user, err := windows.GetCurrentProcessToken().GetTokenUser()
			if err != nil {
				t.Fatal(err)
			}
			owner, _, ownerErr := descriptor.Owner()
			control, _, controlErr := descriptor.Control()
			dacl, _, aclErr := descriptor.DACL()
			if ownerErr != nil || controlErr != nil || aclErr != nil || owner == nil || !owner.Equals(user.User.Sid) || control&windows.SE_DACL_PROTECTED == 0 || dacl == nil || dacl.AceCount != 1 {
				t.Fatal("fixture secret has no protected current-user ACL")
			}
			var ace *windows.ACCESS_ALLOWED_ACE
			if err := windows.GetAce(dacl, 0, &ace); err != nil || ace == nil || !(*windows.SID)(unsafe.Pointer(&ace.SidStart)).Equals(user.User.Sid) {
				t.Fatal("fixture secret ACL grants access to a different principal")
			}
		})
	}
}
