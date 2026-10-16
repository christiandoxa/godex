package main

import "net/http"

const deepSeekLocalError = "upstream request failed"

func deepSeekInvalidInputInvariants(run productRun, prefix string) []string {
	var failures []string
	if run.ExitStatus != 2 {
		failures = append(failures, prefix+".exit_status")
	}
	if run.Client.Status != http.StatusBadGateway || run.Client.Body != deepSeekLocalError {
		failures = append(failures, prefix+".local_error")
	}
	if run.Client.Headers.Get("Content-Type") != "text/plain; charset=utf-8" {
		failures = append(failures, prefix+".error_content_type")
	}
	if len(run.Upstream) != 0 || len(run.Events) != 0 || run.Retries != 0 {
		failures = append(failures, prefix+".upstream_attempts")
	}
	return failures
}
