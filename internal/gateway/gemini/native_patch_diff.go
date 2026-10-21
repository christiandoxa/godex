package gemini

import "strings"

// Tagged gemini_tooling_policy converts only sections with usable file headers.
func geminiNativeUnifiedDiff(lines []string) (string, bool) {
	output := []string{"*** Begin Patch"}
	for index := 0; index < len(lines); {
		git := strings.HasPrefix(lines[index], "diff --git ")
		if !git && !geminiNativeDiffHeaders(lines, index) {
			index++
			continue
		}
		next := index + 1
		if !git {
			next++
		}
		for next < len(lines) && !strings.HasPrefix(lines[next], "diff --git ") && (git || !geminiNativeDiffHeaders(lines, next)) {
			next++
		}
		output = append(output, geminiNativeDiffSection(lines[index:next])...)
		index = next
	}
	if len(output) == 1 {
		return "", false
	}
	return strings.Join(append(output, "*** End Patch"), "\n"), true
}

func geminiNativeDiffHeaders(lines []string, index int) bool {
	return index+1 < len(lines) && strings.HasPrefix(lines[index], "--- ") && strings.HasPrefix(lines[index+1], "+++ ")
}

func geminiNativeDiffSection(lines []string) []string {
	index := 0
	for index < len(lines) && !geminiNativeDiffHeaders(lines, index) {
		index++
	}
	if index+1 >= len(lines) {
		return nil
	}
	oldPath, oldPresent := geminiNativeDiffPath(lines[index])
	newPath, newPresent := geminiNativeDiffPath(lines[index+1])
	body := lines[index+2:]
	if !oldPresent && newPresent {
		output := []string{"*** Add File: " + newPath}
		for _, line := range body {
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				output = append(output, line)
			}
		}
		if len(output) > 1 {
			return output
		}
		return nil
	}
	if oldPresent && !newPresent {
		return []string{"*** Delete File: " + oldPath}
	}
	if !oldPresent || !newPresent {
		return nil
	}
	output := []string{"*** Update File: " + oldPath}
	if oldPath != newPath {
		output = append(output, "*** Move to: "+newPath)
	}
	return geminiNativeDiffHunks(output, body)
}

func geminiNativeDiffHunks(output, body []string) []string {
	sawHunk, changed := false, false
	for _, line := range body {
		switch {
		case strings.HasPrefix(line, "@@"):
			heading := "@@"
			if index := strings.Index(line[2:], "@@"); index >= 0 {
				if context := strings.Trim(line[index+4:], " \t\n\r\v\f"); context != "" {
					heading += " " + context
				}
			}
			output = append(output, heading)
			sawHunk = true
		case sawHunk && len(line) > 0 && (line[0] == '+' || line[0] == '-' || line[0] == ' '):
			output = append(output, line)
			changed = changed || line[0] == '+' || line[0] == '-'
		case sawHunk && line == `\ No newline at end of file`:
			output = append(output, "*** End of File")
		}
	}
	if !changed {
		return nil
	}
	return output
}

func geminiNativeDiffPath(header string) (string, bool) {
	path := header[4:]
	if len(path) >= 2 && path[0] == '"' && path[len(path)-1] == '"' {
		path = path[1 : len(path)-1]
	}
	if path == "/dev/null" {
		return "", false
	}
	if strings.HasPrefix(path, "a/") || strings.HasPrefix(path, "b/") {
		path = path[2:]
	}
	path = strings.Trim(path, " \t\n\r\v\f")
	return path, path != ""
}
