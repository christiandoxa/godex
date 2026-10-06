package runtimebroker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

const (
	brokerPollInterval      = 100 * time.Millisecond
	brokerLeaseScanInterval = time.Second
	brokerIdleGrace         = 5 * time.Second
	brokerReadyTimeout      = 15 * time.Second
)

type profileResolver interface {
	ResolveLaunch(context.Context, string) (profilemodel.LaunchTarget, error)
}

type eventRecorder interface {
	Record(context.Context, runtimemodel.Event) error
}

type Command struct {
	runner       *runtimeusecase.Runner
	profiles     profileResolver
	store        *Store
	activity     eventRecorder
	version      string
	now          func() time.Time
	poll         time.Duration
	leaseScan    time.Duration
	grace        time.Duration
	startupGrace time.Duration
}

func NewCommand(
	runner *runtimeusecase.Runner,
	profiles profileResolver,
	store *Store,
	activity eventRecorder,
	version string,
) *Command {
	return &Command{
		runner: runner, profiles: profiles, store: store, activity: activity, version: version,
		now: time.Now, poll: brokerPollInterval, leaseScan: brokerLeaseScanInterval,
		grace: brokerIdleGrace, startupGrace: brokerStartupGrace(brokerReadyTimeout, brokerIdleGrace),
	}
}

func (command *Command) Run(ctx context.Context, input io.Reader) (runErr error) {
	if command == nil || command.runner == nil || command.profiles == nil || command.store == nil {
		return errors.New("runtime broker support is not configured")
	}
	bootstrap, err := ReadBootstrap(input)
	if err != nil {
		return err
	}
	target, err := command.resolveOpenAIProfile(ctx, bootstrap.CurrentProfile)
	if err != nil {
		return err
	}
	ownerRelease, owner, err := command.store.TryAcquireOwner()
	if err != nil {
		return err
	}
	defer func() {
		if ownerRelease != nil {
			runErr = errors.Join(runErr, ownerRelease())
		}
	}()

	executable, digest := currentExecutableIdentity()
	startedAt := command.now().Unix()
	persistenceRole := "follower"
	if owner {
		persistenceRole = "owner"
	}
	brokerConfig := &proxymodel.BrokerConfig{
		BrokerKey: bootstrap.BrokerKey, InstanceID: bootstrap.InstanceID,
		AdminToken: bootstrap.AdminToken.Expose(), CurrentProfile: bootstrap.CurrentProfile,
		StartedAt: startedAt, IncludeCodeReview: bootstrap.IncludeCodeReview,
		GodexVersion: command.version, ExecutablePath: executable, ExecutableSHA256: digest,
		PersistenceRole: persistenceRole,
	}
	brokerConfig.ResolveProfile = func(resolveCtx context.Context, profile string) (string, error) {
		resolved, err := command.resolveOpenAIProfile(resolveCtx, profile)
		if err != nil {
			return "", err
		}
		return resolved.AccountID, nil
	}
	brokerConfig.OnActivated = func(updateCtx context.Context, profile string) error {
		return command.store.UpdateCurrentProfile(updateCtx, bootstrap.BrokerKey, bootstrap.InstanceID, profile)
	}
	brokerConfig.LogRecovery = func(logCtx context.Context, message string) error {
		if command.activity == nil {
			return nil
		}
		return command.activity.Record(logCtx, runtimemodel.Event{Kind: "runtime_recovery", Message: message})
	}

	gateway, err := command.runner.StartBrokerGateway(ctx, target.AccountID, runtimeusecase.GatewayStartOptions{
		ListenAddr: bootstrap.ListenAddr, UpstreamURL: bootstrap.UpstreamBaseURL,
		SmartContextEnabled: bootstrap.SmartContextEnabled, Broker: brokerConfig,
	})
	if err != nil {
		return err
	}
	gateway.SetPersistenceEnabled(owner)
	gateway.SetPersistenceRole(persistenceRole)
	if !owner {
		gateway.RecordBrokerLog("runtime_broker_persistence_follower reason=owner_lock_busy")
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		runErr = errors.Join(runErr, gateway.Close(closeCtx))
	}()

	listenAddr, err := gatewayListenAddr(gateway.Endpoint())
	if err != nil {
		return err
	}
	mount := OpenAIMountPath
	version := command.version
	executablePath := executable
	executableSHA := digest
	registry := Registry{
		PID: uint32(os.Getpid()), ListenAddr: listenAddr, StartedAt: startedAt,
		UpstreamBaseURL:   bootstrap.UpstreamBaseURL,
		IncludeCodeReview: bootstrap.IncludeCodeReview, UpstreamNoProxy: bootstrap.UpstreamNoProxy,
		SmartContextEnabled: bootstrap.SmartContextEnabled, CurrentProfile: bootstrap.CurrentProfile,
		InstanceID: bootstrap.InstanceID, OpenAIMountPath: &mount,
	}
	if birth := processBirthIdentity(uint32(os.Getpid())); birth != "" {
		registry.ProcessBirthIdentity = &birth
	}
	if version != "" {
		registry.GodexVersion = &version
	}
	if executablePath != "" {
		registry.ExecutablePath = &executablePath
	}
	if executableSHA != "" {
		registry.ExecutableSHA256 = &executableSHA
	}
	if err := command.store.SaveArtifacts(ctx, bootstrap.BrokerKey, Capability{
		InstanceID: bootstrap.InstanceID, AdminToken: bootstrap.AdminToken,
	}, registry); err != nil {
		return err
	}
	defer command.store.RemoveIfMatches(
		context.WithoutCancel(ctx), bootstrap.BrokerKey, bootstrap.InstanceID, bootstrap.AdminToken,
	)

	startupGrace := command.startupGrace
	if startupGrace <= 0 {
		startupGrace = brokerStartupGrace(brokerReadyTimeout, brokerIdleGrace)
	}
	startupGraceUntil := startedAt + int64((startupGrace+time.Second-1)/time.Second)
	return command.waitUntilIdle(ctx, gateway, bootstrap.BrokerKey, startupGraceUntil, &ownerRelease)
}

