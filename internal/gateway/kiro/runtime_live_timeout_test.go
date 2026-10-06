package kiro

import (
	"testing"
	"time"
)

func TestKiroStreamIdleTimeoutUsesProdexDefaultAndEnvironment(t *testing.T) {
	source := NewSource()
	source.getenv = func(string) string { return "" }
	if got := source.streamIdleTimeout(); got != 300*time.Second {
		t.Fatalf("default idle timeout = %v", got)
	}
	source.getenv = func(key string) string {
		if key == "GODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS" {
			return "1250"
		}
		return ""
	}
	if got := source.streamIdleTimeout(); got != 1250*time.Millisecond {
		t.Fatalf("configured idle timeout = %v", got)
	}

	source.getenv = func(key string) string {
		if key == "PRODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS" {
			return "1750"
		}
		return ""
	}
	if got := source.streamIdleTimeout(); got != 1750*time.Millisecond {
		t.Fatalf("legacy configured idle timeout = %v", got)
	}
}
