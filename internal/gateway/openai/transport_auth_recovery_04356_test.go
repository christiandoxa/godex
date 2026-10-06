package openai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type authReloadSequence struct {
	mu     sync.Mutex
	values []proxymodel.Auth
	reads  int
}

type authRefreshSequence struct {
	authReloadSequence
	refreshed    proxymodel.Auth
	refreshErr   error
	refreshCalls int
}

func (reader *authRefreshSequence) RefreshUnauthorizedAuth(
	context.Context,
	string,
	proxymodel.Auth,
) (proxymodel.Auth, error) {
	reader.refreshCalls++
	return reader.refreshed, reader.refreshErr
}

func (reader *authReloadSequence) ReadAuth(context.Context, string) (proxymodel.Auth, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	index := reader.reads
	reader.reads++
	if index >= len(reader.values) {
		index = len(reader.values) - 1
	}
	return reader.values[index], nil
}

func TestProdex04356UnauthorizedReloadRetriesOnlyWhenAuthChanged(t *testing.T) {
	for _, fixture := range []struct {
		name       string
		auth       []proxymodel.Auth
		wantStatus int
		wantSeen   []string
	}{
		{
			name: "changed auth retries same profile",
			auth: []proxymodel.Auth{
				{AccessToken: "old-token", AccountID: "account"},
				{AccessToken: "new-token", AccountID: "account"},
			},
			wantStatus: http.StatusOK,
			wantSeen:   []string{"Bearer old-token", "Bearer new-token"},
		},
		{
			name: "unchanged auth does not blind retry",
			auth: []proxymodel.Auth{
				{AccessToken: "old-token", AccountID: "account"},
				{AccessToken: "old-token", AccountID: "account"},
			},
			wantStatus: http.StatusUnauthorized,
			wantSeen:   []string{"Bearer old-token"},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			var seen []string
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				seen = append(seen, request.Header.Get("Authorization"))
				if request.Header.Get("Authorization") == "Bearer new-token" {
					writer.WriteHeader(http.StatusOK)
					_, _ = io.WriteString(writer, "ok")
					return
				}
				writer.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(writer, "unauthorized")
			}))
			defer upstream.Close()

			reader := &authReloadSequence{values: fixture.auth}
			transport, err := NewTransport(upstream.URL, nil, reader)
			if err != nil {
				t.Fatal(err)
			}
			defer transport.Close()
			response, err := transport.Execute(t.Context(), proxymodel.Request{
				Method: http.MethodPost, Path: "/responses", Header: make(http.Header),
			}, proxymodel.Account{ID: "profile-a", Home: "/profile-a", Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != fixture.wantStatus ||
				strings.Join(seen, ",") != strings.Join(fixture.wantSeen, ",") {
				t.Fatalf("status/seen = %d/%v, want %d/%v",
					response.StatusCode, seen, fixture.wantStatus, fixture.wantSeen)
			}
		})
	}
}

func TestProdex04356UnauthorizedRefreshRetriesAfterUnchangedReload(t *testing.T) {
	var seen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seen = append(seen, request.Header.Get("Authorization"))
		if request.Header.Get("Authorization") == "Bearer refreshed-token" {
			writer.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(writer, "ok")
			return
		}
		writer.WriteHeader(http.StatusUnauthorized)
	}))
	defer upstream.Close()

	reader := &authRefreshSequence{
		authReloadSequence: authReloadSequence{values: []proxymodel.Auth{
			{AccessToken: "old-token", AccountID: "account"},
			{AccessToken: "old-token", AccountID: "account"},
		}},
		refreshed: proxymodel.Auth{AccessToken: "refreshed-token", AccountID: "account"},
	}
	transport, err := NewTransport(upstream.URL, nil, reader)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.Execute(t.Context(), proxymodel.Request{
		Method: http.MethodPost, Path: "/responses", Header: make(http.Header),
	}, proxymodel.Account{ID: "profile-a", Home: "/profile-a", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK ||
		reader.refreshCalls != 1 ||
		strings.Join(seen, ",") != "Bearer old-token,Bearer refreshed-token" {
		t.Fatalf("refresh status/calls/seen = %d/%d/%v",
			response.StatusCode, reader.refreshCalls, seen)
	}
}
