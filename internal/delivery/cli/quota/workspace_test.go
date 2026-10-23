package quota

import (
	"strings"
	"testing"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestQuotaWorkspaceLabelAndDetailIsolation(t *testing.T) {
	for _, fixture := range []struct{ id, name, want string }{
		{"selected", " Team ", "Team"}, {"selected", "", "selected"},
		{"abcdefghijklmnopqrstuvwxy", "", "abcdefghijkl...tuvwxy"},
		{strings.Repeat("β", 25), "", strings.Repeat("β", 12) + "..." + strings.Repeat("β", 6)},
		{"selected", "Team\n\x1b[2J", "Team  [2J"},
	} {
		workspace := &quotamodel.Workspace{ID: fixture.id, Name: fixture.name}
		if got := quotaWorkspaceLabel(workspace); got != fixture.want {
			t.Fatalf("workspace label=%q want=%q", got, fixture.want)
		}
		for _, detail := range []bool{false, true} {
			var output strings.Builder
			report := quotamodel.Report{ProfileName: "main", Workspace: workspace}
			if err := writeReport(&output, report, detail); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(output.String(), "workspace: "+fixture.want) != detail {
				t.Fatalf("detail=%t output=%q", detail, output.String())
			}
		}
	}
}
