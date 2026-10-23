package quota

type Window struct {
	UsedPercent        *int64
	ResetAt            *int64
	LimitWindowSeconds *int64
}

type WindowPair struct {
	Primary   *Window
	Secondary *Window
}

type ResetCredits struct {
	AvailableCount int64
	NextExpiry     *int64 `json:"-"`
	NeverExpires   bool   `json:"-"`
}

type Usage struct {
	PlanType             string
	RateLimitPresent     bool
	Allowed              *bool
	LimitReached         *bool
	Primary              *Window
	Secondary            *Window
	CodeReviewRateLimit  *WindowPair
	AdditionalRateLimits []AdditionalRateLimit
	ResetCredits         *ResetCredits
}

type AdditionalRateLimit struct {
	LimitID         string
	LimitName       string
	MeteredFeature  string
	NormalModelSlug string
	Allowed         *bool
	LimitReached    *bool
	Primary         *Window
	Secondary       *Window
}

type Report struct {
	AccountName string
	ProfileName string
	Provider    string
	Auth        string
	Email       string
	Active      bool
	Enabled     bool
	Usage       Usage
	External    *ExternalInfo
	Workspace   *Workspace `json:"-"`
	State       string
	Err         error
}
