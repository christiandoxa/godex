package openai

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type websocketCookieObservation struct {
	host   string
	path   string
	cookie string
}

type websocketCookieDestination struct {
	url       *url.URL
	transport http.RoundTripper
}

type websocketCookieRouter map[string]websocketCookieDestination

func (router websocketCookieRouter) RoundTrip(request *http.Request) (*http.Response, error) {
	destination, ok := router[request.URL.Hostname()]
	if !ok {
		return nil, fmt.Errorf("no websocket test destination for host %q", request.URL.Hostname())
	}
	rewritten := request.Clone(request.Context())
	target := *request.URL
	target.Scheme, target.Host = destination.url.Scheme, destination.url.Host
	rewritten.URL, rewritten.Host = &target, destination.url.Host
	return destination.transport.RoundTrip(rewritten)
}

func TestExecuteWebSocketCapturesAndReplaysProfileScopedCookies(t *testing.T) {
	observed := make(chan websocketCookieObservation, 32)
	newServer := func(host string, secure bool) (*httptest.Server, websocketCookieDestination) {
		handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			observed <- websocketCookieObservation{host: host, path: request.URL.Path, cookie: request.Header.Get("Cookie")}
			if strings.HasSuffix(request.URL.Path, "/reject") {
				writer.Header().Add("Set-Cookie", "rejected=from-401; Path=/backend-api; Secure")
				writer.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(writer, "unauthorized")
				return
			}
			var setCookies []string
			switch {
			case host == "api-a.test" && strings.HasSuffix(request.URL.Path, "/seed"):
				setCookies = []string{
					"session=from-101; Path=/backend-api/private; Secure",
					"caller=from-101; Path=/backend-api/private; Secure",
					"base=from-101; Path=/backend-api; Secure",
				}
			case host == "api-b.test" && strings.HasSuffix(request.URL.Path, "/seed"):
				setCookies = []string{"foreign=from-b; Path=/backend-api; Secure"}
			case host == "api-c.test" && strings.HasSuffix(request.URL.Path, "/secure-seed"):
				setCookies = []string{"insecure=must-not-store; Path=/backend-api; Secure"}
			}
			writeWebSocketTestUpgrade(t, writer, request, setCookies)
		})
		var server *httptest.Server
		if secure {
			server = httptest.NewTLSServer(handler)
		} else {
			server = httptest.NewServer(handler)
		}
		t.Cleanup(server.Close)
		serverURL, err := url.Parse(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		return server, websocketCookieDestination{url: serverURL, transport: server.Client().Transport}
	}
	serverA, destinationA := newServer("api-a.test", true)
	serverB, destinationB := newServer("api-b.test", true)
	serverC, destinationC := newServer("api-c.test", false)
	_ = serverA
	_ = serverB
	_ = serverC
	client := &http.Client{Transport: websocketCookieRouter{
		"api-a.test": destinationA,
		"api-b.test": destinationB,
		"api-c.test": destinationC,
	}}
	transport, err := NewTransport("https://api-a.test/backend-api", client, websocketAuth{})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	accountA := proxymodel.Account{ID: "profile-a", Home: "home-a"}
	accountB := proxymodel.Account{ID: "profile-b", Home: "home-b"}
	do := func(account proxymodel.Account, path string, headers http.Header, wantStatus int) websocketCookieObservation {
		t.Helper()
		if headers == nil {
			headers = make(http.Header)
		}
		headers.Set("Connection", "Upgrade")
		headers.Set("Upgrade", "websocket")
		headers.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
		headers.Set("Sec-WebSocket-Version", "13")
		response, err := transport.ExecuteWebSocket(context.Background(), proxymodel.Request{
			Method: http.MethodGet,
			Path:   path,
			Header: headers,
		}, account)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != wantStatus {
			t.Fatalf("websocket response status = %d, want %d", response.StatusCode, wantStatus)
		}
		return <-observed
	}
	check := func(host, path string, account proxymodel.Account, headers http.Header, want string) {
		t.Helper()
		observation := do(account, path, headers, http.StatusSwitchingProtocols)
		if observation.host != host || observation.path != path || observation.cookie != want {
			t.Fatalf("upstream %s %s Cookie = %q, want %q", observation.host, observation.path, observation.cookie, want)
		}
	}

	seed := do(accountA, "/backend-api/seed", nil, http.StatusSwitchingProtocols)
	if seed.cookie != "" {
		t.Fatalf("first handshake Cookie = %q, want empty", seed.cookie)
	}
	rejected := do(accountA, "/backend-api/reject", nil, http.StatusUnauthorized)
	if rejected.cookie != "base=from-101" {
		t.Fatalf("rejection handshake Cookie = %q, want %q", rejected.cookie, "base=from-101")
	}
	check("api-a.test", "/backend-api/private/check", accountB, nil, "")

	transport.upstream, _ = url.Parse("https://api-b.test/backend-api")
	check("api-b.test", "/backend-api/seed", accountA, nil, "")
	transport.upstream, _ = url.Parse("https://api-a.test/backend-api")
	check("api-a.test", "/backend-api/private/check", accountA, nil,
		"caller=from-101; session=from-101; base=from-101; rejected=from-401")
	check("api-a.test", "/backend-api/privateish/check", accountA, nil,
		"base=from-101; rejected=from-401")
	check("api-a.test", "/backend-api/private/check", accountA,
		http.Header{"Cookie": {"session=caller; caller=caller"}},
		"session=caller; caller=caller; base=from-101; rejected=from-401")

	transport.upstream, _ = url.Parse("http://api-c.test/backend-api")
	check("api-c.test", "/backend-api/secure-seed", accountA, nil, "")
	check("api-c.test", "/backend-api/secure-check", accountA, nil, "")
}

