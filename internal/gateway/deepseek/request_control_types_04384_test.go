package deepseek

import "testing"

func TestProdex04384DeepSeekControlTypesAreCheckedBeforeTranslation(t *testing.T) {
	for _, field := range []string{"stream", "instructions", "previous_response_id"} {
		for _, value := range []string{"null", "false", "true", `"note"`, "7", "[]", "{}"} {
			t.Run(field+"/"+value, func(t *testing.T) {
				_, err := ResponsesRequest([]byte(`{"input":"same request","`+field+`":`+value+`}`), RequestOptions{})
				valid := field == "stream" && (value == "false" || value == "true") || field != "stream" && value == `"note"`
				if valid {
					if err != nil {
						t.Fatalf("valid top-level control rejected: %v", err)
					}
					return
				}
				if err == nil {
					t.Fatal("wrong-type top-level control was accepted")
				}
			})
		}
	}
}
