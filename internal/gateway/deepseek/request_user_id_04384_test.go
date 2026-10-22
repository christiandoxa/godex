package deepseek

import (
	"errors"
	"strings"
	"testing"
)

func TestProdex04384DeepSeekUserIDUsesAliasesAndTerminalValidation(t *testing.T) {
	for _, fixture := range []struct{ name, fields, want string }{
		{"canonical", `"user_id":" user_04384 ","user":"ignored_user","safety_identifier":17`, "user_04384"},
		{"user_alias", `"user":" user_04384 "`, "user_04384"},
		{"safety_alias", `"safety_identifier":" user_04384 "`, "user_04384"},
		{"trim_limit", `"user_id":" \u2003` + strings.Repeat("u", 512) + `\u00a0 "`, strings.Repeat("u", 512)},
		{"null_blocks_alias", `"user_id":null,"user":"ignored_user"`, ""},
		{"number", `"user_id":7`, ""},
		{"over_limit", `"user_id":"` + strings.Repeat("u", 513) + `"`, ""},
		{"invalid_character", `"user_id":"user.has.dot"`, ""},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			body, err := ResponsesRequest([]byte(`{"model":"deepseek-v4-pro","input":"same request",`+fixture.fields+`}`), RequestOptions{})
			if fixture.want == "" {
				var invalid *invalidResponsesInputError
				if !errors.As(err, &invalid) {
					t.Fatalf("invalid identity lost terminal admission: %v", err)
				}
				return
			}
			want := `{"messages":[{"content":"same request","role":"user"}],"model":"deepseek-v4-pro","stream":false,"user_id":"` + fixture.want + `"}`
			if err != nil || string(body) != want {
				t.Fatalf("identity projection differs: error=%v body=%s", err, body)
			}
		})
	}
	for _, fields := range []string{"", `,"user_id":" \t ","user":"ignored_user"`} {
		body, err := ResponsesRequest([]byte(`{"model":"deepseek-v4-pro","input":"same request"`+fields+`}`), RequestOptions{})
		want := `{"messages":[{"content":"same request","role":"user"}],"model":"deepseek-v4-pro","stream":false}`
		if err != nil || string(body) != want {
			t.Fatalf("absent or blank identity did not omit field: error=%v body=%s", err, body)
		}
	}
}
