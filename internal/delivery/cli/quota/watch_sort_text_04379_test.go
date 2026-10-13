package quota

import (
	"testing"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

// Tagged Prodex 0.437.9 trims Unicode whitespace but folds only ASCII
// A..Z bytes when comparing quota labels; non-ASCII letters stay distinct.
func TestProdex04379QuotaTextSortKeepsNonASCIIByteIdentity(t *testing.T) {
	for _, tc := range []struct {
		left, right string
		want        int
	}{
		{"É", "é", -1},
		{"Μ", "μ", -1},
		{"  BETA ", "beta", 0},
		{"　 Alpha　", "alpha", 0},
		{"alpha", " BETA ", -1},
	} {
		result := compareQuotaText(tc.left, tc.right)
		if result < 0 && tc.want >= 0 || result == 0 && tc.want != 0 ||
			result > 0 && tc.want <= 0 {
			t.Errorf("tagged label compare (%q, %q) = %d, want sign %d",
				tc.left, tc.right, result, tc.want)
		}
	}
	reports := []quotamodel.Report{
		{ProfileName: "a-profile", Auth: "é", Provider: "openai", State: "ready"},
		{ProfileName: "z-profile", Auth: "É", Provider: "openai", State: "ready"},
	}
	got := quotaSortedReports(reports, quotaProviderAll, quotaSortAuth)
	if got[0].ProfileName != "z-profile" || got[1].ProfileName != "a-profile" {
		t.Fatalf("non-ASCII auth case was incorrectly folded: %q before %q",
			got[0].ProfileName, got[1].ProfileName)
	}
}
