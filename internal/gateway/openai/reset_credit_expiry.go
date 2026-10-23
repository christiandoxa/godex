package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/christiandoxa/godex/internal/helper/jsonutil"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func (client *QuotaClient) FetchForDisplayAt(ctx context.Context, home, upstream string) (quotamodel.Usage, error) {
	body, acceptedAuth, _, err := client.fetchBodyWithIdentity043714(ctx, home, upstream, false)
	if err != nil {
		return quotamodel.Usage{}, err
	}
	usage, err := decodeQuotaUsage(body)
	if err != nil {
		return quotamodel.Usage{}, err
	}
	if usage.ResetCredits != nil && usage.ResetCredits.AvailableCount > 0 {
		client.enrichResetCreditExpiry(ctx, home, upstream, acceptedAuth, usage.ResetCredits)
	}
	return usage, nil
}

func (client *QuotaClient) enrichResetCreditExpiry(ctx context.Context, home, upstream string, auth proxymodel.Auth, credits *quotamodel.ResetCredits) {
	endpoint, err := client.usageURL(upstream)
	if err != nil {
		return
	}
	endpoint = strings.TrimSuffix(endpoint, "/usage") + "/rate-limit-reset-credits"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return
	}
	residency, err := client.auth.ReadQuotaResidency(ctx, home)
	if err != nil {
		return
	}
	request.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	request.Header.Set("originator", "codex_cli_rs")
	request.Header.Set("x-openai-codex-luna-reserve", "1")
	if auth.AccountID != "" {
		request.Header.Set("ChatGPT-Account-Id", auth.AccountID)
	}
	if residency != "" {
		request.Header.Set("x-openai-internal-codex-residency", residency)
	}
	response, err := client.doWithClient(client.client, request)
	if err != nil {
		return
	}
	defer response.Body.Close()
	const maxDetailBytes = 4 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, maxDetailBytes+1))
	if err != nil || len(body) > maxDetailBytes || response.StatusCode < 200 || response.StatusCode >= 300 {
		return
	}
	setResetCreditExpiry(credits, body)
}

func setResetCreditExpiry(summary *quotamodel.ResetCredits, body []byte) {
	summary.NextExpiry, summary.NeverExpires = nil, false
	if summary.AvailableCount <= 0 || jsonutil.CheckSerdeBoundary(string(body)) != nil || !jsonutil.CheckSerdeNumberTokens(body) {
		return
	}
	var details struct {
		Credits *[]struct {
			Status    *string         `json:"status"`
			ExpiresAt json.RawMessage `json:"expires_at"`
		} `json:"credits"`
	}
	if json.Unmarshal(body, &details) != nil || details.Credits == nil {
		return
	}
	var next *int64
	never := false
	for _, credit := range *details.Credits {
		if credit.Status == nil {
			return
		}
		if *credit.Status != "available" {
			continue
		}
		if string(credit.ExpiresAt) == "null" {
			never = true
			continue
		}
		var value string
		if json.Unmarshal(credit.ExpiresAt, &value) != nil {
			return
		}
		expiry, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return
		}
		if epoch := expiry.Unix(); next == nil || epoch < *next {
			next = &epoch
		}
	}
	summary.NextExpiry, summary.NeverExpires = next, next == nil && never
}
