package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"time"
)

type transportLifecycleResult04383 struct {
	scenario, code, message, termination string
	status, heartbeats                   int
	elapsed                              time.Duration
	canceled, invalid                    bool
	handshakes                           []string
	attempts                             []transportAttempt04383
	frames                               []string
}

func runTransportLifecycle04383(p product, b *transportBackend04383, connection net.Conn, reader *bufio.Reader) error {
	result := transportLifecycleResult04383{scenario: b.scenario}
	switch b.scenario {
	case "deadline", "busy-history":
		_ = connection.SetDeadline(time.Now().Add(34 * time.Second))
		start := time.Now()
		var err error
		result.code, result.status, result.message, result.heartbeats, err = readDeadlineError04382(connection, reader)
		result.elapsed = time.Since(start)
		if err != nil {
			return err
		}
	case "cancellation":
		// Wait for a real upstream inference before canceling the eligible
		// full-history request, then require transport teardown, not a timer.
		deadline := time.NewTimer(5 * time.Second)
		defer deadline.Stop()
		for {
			b.mu.Lock()
			started := len(b.attempts) > 0
			b.mu.Unlock()
			if started {
				break
			}
			select {
			case <-deadline.C:
				return fmt.Errorf("cancellation never reached upstream")
			case <-time.After(10 * time.Millisecond):
			}
		}
		_ = connection.Close()
		select {
		case <-b.canceled:
			result.canceled = true
		case <-deadline.C:
			return fmt.Errorf("client cancellation did not close upstream")
		}
	case "postcommit":
		for {
			opcode, payload, _, err := readWebSocketLivenessFrame04383(connection, reader)
			if err == io.EOF {
				result.termination = "tcp-eof"
				break
			}
			if err != nil {
				return err
			}
			if opcode != 1 {
				return fmt.Errorf("postcommit opcode %d", opcode)
			}
			result.frames = append(result.frames, string(payload))
		}
	}
	b.mu.Lock()
	result.invalid = b.invalid
	if b.scenario == "busy-history" && !b.occupied {
		result.invalid = true
	}
	result.attempts = append([]transportAttempt04383(nil), b.attempts...)
	result.handshakes = append([]string(nil), b.handshakes...)
	b.mu.Unlock()
	if err := verifyTransportLifecycle04383(result); err != nil {
		return err
	}
	fmt.Printf("%s %s: eligible full history; authenticated Ready alternate; one account-a attempt; no replay\n", p.name, b.scenario)
	return nil
}

func verifyTransportLifecycle04383(result transportLifecycleResult04383) error {
	if result.invalid || len(result.attempts) != 1 || len(result.handshakes) != 1 || result.handshakes[0] != "account-a" || result.attempts[0].owner != "account-a" || !result.attempts[0].ready || result.attempts[0].turn != "turn-a" || !sameTransportJSON04383(result.attempts[0].body, transportHistory04383) {
		return fmt.Errorf("%s replayed or changed context: handshakes=%v attempts=%+v", result.scenario, result.handshakes, result.attempts)
	}
	switch result.scenario {
	case "deadline", "busy-history":
		minimumHeartbeats := 1
		if result.scenario == "busy-history" {
			minimumHeartbeats = 0
		}
		if result.code != "request_timeout" || result.status != 504 || result.message != "request timed out" || result.heartbeats < minimumHeartbeats || result.heartbeats > 40 || result.elapsed < 27500*time.Millisecond || result.elapsed > 31*time.Second {
			return fmt.Errorf("full-history deadline mutated: %s/%d/%s heartbeats=%d elapsed=%s", result.code, result.status, result.message, result.heartbeats, result.elapsed)
		}
	case "cancellation":
		if !result.canceled {
			return fmt.Errorf("upstream transport was not canceled")
		}
	case "postcommit":
		if result.termination != "tcp-eof" || len(result.frames) != 2 || result.frames[0] != `{"type":"response.created","response":{"id":"discarded-transport"}}` || result.frames[1] != `{"type":"response.output_text.delta","delta":"visible"}` {
			return fmt.Errorf("postcommit output/termination changed: %q %s", result.frames, result.termination)
		}
	default:
		return fmt.Errorf("unknown lifecycle control %q", result.scenario)
	}
	return nil
}
