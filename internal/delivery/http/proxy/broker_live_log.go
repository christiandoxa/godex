package proxy

import (
	"encoding/json"
	"strings"
	"sync"
)

const (
	brokerLiveLogMaxEntries = 512
	brokerLiveLogMaxBytes   = 2 * 1024 * 1024
	brokerLiveLogRecordMax  = 128 * 1024
)

type brokerLiveLogEntry struct {
	Sequence uint64 `json:"sequence"`
	Line     string `json:"line"`
}

type brokerLiveLogSnapshot struct {
	Cursor  uint64               `json:"cursor"`
	Dropped uint64               `json:"dropped"`
	Entries []brokerLiveLogEntry `json:"entries"`
}

type brokerLiveLog struct {
	mu           sync.Mutex
	entries      []brokerLiveLogEntry
	bytes        int
	dropped      uint64
	nextSequence uint64
}

func (log *brokerLiveLog) append(line string) {
	if log == nil {
		return
	}
	line = boundBrokerLiveLogLine(line)
	log.mu.Lock()
	defer log.mu.Unlock()
	log.nextSequence++
	entry := brokerLiveLogEntry{Sequence: log.nextSequence, Line: line}
	log.entries = append(log.entries, entry)
	log.bytes += len(line)
	for len(log.entries) > brokerLiveLogMaxEntries || log.bytes > brokerLiveLogMaxBytes {
		if len(log.entries) == 0 {
			break
		}
		log.bytes -= len(log.entries[0].Line)
		log.entries = log.entries[1:]
		log.dropped++
	}
}

func (log *brokerLiveLog) snapshot(after uint64, limit int) brokerLiveLogSnapshot {
	if log == nil {
		return brokerLiveLogSnapshot{}
	}
	if limit < 0 {
		limit = 0
	}
	if limit > brokerLiveLogMaxEntries {
		limit = brokerLiveLogMaxEntries
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	result := brokerLiveLogSnapshot{Cursor: log.nextSequence, Dropped: log.dropped}
	if limit == 0 {
		return result
	}
	for _, entry := range log.entries {
		if entry.Sequence <= after {
			continue
		}
		result.Entries = append(result.Entries, entry)
		if len(result.Entries) == limit {
			break
		}
	}
	if len(result.Entries) > 0 {
		result.Cursor = result.Entries[len(result.Entries)-1].Sequence
	}
	return result
}

func boundBrokerLiveLogLine(line string) string {
	if len(line) <= brokerLiveLogRecordMax {
		return line
	}
	trimmed := strings.TrimSpace(line)
	var value map[string]any
	if json.Unmarshal([]byte(trimmed), &value) == nil {
		compact := map[string]any{"message": "[live log record truncated]"}
		for _, key := range []string{"timestamp", "pid", "event"} {
			if current, ok := value[key]; ok {
				compact[key] = current
			}
		}
		if encoded, err := json.Marshal(compact); err == nil {
			return string(encoded) + "\n"
		}
	}
	suffix := " …[truncated]"
	limit := brokerLiveLogRecordMax - len(suffix)
	if limit < 0 {
		return suffix[:brokerLiveLogRecordMax]
	}
	for limit > 0 && limit < len(line) && (line[limit]&0xC0) == 0x80 {
		limit--
	}
	return line[:limit] + suffix
}

func brokerContinuityReasons(snapshot brokerLiveLogSnapshot) brokerContinuityFailureReasons {
	result := brokerContinuityFailureReasons{
		ChainRetriedOwner:          map[string]int{},
		ChainDeadUpstreamConfirmed: map[string]int{},
		StaleContinuation:          map[string]int{},
	}
	for _, entry := range snapshot.Entries {
		event, reason := brokerContinuityEventReason(entry.Line)
		if reason == "" {
			continue
		}
		switch event {
		case "chain_retried_owner":
			result.ChainRetriedOwner[reason]++
		case "chain_dead_upstream_confirmed":
			result.ChainDeadUpstreamConfirmed[reason]++
		case "stale_continuation":
			result.StaleContinuation[reason]++
		}
	}
	return result
}

func brokerContinuityEventReason(line string) (string, string) {
	var value map[string]any
	if json.Unmarshal([]byte(strings.TrimSpace(line)), &value) == nil {
		event, _ := value["event"].(string)
		reason, _ := value["reason"].(string)
		if event != "" && reason != "" {
			return event, reason
		}
		if message, _ := value["message"].(string); message != "" {
			return brokerContinuityEventReason(message)
		}
	}
	event := ""
	for _, candidate := range []string{"chain_retried_owner", "chain_dead_upstream_confirmed", "stale_continuation"} {
		if strings.Contains(line, candidate) {
			event = candidate
			break
		}
	}
	if event == "" {
		return "", ""
	}
	const marker = "reason="
	index := strings.Index(line, marker)
	if index < 0 {
		return event, ""
	}
	reason := line[index+len(marker):]
	if end := strings.IndexAny(reason, " \t\r\n"); end >= 0 {
		reason = reason[:end]
	}
	return event, strings.Trim(reason, "\"")
}
