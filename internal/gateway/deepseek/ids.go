package deepseek

import (
	"encoding/binary"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

var deepSeekFallbackIDSequence atomic.Uint64

func deepSeekResponseFallbackID() string { return "resp_deepseek_" + deepSeekUUIDv7().String() }

func deepSeekCallFallbackID() string { return "call_deepseek_" + deepSeekUUIDv7().String() }

func deepSeekUUIDv7() uuid.UUID {
	if value, err := uuid.NewV7(); err == nil {
		return value
	}
	// Preserve the UUIDv7 contract even if the platform random source fails.
	// Timestamp plus a process-local monotonic sequence keeps fallback IDs unique.
	var value uuid.UUID
	millis := uint64(time.Now().UnixMilli())
	value[0] = byte(millis >> 40)
	value[1] = byte(millis >> 32)
	value[2] = byte(millis >> 24)
	value[3] = byte(millis >> 16)
	value[4] = byte(millis >> 8)
	value[5] = byte(millis)
	sequence := deepSeekFallbackIDSequence.Add(1)
	binary.BigEndian.PutUint64(value[8:], sequence)
	value[6] = 0x70 | byte(sequence>>8)&0x0f
	value[7] = byte(sequence)
	value[8] = value[8]&0x3f | 0x80
	return value
}
