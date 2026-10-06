package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

type brokerAdminGateway struct{}

func (*brokerAdminGateway) Execute(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error) {
	return &proxymodel.Response{StatusCode: http.StatusNoContent, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
}

func TestProdex04356BrokerAdminHealthActivationMetricsAndBounds(t *testing.T) {
	resolved := ""
	activated := ""
	logged := ""
	broker := &proxymodel.BrokerConfig{
		BrokerKey: "broker", InstanceID: "instance", AdminToken: "secret",
		CurrentProfile: "one", StartedAt: 42, IncludeCodeReview: true,
		GodexVersion: "0.435.6", PersistenceRole: "owner",
		ResolveProfile: func(_ context.Context, profile string) (string, error) {
			resolved = profile
			if profile == "two" {
				return "account-b", nil
			}
			return "account-a", nil
		},
		OnActivated: func(_ context.Context, profile string) error {
			activated = profile
			return nil
		},
		LogRecovery: func(_ context.Context, message string) error {
			logged = message
			return nil
		},
	}
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway: &brokerAdminGateway{},
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{
				{ID: "account-a", Home: "/a", Enabled: true},
				{ID: "account-b", Home: "/b", Enabled: true},
			}, nil
		},
		PreferredAccount: "account-a",
		Now:              time.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := NewProxy(Config{Router: router, Broker: broker})
	if err != nil {
		t.Fatal(err)
	}
	if err := proxy.Start(); err != nil {
		t.Fatal(err)
	}
	defer proxy.Close(context.Background())

	request := func(method, path, token, body string) *http.Response {
		req, err := http.NewRequest(method, proxy.Endpoint()+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			req.Header.Set(brokerAdminTokenHeader, token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	for _, token := range []string{"", "wrong"} {
		resp := request(http.MethodGet, brokerHealthPath, token, "")
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("health token %q status = %d", token, resp.StatusCode)
		}
		resp.Body.Close()
	}

	resp := request(http.MethodGet, brokerHealthPath, "secret", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d", resp.StatusCode)
	}
	var health brokerHealth
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if health.CurrentProfile != "one" || health.InstanceID != "instance" ||
		!health.IncludeCodeReview || health.PersistenceRole != "owner" || health.ActiveRequests != 0 ||
		health.GodexVersion == nil || *health.GodexVersion != "0.435.6" {
		t.Fatalf("health = %#v", health)
	}

	resp = request(http.MethodPost, brokerActivatePath, "secret", `{"current_profile":"two"}`)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("activate status/body = %d / %s", resp.StatusCode, body)
	}
	resp.Body.Close()
	if resolved != "two" || activated != "two" {
		t.Fatalf("activation callbacks = resolved:%q activated:%q", resolved, activated)
	}

	resp = request(http.MethodGet, brokerHealthPath, "secret", "")
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if health.CurrentProfile != "two" {
		t.Fatalf("activated health profile = %q", health.CurrentProfile)
	}

	resp = request(http.MethodGet, brokerMetricsPath, "secret", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("metrics status = %d", resp.StatusCode)
	}
	var metrics brokerMetrics
	if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if metrics.Health.CurrentProfile != "two" || metrics.ActiveRequestLimit < 64 ||
		metrics.Traffic.Responses.Limit <= 0 || metrics.Traffic.Compact.Limit <= 0 {
		t.Fatalf("metrics = %#v", metrics)
	}
	if metrics.Traffic.Responses.AdmissionsTotal != 0 ||
		metrics.Traffic.Compact.AdmissionsTotal != 0 ||
		metrics.Traffic.WebSocket.AdmissionsTotal != 0 ||
		metrics.Traffic.Standard.AdmissionsTotal != 0 {
		t.Fatalf("admin polling polluted traffic metrics: %#v", metrics.Traffic)
	}

	resp = request(http.MethodPost, brokerReleaseAffinityPath, "secret", `{"session_id":"session-a"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("release affinity status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = request(http.MethodPost, brokerLogEventPath, "secret",
		`{"message":"event=runtime_recovery route=responses reason=quota"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("log event status = %d", resp.StatusCode)
	}
	resp.Body.Close()
	if logged == "" {
		t.Fatal("runtime recovery log callback was not invoked")
	}

	oversized := strings.Repeat("x", brokerActivationMaxBytes+1)
	resp = request(http.MethodPost, brokerActivatePath, "secret", oversized)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized activation status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = request(http.MethodGet, brokerLogSnapshotPath+"?after=0&limit=8", "secret", "")
	var snapshot brokerLiveLogSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if snapshot.Cursor < 3 || snapshot.Dropped != 0 || len(snapshot.Entries) < 3 {
		t.Fatalf("live log snapshot = %#v", snapshot)
	}
	foundRecovery := false
	for _, entry := range snapshot.Entries {
		if strings.Contains(entry.Line, "event=runtime_recovery") {
			foundRecovery = true
		}
	}
	if !foundRecovery {
		t.Fatalf("live log snapshot missing recovery event: %#v", snapshot)
	}
	latest := snapshot.Cursor
	resp = request(http.MethodGet, brokerLogSnapshotPath+"?after=999999&limit=8", "secret", "")
	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if snapshot.Cursor != latest || len(snapshot.Entries) != 0 {
		t.Fatalf("future-cursor snapshot = %#v, latest %d", snapshot, latest)
	}

	if !brokerTokenMatches("secret", "secret") || brokerTokenMatches("secret", "secreu") ||
		brokerTokenMatches("secret", "secret-extra") {
		t.Fatal("broker token comparison contract failed")
	}
}

func TestProdex04356BrokerAdminDoesNotCountControlPlaneAsActiveRequest(t *testing.T) {
	blocked := make(chan struct{})
	release := make(chan struct{})
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway: gatewayFunc(func(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error) {
			close(blocked)
			<-release
			return &proxymodel.Response{StatusCode: 204, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(nil))}, nil
		}),
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "a", Home: "/a", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := NewProxy(Config{Router: router, Broker: &proxymodel.BrokerConfig{
		AdminToken: "secret", InstanceID: "i", CurrentProfile: "p",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := proxy.Start(); err != nil {
		t.Fatal(err)
	}
	defer proxy.Close(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		resp, _ := http.Post(proxy.Endpoint()+"/responses", "application/json", strings.NewReader("{}"))
		if resp != nil {
			resp.Body.Close()
		}
	}()
	<-blocked
	if proxy.ActiveRequests() != 1 {
		t.Fatalf("model active requests = %d", proxy.ActiveRequests())
	}
	req, _ := http.NewRequest(http.MethodGet, proxy.Endpoint()+brokerHealthPath, nil)
	req.Header.Set(brokerAdminTokenHeader, "secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var health brokerHealth
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if health.ActiveRequests != 1 || proxy.ActiveRequests() != 1 {
		t.Fatalf("control-plane changed active count: health=%d proxy=%d", health.ActiveRequests, proxy.ActiveRequests())
	}
	metricsReq, _ := http.NewRequest(http.MethodGet, proxy.Endpoint()+brokerMetricsPath, nil)
	metricsReq.Header.Set(brokerAdminTokenHeader, "secret")
	metricsResp, err := http.DefaultClient.Do(metricsReq)
	if err != nil {
		t.Fatal(err)
	}
	var metrics brokerMetrics
	if err := json.NewDecoder(metricsResp.Body).Decode(&metrics); err != nil {
		t.Fatal(err)
	}
	metricsResp.Body.Close()
	if metrics.Traffic.Responses.Active != 1 ||
		metrics.Traffic.Responses.AdmissionsTotal != 1 ||
		metrics.Traffic.Responses.ReleasesTotal != 0 ||
		metrics.Traffic.Standard.AdmissionsTotal != 0 {
		t.Fatalf("model/admin traffic metrics = %#v", metrics.Traffic)
	}
	close(release)
	<-done
}

type gatewayFunc func(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error)

func (fn gatewayFunc) Execute(ctx context.Context, request proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	return fn(ctx, request, account)
}
