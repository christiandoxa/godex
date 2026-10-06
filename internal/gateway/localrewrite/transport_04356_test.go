package localrewrite

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04356LocalRewriteForwardsSafeMountedPathQueryAndSyntheticAuth(t *testing.T) {
	var seenPath, seenQuery, seenAuth, seenAccount, seenConnection, seenBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seenPath = request.URL.Path
		seenQuery = request.URL.RawQuery
		seenAuth = request.Header.Get("Authorization")
		seenAccount = request.Header.Get("ChatGPT-Account-Id")
		seenConnection = request.Header.Get("Connection")
		body, _ := io.ReadAll(request.Body)
		seenBody = string(body)
		writer.Header().Set("X-Upstream", "ok")
		writer.Header().Set("Connection", "keep-alive")
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	transport, err := NewTransport(upstream.URL+"/v1", upstream.Client())
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method:   http.MethodPost,
		Path:     "/v1/responses",
		RawQuery: "trace=one%20two",
		Header: http.Header{
			"Authorization":      {"Bearer profile-secret"},
			"ChatGPT-Account-Id": {"workspace-secret"},
			"Connection":         {"keep-alive"},
			"Content-Type":       {"application/json"},
		},
		Body: []byte(`{"model":"qwen-local"}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusCreated || string(body) != `{"ok":true}` {
		t.Fatalf("response = status:%d body:%q", response.StatusCode, body)
	}
	if seenPath != "/v1/responses" || seenQuery != "trace=one%20two" {
		t.Fatalf("upstream target = %q?%s", seenPath, seenQuery)
	}
	if seenAuth != "Bearer "+ProviderRuntimeAPIKey {
		t.Fatalf("upstream auth = %q", seenAuth)
	}
	if seenAccount != "" || seenConnection != "" {
		t.Fatalf("sensitive/transport headers leaked: account=%q connection=%q", seenAccount, seenConnection)
	}
	if seenBody != `{"model":"qwen-local"}` {
		t.Fatalf("upstream body = %q", seenBody)
	}
	if response.Header.Get("X-Upstream") != "ok" || response.Header.Get("Connection") != "" {
		t.Fatalf("response headers = %#v", response.Header)
	}
}

func TestProdex04356LocalRewriteAcceptsManagedMountAndRejectsUnsafeTargets(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/custom/responses/compact" {
			t.Fatalf("upstream path = %q", request.URL.Path)
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	transport, err := NewTransport(upstream.URL+"/custom", upstream.Client())
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost,
		Path:   "/backend-api/godex/responses/compact",
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()

	for _, target := range []string{
		"/v1/../admin",
		"/v1/%2e%2e/admin",
		"//v1/responses",
		"/v1/responses#fragment",
		"/v1/responses bad",
		"/v1/%zz",
	} {
		if RequestTargetValid(target) {
			t.Fatalf("unsafe target accepted: %q", target)
		}
	}
	for _, target := range []string{
		"/v1/responses",
		"/v1/responses?x=a%2Fb",
		"/backend-api/godex/responses/compact",
	} {
		if !RequestTargetValid(target) {
			t.Fatalf("safe target rejected: %q", target)
		}
	}

	_, err = transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodGet,
		Path:   "/unsupported",
	}, proxymodel.Account{})
	var presentation *proxymodel.Error
	if err == nil || !strings.Contains(err.Error(), "not supported") ||
		!errors.As(err, &presentation) || presentation.StatusCode != http.StatusNotFound {
		t.Fatalf("unsupported route = %v", err)
	}
}
