package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReleaseClientResolvesLatestFromRedirect(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/owner/project/releases/latest":
			http.Redirect(writer, request, server.URL+"/owner/project/releases/tag/v1.2.3", http.StatusFound)
		case "/owner/project/releases/tag/v1.2.3":
			if request.Header.Get("User-Agent") != "godex/test" {
				t.Fatalf("user agent = %q", request.Header.Get("User-Agent"))
			}
			writer.WriteHeader(http.StatusOK)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	client, err := NewReleaseClient(server.URL+"/owner/project/releases/latest", "owner/project", "godex/test", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	version, err := client.LatestVersion(context.Background())
	if err != nil || version != "1.2.3" {
		t.Fatalf("version = %q, err = %v", version, err)
	}
}

func TestReleaseClientRejectsInvalidRepositoryAndRedirect(t *testing.T) {
	if _, err := NewReleaseClient("", "https://example.test/repo", "", nil); err == nil {
		t.Fatal("URL repository unexpectedly accepted")
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, strings.TrimSuffix(request.URL.String(), request.URL.Path)+"/wrong/tag/v1.2.3", http.StatusFound)
	}))
	defer server.Close()
	client, err := NewReleaseClient(server.URL+"/owner/project/releases/latest", "owner/project", "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.LatestVersion(context.Background()); err == nil {
		t.Fatal("unexpected redirect path accepted")
	}
}
