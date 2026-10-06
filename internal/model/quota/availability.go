package quota

import "time"

type Availability struct {
	Ready    bool
	RetryAt  time.Time
	Pressure Pressure
	Source   Source
}

type Pressure struct {
	Known             bool
	Band              uint8
	Total             int64
	Weekly            int64
	FiveHour          int64
	ReserveFloor      int64
	WeeklyRemaining   int64
	FiveHourRemaining int64
	WeeklyResetAt     int64
	FiveHourResetAt   int64
}
