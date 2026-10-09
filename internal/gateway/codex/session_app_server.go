package codex

import (
	"fmt"
	"strings"
)

// sessionAppServerEligible limits the private companion to interactive
// foreground launches. Exec/resume and an already remote child own their
// session transport and must keep their native argument shape.
func sessionAppServerEligible(arguments []string) bool {
	if len(arguments) == 0 || codexCommandServerSubcommand(arguments) || codexExecInvocation(arguments) {
		return false
	}
	command := commandAfterOptions(arguments, 0)
	if command >= 0 && (arguments[command] == "resume" || arguments[command] == "app-server") {
		return false
	}
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" {
			break
		}
		if argument == "--remote" || strings.HasPrefix(argument, "--remote=") {
			return false
		}
		if _, consumed, ok := configArgument(arguments, index); ok {
			index += consumed - 1
			continue
		}
		if codexOptionTakesValue(argument) && index+1 < len(arguments) {
			index++
		}
	}
	return true
}

func sessionAppServerCompanionArguments(arguments []string, socket string) []string {
	result := []string{"app-server", "--listen", "unix://" + socket}
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" {
			break
		}
		switch argument {
		case "-c", "--config", "--enable", "--disable", "--code-mode-host":
			if index+1 < len(arguments) {
				result = append(result, argument, arguments[index+1])
				index++
			}
		case "--strict-config":
			result = append(result, argument)
		case "-m", "--model":
			if index+1 < len(arguments) {
				result = append(result, "-c", "model="+tomlStringLiteral(arguments[index+1]))
				index++
			}
		default:
			if strings.HasPrefix(argument, "--model=") {
				result = append(result, "-c", "model="+tomlStringLiteral(strings.TrimPrefix(argument, "--model=")))
			}
		}
	}
	return result
}

func tomlStringLiteral(value string) string {
	var escaped strings.Builder
	escaped.Grow(len(value) + 2)
	escaped.WriteByte('"')
	for _, char := range value {
		switch char {
		case '\\':
			escaped.WriteString(`\\`)
		case '"':
			escaped.WriteString(`\"`)
		case '\b':
			escaped.WriteString(`\b`)
		case '\t':
			escaped.WriteString(`\t`)
		case '\n':
			escaped.WriteString(`\n`)
		case '\f':
			escaped.WriteString(`\f`)
		case '\r':
			escaped.WriteString(`\r`)
		default:
			if char < 0x20 || char == 0x7f {
				fmt.Fprintf(&escaped, `\u%04X`, char)
				continue
			}
			escaped.WriteRune(char)
		}
	}
	escaped.WriteByte('"')
	return escaped.String()
}
