package quota

type Window struct {
	UsedPercent        *int64
	ResetAt            *int64
	LimitWindowSeconds *int64
}

type Usage struct {
	PlanType     string
	Allowed      *bool
	LimitReached *bool
	Primary      *Window
	Secondary    *Window
}

type Report struct {
	AccountName string
	Email       string
	Active      bool
	Enabled     bool
	Usage       Usage
	State       string
	Err         error
}
