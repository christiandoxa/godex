package main

import (
	"net/http"
	"testing"
)

func rawKeyLocalFixtureHeaders04384(phase string) http.Header {
	header := http.Header{"Content-Type": {"application/json; charset=utf-8"}}
	if phase == "resume-compact" {
		header.Set("X-Prodex-Compact-Degraded", "true")
		header.Set("X-Prodex-Compact-Mode", "local-fallback")
		header.Set("X-Prodex-Compact-Provider", "deepseek")
		header.Set("X-Prodex-Compact-Reason", "local-policy")
	}
	return header
}

func TestProdex04384RawKeyLocalRouteOracleRejectsChangedOutput(t *testing.T) {
	for _, phase := range []string{"resume-compact", "resume-models", "resume-models-single"} {
		run := productRun{Client: exchange{Status: 200, Body: rawKeyLocalRouteFixture04384(phase),
			Headers: rawKeyLocalFixtureHeaders04384(phase)}}
		if !validRawKeyLocalRoute04384(phase, run) {
			t.Fatalf("valid local fixture rejected: %s", phase)
		}
		for _, mutate := range []func(*productRun){
			func(r *productRun) { r.Client.Body = `{}` },
			func(r *productRun) { r.Client.Body += " " },
			func(r *productRun) { r.Client.Status = 503 },
			func(r *productRun) { r.ExitStatus = 2 },
			func(r *productRun) { r.Client.Headers = nil },
		} {
			changed := run
			mutate(&changed)
			if validRawKeyLocalRoute04384(phase, changed) {
				t.Fatalf("mutated local fixture accepted: %s", phase)
			}
		}
	}
}
