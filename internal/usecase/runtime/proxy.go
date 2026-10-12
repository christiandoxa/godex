package runtime

import (
	"context"

	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
)

type Proxy interface {
	Start() error
	Endpoint() string
	Close(context.Context) error
}

type ProxyFactory func(proxyconfig.Config) (Proxy, error)

type sessionRecoveryProxyCodex interface {
	RunThroughProxyWithSessionServerAndRecovery(
		context.Context, string, string, []string, string,
		func(context.Context, string) error,
	) error
}

type sessionAffinityReleaser interface {
	ReleaseSessionAffinity(context.Context, string) error
}

func (runner *Runner) runThroughRuntimeProxy(
	ctx context.Context,
	proxyRunner proxyCodex,
	home, endpoint string,
	arguments []string,
	providerKind string,
	sessionServer bool,
	sessionID string,
	releaseAffinity func(context.Context, string) error,
	allowAutoRotate bool,
) error {
	run := func(runContext context.Context) error {
		if sessionServer {
			if companion, ok := runner.process.(sessionRecoveryProxyCodex); ok {
				return companion.RunThroughProxyWithSessionServerAndRecovery(
					runContext, home, endpoint, arguments, providerKind, releaseAffinity,
				)
			}
			if companion, ok := runner.process.(sessionProxyCodex); ok {
				return companion.RunThroughProxyWithSessionServer(runContext, home, endpoint, arguments, providerKind)
			}
		}
		if providerKind != "" {
			if isolated, ok := runner.process.(providerProxyCodex); ok {
				return isolated.RunThroughProxyProvider(runContext, home, endpoint, arguments, providerKind)
			}
		}
		return proxyRunner.RunThroughProxy(runContext, home, endpoint, arguments)
	}
	if sessionServer && sessionID != "" && allowAutoRotate {
		return runner.runSessionWithNativeRecovery(ctx, sessionID, allowAutoRotate, run)
	}
	return run(ctx)
}
