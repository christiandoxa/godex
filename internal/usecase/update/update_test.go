package update

import (
	"context"
	"errors"
	"testing"
	"time"

	updatemodel "github.com/christiandoxa/godex/internal/model/update"
)

type fakeReleaseSource struct {
	latest string
	calls  int
	err    error
}

func (fake *fakeReleaseSource) LatestVersion(context.Context) (string, error) {
	fake.calls++
	return fake.latest, fake.err
}

type fakeState struct {
	cached       string
	cachedOK     bool
	saved        string
	checkLocks   int
	installLocks int
}

func (fake *fakeState) CachedLatest(time.Time) (string, bool) { return fake.cached, fake.cachedOK }
func (fake *fakeState) SaveLatest(version string, _ time.Time) error {
	fake.saved = version
	fake.cached, fake.cachedOK = version, true
	return nil
}
func (fake *fakeState) AcquireCheck(context.Context) (func() error, error) {
	fake.checkLocks++
	return func() error { return nil }, nil
}
func (fake *fakeState) AcquireInstall(context.Context) (func() error, error) {
	fake.installLocks++
	return func() error { return nil }, nil
}

type fakeInstaller struct {
	path       string
	version    string
	installed  string
	installOut updatemodel.InstallResult
	installErr error
	probes     int
}

func (fake *fakeInstaller) CurrentExecutable() (string, error) { return fake.path, nil }
func (fake *fakeInstaller) ProbeVersion(context.Context, string) (string, error) {
	fake.probes++
	return fake.version, nil
}
func (fake *fakeInstaller) Install(_ context.Context, _ string, target string) (updatemodel.InstallResult, error) {
	fake.installed = target
	return fake.installOut, fake.installErr
}

func TestUpdateDecisionMatchesSemverPrecedence(t *testing.T) {
	tests := []struct {
		current, target string
		want            updatemodel.Decision
	}{
		{"0.417.9", "0.418.0", updatemodel.UpdateAvailable},
		{"0.418.0", "0.418.0", updatemodel.UpToDate},
		{"0.419.0-dev", "0.418.0", updatemodel.LocalNewer},
		{"0.418.0-rc.1", "0.418.0", updatemodel.UpdateAvailable},
		{"0.418.0+build-a", "0.418.0+build-b", updatemodel.UpToDate},
	}
	for _, test := range tests {
		got, err := updateDecision(test.current, test.target)
		if err != nil || got != test.want {
			t.Fatalf("decision(%q,%q) = %q, %v", test.current, test.target, got, err)
		}
	}
	if _, err := updateDecision("0.418", "0.418.0"); err == nil {
		t.Fatal("invalid installed version accepted")
	}
}

func TestUpdaterUsesCachedLatestAndAvoidsDowngrade(t *testing.T) {
	releases := &fakeReleaseSource{latest: "9.9.9"}
	state := &fakeState{cached: "1.0.0", cachedOK: true}
	installer := &fakeInstaller{path: "/bin/godex", version: "1.1.0"}
	updater := NewUpdater(releases, state, installer, "1.1.0")
	report, err := updater.Run(context.Background())
	if err != nil || report.Status != updatemodel.LocalNewer || releases.calls != 0 || installer.installed != "" {
		t.Fatalf("report=%+v calls=%d installed=%q err=%v", report, releases.calls, installer.installed, err)
	}
}

func TestUpdaterReprobesInstalledBinaryUnderLock(t *testing.T) {
	releases := &fakeReleaseSource{latest: "1.1.0"}
	state := &fakeState{}
	installer := &fakeInstaller{path: "/bin/godex", version: "1.1.0"}
	updater := NewUpdater(releases, state, installer, "1.0.0")
	report, err := updater.Run(context.Background())
	if err != nil || report.Status != updatemodel.UpToDate || installer.installed != "" || state.installLocks != 1 || installer.probes != 1 {
		t.Fatalf("report=%+v state=%+v installer=%+v err=%v", report, state, installer, err)
	}
}

