package codex

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const sessionTimestampPrefix = "\"timestamp\":\""

func lastSessionEventTimestamp(contents string) (time.Time, bool) {
	lines := strings.Split(contents, "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		if timestamp, ok := sessionLineTimestamp(lines[index]); ok {
			return timestamp, true
		}
	}
	return time.Time{}, false
}

func sessionLineTimestamp(line string) (time.Time, bool) {
	start := strings.Index(line, sessionTimestampPrefix)
	if start < 0 {
		return time.Time{}, false
	}
	start += len(sessionTimestampPrefix)
	end := strings.IndexByte(line[start:], '"')
	if end < 0 {
		return time.Time{}, false
	}
	timestamp, err := time.Parse(time.RFC3339Nano, line[start:start+end])
	if err != nil {
		return time.Time{}, false
	}
	return timestamp.UTC(), true
}

func restoreSessionFileModifiedTime(path, contents string) error {
	timestamp, ok := lastSessionEventTimestamp(contents)
	if !ok {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect session file for modified-time restore: %w", err)
	}
	if err := os.Chtimes(path, sessionFileAccessTime(info), timestamp); err != nil {
		return fmt.Errorf("restore session modified time: %w", err)
	}
	return nil
}
