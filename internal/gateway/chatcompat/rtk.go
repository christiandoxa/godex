package chatcompat

import (
	"encoding/json"
	"strings"
)

var rtkNoisyCommands = map[string]map[string]bool{
	"git":            {"diff": true, "show": true, "log": true, "status": true, "grep": true, "blame": true},
	"cargo":          {"test": true, "build": true, "check": true, "clippy": true, "bench": true, "run": true},
	"npm":            {"test": true, "run": true, "build": true, "install": true, "ci": true, "update": true, "audit": true},
	"yarn":           {"test": true, "run": true, "build": true, "install": true, "add": true, "upgrade": true},
	"pnpm":           {"test": true, "run": true, "build": true, "install": true, "add": true, "update": true},
	"bun":            {"test": true, "run": true, "build": true, "install": true, "add": true},
	"go":             {"test": true, "build": true, "vet": true},
	"docker":         {"build": true, "compose": true, "logs": true, "pull": true, "push": true, "run": true},
	"kubectl":        {"logs": true, "describe": true, "get": true, "events": true, "top": true},
	"claw-compactor": {"benchmark": true},
	"pytest":         {}, "rg": {}, "find": {}, "ls": {}, "tree": {}, "echo": {},
}

func wrapRTKArguments(name, arguments string) string {
	baseName := name
	if _, suffix, ok := strings.Cut(name, "."); ok {
		baseName = suffix
	}
	if baseName != "shell" && baseName != "exec_command" {
		return arguments
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(arguments), &value); err != nil {
		return arguments
	}
	for _, key := range []string{"cmd", "command"} {
		command, ok := value[key].(string)
		if !ok {
			continue
		}
		wrapped, ok := wrapRTKCommand(command)
		if !ok {
			continue
		}
		value[key] = wrapped
		content, err := json.Marshal(value)
		if err == nil {
			return string(content)
		}
	}
	return arguments
}

func wrapRTKCommand(command string) (string, bool) {
	segmentStart := 0
	for index := 0; index <= len(command); {
		if index == len(command) {
			if offset, ok := rtkSegmentInsert(command[segmentStart:index]); ok {
				position := segmentStart + offset
				return command[:position] + "rtk " + command[position:], true
			}
			return command, false
		}
		rest := command[index:]
		separator := 0
		switch {
		case strings.HasPrefix(rest, "&&"), strings.HasPrefix(rest, "||"):
			separator = 2
		case strings.HasPrefix(rest, ";"), strings.HasPrefix(rest, "|"), strings.HasPrefix(rest, "\n"):
			separator = 1
		}
		if separator > 0 {
			if offset, ok := rtkSegmentInsert(command[segmentStart:index]); ok {
				position := segmentStart + offset
				return command[:position] + "rtk " + command[position:], true
			}
			index += separator
			segmentStart = index
			continue
		}
		_, width := runeAt(command, index)
		index += width
	}
	return command, false
}

func rtkSegmentInsert(segment string) (int, bool) {
	offset := skipSpaces(segment, 0)
	first, end, ok := shellToken(segment, offset)
	if !ok || first == "rtk" {
		return 0, false
	}
	for isEnvAssignment(first) {
		offset = skipSpaces(segment, end)
		first, end, ok = shellToken(segment, offset)
		if !ok {
			return 0, false
		}
	}
	commandName := first
	if slash := strings.LastIndex(commandName, "/"); slash >= 0 {
		commandName = commandName[slash+1:]
	}
	subcommands, found := rtkNoisyCommands[commandName]
	if !found {
		return 0, false
	}
	if len(subcommands) == 0 {
		return offset, true
	}
	for _, token := range strings.Fields(segment[end:]) {
		token = strings.Trim(token, `"'(){} `)
		if subcommands[token] {
			return offset, true
		}
	}
	return 0, false
}

func runeAt(value string, index int) (rune, int) {
	for _, current := range value[index:] {
		return current, len(string(current))
	}
	return 0, 1
}
func skipSpaces(value string, offset int) int {
	for offset < len(value) {
		current, width := runeAt(value, offset)
		if !isSpace(current) {
			break
		}
		offset += width
	}
	return offset
}
func isSpace(value rune) bool { return value == ' ' || value == '\t' || value == '\r' || value == '\n' }
func shellToken(value string, offset int) (string, int, bool) {
	if offset >= len(value) {
		return "", offset, false
	}
	end := offset
	for end < len(value) {
		current, width := runeAt(value, end)
		if isSpace(current) {
			break
		}
		end += width
	}
	return value[offset:end], end, end > offset
}
func isEnvAssignment(token string) bool {
	name, value, ok := strings.Cut(token, "=")
	if !ok || name == "" || value == "" {
		return false
	}
	for index, current := range name {
		if !(current == '_' || current >= 'a' && current <= 'z' || current >= 'A' && current <= 'Z' || index > 0 && current >= '0' && current <= '9') {
			return false
		}
	}
	return true
}
