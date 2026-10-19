package main

import (
	"fmt"
	"time"
)

func waitHTTPTransportDurable04383(p product, scenario string, result *httpTransportResult04383, verify func(string, httpTransportResult04383) error) error {
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		owners, err := readTransportOwners04383(p)
		if err == nil {
			result.owners = owners
			err = verify(scenario, *result)
		}
		if err == nil {
			return nil
		}
		select {
		case <-deadline.C:
			return fmt.Errorf("durable HTTP transport state did not settle: %w", err)
		case <-ticker.C:
		}
	}
}
