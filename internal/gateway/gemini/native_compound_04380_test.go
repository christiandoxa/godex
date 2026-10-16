package gemini

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// Golden outputs come from executing the tagged Mojo kernel, not a Go
// reconstruction. Every comparison includes metadata, signatures and errors.
func TestProdex04380GeminiNativeCompoundHistoryMatchesCompiledSource(t *testing.T) {
	data, err := os.ReadFile("testdata/native_compound_04380.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		SourceCommit   string `json:"source_commit"`
		OperationOrder []int  `json:"operation_order"`
		Cases          []struct {
			Name     string `json:"name"`
			Input    []any  `json:"input"`
			Expected []any  `json:"expected"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.SourceCommit != "8cab9794b0e08b2cb8507df8321a8e38ebd6492f" ||
		!reflect.DeepEqual(fixture.OperationOrder, []int{0, 1, 2, 4, 3}) || len(fixture.Cases) != 15 {
		t.Fatal("native Gemini oracle provenance or coverage changed")
	}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			before, err := json.Marshal(tc.Input)
			if err != nil {
				t.Fatal(err)
			}
			got := geminiHardenContents04380(tc.Input)
			if !reflect.DeepEqual(got, tc.Expected) {
				encoded, _ := json.Marshal(got)
				t.Fatalf("compound history differs from tagged compiled kernel: %s", encoded)
			}
			after, err := json.Marshal(tc.Input)
			if err != nil || string(before) != string(after) {
				t.Fatal("native hardening mutated caller-owned history")
			}
			if again := geminiHardenContents04380(got); !reflect.DeepEqual(again, got) {
				t.Fatal("repeated native hardening duplicated a turn or tool part")
			}
		})
	}
}
