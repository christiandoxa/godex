package quota

import (
	"fmt"
	"time"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func formatResetCredits(credits *quotamodel.ResetCredits) string {
	if credits == nil {
		return ""
	}
	summary := fmt.Sprintf("%d available", credits.AvailableCount)
	if credits.AvailableCount <= 0 {
		return summary
	}
	expiry := "unavailable"
	if credits.NextExpiry != nil {
		expiry = time.Unix(*credits.NextExpiry, 0).Local().Format("2006-01-02 15:04:05 -07:00")
	} else if credits.NeverExpires {
		expiry = "never"
	}
	return summary + " | next expiry: " + expiry
}