func writeWebSocketTestUpgrade(t *testing.T, writer http.ResponseWriter, request *http.Request, setCookies []string) {
	t.Helper()
	connection, buffered, err := writer.(http.Hijacker).Hijack()
	if err != nil {
		t.Errorf("hijack websocket test upstream: %v", err)
		return
	}
	defer connection.Close()
	digest := sha1.Sum([]byte(request.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	_, _ = fmt.Fprintf(buffered, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n", base64.StdEncoding.EncodeToString(digest[:]))
	for _, cookie := range setCookies {
		_, _ = fmt.Fprintf(buffered, "Set-Cookie: %s\r\n", cookie)
	}
	_, _ = io.WriteString(buffered, "\r\n")
	if err := buffered.Flush(); err != nil {
		t.Errorf("flush websocket test handshake: %v", err)
	}
}

func TestWebSocketCookieJarEnforcesTaggedStorageBounds(t *testing.T) {
	jar := newWebSocketCookieJar()
	target, _ := url.Parse("https://api.example.test/responses")
	headers := make(http.Header)
	for index := 0; index <= webSocketCookieMaxPerHost; index++ {
		headers.Add("Set-Cookie", fmt.Sprintf("cookie%02d=value; Path=/", index))
	}
	jar.capture("profile-a", target, headers)
	if got := len(jar.hosts[webSocketCookieKey{profile: "profile-a", host: "api.example.test"}]); got != webSocketCookieMaxPerHost {
		t.Fatalf("cookies retained for host = %d, want %d", got, webSocketCookieMaxPerHost)
	}
	for index := 0; index <= webSocketCookieMaxHosts; index++ {
		target, _ = url.Parse(fmt.Sprintf("https://host-%03d.example.test/responses", index))
		jar.capture("profile-b", target, http.Header{"Set-Cookie": {"session=value; Path=/"}})
	}
	if got := len(jar.hosts); got != webSocketCookieMaxHosts {
		t.Fatalf("host/profile slots retained = %d, want %d", got, webSocketCookieMaxHosts)
	}
}
