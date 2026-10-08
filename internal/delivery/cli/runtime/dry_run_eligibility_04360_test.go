package runtime

import (
	"bytes"
	"context"
	"strings"
	"testing"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

type rotationPreview04360Profiles struct {
	*fakeLocalLaunchProfiles
	eligible bool
}

func (source *rotationPreview04360Profiles) RuntimeRotationEligible(context.Context, string) (bool, error) {
	return source.eligible, nil
}

func TestProdex04360RunDryRunDisablesProxyWithoutQualifiedProfilePool(t *testing.T) {
	for _, tc := range []struct {
		eligible  bool
		wantProxy bool
	}{
		{false, false},
		{true, true},
	} {
		profiles := &rotationPreview04360Profiles{fakeLocalLaunchProfiles: &fakeLocalLaunchProfiles{
			target: profilemodel.LaunchTarget{Name: "fixture", CodexHome: t.TempDir(), Provider: "openai"},
			active: true,
		}, eligible: tc.eligible}
		runner := runtimeusecase.NewRunner(&fakeRunnerAccounts{}, &fakeRunnerProcess{}, nil)
		var output bytes.Buffer
		err := RunProfiles(t.Context(), runner, nil, profiles, []string{
			"--dry-run", "exec", "--cyber-access-program", "standard", "review", "--uncommitted",
		}, &output)
		if err != nil {
			t.Fatal(err)
		}
		got := strings.Contains(output.String(), "Runtime proxy: would be enabled")
		if got != tc.wantProxy {
			t.Fatalf("eligible=%t got proxy=%t output=%s", tc.eligible, got, output.String())
		}
		if !tc.eligible {
			if !strings.Contains(output.String(), "Runtime proxy: disabled") ||
				strings.Contains(output.String(), "model_providers.godex-openai") ||
				!strings.Contains(output.String(), "Provider: openai") {
				t.Fatalf("synthetic credential-free profile received synthetic proxy identity: %s", output.String())
			}
		}
		if !strings.Contains(output.String(), "  --cyber-access-program\n  standard\n") {
			t.Fatalf("native program lost: %s", output.String())
		}
	}
}
