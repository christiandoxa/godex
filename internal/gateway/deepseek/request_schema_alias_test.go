package deepseek

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFunctionSchemaAliasesFollowRequestProjection(t *testing.T) {
	for _, alias := range []string{"parametersJsonSchema", "input_schema", "schema"} {
		for _, mode := range []string{"standalone", "normal", "simple"} {
			t.Run(alias+"/"+mode, func(t *testing.T) {
				fields := ""
				if mode == "normal" {
					fields = `,"safety_identifier":"synthetic_user"`
				}
				body := []byte(`{"input":"same request","tools":[{"type":"function","function":{"name":"lookup","` + alias + `":{"type":"object","properties":{"id":{"enum":[9007199254740993]}}}}}]` + fields + `}`)
				var contains func(string) bool
				if mode != "standalone" {
					contains = func(string) bool { return false }
				}
				result, err := translateResponsesRequestWithHistory(body, RequestOptions{}, nil, contains)
				if err != nil {
					t.Fatal(err)
				}
				var wire map[string]any
				if err := json.Unmarshal(result.Body, &wire); err != nil {
					t.Fatal(err)
				}
				function := wire["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)
				key, absent := "parameters", alias
				if mode == "simple" {
					key, absent = alias, "parameters"
				}
				if function[key] == nil || function[absent] != nil || !strings.Contains(string(result.Body), "9007199254740993") {
					t.Fatalf("schema projection or precision changed: %s", result.Body)
				}
			})
		}
	}
}

func TestFunctionSchemaAliasPresencePrecedesFallback(t *testing.T) {
	for _, fields := range []string{
		`"parameters":null,"parametersJsonSchema":{"type":"object"}`,
		`"parametersJsonSchema":null,"input_schema":{"type":"object"}`,
		`"input_schema":false,"schema":{"type":"object"}`,
	} {
		_, err := ResponsesRequest([]byte(`{"input":"same request","tools":[{"type":"function","name":"lookup",`+fields+`}]}`), RequestOptions{})
		if err == nil {
			t.Fatalf("invalid selected schema fell through: %s", fields)
		}
	}
}
