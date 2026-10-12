package runtime

import (
	"strings"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
)

// nativeResumeSessionID recognizes only the resolved TUI resume form. An exec
// resume owns a separate child transport and must never be attached here.
func nativeResumeSessionID(arguments []string) string {
	for _, argument := range arguments {
		if argument == "--remote" || strings.HasPrefix(argument, "--remote=") {
			return ""
		}
	}
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" {
			return ""
		}
		if argument == "exec" || argument == "e" {
			return ""
		}
		if argument == "resume" {
			if index+1 < len(arguments) && sessionentity.ValidID(arguments[index+1]) {
				return arguments[index+1]
			}
			return ""
		}
		if strings.HasPrefix(argument, "-") {
			if nativeRecoveryOptionTakesValue(argument) {
				index++
			}
			continue
		}
		return ""
	}
	return ""
}

func nativeRecoveryOptionTakesValue(argument string) bool {
	if strings.Contains(argument, "=") {
		return false
	}
	switch argument {
	case "-c", "--config", "-m", "--model", "-C", "--cd", "--enable", "--disable":
		return true
	default:
		return false
	}
}
