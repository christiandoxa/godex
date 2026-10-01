package profile

import "testing"

func TestValidateNameMatchesProdexSurface(t *testing.T) {
	for _, value := range []string{"main.profile_1", "Alpha-1"} {
		if err := ValidateName(value); err != nil {
			t.Fatalf("valid name %q: %v", value, err)
		}
	}
	for _, value := range []string{"", ".", "..", "bad/name", `bad\\name`, "bad name"} {
		if err := ValidateName(value); err == nil {
			t.Fatalf("invalid name %q accepted", value)
		}
	}
}

func TestResolveSourceMatchesProdexRules(t *testing.T) {
	tests := []struct {
		home, copy string
		current    bool
		want       SourceKind
		wantErr    bool
	}{
		{"/external", "", false, SourceExternalHome, false},
		{"", "/source", false, SourceCopyFrom, false},
		{"", "", true, SourceCopyCurrent, false},
		{"", "", false, SourceEmptyManaged, false},
		{"/external", "/source", false, 0, true},
		{"", "/source", true, 0, true},
	}
	for _, test := range tests {
		got, err := ResolveSource(test.home, test.copy, test.current)
		if (err != nil) != test.wantErr || (!test.wantErr && got != test.want) {
			t.Fatalf("source(%q,%q,%t) = %v, %v", test.home, test.copy, test.current, got, err)
		}
	}
}
