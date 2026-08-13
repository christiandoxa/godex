package version

import "testing"

func TestStringIncludesBuildMetadata(t *testing.T) {
	original := [3]string{Version, Commit, Date}
	Version, Commit, Date = "1.2.3", "synthetic-commit", "synthetic-date"
	t.Cleanup(func() { Version, Commit, Date = original[0], original[1], original[2] })
	if got := String(); got != "godex 1.2.3 (commit synthetic-commit, built synthetic-date)" {
		t.Fatalf("version string = %q", got)
	}
}
