package jsonutil

import (
	"encoding/json"
	"os"
	"testing"
)

// These are source goldens from the 0.438.4 lockfile's default features:
// serde 1.0.229, serde_json 1.0.154 and zmij 1.0.23.
func TestProdex04384LockedSerdeNumberGoldens(t *testing.T) {
	data, err := os.ReadFile("testdata/serde_locked_04384.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Input, Canonical string
		Valid            bool
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Input, func(t *testing.T) {
			if valid := CheckSerdeNumberTokens([]byte(tc.Input)); valid != tc.Valid {
				t.Errorf("locked serde validity=%v want=%v", valid, tc.Valid)
			}
			got, err := MarshalSerdeValue04384(json.Number(tc.Input))
			if (err == nil) != tc.Valid || tc.Valid && string(got) != tc.Canonical {
				t.Errorf("locked serde canonical=%q error=%v want=%q valid=%v", got, err, tc.Canonical, tc.Valid)
			}
		})
	}
}
