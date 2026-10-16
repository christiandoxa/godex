package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Prodex 0.438.0's three-profile quota transition keeps a committed owner
// stable while a pre-commit auth failure moves one request to the next ready
// profile. The restart probe then revalidates the stale profile with its exact
// current credential; a persisted quota snapshot is not an auth receipt.
const (
	quotaRestartMainAccount04380    = "account-main"
	quotaRestartSecondAccount04380  = "account-second"
	quotaRestartThirdAccount04380   = "account-third"
	quotaRestartResponsesPath04380  = "/backend-api/codex/responses"
	quotaRestartMaxQuotaProbes04380 = 6
)

type quotaRestartObservation04380 struct {
	account       string
	path          string
	status        int
	authenticated bool
}

type quotaRestartPhase04380 struct {
	quota     []quotaRestartObservation04380
	responses []quotaRestartObservation04380
	snapshot  bool
}

type quotaRestartResult04380 struct {
	first, restart quotaRestartPhase04380
	firstExit      int
	restartExit    int
}

func sameQuotaRestartResult04380(left, right quotaRestartResult04380) bool {
	// Probe order and duplicate warmups differ between the two executables; the
	// bounded authenticated outcomes are the parity contract.
	return quotaRestartResultValid04380(left) && quotaRestartResultValid04380(right)
}

func quotaRestartResultValid04380(result quotaRestartResult04380) bool {
	if result.firstExit != 0 || result.restartExit != 0 ||
		!quotaRestartPhaseValid04380(result.first, false) ||
		!quotaRestartPhaseValid04380(result.restart, true) {
		return false
	}
	return true
}

func quotaRestartPhaseValid04380(phase quotaRestartPhase04380, restart bool) bool {
	// The initial phase must cover every profile. After restart the official
	// binary may stop after the rotated active profile is live-authenticated.
	minimumQuotaProbes := len(quotaRestartAccounts04380)
	if restart {
		minimumQuotaProbes = 1
	}
	if !phase.snapshot || len(phase.quota) < minimumQuotaProbes ||
		len(phase.quota) > quotaRestartMaxQuotaProbes04380 {
		return false
	}
	seen := make(map[string]bool, len(quotaRestartAccounts04380))
	for _, observation := range phase.quota {
		if observation.path != startupUsagePath || observation.status != httpStatusOK04380 ||
			!observation.authenticated || !quotaRestartAccounts04380[observation.account] {
			return false
		}
		seen[observation.account] = true
	}
	if !restart {
		for account := range quotaRestartAccounts04380 {
			if !seen[account] {
				return false
			}
		}
	}
	if restart && !seen[quotaRestartMainAccount04380] {
		return false
	}
	if restart {
		return len(phase.responses) == 1 && quotaRestartResponseValid04380(phase.responses[0], httpStatusOK04380) &&
			seen[phase.responses[0].account]
	}
	if len(phase.responses) != 2 || !quotaRestartResponseValid04380(phase.responses[0], httpStatusUnauthorized04380) ||
		!quotaRestartResponseValid04380(phase.responses[1], httpStatusOK04380) {
		return false
	}
	return phase.responses[0].account != phase.responses[1].account
}

func quotaRestartResponseValid04380(observation quotaRestartObservation04380, status int) bool {
	return observation.path == quotaRestartResponsesPath04380 &&
		observation.status == status && observation.authenticated &&
		quotaRestartAccounts04380[observation.account]
}

func checkQuotaRestart04380(root string, opts cliOptions) ([]stepResult, error) {
	if _, err := exec.LookPath("curl"); err != nil {
		return nil, errors.New("three-profile quota/restart fixture requires curl")
	}
	results := make([]quotaRestartResult04380, 0, 2)
	for _, item := range []struct{ name, binary string }{
		{name: "prodex", binary: opts.prodex}, {name: "godex", binary: opts.godex},
	} {
		result, err := runQuotaRestart04380(root, item.name, item.binary)
		if err != nil {
			return nil, fmt.Errorf("%s three-profile quota/restart: %w", item.name, err)
		}
		results = append(results, result)
	}
	if len(results) != 2 || !sameQuotaRestartResult04380(results[0], results[1]) {
		return nil, errors.New("three-profile quota/restart behavior diverged")
	}
	requests := quotaRestartRequestCount04380(results[0])
	if other := quotaRestartRequestCount04380(results[1]); other > requests {
		requests = other
	}
	return []stepResult{{
		Name: "quota_three_profile_auth_restart_04380", ProdexExit: results[0].firstExit,
		GodexExit: results[1].firstExit, Requests: requests, Snapshot: true,
	}}, nil
}

