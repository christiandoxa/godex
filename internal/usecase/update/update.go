package update

import (
	"context"
	"errors"
	"fmt"
	"time"

	updatemodel "github.com/christiandoxa/godex/internal/model/update"
)

type releaseSource interface {
	LatestVersion(context.Context) (string, error)
}

type stateStore interface {
	CachedLatest(time.Time) (string, bool)
	SaveLatest(string, time.Time) error
	AcquireCheck(context.Context) (func() error, error)
	AcquireInstall(context.Context) (func() error, error)
}

type installer interface {
	CurrentExecutable() (string, error)
	ProbeVersion(context.Context, string) (string, error)
	Install(context.Context, string, string) (updatemodel.InstallResult, error)
}

type Updater struct {
	releases       releaseSource
	state          stateStore
	installer      installer
	currentVersion string
	now            func() time.Time
}

func NewUpdater(releases releaseSource, state stateStore, installer installer, currentVersion string) *Updater {
	return &Updater{releases: releases, state: state, installer: installer, currentVersion: currentVersion, now: time.Now}
}

func (updater *Updater) Run(ctx context.Context) (updatemodel.Report, error) {
	if updater == nil || updater.releases == nil || updater.state == nil || updater.installer == nil {
		return updatemodel.Report{}, errors.New("Godex update support is not configured")
	}
	latest, err := updater.latestVersion(ctx)
	if err != nil {
		return updatemodel.Report{}, err
	}
	decision, err := updateDecision(updater.currentVersion, latest)
	if err != nil {
		return updatemodel.Report{}, err
	}
	if decision != updatemodel.UpdateAvailable {
		return updatemodel.Report{Installed: updater.currentVersion, Latest: latest, Status: decision}, nil
	}
	return updater.installLatest(ctx, latest)
}

func (updater *Updater) latestVersion(ctx context.Context) (string, error) {
	if latest, ok := updater.state.CachedLatest(updater.now()); ok {
		if _, err := parseVersion(latest); err == nil {
			return latest, nil
		}
	}
	release, err := updater.state.AcquireCheck(ctx)
	if err != nil {
		return "", err
	}
	defer release()
	if latest, ok := updater.state.CachedLatest(updater.now()); ok {
		if _, err := parseVersion(latest); err == nil {
			return latest, nil
		}
	}
	latest, err := updater.releases.LatestVersion(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to resolve the latest Godex release before updating: %w", err)
	}
	if _, err := parseVersion(latest); err != nil {
		return "", errors.New("invalid latest Godex release version")
	}
	_ = updater.state.SaveLatest(latest, updater.now())
	return latest, nil
}

func (updater *Updater) installLatest(ctx context.Context, latest string) (report updatemodel.Report, runErr error) {
	release, err := updater.state.AcquireInstall(ctx)
	if err != nil {
		return updatemodel.Report{}, err
	}
	defer func() { runErr = errors.Join(runErr, release()) }()

	runningExe, err := updater.installer.CurrentExecutable()
	if err != nil {
		return updatemodel.Report{}, err
	}
	installed, err := updater.installer.ProbeVersion(ctx, runningExe)
	if err != nil {
		return updatemodel.Report{}, err
	}
	decision, err := updateDecision(installed, latest)
	if err != nil {
		return updatemodel.Report{}, err
	}
	if decision != updatemodel.UpdateAvailable {
		return updatemodel.Report{Installed: installed, Latest: latest, Status: decision}, nil
	}
	output, installErr := updater.installer.Install(ctx, runningExe, latest)
	if installErr != nil {
		return updatemodel.Report{Installed: installed, Latest: latest, Status: updatemodel.UpdateAvailable, Stdout: output.Stdout, Stderr: output.Stderr}, installErr
	}
	return updatemodel.Report{Installed: latest, Latest: latest, Status: updatemodel.Updated, Stdout: output.Stdout, Stderr: output.Stderr}, nil
}

func updateDecision(currentVersion, targetVersion string) (updatemodel.Decision, error) {
	current, err := parseVersion(currentVersion)
	if err != nil {
		return "", fmt.Errorf("invalid installed Godex version: %s", currentVersion)
	}
	target, err := parseVersion(targetVersion)
	if err != nil {
		return "", fmt.Errorf("invalid target Godex version: %s", targetVersion)
	}
	switch compareVersions(current, target) {
	case -1:
		return updatemodel.UpdateAvailable, nil
	case 0:
		return updatemodel.UpToDate, nil
	default:
		return updatemodel.LocalNewer, nil
	}
}
