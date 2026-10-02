package kiro

import "testing"

func TestKiroStreamQueueAppliesBackpressure(t *testing.T) {
	chunks := make(chan kiroStreamChunk, kiroStreamQueueCapacity)
	for index := 0; index < kiroStreamQueueCapacity; index++ {
		select {
		case chunks <- kiroStreamChunk{data: []byte("x")}:
		default:
			t.Fatalf("queue filled early at %d", index)
		}
	}
	select {
	case chunks <- kiroStreamChunk{data: []byte("overflow")}:
		t.Fatal("queue accepted work beyond configured capacity")
	default:
	}
}
