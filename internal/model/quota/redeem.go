package quota

type RedeemOutcome string

const (
	RedeemReset           RedeemOutcome = "reset"
	RedeemNothingToReset  RedeemOutcome = "nothing-to-reset"
	RedeemNoCredit        RedeemOutcome = "no-credit"
	RedeemAlreadyRedeemed RedeemOutcome = "already-redeemed"
)

type RedeemInput struct {
	Profile string
	BaseURL string
	NoProxy bool
}

type NearReset struct {
	Label   string
	ResetAt int64
}

type RedeemPlan struct {
	Profile   string
	CodexHome string
	BaseURL   string
	NoProxy   bool
	NearReset *NearReset
}

type RedeemResult struct {
	Profile   string
	Outcome   RedeemOutcome
	RequestID string
}
