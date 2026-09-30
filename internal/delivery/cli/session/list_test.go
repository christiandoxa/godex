package session

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

func TestArgumentsValidateFiltersAndOutputModes(t *testing.T) {
	for _, args := range [][]string{{"--json", "--id-only"}, {"--parent-only", "--include-subagents"}, {"--limit", "-1"}, {"extra"}, {"--cwd", "."}, {"--unknown"}} {
		if _, err := parseArguments(false, args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	options, err := parseArguments(true, []string{"--cwd", ".", "--profile", "work", "--query", "repair", "--limit", "0", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(options.query.CurrentDir) || !options.query.LimitSet || options.query.Limit != 0 || !options.json {
		t.Fatalf("options = %#v", options)
	}
}
func TestOutputJSONIDsCommandsAndControlCharacters(t *testing.T) {
	reports := []sessionmodel.Report{{ID: "00000000-0000-4000-8000-000000000001", Profile: "work", ThreadName: "name\x1b[2J\nnext"}}
	for _, options := range []listOptions{{json: true}, {idOnly: true}, {resumeCommand: true}, {}} {
		var out bytes.Buffer
		if err := printReports(&out, reports, options); err != nil {
			t.Fatal(err)
		}
		if options.json {
			var decoded []sessionmodel.Report
			if err := json.Unmarshal(out.Bytes(), &decoded); err != nil || len(decoded) != 1 {
				t.Fatalf("json = %s: %v", out.String(), err)
			}
		} else if strings.ContainsRune(out.String(), '\x1b') {
			t.Fatalf("unsafe terminal output = %q", out.String())
		}
		if options.resumeCommand && !strings.HasPrefix(out.String(), "godex session resume ") {
			t.Fatalf("command = %q", out.String())
		}
	}
	var out bytes.Buffer
	if err := printReports(&out, []sessionmodel.Report{}, listOptions{json: true}); err != nil || strings.TrimSpace(out.String()) != "[]" {
		t.Fatalf("empty = %q, %v", out.String(), err)
	}
}
