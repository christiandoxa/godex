package main

import (
	"net"
	"net/url"
	"strconv"
	"strings"
)

const prodexDeepSeekLaunchBanner = "Prodex launch: preparing runtime launch...\n" +
	"Prodex launch: Smart Context runtime proxy requested.\n" +
	"Prodex launch: local provider bridge requested.\n" +
	"[ Runtime Provider ] ============================================================================================\n" +
	"Using provider 'deepseek' through the local compatibility proxy. Smart Context rewrites require a proven\n" +
	"tokenizer. Using one provider API key; API-key rotation is skipped and quota preflight stays disabled.\n" +
	"Prodex launch: starting child process...\n"

const prodexDeepSeekTwoKeyLaunchBanner = "Prodex launch: preparing runtime launch...\n" +
	"Prodex launch: Smart Context runtime proxy requested.\n" +
	"Prodex launch: local provider bridge requested.\n" +
	"[ Runtime Provider ] ============================================================================================\n" +
	"Using provider 'deepseek' through the local compatibility proxy. Smart Context rewrites require a proven\n" +
	"tokenizer. API-key rotation is enabled across 2 keys; quota preflight stays disabled.\n" +
	"Prodex launch: starting child process...\n"

func expectedProdexLaunchBanner(command []string) string {
	if contains(command, "<synthetic-multiple-credentials-in-environment>") {
		return prodexDeepSeekTwoKeyLaunchBanner
	}
	return prodexDeepSeekLaunchBanner
}

const cancelledCodexRequestSuffix = ": context deadline exceeded (Client.Timeout exceeded while awaiting headers)\n"

// Only the exact tagged startup banner is considered presentation metadata.
// Any unexpected warning, provider error, auth/quota diagnostic, or changed
// cancellation reason remains a hard differential mismatch.
func equivalentRuntimeDiagnostics(reference, candidate productRun) bool {
	if reference.Stderr == candidate.Stderr {
		return true
	}
	banner := expectedProdexLaunchBanner(reference.Command)
	if !strings.HasPrefix(reference.Stderr, banner) {
		return false
	}
	remaining := strings.TrimPrefix(reference.Stderr, banner)
	if remaining == "" && candidate.Stderr == "" {
		return true
	}
	if reference.ExitStatus == 1 && candidate.ExitStatus == 1 &&
		reference.Cancelled && candidate.Cancelled {
		return validFixtureCancellation(remaining, "/v1/responses") &&
			validFixtureCancellation(candidate.Stderr, "/backend-api/godex/responses")
	}
	return false
}

func validFixtureCancellation(value, path string) bool {
	const prefix = "synthetic Codex shim request failed: Post \""
	if !strings.HasPrefix(value, prefix) ||
		!strings.HasSuffix(value, cancelledCodexRequestSuffix) {
		return false
	}
	middle := strings.TrimSuffix(strings.TrimPrefix(value, prefix), "\""+cancelledCodexRequestSuffix)
	endpoint, err := url.Parse(middle)
	if err != nil || endpoint.Scheme != "http" || endpoint.Hostname() != "127.0.0.1" ||
		endpoint.Path != path || endpoint.RawQuery != "" || endpoint.Fragment != "" ||
		endpoint.User != nil {
		return false
	}
	port, err := strconv.Atoi(endpoint.Port())
	return err == nil && port > 0 && port < 65536 &&
		net.ParseIP(endpoint.Hostname()).IsLoopback()
}
