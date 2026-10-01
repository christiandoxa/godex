package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type redeemRequest struct {
	RequestID string `json:"redeem_request_id"`
}

type redeemResponse struct {
	Outcome string `json:"outcome"`
}

func (client *QuotaClient) ConsumeResetCredit(
	ctx context.Context,
	codexHome, upstream string,
	noProxy bool,
	requestID string,
) (quotamodel.RedeemOutcome, error) {
	if strings.TrimSpace(requestID) == "" {
		return "", errors.New("redeem request id is required")
	}
	endpoint, err := client.resetCreditURL(upstream)
	if err != nil {
		return "", err
	}
	httpClient, err := client.clientForPolicy(noProxy)
	if err != nil {
		return "", err
	}
	for attempt := 0; attempt < 2; attempt++ {
		status, body, sendErr := client.sendRedeem(ctx, httpClient, codexHome, endpoint, requestID)
		if sendErr != nil {
			return "", sendErr
		}
		if status == http.StatusUnauthorized && attempt == 0 {
			continue
		}
		if status < 200 || status >= 300 {
			return "", fmt.Errorf("reset-credit endpoint returned HTTP %d", status)
		}
		return decodeRedeemOutcome(body)
	}
	return "", errors.New("reset-credit authentication retry failed")
}

func (client *QuotaClient) sendRedeem(
	ctx context.Context,
	httpClient *http.Client,
	codexHome, endpoint, requestID string,
) (int, []byte, error) {
	auth, err := client.auth.ReadAuth(ctx, codexHome)
	if err != nil {
		return 0, nil, err
	}
	payload, err := json.Marshal(redeemRequest{RequestID: requestID})
	if err != nil {
		return 0, nil, errors.New("encode reset-credit request")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, errors.New("create reset-credit request")
	}
	request.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("originator", "codex_cli_rs")
	request.Header.Set("x-openai-codex-luna-reserve", "1")
	if auth.AccountID != "" {
		request.Header.Set("ChatGPT-Account-Id", auth.AccountID)
	}
	response, err := httpClient.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, ctx.Err()
		}
		return 0, nil, fmt.Errorf("request reset-credit endpoint: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxQuotaResponseBytes+1))
	if err != nil {
		return 0, nil, errors.New("read reset-credit response")
	}
	if len(body) > maxQuotaResponseBytes {
		return 0, nil, errors.New("reset-credit response exceeded safe size limit")
	}
	return response.StatusCode, body, nil
}

func (client *QuotaClient) resetCreditURL(override string) (string, error) {
	target := client.upstream
	if strings.TrimSpace(override) != "" {
		parsed, err := parseQuotaUpstream(override)
		if err != nil {
			return "", err
		}
		target = parsed
	}
	copy := *target
	base := strings.TrimRight(copy.Path, "/")
	if strings.Contains(base, "/backend-api") {
		copy.Path = base + "/wham/rate-limit-reset-credits/consume"
	} else {
		copy.Path = base + "/api/codex/rate-limit-reset-credits/consume"
	}
	copy.RawPath = ""
	return copy.String(), nil
}

func decodeRedeemOutcome(body []byte) (quotamodel.RedeemOutcome, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return quotamodel.RedeemReset, nil
	}
	var response redeemResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", errors.New("invalid JSON returned by reset-credit backend")
	}
	switch response.Outcome {
	case "", "reset":
		return quotamodel.RedeemReset, nil
	case "nothingToReset":
		return quotamodel.RedeemNothingToReset, nil
	case "noCredit":
		return quotamodel.RedeemNoCredit, nil
	case "alreadyRedeemed":
		return quotamodel.RedeemAlreadyRedeemed, nil
	default:
		return "", errors.New("reset-credit backend returned an unknown outcome")
	}
}
