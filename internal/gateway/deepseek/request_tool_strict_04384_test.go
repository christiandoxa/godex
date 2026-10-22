package deepseek

import (
	"fmt"
	"strings"
	"testing"
)

func TestProdex04384DeepSeekPreservesPresentFalseStrictFlag(t *testing.T) {
	missing := `{"type":"function","name":"lookup","parameters":{"type":"object"}}`
	flat := `{"type":"function","name":"lookup","parameters":{"type":"object"},"strict":false}`
	for _, fixture := range []struct {
		name, tools string
		present     bool
		reject      bool
	}{
		{"absent", missing, false, false},
		{"flat", flat, true, false},
		{"nested", `{"type":"function","function":{"name":"lookup","parameters":{"type":"object"},"strict":false}}`, true, false},
		{"duplicate", flat + `,` + flat, true, false},
		{"missing_conflict", flat + `,` + missing, true, true},
		{"true_requires_opt_in", strings.Replace(flat, `false`, `true`, 1), true, true},
	} {
		for _, strict := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/strict=%t", fixture.name, strict), func(t *testing.T) {
				body, err := ResponsesRequest([]byte(`{"input":"same request","tools":[`+fixture.tools+`]}`), RequestOptions{StrictTools: strict})
				if fixture.reject && !strict {
					if err == nil {
						t.Fatal("distinct flag presence or strict opt-in requirement was ignored")
					}
					return
				}
				flag := fmt.Sprintf(`"strict":%t`, strict)
				if err != nil || strings.Count(string(body), `"name":"lookup"`) != 1 || (strict || fixture.present) != strings.Contains(string(body), flag) || !strict && !fixture.present && strings.Contains(string(body), `"strict":`) {
					t.Fatalf("strict flag presence or normalization changed: error=%v body=%s", err, body)
				}
			})
		}
	}
}
