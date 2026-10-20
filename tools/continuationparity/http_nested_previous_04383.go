package main

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"time"
)

const httpNestedPreviousReply04383 = `{"id":"resp-http-nested-previous","object":"response","output":[]}`

func httpNestedPreviousBody04383(shape string) string {
	field := ""
	switch shape {
	case "null":
		field = `"previous_response_id":null,`
	case "blank":
		field = `"previous_response_id":"  ",`
	case "direct":
		field = `"previous_response_id":"resp-nested",`
	}
	return `{"model":"gpt-6",` + field + `"response":{"previous_response_id":"resp-nested"},"input":[{"role":"user","content":"independent nested previous"}]}`
}

func runHTTPNestedPreviousProduct04383(root string, p product) error {
	for _, shape := range []string{"missing", "null", "blank", "direct"} {
		if err := runHTTPNestedPreviousCase04383(filepath.Join(root, shape), p, shape); err != nil {
			return fmt.Errorf("HTTP nested previous %s: %w", shape, err)
		}
	}
	return nil
}

func runHTTPNestedPreviousCase04383(root string, p product, shape string) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	if err := seedTransportAlternate04383(p, "http-nested-prev-"+shape); err != nil {
		return err
	}
	original, err := readHTTPResponseOwner04383(p, "resp-nested")
	if err != nil || original != "second" {
		return fmt.Errorf("unverified nested previous fixture: owner=%q err=%v", original, err)
	}
	p.env = append(p.env, "PRODEX_RUNTIME_LOG_RECORD=true")
	b := newTransportBackend04383("http-nested-prev-" + shape)
	defer b.server.Close()
	endpoint, stop, err := startTransportLaunch04383(p, b.server.URL+"/backend-api")
	if err != nil {
		return err
	}
	defer stop()
	client := &http.Client{Timeout: 8 * time.Second}
	for _, previous := range []string{"resp-ready-b", "resp-main"} {
		warmup, err := post(client, endpoint+"/responses", fmt.Sprintf(`{"previous_response_id":%q,"input":[]}`, previous))
		if err != nil || warmup.status != http.StatusOK {
			return fmt.Errorf("HTTP nested previous warmup %s status=%d err=%v", previous, warmup.status, err)
		}
	}
	until := time.Now().Add(5 * time.Second)
	for !transportProfilesReady04383(p, b) {
		if time.Now().After(until) {
			return errors.New("nested previous fixture has no applied authenticated Ready receipts")
		}
		time.Sleep(10 * time.Millisecond)
	}
	reply, err := post(client, endpoint+"/responses", httpNestedPreviousBody04383(shape))
	if err != nil {
		return err
	}
	b.mu.Lock()
	r := httpSoftConflictResult04383{reply: reply, attempts: append([]transportAttempt04383(nil), b.attempts...), invalid: b.invalid}
	b.mu.Unlock()
	if err := verifyHTTPNestedPrevious04383(shape, r); err != nil {
		return err
	}
	want := "main"
	if shape == "direct" {
		want = "second"
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		oldOwner, oldErr := readHTTPResponseOwner04383(p, "resp-nested")
		newOwner, newErr := readHTTPResponseOwner04383(p, "resp-http-nested-previous")
		if oldErr == nil && newErr == nil && oldOwner == "second" && newOwner == want {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("HTTP nested previous changed durable owner: old=%q new=%q want=%q errors=%v/%v", oldOwner, newOwner, want, oldErr, newErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
	fmt.Printf("%s HTTP nested previous=%s: one %s Ready request, original bytes, 200 and durable owners verified\n", p.name, shape, want)
	return nil
}

func verifyHTTPNestedPrevious04383(shape string, r httpSoftConflictResult04383) error {
	owner := "account-a"
	if shape == "direct" {
		owner = "account-b"
	}
	if r.invalid || r.reply.status != http.StatusOK || r.reply.body != httpNestedPreviousReply04383 ||
		len(r.attempts) != 1 || !r.attempts[0].ready || r.attempts[0].owner != owner ||
		r.attempts[0].turn != "" || r.attempts[0].body != httpNestedPreviousBody04383(shape) {
		return fmt.Errorf("HTTP nested previous mismatched route/wire: shape=%s invalid=%t status=%d attempts=%+v", shape, r.invalid, r.reply.status, r.attempts)
	}
	return nil
}
