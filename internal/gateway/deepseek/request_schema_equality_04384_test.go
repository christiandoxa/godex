package deepseek

import (
	"fmt"
	"strings"
	"testing"
)

func TestProdex04384DeepSeekSchemaDuplicatesUseSerdeNumericEquality(t *testing.T) {
	for _, fixture := range []struct {
		name, first, second, canonical string
		reject                         bool
	}{
		{"exact", "9007199254740993", "9007199254740993", "9007199254740993", false},
		{"exponent", "1e0", "1.0", "1.0", false},
		{"signed_zero", "-0.0", "0.0", "-0.0", false},
		{"fraction", "0.123456789012345678901234567890", "0.12345678901234568", "0.12345678901234568", false},
		{"int_float", "1", "1.0", "", true},
		{"zero_kinds", "0", "-0", "", true},
		{"string_zero", `"-0.0"`, `"0.0"`, "", true},
		{"map_order", `{"a":1,"b":[true,null]}`, `{"b":[true,null],"a":1}`, `{"a":1,"b":[true,null]}`, false},
		{"array_length", `[1]`, `[1,2]`, "", true},
		{"boolean_kind", "true", "1", "", true},
		{"null_kind", "null", "false", "", true},
	} {
		for _, strict := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/strict=%t", fixture.name, strict), func(t *testing.T) {
				tool := func(value string) string {
					return `{"type":"function","name":"lookup","parameters":{"type":"number","enum":[` + value + `]}}`
				}
				body, err := ResponsesRequest([]byte(`{"input":"same request","tools":[`+tool(fixture.first)+`,`+tool(fixture.second)+`]}`), RequestOptions{StrictTools: strict})
				if fixture.reject {
					if err == nil || !strings.Contains(err.Error(), "duplicated after translation") {
						t.Fatalf("distinct schema number kinds/values were merged: %v", err)
					}
					return
				}
				if err != nil || !strings.Contains(string(body), `"enum":[`+fixture.canonical+`]`) || strings.Count(string(body), `"name":"lookup"`) != 1 {
					t.Fatalf("equivalent schemas changed first value or failed to deduplicate: error=%v body=%s", err, body)
				}
			})
		}
	}
}