func brokerStartupGrace(readyTimeout, idleGrace time.Duration) time.Duration {
	readySeconds := (readyTimeout + time.Second - 1) / time.Second
	grace := (readySeconds + 1) * time.Second
	if grace < idleGrace {
		return idleGrace
	}
	return grace
}

func (command *Command) resolveOpenAIProfile(ctx context.Context, name string) (profilemodel.LaunchTarget, error) {
	target, err := command.profiles.ResolveLaunch(ctx, name)
	if err != nil {
		return profilemodel.LaunchTarget{}, err
	}
	if target.AccountID == "" || (target.Provider != "" && target.Provider != "openai") {
		return profilemodel.LaunchTarget{}, errors.New("runtime broker requires a managed OpenAI profile")
	}
	return target, nil
}

func (command *Command) waitUntilIdle(
	ctx context.Context,
	gateway *runtimeusecase.Gateway,
	brokerKey string,
	startupGraceUntil int64,
	ownerRelease *func() error,
) error {
	poll := command.poll
	if poll <= 0 {
		poll = brokerPollInterval
	}
	leaseScan := command.leaseScan
	if leaseScan <= 0 {
		leaseScan = brokerLeaseScanInterval
	}
	if leaseScan < poll {
		leaseScan = poll
	}
	grace := command.grace
	if grace <= 0 {
		grace = brokerIdleGrace
	}
	var idleStartedAt int64
	cachedLiveLeases := 0
	lastLeaseScan := time.Now().Add(-leaseScan)
	lastPromotion := time.Now().Add(-leaseScan)
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			active := gateway.ActiveRequests()
			if ownerRelease != nil && *ownerRelease == nil && time.Since(lastPromotion) >= leaseScan {
				release, owner, err := command.store.TryAcquireOwner()
				lastPromotion = time.Now()
				switch {
				case err != nil:
					gateway.RecordBrokerLog("runtime_broker_persistence_promotion_error error=owner_lock")
				case owner:
					*ownerRelease = release
					gateway.SetPersistenceEnabled(true)
					gateway.SetPersistenceRole("owner")
					gateway.RecordBrokerLog("runtime_broker_persistence_promoted role=owner")
				}
			}
			if active == 0 && time.Since(lastLeaseScan) >= leaseScan {
				cachedLiveLeases = command.store.CleanupStaleLeases(brokerKey)
				lastLeaseScan = time.Now()
			}
			now := command.now().Unix()
			if cachedLiveLeases > 0 || active > 0 {
				idleStartedAt = 0
				continue
			}
			if now < startupGraceUntil {
				idleStartedAt = 0
				continue
			}
			if idleStartedAt == 0 {
				idleStartedAt = now
				continue
			}
			if time.Duration(now-idleStartedAt)*time.Second < grace {
				continue
			}
			gateway.RecordBrokerLog(
				fmt.Sprintf("runtime_broker_idle_shutdown broker_key=%s idle_seconds=%d", brokerKey, now-idleStartedAt),
			)
			return nil
		}
	}
}

func gatewayListenAddr(endpoint string) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" {
		return "", errors.New("runtime broker gateway endpoint is invalid")
	}
	return parsed.Host, nil
}

func currentExecutableIdentity() (string, string) {
	path, err := os.Executable()
	if err != nil {
		return "", ""
	}
	path, _ = filepathAbs(path)
	content, err := os.ReadFile(path)
	if err != nil {
		return path, ""
	}
	sum := sha256.Sum256(content)
	return path, hex.EncodeToString(sum[:])
}

var filepathAbs = func(path string) (string, error) {
	return filepath.Abs(path)
}
