package session

import (
	"context"
	"testing"

	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

type recoveryAwareSessionLauncher04360 struct {
	launcherFake
	invoked        bool
	bindingRelease bool
}

func (launcher *recoveryAwareSessionLauncher04360) RunSessionReportWithRecovery(
	ctx context.Context, report sessionmodel.Report, args []string, local bool,
	forget func(context.Context, string) error,
) error {
	launcher.invoked = true
	if local || report.ID != "b111" || report.AccountID != "two" || len(args) != 3 || args[1] != "b111" {
		return context.Canceled
	}
	if forget == nil {
		return context.DeadlineExceeded
	}
	if err := forget(ctx, report.ID); err != nil {
		return err
	}
	launcher.bindingRelease = true
	return nil
}

func TestProdex04360CatalogRoutesResolvedSessionThroughRecoveryAwareLaunch(t *testing.T) {
	catalog, _ := testCatalog()
	launcher := &recoveryAwareSessionLauncher04360{}
	forgot := false
	catalog.SetBindingForget(func(_ context.Context, session string) error {
		if session != "b111" {
			t.Fatalf("wrong binding identity: %q", session)
		}
		forgot = true
		return nil
	})
	err := catalog.ResumeArgumentsWithLauncher(t.Context(), sessionmodel.Launch{
		SessionSelector: "b1", IDIndex: 1,
		Arguments: []string{"resume", "b1", "original prompt"},
	}, launcher)
	if err != nil || !launcher.invoked || !launcher.bindingRelease || !forgot {
		t.Fatalf("recovery dispatcher not reached or binding release lost: invoked %t release %t forgot %t err=%v", launcher.invoked, launcher.bindingRelease, forgot, err)
	}
}