func TestUpdaterInstallsNewerReleaseAndPreservesDiagnostics(t *testing.T) {
	releases := &fakeReleaseSource{latest: "1.1.0"}
	state := &fakeState{}
	installer := &fakeInstaller{path: "/bin/godex", version: "1.0.0", installOut: updatemodel.InstallResult{Stdout: "verified installer output"}}
	updater := NewUpdater(releases, state, installer, "1.0.0")
	report, err := updater.Run(context.Background())
	if err != nil || report.Status != updatemodel.Updated || report.Installed != "1.1.0" || installer.installed != "1.1.0" || report.Stdout == "" {
		t.Fatalf("report=%+v installer=%+v err=%v", report, installer, err)
	}
}

func TestUpdaterReturnsInstallerFailureWithBoundedReport(t *testing.T) {
	releases := &fakeReleaseSource{latest: "1.1.0"}
	state := &fakeState{}
	want := errors.New("installer failed")
	installer := &fakeInstaller{path: "/bin/godex", version: "1.0.0", installErr: want, installOut: updatemodel.InstallResult{Stderr: "diagnostic"}}
	updater := NewUpdater(releases, state, installer, "1.0.0")
	report, err := updater.Run(context.Background())
	if !errors.Is(err, want) || report.Stderr != "diagnostic" || report.Status != updatemodel.UpdateAvailable {
		t.Fatalf("report=%+v err=%v", report, err)
	}
}

func TestProdex04356UpdateStatusUsesTotalBuildMetadataOrder(t *testing.T) {
	releases := &fakeReleaseSource{latest: "1.0.0+build.10"}
	state := &fakeState{}
	installer := &fakeInstaller{path: "/bin/godex", version: "1.0.0+build.2"}
	updater := NewUpdater(releases, state, installer, "1.0.0+build.2")
	report, err := updater.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != updatemodel.UpdateAvailable {
		t.Fatalf("build metadata status = %q, want %q", report.Status, updatemodel.UpdateAvailable)
	}
	decision, err := updateDecision("1.0.0+build.2", "1.0.0+build.10")
	if err != nil || decision != updatemodel.UpToDate {
		t.Fatalf("install precedence decision = %q, %v, want up-to-date", decision, err)
	}
}

func TestProdex04356UpdateRejectsEmptyBuildMetadata(t *testing.T) {
	releases := &fakeReleaseSource{latest: "1.0.0+"}
	state := &fakeState{}
	installer := &fakeInstaller{path: "/bin/godex", version: "1.0.0"}
	updater := NewUpdater(releases, state, installer, "1.0.0")
	if _, err := updater.Status(t.Context()); err == nil {
		t.Fatal("empty build metadata was accepted")
	}
}

func TestProdex04356ReleaseVersionOrderingMatchesTaggedPolicy(t *testing.T) {
	compare := func(left, right string) int {
		t.Helper()
		leftVersion, err := parseVersion(left)
		if err != nil {
			t.Fatalf("parse left %q: %v", left, err)
		}
		rightVersion, err := parseVersion(right)
		if err != nil {
			t.Fatalf("parse right %q: %v", right, err)
		}
		return compareVersionsTotal(leftVersion, rightVersion)
	}
	for _, fixture := range []struct {
		left, right string
		want        int
	}{
		{"v0.297.0", "0.296.0", 1},
		{"1.10.0", "1.9.0", 1},
		{"1.0.0", "1.0.0-rc.1", 1},
		{"1.0.0-rc.10", "1.0.0-rc.2", 1},
		{"1.0.0+build.10", "1.0.0+build.2", 1},
		{"1.0.0+build-a", "1.0.0+build-b", -1},
		{"1.0.0", "1.0.0+build.1", -1},
	} {
		got := compare(fixture.left, fixture.right)
		if got != fixture.want {
			t.Fatalf("compare(%q,%q) = %d, want %d", fixture.left, fixture.right, got, fixture.want)
		}
	}
	if _, err := parseVersion("vv1.0.0"); err == nil {
		t.Fatal("double v prefix was accepted")
	}
	if got := compare("\u2003v1.0.0\u3000", "1.0.0"); got != 0 {
		t.Fatalf("unicode-trimmed version comparison = %d, want equal", got)
	}
}
