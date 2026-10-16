package gemini

import (
	"reflect"
	"testing"
)

// Directly transcribed from tagged Prodex 0.438.0
// crates/prodex-provider-core/src/gemini_bridge/hardening/contents/tests.rs.
// Decisions use RAW first/last roles, before invalid parts are removed.
func TestProdex04380GeminiNativeRawBoundaryPrecedesInvalidRowFiltering(t *testing.T) {
	cases := []struct {
		name           string
		original, want []any
	}{
		{name: "invalid leading null does not request synthetic lead",
			original: []any{nil, map[string]any{"role": "model", "parts": []any{map[string]any{"text": "reply"}}}},
			want: []any{
				map[string]any{"role": "model", "parts": []any{map[string]any{"text": "reply"}}},
				map[string]any{"role": "user", "parts": []any{map[string]any{"text": "Please continue."}}},
			},
		},
		{name: "entirely invalid model row still creates both boundaries",
			original: []any{map[string]any{"role": "model", "parts": false}},
			want: []any{map[string]any{"role": "user", "parts": []any{
				map[string]any{"text": "[Continuing from previous Codex context.]"},
				map[string]any{"text": "Please continue."},
			}}},
		},
		{name: "synthetic prompt merges with retained user and clears first metadata",
			original: []any{
				map[string]any{"role": "model", "parts": []any{}},
				map[string]any{"role": "user", "parts": []any{map[string]any{"text": "existing"}}, "metadata": "must not replace synthetic first"},
				map[string]any{"role": "model", "parts": false},
			},
			want: []any{map[string]any{"role": "user", "parts": []any{
				map[string]any{"text": "[Continuing from previous Codex context.]"},
				map[string]any{"text": "existing"},
				map[string]any{"text": "Please continue."},
			}}},
		},
		{name: "raw leading null and invalid trailing row keep no boundaries",
			original: []any{nil, map[string]any{"role": "model", "parts": []any{map[string]any{"text": "reply"}}}, map[string]any{"role": "user", "parts": []any{}}},
			want:     []any{map[string]any{"role": "model", "parts": []any{map[string]any{"text": "reply"}}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := geminiEnforceContentRoleShape04380(tc.original)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("tagged source raw-boundary policy diverged: got=%+v want=%+v", got, tc.want)
			}
		})
	}
}

func TestProdex04380GeminiNativeCoalesceUnknownRolesAndPrimitiveParts(t *testing.T) {
	original := []any{
		map[string]any{"role": "", "parts": []any{1}},
		map[string]any{"role": "", "parts": []any{2}},
		map[string]any{"role": "モデル", "parts": []any{3}},
		map[string]any{"role": "model", "parts": []any{4}},
		map[string]any{"role": 42, "parts": []any{map[string]any{"text": "next"}}},
	}
	want := []any{
		map[string]any{"role": "", "parts": []any{1, 2}},
		map[string]any{"role": "モデル", "parts": []any{3}},
		map[string]any{"role": "model", "parts": []any{4}},
		map[string]any{"role": "user", "parts": []any{map[string]any{"text": "next"}}},
	}
	if got := geminiCoalesceContents04380(original); !reflect.DeepEqual(got, want) {
		t.Fatalf("non-ASCII and empty custom roles were conflated: got=%+v", got)
	}
}
