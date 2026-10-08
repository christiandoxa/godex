package runtime

import "strings"

// retargetCodexExecRecovery04360 mirrors the source-scoped RetargetExec
// operation in Prodex 0.436.0's launch_args.mojo planner. In particular,
// a separated Cyber program is a *value*, not the previous prompt, and an
// existing resume session does not survive a new explicit recovery plan.
//
// This planner alone never decides whether a model turn may be retried.
func retargetCodexExecRecovery04360(args []string, sessionID string) ([]string, bool) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, false
	}
	command := nextCommandWord(args, 0)
	if command < 0 || (args[command] != "exec" && args[command] != "e") {
		return nil, false
	}
	nested := nextCommandWord(args, command+1)
	if nested >= 0 && args[nested] == "review" {
		return nil, false
	}
	// Exec may take its prompt from stdin ('-'); only the explicit
	// literal terminator prevents safely inferring a recoverable command.
	if nested < 0 {
		for _, arg := range args[command+1:] {
			if arg == "--" {
				return nil, false
			}
		}
	}
	result := make([]string, 0, len(args)+3)
	result = appendRecoveryOptions04360(result, args[:command])
	result = append(result, "exec", "resume", sessionID)
	result = appendRecoveryOptions04360(result, args[command+1:])
	return result, true
}

func appendRecoveryOptions04360(result, segment []string) []string {
	for index := 0; index < len(segment); {
		argument := segment[index]
		if argument == "--" {
			break
		}
		if argument == "--thread-source" {
			index += 2
			continue
		}
		if strings.HasPrefix(argument, "--thread-source=") || argument == "--last" {
			index++
			continue
		}
		if nativeOptionTakesValue(argument) {
			// A dangling option cannot be made valid by attaching the
			// recovery session ID to it.
			if index+1 < len(segment) {
				result = append(result, argument, segment[index+1])
				index += 2
				continue
			}
			index++
			continue
		}
		if argument != "-" && strings.HasPrefix(argument, "-") {
			result = append(result, argument)
		}
		// Discard all positionals, including original prompts,
		// original session IDs and literal stdin ('-').
		index++
	}
	return result
}
