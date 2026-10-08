package runtime

import (
	"context"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

type nativeSelectedProcess04360 struct {
	native int
	proxy  int
	args   []string
}

func (p *nativeSelectedProcess04360) Run(_ context.Context, _ string, args []string) error {
	p.native++
	p.args = append([]string(nil), args...)
	return nil
}
func (p *nativeSelectedProcess04360) CheckProxySupport(context.Context) error { return nil }
func (p *nativeSelectedProcess04360) RunThroughProxy(context.Context, string, string, []string) error {
	p.proxy++
	return nil
}

func TestProdex04360UnqualifiedOpenAIProfileLaunchesNativeRatherThanSyntheticProxy(t *testing.T) {
	home := t.TempDir()
	accounts := &runPolicyAccounts{
		values: []accountentity.Account{{ID: "a", Name: "fixture", Enabled: true}},
		homes:  map[string]string{"a": home},
	}
	process := &nativeSelectedProcess04360{}
	runner := runtimeusecase.NewRunner(accounts, process, nil)
	profiles := &rotationPreview04360Profiles{
		fakeLocalLaunchProfiles: &fakeLocalLaunchProfiles{
			target: profilemodel.LaunchTarget{Name: "fixture", AccountID: "a", CodexHome: home, Provider: "openai", Auth: "chatgpt"},
			active: true,
		},
		eligible: false,
	}
	err := runLaunchTarget(t.Context(), runner, nil, profiles, profiles.target, []string{
		"exec", "--cyber-access-program", "standard", "review", "--uncommitted",
	}, true, runtimeusecase.RuntimeLaunchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if process.native != 1 || process.proxy != 0 {
		t.Fatalf("unqualified profile launched synthetic proxy: native=%d proxy=%d", process.native, process.proxy)
	}
	if len(process.args) < 3 || process.args[0] != "exec" || process.args[1] != "--cyber-access-program" || process.args[2] != "standard" {
		t.Fatalf("native Codex arguments changed: %#v", process.args)
	}
}
