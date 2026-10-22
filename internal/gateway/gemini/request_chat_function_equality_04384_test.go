package gemini

import "testing"

func TestProdex04384GeminiFunctionEqualityPreservesSerdeNumberKinds(t *testing.T) {
	for _, fixture := range []struct {
		name, left, right string
		equal             bool
	}{
		{"floating-zero", `{"x":-0.0}`, `{"x":0.0}`, true},
		{"reverse-zero", `{"x":0.0}`, `{"x":-0.0}`, true},
		{"integer-zero", `{"x":0}`, `{"x":0.0}`, false},
		{"zero-prefix", `{"x":-0.0}`, `{"x":-0.01}`, false},
		{"quoted-zero", `{"x":"-0.0"}`, `{"x":"0.0"}`, false},
		{"quoted-prefix", `{"x":"before -0.0"}`, `{"x":"before 0.0"}`, false},
		{"escaped-quote", `{"x":"\\\"-0.0"}`, `{"x":"\\\"0.0"}`, false},
		{"equivalent-exponent", `{"x":1e3}`, `{"x":1000.0}`, true},
		{"object-delimiter", `{"x":0.0,"y":2}`, `{"x":-0.0,"y":2}`, true},
		{"array-delimiter", `{"x":[-0.0,0.0]}`, `{"x":[0.0,-0.0]}`, true},
		{"mixed", `{"x":"-0.0","y":-0.0}`, `{"x":"-0.0","y":0.0}`, true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			left, err := decodeGeminiChatRequest04384([]byte(fixture.left))
			if err != nil {
				t.Fatal(err)
			}
			right, err := decodeGeminiChatRequest04384([]byte(fixture.right))
			if err != nil {
				t.Fatal(err)
			}
			if equalGeminiChatFunctions04384(left, right) != fixture.equal {
				t.Fatalf("unexpected equality for %s and %s", fixture.left, fixture.right)
			}
		})
	}
}
