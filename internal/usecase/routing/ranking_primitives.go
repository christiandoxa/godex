package routing

import (
	"crypto/rand"
	"encoding/binary"
	"math"
	"strings"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type candidateBackoffSortKey struct {
	class  uint8
	first  int64
	second int64
	retry  int64
}

func profileBackoffSortKey(circuitUntil, transportUntil, retryUntil time.Time, now time.Time) candidateBackoffSortKey {
	active := func(value time.Time) int64 {
		if value.IsZero() || !value.After(now) {
			return 0
		}
		return value.Unix()
	}
	circuit, transport, retry := active(circuitUntil), active(transportUntil), active(retryUntil)
	switch {
	case circuit == 0 && transport == 0 && retry == 0:
		return candidateBackoffSortKey{}
	case circuit > 0 && transport == 0 && retry == 0:
		return candidateBackoffSortKey{class: 1, first: circuit}
	case circuit == 0 && transport > 0 && retry == 0:
		return candidateBackoffSortKey{class: 2, first: transport}
	case circuit == 0 && transport == 0 && retry > 0:
		return candidateBackoffSortKey{class: 3, first: retry}
	case circuit > 0 && transport > 0 && retry == 0:
		return candidateBackoffSortKey{class: 4, first: transport, second: circuit}
	case circuit > 0 && transport == 0 && retry > 0:
		return candidateBackoffSortKey{class: 5, first: retry, second: circuit}
	case circuit == 0 && transport > 0 && retry > 0:
		return candidateBackoffSortKey{class: 6, first: transport, second: retry}
	default:
		return candidateBackoffSortKey{class: 7, first: transport, second: circuit, retry: retry}
	}
}

func compareBackoffSortKey(left, right candidateBackoffSortKey) int {
	for _, pair := range [][2]int64{{int64(left.class), int64(right.class)}, {left.first, right.first}, {left.second, right.second}, {left.retry, right.retry}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}

func promptCacheAffinitySortKey(key, owner, profile string) (uint8, uint64) {
	key = strings.TrimSpace(key)
	owner = strings.TrimSpace(owner)
	if key == "" {
		return 0, 0
	}
	if owner != "" && profile == owner {
		return 0, 0
	}
	priority := uint8(0)
	if owner != "" {
		priority = 1
	}
	const offset = uint64(14695981039346656037)
	const prime = uint64(1099511628211)
	hash := offset
	for _, part := range [][]byte{[]byte("prodex-prompt-cache-affinity-v1"), {0}, []byte(key), {0}, []byte(profile)} {
		for _, value := range part {
			hash = (hash ^ uint64(value)) * prime
		}
	}
	return priority, math.MaxUint64 - hash
}

func selectionJitter(sequence uint64, profile string, route quotamodel.RouteKind) uint64 {
	buffer := make([]byte, 0, 8+len(profile)+len(routeHealthRoute(route))+2)
	var sequenceBytes [8]byte
	binary.LittleEndian.PutUint64(sequenceBytes[:], sequence)
	buffer = append(buffer, sequenceBytes[:]...)
	buffer = append(buffer, profile...)
	buffer = append(buffer, 0xff)
	buffer = append(buffer, routeHealthRoute(route)...)
	buffer = append(buffer, 0xff)
	return sipHash13(buffer)
}

func sipHash13(data []byte) uint64 {
	v0 := uint64(0x736f6d6570736575)
	v1 := uint64(0x646f72616e646f6d)
	v2 := uint64(0x6c7967656e657261)
	v3 := uint64(0x7465646279746573)
	round := func() {
		v0 += v1
		v1 = bitsRotateLeft64(v1, 13)
		v1 ^= v0
		v0 = bitsRotateLeft64(v0, 32)
		v2 += v3
		v3 = bitsRotateLeft64(v3, 16)
		v3 ^= v2
		v0 += v3
		v3 = bitsRotateLeft64(v3, 21)
		v3 ^= v0
		v2 += v1
		v1 = bitsRotateLeft64(v1, 17)
		v1 ^= v2
		v2 = bitsRotateLeft64(v2, 32)
	}
	offset := 0
	for len(data)-offset >= 8 {
		message := binary.LittleEndian.Uint64(data[offset:])
		v3 ^= message
		round()
		v0 ^= message
		offset += 8
	}
	last := uint64(len(data)) << 56
	for index, value := range data[offset:] {
		last |= uint64(value) << (8 * index)
	}
	v3 ^= last
	round()
	v0 ^= last
	v2 ^= 0xff
	round()
	round()
	round()
	return v0 ^ v1 ^ v2 ^ v3
}

func bitsRotateLeft64(value uint64, shift uint) uint64 { return value<<shift | value>>(64-shift) }

func newSelectionSequenceSeed() uint64 {
	var random [4]byte
	prefix := uint32(1)
	if _, err := rand.Read(random[:]); err == nil {
		prefix = binary.LittleEndian.Uint32(random[:])
		if prefix == 0 {
			prefix = 1
		}
	}
	return uint64(prefix)<<32 | 1
}

func requestPromptCacheKey(request proxymodel.Request) string {
	return promptCacheKeyFromBody(request.Body)
}

func (router *Router) transportBackoffUntil(accountID string, selection quotamodel.Selection, now time.Time) time.Time {
	remaining := router.transportBackoffRemaining(accountID, selection, now)
	if remaining <= 0 {
		return time.Time{}
	}
	return now.Add(remaining)
}

func (router *Router) routeCircuitUntil(accountID string, selection quotamodel.Selection, now time.Time) time.Time {
	remaining := router.routeCircuitRemaining(accountID, selection, now)
	if remaining <= 0 {
		return time.Time{}
	}
	return now.Add(remaining)
}
