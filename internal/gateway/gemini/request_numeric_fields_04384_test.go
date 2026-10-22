package gemini

import (
	"errors"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiNumericControlsRequireNumberTypes(t *testing.T) {
	for _, field := range []string{"temperature", "top_p"} {
		for _, value := range []string{"null", "false", `"0.2"`, "[]", "{}", "0.2", "-0", "1e3", "18446744073709551615"} {
			t.Run(field+"/"+value, func(t *testing.T) {
				_, err := normalizeGeminiChatInput043711([]byte(`{"model":"gemini-3.1-pro-preview","input":"same request","` + field + `":` + value + `}`))
				valid := value == "0.2" || value == "-0" || value == "1e3" || value == "18446744073709551615"
				if valid {
					if err != nil {
						t.Fatalf("valid numeric control was rejected: %v", err)
					}
					return
				}
				var failure *proxymodel.Error
				if !errors.As(err, &failure) || failure.StatusCode != 502 || failure.Message != "upstream request failed" {
					t.Fatalf("invalid numeric control lost terminal admission: %v", err)
				}
			})
		}
	}
}
