package gemini

import "bytes"

func geminiSSEDone04384(data []byte) bool {
	for _, line := range bytes.Split(data, []byte("\n")) {
		if bytes.Equal(bytes.TrimSpace(line), []byte("[DONE]")) {
			return true
		}
	}
	return false
}

// Joined JSON takes precedence; this fallback validates every separate value
// before any is observed, matching the tagged shared provider reader.
func geminiSeparateSSEValues04384(data []byte) ([][]byte, bool) {
	lines := bytes.Split(data, []byte("\n"))
	if len(lines) < 2 {
		return nil, false
	}
	var values [][]byte
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if _, valid := geminiSerdeResponseObject04384(line); !valid {
			return nil, false
		}
		values = append(values, line)
	}
	return values, len(values) > 0
}
