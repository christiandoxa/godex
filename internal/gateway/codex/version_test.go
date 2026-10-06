package codex

import "testing"

func TestRequireSupportedVersion(t *testing.T) {
	for _, test := range []struct {
		output  string
		wantErr bool
	}{
		{output: "codex-cli 0.153.2"},
		{output: "codex-cli 0.159.2"},
		{output: "codex-cli 0.160.1"},
		{output: "codex 1.0.0"},
		{output: "codex-cli 0.153.1", wantErr: true},
		{output: "codex-cli 0.152.99", wantErr: true},
		{output: "unexpected", wantErr: true},
	} {
		t.Run(test.output, func(t *testing.T) {
			err := requireSupportedVersion(test.output)
			if (err != nil) != test.wantErr {
				t.Fatalf("requireSupportedVersion(%q) error = %v", test.output, err)
			}
		})
	}
}

func TestParseCodexVersionIgnoresSurroundingText(t *testing.T) {
	version, err := parseCodexVersion("official codex-cli 0.159.2 (linux)")
	if err != nil {
		t.Fatal(err)
	}
	if version.String() != "0.159.2" {
		t.Fatalf("version = %s", version.String())
	}
}

func TestAuditedVersionIsParseable(t *testing.T) {
	version, err := parseCodexVersion(AuditedVersion)
	if err != nil {
		t.Fatal(err)
	}
	if version.String() != "0.160.1" {
		t.Fatalf("audited version = %s", version.String())
	}
}
