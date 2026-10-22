package main

import (
	"encoding/json"
	"strings"
)

func geminiCompactNegativeControl04384() bool {
	for _, scenario := range geminiCompactCases04384() {
		good := geminiCompactExpected04384(scenario)
		if len(geminiCompare(scenario, good, good)) != 0 {
			return false
		}
		for _, mutate := range []func(*geminiCandidateResult){
			func(r *geminiCandidateResult) { r.Client.Status++ },
			func(r *geminiCandidateResult) { r.Client.Body = `{}` },
			func(r *geminiCandidateResult) { r.Stdout = "wrong" },
			func(r *geminiCandidateResult) { r.ExitCode++ },
			func(r *geminiCandidateResult) { r.StateIntegrity = []string{"wrong owner"} },
			func(r *geminiCandidateResult) { r.Client.Headers.Set("X-Prodex-Compact-Mode", "wrong") },
			func(r *geminiCandidateResult) { r.Client.Headers.Set("X-Prodex-Compact-Reason", "wrong") },
			func(r *geminiCandidateResult) { r.Upstream = append(r.Upstream, geminiCapturedRequest{}) },
		} {
			run := good
			run.Client.Headers = good.Client.Headers.Clone()
			run.Upstream = append([]geminiCapturedRequest(nil), good.Upstream...)
			mutate(&run)
			if len(geminiCompare(scenario, run, run)) == 0 {
				return false
			}
		}
		if len(good.Upstream) != 0 {
			run := geminiCompactExpected04384(scenario)
			run.Upstream[0].Body["model"] = "wrong-model"
			wire, _ := json.Marshal(run.Upstream[0].Body)
			run.Upstream[0].RawBody = string(wire)
			if len(geminiCompare(scenario, run, run)) == 0 {
				return false
			}
		}
		if geminiCompactOwnershipSeed04384(scenario) {
			run := geminiCompactExpected04384(scenario)
			run.Client.Seed = nil
			if len(geminiCompare(scenario, run, run)) == 0 {
				return false
			}
			run = geminiCompactExpected04384(scenario)
			run.Upstream[len(run.Upstream)-1].RawBody = `{}`
			if len(geminiCompare(scenario, run, run)) == 0 {
				return false
			}
		}
		if scenario == "compact-owner-numeric" {
			run := geminiCompactExpected04384(scenario)
			last := &run.Upstream[len(run.Upstream)-1]
			last.RawBody = strings.Replace(last.RawBody, "9007199254740993", "9007199254740992", 1)
			if len(geminiCompare(scenario, run, run)) == 0 {
				return false
			}
		}
	}
	return true
}
