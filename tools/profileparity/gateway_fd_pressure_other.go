//go:build !linux

package main

// Physical Linux RLIMIT/EMFILE parity is qualified only on Linux. No
// synthetic pass is awarded on other operating systems.
func checkGatewayFDPressure(_ string, _ cliOptions) ([]stepResult, error) { return nil, nil }
