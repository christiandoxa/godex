package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (b *transportBackend04383) holdCapacity04383(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	b.invalid = b.invalid || b.occupied || r.Header.Get("ChatGPT-Account-Id") != "account-b"
	b.occupied = true
	b.mu.Unlock()
	defer func() { b.mu.Lock(); b.occupied = false; b.mu.Unlock() }()
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = fmt.Fprint(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp-capacity-blocker\"}}\n\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"occupied\"}\n\n")
	w.(http.Flusher).Flush()
	b.busy <- struct{}{}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			w.(http.Flusher).Flush()
		}
	}
}

func startTransportCapacityBlocker04383(p product, b *transportBackend04383, endpoint string) (func(), error) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		result, err := postContext(ctx, &http.Client{Timeout: 40 * time.Second}, endpoint+"/responses", `{"previous_response_id":"resp-ready-b","mode":"capacity-blocker","stream":true,"input":[]}`)
		if err == nil && (result.status != 200 || !strings.Contains(result.body, "resp-capacity-blocker")) {
			err = fmt.Errorf("capacity blocker response: %d %s", result.status, result.body)
		}
		done <- err
	}()
	stop := func() { cancel(); <-done }
	select {
	case <-b.busy:
		fmt.Printf("%s busy-history: account-b capacity occupied by one authenticated Responses request\n", p.name)
		return stop, nil
	case err := <-done:
		cancel()
		return nil, fmt.Errorf("capacity blocker ended before dispatch: %v", err)
	case <-time.After(5 * time.Second):
		stop()
		return nil, errors.New("capacity blocker was not dispatched")
	}
}
