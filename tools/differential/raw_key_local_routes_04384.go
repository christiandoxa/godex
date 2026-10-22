package main

import _ "embed"

// Exact pinned local-route output for the synthetic request and built-in catalog.
//
//go:embed testdata/deepseek_raw_key_compact_04384.json
var rawKeyCompactFixture04384 string

//go:embed testdata/deepseek_raw_key_models_04384.json
var rawKeyModelsFixture04384 string

//go:embed testdata/deepseek_raw_key_model_04384.json
var rawKeyModelFixture04384 string

func rawKeyLocalRouteFixture04384(phase string) string {
	switch phase {
	case "resume-compact":
		return rawKeyCompactFixture04384
	case "resume-models":
		return rawKeyModelsFixture04384
	case "resume-models-single":
		return rawKeyModelFixture04384
	}
	return ""
}

func validRawKeyLocalRoute04384(phase string, run productRun) bool {
	want := rawKeyLocalRouteFixture04384(phase)
	if want == "" || run.ExitStatus != 0 || run.Client.Status != 200 || run.Client.Body != want ||
		run.Client.Headers.Get("Content-Type") != "application/json; charset=utf-8" {
		return false
	}
	if phase == "resume-compact" {
		return run.Client.Headers.Get("X-Prodex-Compact-Degraded") == "true" &&
			run.Client.Headers.Get("X-Prodex-Compact-Mode") == "local-fallback" &&
			run.Client.Headers.Get("X-Prodex-Compact-Provider") == "deepseek" &&
			run.Client.Headers.Get("X-Prodex-Compact-Reason") == "local-policy"
	}
	return true
}
