//go:build !linux && !darwin

package main

import "io"

func realtimeSocketMode04383(output io.Writer) error {
	_, err := io.WriteString(output, "realtime raw accept diagnostic: measured=false\n")
	return err
}
