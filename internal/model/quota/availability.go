package quota

import "time"

type Availability struct {
	Ready   bool
	RetryAt time.Time
}
