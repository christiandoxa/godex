package quota

import (
	"context"
	"errors"
	"testing"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type workspaceQuotaFixture struct {
	countingUsage
	workspaces int
	err        error
}

func (fixture *workspaceQuotaFixture) FetchWorkspaceAt(context.Context, string, string) (*quotamodel.Workspace, error) {
	fixture.workspaces++
	return &quotamodel.Workspace{ID: "selected", Name: "Team"}, fixture.err
}

func TestQuotaWorkspaceLookupBelongsOnlyToAggregateReports(t *testing.T) {
	for _, mode := range []string{"aggregate", "single", "failed-metadata", "doctor"} {
		t.Run(mode, func(t *testing.T) {
			gateway := &workspaceQuotaFixture{}
			if mode == "failed-metadata" {
				gateway.err = errors.New("synthetic metadata failure")
			}
			status := NewStatus(fakeAccounts{}, gateway)
			status.SetProfiles(fakeProfileSource{targets: []profilemodel.QuotaTarget{{Name: "main", Provider: "openai", Auth: "chatgpt", CodexHome: "/synthetic", Compatible: true, Enabled: true, Active: true}}})
			var reports []quotamodel.Report
			var err error
			if mode == "doctor" {
				reports, err = status.DoctorReports(t.Context())
			} else {
				reports, err = status.Run(t.Context(), Options{All: mode != "single", IncludeWorkspace: true})
			}
			if err != nil || len(reports) != 1 || reports[0].Err != nil {
				t.Fatalf("metadata changed quota outcome: %+v err=%v", reports, err)
			}
			wantCalls := 1
			if mode == "single" || mode == "doctor" {
				wantCalls = 0
			}
			if gateway.workspaces != wantCalls || (reports[0].Workspace != nil) != (mode == "aggregate") {
				t.Fatalf("workspace=%+v calls=%d", reports[0].Workspace, gateway.workspaces)
			}
		})
	}
}
