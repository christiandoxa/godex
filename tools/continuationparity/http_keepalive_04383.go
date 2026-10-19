package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"
)

type httpKeepAliveResult04383 struct {
	replies                []exchange
	attempts               []attempt
	connections            []int
	reused, sameConnection bool
	elapsed                time.Duration
}

func runHTTPKeepAliveProduct04383(root string, p product) error {
	if err := prepareProduct(root, &p); err != nil {
		return err
	}
	p.env = append(p.env, "PRODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS=2000", "GODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS=2000")
	b := newBackend()
	defer b.close()
	b.mu.Lock()
	b.separateUsage = true
	b.mu.Unlock()
	endpoint, process, err := startGateway(p, b.server.URL+"/backend-api")
	if err != nil {
		return err
	}
	defer stopGateway(process)
	transport := &http.Transport{MaxIdleConnsPerHost: 1, IdleConnTimeout: time.Minute}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	var first net.Conn
	result := httpKeepAliveResult04383{}
	started := time.Now()
	for index := range 2 {
		var mu sync.Mutex
		var connection httptrace.GotConnInfo
		count := 0
		ctx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) { mu.Lock(); connection = info; count++; mu.Unlock() }})
		reply, err := postContext(ctx, client, endpoint+"/responses", `{"input":[]}`)
		if err != nil {
			return err
		}
		mu.Lock()
		result.connections = append(result.connections, count)
		if index == 0 {
			first = connection.Conn
		} else {
			result.reused, result.sameConnection = connection.Reused, first == connection.Conn
		}
		mu.Unlock()
		result.replies = append(result.replies, reply)
		if index == 0 {
			<-time.NewTimer(time.Until(started.Add(31 * time.Second))).C
		}
	}
	result.elapsed = time.Since(started)
	result.attempts = mustAttempts(b)
	if err := verifyHTTPKeepAlive04383(result); err != nil {
		return err
	}
	fmt.Printf("%s HTTP keepalive: same TCP connection; fresh 30-second budget after %s; statuses=200,200; upstream_requests=2 owner=account-a\n", p.name, result.elapsed)
	return nil
}

func verifyHTTPKeepAlive04383(result httpKeepAliveResult04383) error {
	if len(result.replies) != 2 || len(result.attempts) != 2 || len(result.connections) != 2 || !result.reused || !result.sameConnection || result.elapsed < 30*time.Second || result.elapsed > 36*time.Second {
		return errors.New("HTTP keepalive did not prove two requests on one socket past the original deadline")
	}
	for index, reply := range result.replies {
		upstream := result.attempts[index]
		if result.connections[index] != 1 || reply.status != 200 || reply.body != `{"id":"resp-main","object":"response","output":[]}` || upstream.account != "account-a" || !sameTransportJSON04383(upstream.body, `{"input":[]}`) {
			return fmt.Errorf("HTTP keepalive request %d changed status, body, owner or dispatch count: status=%d body=%s connections=%d", index+1, reply.status, reply.body, result.connections[index])
		}
	}
	return nil
}
