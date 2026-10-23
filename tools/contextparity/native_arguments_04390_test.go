package main

import "testing"

func TestNativeArgumentOracleRejectsScopeAndPromptMutations(t *testing.T) {
	for _, probe := range nativeArgumentProbes04390() {
		t.Run(probe.name, func(t *testing.T) {
			if err := verifyNativeArguments04390(probe.want, probe.want); err != nil {
				t.Fatal(err)
			}
			mutations := [][]string{
				probe.want[:len(probe.want)-1],
				append(append([]string(nil), probe.want...), "extra-prompt"),
			}
			swapped := append([]string(nil), probe.want...)
			swapped[0], swapped[1] = swapped[1], swapped[0]
			mutations = append(mutations, swapped)
			if probe.name == "root_exec_resume" {
				mutations = append(mutations, probe.input)
			}
			for _, changed := range mutations {
				if err := verifyNativeArguments04390(changed, probe.want); err == nil {
					t.Fatalf("accepted changed config order or prompt: %q", changed)
				}
			}
		})
	}
}
