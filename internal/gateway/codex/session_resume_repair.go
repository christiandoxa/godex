package codex

import "strings"

// repairResumeSessionBeforeLaunch repairs native goal attachment references for
// an explicit TUI resume. Rollout bytes stay untouched while Codex may own them.
// Picker resumes have no id yet and are repaired during shared-home preparation.
func repairResumeSessionBeforeLaunch(codexHome, sharedHome string, arguments []string) error {
	selector := codexResumeSessionID(arguments)
	if selector == "" {
		return nil
	}
	root := strings.TrimSpace(sharedHome)
	if root == "" {
		root = codexHome
	}
	return persistSessionGoalAttachmentPathForThread(root, selector)
}

func codexResumeSessionID(arguments []string) string {
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" || argument == "--remote" || strings.HasPrefix(argument, "--remote=") {
			return ""
		}
		if argument == "exec" || argument == "e" {
			continue
		}
		if strings.HasPrefix(argument, "-") {
			if codexResumeOptionTakesValue(argument) && !strings.Contains(argument, "=") {
				index++
			}
			continue
		}
		if argument != "resume" {
			return ""
		}
		for next := index + 1; next < len(arguments); next++ {
			argument = arguments[next]
			if argument == "--" {
				return ""
			}
			if strings.HasPrefix(argument, "-") {
				if strings.Contains(argument, "=") {
					continue
				}
				if codexResumeOptionTakesValue(argument) {
					next++
				}
				continue
			}
			if fullSessionID(argument) {
				return argument
			}
			return ""
		}
		return ""
	}
	return ""
}

func codexResumeOptionTakesValue(argument string) bool {
	switch argument {
	case "-c", "--config", "-m", "--model", "-C", "--cd", "-i", "--image", "-p", "--profile", "-s", "--sandbox", "-a", "--ask-for-approval", "--enable", "--disable", "--add-dir", "--color", "-o", "--output-last-message", "--output-schema", "--thread-source", "--cyber-access-program", "--local-provider", "--listen", "--code-mode-host", "--remote-auth-token-env", "--ws-auth", "--ws-token-file", "--ws-token-sha256", "--ws-shared-secret-file", "--ws-issuer", "--ws-audience", "--ws-max-clock-skew-seconds":
		return true
	default:
		return false
	}
}
