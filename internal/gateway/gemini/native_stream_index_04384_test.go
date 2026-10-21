package gemini

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// Goldens come from the unmodified tagged Mojo index-selection ABI.
func TestProdex04384GeminiStreamIndexMatchesCompiledPolicy(t *testing.T) {
	data, err := os.ReadFile("testdata/native_stream_index_04384.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Part, Want int
		ID, Name   string
		Records    []struct {
			Index    int
			ID, Name string
			Explicit bool
		}
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for index, tc := range fixtures {
		state := &geminiStreamState{}
		for _, record := range tc.Records {
			state.toolCalls = append(state.toolCalls, geminiStreamToolCall{
				index: record.Index, name: record.Name, id: record.ID, explicit: record.Explicit,
			})
		}
		_, actual := state.toolPosition(tc.Part, tc.ID, tc.Name)
		if actual != tc.Want {
			t.Errorf("fixture %d selected %d want %d; input=%+v", index, actual, tc.Want, tc)
		}
	}
}

func TestProdex04384GeminiStreamBoundsRetainedToolSnapshots(t *testing.T) {
	state := &geminiStreamState{responseID: "resp-budget"}
	call := map[string]any{"name": "read_file", "args": strings.Repeat("x", 1<<20)}
	part := map[string]any{"functionCall": call}
	for index := 0; index < 12; index++ {
		err := state.observeTool(io.Discard, index, part, call)
		if err != nil {
			t.Fatalf("same anonymous tool accumulated obsolete snapshot bytes: %v", err)
		}
	}
	if len(state.toolCalls) != 1 || state.toolBytes > bodyMaxBytes {
		t.Fatalf("obsolete snapshot retention: %d calls, %d bytes", len(state.toolCalls), state.toolBytes)
	}
	for index := 1; index < 12; index++ {
		call["id"] = strings.Repeat("i", index)
		if err := state.observeTool(io.Discard, index, part, call); err != nil {
			if state.toolBytes > bodyMaxBytes || len(state.toolCalls) == 0 {
				t.Fatalf("tool state became corrupt at its memory bound: %d bytes", state.toolBytes)
			}
			return
		}
	}
	t.Fatal("distinct tool snapshots exceeded the retained response budget")
}
