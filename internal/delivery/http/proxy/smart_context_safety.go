package proxy

import (
	"regexp"
	"strings"
)

var smartContextFileLocationPattern = regexp.MustCompile("[A-Za-z0-9_./\\\\-]+:[0-9]+(?::[0-9]+)?")

type smartContextSignalCounts struct {
	errors          int
	fileLocations   int
	diffHunks       int
	testFailures    int
	exitCodes       int
	stackMarkers    int
	rustDiagnostics int
}

func smartContextCriticalSignalsPreserved(before, after []byte) bool {
	beforeCounts := smartContextCriticalSignalCounts(string(before))
	afterCounts := smartContextCriticalSignalCounts(string(after))
	return afterCounts.errors >= beforeCounts.errors &&
		afterCounts.fileLocations >= beforeCounts.fileLocations &&
		afterCounts.diffHunks >= beforeCounts.diffHunks &&
		afterCounts.testFailures >= beforeCounts.testFailures &&
		afterCounts.exitCodes >= beforeCounts.exitCodes &&
		afterCounts.stackMarkers >= beforeCounts.stackMarkers &&
		afterCounts.rustDiagnostics >= beforeCounts.rustDiagnostics
}

func smartContextCriticalSignalCounts(text string) smartContextSignalCounts {
	lower := strings.ToLower(text)
	counts := smartContextSignalCounts{}
	for _, marker := range []string{
		"error[", "error:", "assertionerror:", "runtimeerror:", "syntaxerror:",
		"typeerror:", "valueerror:", "exception:",
	} {
		if strings.Contains(lower, marker) {
			counts.errors++
		}
	}
	if smartContextFileLocationPattern.MatchString(text) ||
		strings.Contains(lower, "file \\") ||
		strings.Contains(lower, "file '") {
		counts.fileLocations++
	}
	if strings.Contains(text, "@@ -") {
		counts.diffHunks++
	}
	if strings.Contains(lower, " failed") || strings.Contains(lower, "... failed") ||
		strings.Contains(lower, "failures=") {
		counts.testFailures++
	}
	if strings.Contains(lower, "exit code ") || strings.Contains(lower, "exit status ") {
		counts.exitCodes++
	}
	if strings.Contains(lower, "stack backtrace:") || strings.Contains(lower, "backtrace:") {
		counts.stackMarkers++
	}
	if strings.Contains(lower, "warning:") || strings.Contains(lower, "error[e") {
		counts.rustDiagnostics++
	}
	return counts
}
