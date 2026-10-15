package routing

import (
	"testing"
	"time"
)

// Exact tagged Prodex 0.437.14 Mojo text parser vectors. These detect
// Go strconv's acceptance of signed integers, zero intervals, floating-point
// overflows and Unicode/ASCII marker behavior that differ from the oracle.
func TestProdex043714RetryAfterMessageTextBoundary(t *testing.T) {
	for _, tc := range []struct {
		message string
		want    time.Duration
		valid   bool
	}{
		{"Please TRY AGAIN IN 11.054s.", 11054 * time.Millisecond, true},
		{"try again in　1.1ms", 2 * time.Millisecond, true},
		{"try again in\t0.9991second", 1000 * time.Millisecond, true},
		{"try again in 301S", 300 * time.Second, true},
		{"🧭 Please TRY AGAIN IN\u20031.25MS", 2 * time.Millisecond, true},
		{"try again in 7s; try again in 20s", 7 * time.Second, true},
		{"try again in 0s", 0, false},
		{"try again in 1 minute", 0, false},
		{"try again in 1.2.3s", 0, false},
		{"try again in NaNs", 0, false},
		{"unrelated 42s", 0, false},
		{"try again in 340282366920938463463374607431768211456ms", 0, false},
		{"try again in 340282366920938463463374607431768211455ms", 300 * time.Second, true},
		{"try again in 340282366920938463463374607431768211.456s", 0, false},
		{"try again in 340282366920938463463374607431768211.455s", 300 * time.Second, true},
		{"try again in .1s", 0, false},
		{"try again in 1.s", time.Second, true},
		{"try again in 0.0001s", time.Millisecond, true},
	} {
		got, ok := retryAfterMessage(tc.message)
		if ok != tc.valid || got != tc.want {
			t.Errorf("message %q = (%v,%t), want (%v,%t)", tc.message, got, ok, tc.want, tc.valid)
		}
	}
}

func TestProdex043714RetryAfterHeaderTextBoundary(t *testing.T) {
	for _, tc := range []struct {
		header string
		want   time.Duration
		valid  bool
	}{
		{"   +1  ", 0, false},
		{"　301\t", 300 * time.Second, true},
		{" 1 ", time.Second, true},
		{"0", 0, false},
		{"1.1", 0, false},
		{"18446744073709551615", 300 * time.Second, true},
		{"18446744073709551616", 0, false},
		{"", 0, false},
	} {
		got, ok := retryAfterHeader(tc.header)
		if ok != tc.valid || got != tc.want {
			t.Errorf("header %q = (%v,%t), want (%v,%t)", tc.header, got, ok, tc.want, tc.valid)
		}
	}
}
