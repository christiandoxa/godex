package chatcompat

import "testing"

func TestProdex04384RTKChangedJSONCanonicalSerde(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"canonical", `{"cmd":"git status","html":"<a>&"}`, `{"cmd":"rtk git status","html":"<a>&"}`},
		{"unicode", `{"cmd":"git status","line":"\u2028"}`, "{\"cmd\":\"rtk git status\",\"line\":\"\u2028\"}"},
		{"number", `{"cmd":"git status","number":1e-5}`, `{"cmd":"rtk git status","number":0.00001}`},
		{"no-change", `{  "cmd": "pwd" }`, `{  "cmd": "pwd" }`},
		{"serde-invalid-surrogate", `{"cmd":"git status","unused":"\ud800"}`, `{"cmd":"git status","unused":"\ud800"}`},
		{"serde-invalid-f64", `{"cmd":"git status","unused":1.7976931348623158e308}`, `{"cmd":"git status","unused":1.7976931348623158e308}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := WrapRTKArguments("exec_command", tc.input); got != tc.want {
				t.Fatalf("RTK canonical serde %s got=%q want=%q", tc.name, got, tc.want)
			}
		})
	}
}
