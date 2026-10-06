package proxy

import (
	"fmt"
	"strings"
	"testing"
)

func TestProdex04356BrokerLiveLogIsBoundedAndCursorBased(t *testing.T) {
	log := &brokerLiveLog{}
	for index := 0; index < brokerLiveLogMaxEntries+10; index++ {
		log.append(fmt.Sprintf("line-%d", index))
	}
	snapshot := log.snapshot(0, brokerLiveLogMaxEntries)
	if len(snapshot.Entries) != brokerLiveLogMaxEntries || snapshot.Dropped != 10 ||
		snapshot.Cursor != uint64(brokerLiveLogMaxEntries+10) {
		t.Fatalf("bounded snapshot = %#v", snapshot)
	}
	first := snapshot.Entries[0].Sequence
	if first != 11 {
		t.Fatalf("oldest retained sequence = %d, want 11", first)
	}
	next := log.snapshot(snapshot.Cursor, brokerLiveLogMaxEntries)
	if next.Cursor != snapshot.Cursor || next.Dropped != 10 || len(next.Entries) != 0 {
		t.Fatalf("empty incremental snapshot = %#v", next)
	}
}

func TestProdex04356BrokerLiveLogCompactsOversizedRecords(t *testing.T) {
	log := &brokerLiveLog{}
	log.append(strings.Repeat("x", brokerLiveLogRecordMax+1024))
	snapshot := log.snapshot(0, 1)
	if len(snapshot.Entries) != 1 || len(snapshot.Entries[0].Line) > brokerLiveLogRecordMax {
		t.Fatalf("oversized live log entry = %#v", snapshot)
	}
	if !strings.Contains(snapshot.Entries[0].Line, "truncated") {
		t.Fatalf("oversized record missing truncation marker: %q", snapshot.Entries[0].Line)
	}
}

func TestProdex04356BrokerContinuityReasonsParseLiveLines(t *testing.T) {
	log := &brokerLiveLog{}
	log.append(`{"event":"chain_retried_owner","reason":"quota"}`)
	log.append("event=chain_dead_upstream_confirmed reason=previous_response_not_found route=responses")
	log.append("stale_continuation reason=previous_response_not_found route=websocket")
	reasons := brokerContinuityReasons(log.snapshot(0, 16))
	if reasons.ChainRetriedOwner["quota"] != 1 ||
		reasons.ChainDeadUpstreamConfirmed["previous_response_not_found"] != 1 ||
		reasons.StaleContinuation["previous_response_not_found"] != 1 {
		t.Fatalf("continuity reasons = %#v", reasons)
	}
}
