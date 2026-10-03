package gemini

import (
	"context"
	"errors"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

const oauthProfileDisabled = "Google Gemini OAuth profiles are unsupported and disabled. For Codex-fronted Gemini, migrate to a Gemini API key (`--api-key`, `GEMINI_API_KEY`, or `GOOGLE_API_KEY`). Native Gemini CLI / Vertex AI compatibility was retired; use the Gemini provider bridge with API-key authentication."

type ProfileQuota struct{}

func (ProfileQuota) FetchQuota(context.Context, profilemodel.QuotaTarget) (quotamodel.ExternalInfo, error) {
	return quotamodel.ExternalInfo{}, errors.New(oauthProfileDisabled)
}