const quotaRestartRequiredRequests04380 = 9

func quotaRestartRequestCount04380(result quotaRestartResult04380) int {
	return len(result.first.quota) + len(result.first.responses) +
		len(result.restart.quota) + len(result.restart.responses)
}

var quotaRestartAccounts04380 = map[string]bool{
	quotaRestartMainAccount04380:   true,
	quotaRestartSecondAccount04380: true,
	quotaRestartThirdAccount04380:  true,
}

type quotaRestartServer04380 struct {
	server       *httptest.Server
	credentials  map[string]string
	failFirst401 bool

	mu        sync.Mutex
	quota     []quotaRestartObservation04380
	responses []quotaRestartObservation04380
}

func newQuotaRestartServer04380(credentials map[string]string, failFirst401 bool) *quotaRestartServer04380 {
	fixture := &quotaRestartServer04380{
		credentials:  credentials,
		failFirst401: failFirst401,
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.handle))
	return fixture
}

func (fixture *quotaRestartServer04380) handle(writer http.ResponseWriter, request *http.Request) {
	account := request.Header.Get("ChatGPT-Account-Id")
	token := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
	wantToken, known := fixture.credentials[account]
	authenticated := known && token != "" && token == wantToken
	path := request.URL.Path
	switch {
	case request.Method == http.MethodGet && path == startupUsagePath:
		status := http.StatusOK
		if !authenticated {
			status = http.StatusUnauthorized
		}
		fixture.mu.Lock()
		fixture.quota = append(fixture.quota, quotaRestartObservation04380{
			account: account, path: path, status: status, authenticated: authenticated,
		})
		fixture.mu.Unlock()
		if !authenticated {
			writer.WriteHeader(status)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, quotaRestartUsageBody04380(account))
	case request.Method == http.MethodPost && path == "/backend-api/codex/responses":
		fixture.mu.Lock()
		status := http.StatusOK
		if !authenticated {
			status = http.StatusUnauthorized
		} else if fixture.failFirst401 && len(fixture.responses) == 0 {
			status = http.StatusUnauthorized
		}
		fixture.responses = append(fixture.responses, quotaRestartObservation04380{
			account: account, path: path, status: status, authenticated: authenticated,
		})
		fixture.mu.Unlock()
		if !authenticated {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(status)
		if status == http.StatusUnauthorized {
			_, _ = io.WriteString(writer, `{"error":{"code":"invalid_api_key"}}`)
			return
		}
		_, _ = io.WriteString(writer, `{"id":"quota-restart-response","object":"response","status":"completed","output":[]}`)
	default:
		writer.WriteHeader(http.StatusNotFound)
	}
}

func (fixture *quotaRestartServer04380) result(snapshot bool) quotaRestartPhase04380 {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return quotaRestartPhase04380{
		quota:     append([]quotaRestartObservation04380(nil), fixture.quota...),
		responses: append([]quotaRestartObservation04380(nil), fixture.responses...),
		snapshot:  snapshot,
	}
}

func (fixture *quotaRestartServer04380) close() { fixture.server.Close() }

func quotaRestartUsageBody04380(account string) string {
	usedFive, usedWeekly := 20, 40
	switch account {
	case quotaRestartSecondAccount04380:
		usedFive, usedWeekly = 10, 20
	case quotaRestartThirdAccount04380:
		usedFive, usedWeekly = 0, 54
	}
	return fmt.Sprintf(`{"plan_type":"plus","rate_limit":{"primary_window":{"used_percent":%d,"reset_at":4102444800,"limit_window_seconds":18000},"secondary_window":{"used_percent":%d,"reset_at":4102444800,"limit_window_seconds":604800}}}`, usedFive, usedWeekly)
}

func quotaSnapshotPresent04380(configHome string) bool {
	path := filepath.Join(configHome, "runtime-usage-snapshots.json")
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}
