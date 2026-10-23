package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/christiandoxa/godex/internal/helper/jsonutil"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

var resetCreditTimePattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}[Tt ][0-9]{2}:[0-9]{2}:(?:[0-5][0-9]|60)(?:\.[0-9]+)?(?:[Zz]|[+-](?:[01][0-9]|2[0-3]):[0-5][0-9])$`)

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
	headers, err := client.auth.ReadQuotaHeaders(ctx, home)
	if err != nil {
		return
	}
	request.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	request.Header.Set("originator", headers.Originator)
	request.Header.Set("x-openai-codex-luna-reserve", "1")
	if auth.AccountID != "" {
		request.Header.Set("ChatGPT-Account-Id", auth.AccountID)
	}
	if headers.Residency != "" {
		request.Header.Set("x-openai-internal-codex-residency", headers.Residency)
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
	if summary.AvailableCount <= 0 || !json.Valid(body) || jsonutil.CheckSerdeBoundary(string(body)) != nil || !jsonutil.CheckSerdeNumberTokens(body) {
		return
	}
	fields, ok := resetCreditObjectFields(body, "credits")
	var credits []json.RawMessage
	if !ok || json.Unmarshal(fields["credits"], &credits) != nil {
		return
	}
	var next *int64
	never := false
	for _, credit := range credits {
		fields, ok := resetCreditObjectFields(credit, "status", "expires_at")
		var status *string
		if !ok || json.Unmarshal(fields["status"], &status) != nil || status == nil {
			return
		}
		if *status != "available" {
			continue
		}
		if string(fields["expires_at"]) == "null" {
			never = true
			continue
		}
		var value string
		if json.Unmarshal(fields["expires_at"], &value) != nil {
			return
		}
		epoch, ok := resetCreditExpiryEpoch(value)
		if !ok {
			return
		}
		if next == nil || epoch < *next {
			next = &epoch
		}
	}
	summary.NextExpiry, summary.NeverExpires = next, next == nil && never
}

// Serde structs match exact field names and reject repeated known fields.
func resetCreditObjectFields(body []byte, names ...string) (map[string]json.RawMessage, bool) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil, false
	}
	fields := make(map[string]json.RawMessage, len(names))
	for decoder.More() {
		token, err := decoder.Token()
		name, ok := token.(string)
		if err != nil || !ok {
			return nil, false
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return nil, false
		}
		if !slices.Contains(names, name) {
			continue
		}
		if _, exists := fields[name]; exists {
			return nil, false
		}
		fields[name] = value
	}
	return fields, true
}

func resetCreditExpiryEpoch(value string) (int64, bool) {
	if !resetCreditTimePattern.MatchString(value) {
		return 0, false
	}
	value = value[:10] + "T" + value[11:]
	if strings.HasSuffix(value, "z") {
		value = value[:len(value)-1] + "Z"
	}
	// Chrono's Unix timestamp projects a leap second onto its preceding second.
	if value[17:19] == "60" {
		value = value[:17] + "59" + value[19:]
	}
	expiry, err := time.Parse(time.RFC3339Nano, value)
	return expiry.Unix(), err == nil
}
