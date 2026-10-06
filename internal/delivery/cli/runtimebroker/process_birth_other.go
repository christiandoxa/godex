//go:build !linux && !windows

package runtimebroker

func processBirthIdentity(uint32) string { return "" }
