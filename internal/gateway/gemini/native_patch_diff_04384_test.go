package gemini

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProdex04384GeminiApplyPatchNormalizesUnifiedDiff(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"update", "--- a/src/file.go\n+++ b/src/file.go\n@@ -1,2 +1,2 @@ func run()\n keep\n-old\n+new",
			"*** Begin Patch\n*** Update File: src/file.go\n@@ func run()\n keep\n-old\n+new\n*** End Patch"},
		{"add-quoted-path", "--- /dev/null\n+++ \"b/new file.txt\"\n@@ -0,0 +1,2 @@\n+alpha\n+beta\n",
			"*** Begin Patch\n*** Add File: new file.txt\n+alpha\n+beta\n*** End Patch"},
		{"delete", "--- a/gone.txt\n+++ /dev/null\n@@ -1 +0,0 @@\n-gone",
			"*** Begin Patch\n*** Delete File: gone.txt\n*** End Patch"},
		{"rename", "diff --git a/old.txt b/new.txt\nsimilarity index 60%\nrename from old.txt\nrename to new.txt\n--- a/old.txt\n+++ b/new.txt\n@@ -1 +1 @@\n-old\n+new",
			"*** Begin Patch\n*** Update File: old.txt\n*** Move to: new.txt\n@@\n-old\n+new\n*** End Patch"},
		{"end-of-file", "--- a/file\n+++ b/file\n@@ -1 +1 @@\n-old\n+new\n\\ No newline at end of file",
			"*** Begin Patch\n*** Update File: file\n@@\n-old\n+new\n*** End of File\n*** End Patch"},
		{"malformed-hunk-label", "--- a/file\n+++ b/file\n@@ broken\n-old\n+new",
			"*** Begin Patch\n*** Update File: file\n@@\n-old\n+new\n*** End Patch"},
		{"no-hunk", "--- a/file\n+++ b/file\n-old\n+new", "--- a/file\n+++ b/file\n-old\n+new"},
		{"context-only", "--- a/file\n+++ b/file\n@@ -1 +1 @@\n keep", "--- a/file\n+++ b/file\n@@ -1 +1 @@\n keep"},
		{"no-added-lines", "--- /dev/null\n+++ b/file\n@@ -0 +0 @@", "--- /dev/null\n+++ b/file\n@@ -0 +0 @@"},
		{"incomplete-header", "--- a/file\r\n+++", "--- a/file\r\n+++"},
		{"plain-CRLF", "literal\r\ntext\r", "literal\r\ntext\r"},
		{"empty", "", ""},
		{"native-block-control", "fenced\n*** Begin Patch\n*** Add File: file\nhello\n*** End Patch\ntrailing",
			"*** Begin Patch\n*** Add File: file\n+hello\n*** End Patch"},
		{"Unicode-block-marker", "fenced\n\u2003*** Begin Patch\n*** Add File: file\nhello\n*** End Patch\ntrailing",
			"\u2003*** Begin Patch\n*** Add File: file\n+hello\n*** End Patch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, input := range []string{tc.input, strings.ReplaceAll(tc.input, "\n", "\r\n")} {
				want := tc.want
				if tc.input == tc.want {
					want = input
				}
				if got := geminiNativeApplyPatchInput(map[string]any{"input": input}); got != want {
					t.Fatalf("custom patch=%q want=%q", got, want)
				}
			}
		})
	}
}

func TestProdex04384GeminiUnifiedDiffSurvivesCompleteAPIKeyEnvelope(t *testing.T) {
	input := "diff --git a/old.txt b/new.txt\n--- a/old.txt\n+++ b/new.txt\n@@ -1 +1 @@\n-old\n+new\ndiff --git a/add.txt b/add.txt\n--- /dev/null\n+++ b/add.txt\n@@ -0,0 +1 @@\n+added\ndiff --git a/gone.txt b/gone.txt\n--- a/gone.txt\n+++ /dev/null\n-gone"
	want := "*** Begin Patch\n*** Update File: old.txt\n*** Move to: new.txt\n@@\n-old\n+new\n*** Add File: add.txt\n+added\n*** Delete File: gone.txt\n*** End Patch"
	args, _ := json.Marshal(map[string]any{"input": input})
	wire := `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"apply_patch","id":"call-patch","args":` + string(args) + `}}]},"finishReason":"STOP"}]}`
	body, err := geminiEnvelopeResponse04384(wire, true)
	if err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if json.Unmarshal(body, &response) != nil {
		t.Fatalf("invalid envelope: %s", body)
	}
	output := response["output"].([]any)
	if len(output) != 1 {
		t.Fatalf("custom patch output changed: %#v", output)
	}
	item := output[0].(map[string]any)
	if item["type"] != "custom_tool_call" || item["call_id"] != "call-patch" || item["input"] != want {
		t.Fatalf("custom patch envelope=%#v want input=%q", item, want)
	}
}
