package main

import (
	"fmt"

	claudegateway "github.com/christiandoxa/godex/internal/gateway/claude"
	"github.com/christiandoxa/godex/internal/gateway/codex"
	copilotgateway "github.com/christiandoxa/godex/internal/gateway/copilot"
	deepseekgateway "github.com/christiandoxa/godex/internal/gateway/deepseek"
	geminigateway "github.com/christiandoxa/godex/internal/gateway/gemini"
	kirogateway "github.com/christiandoxa/godex/internal/gateway/kiro"
	localrewritegateway "github.com/christiandoxa/godex/internal/gateway/localrewrite"
	"github.com/christiandoxa/godex/internal/gateway/openai"
	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
	runtimerepo "github.com/christiandoxa/godex/internal/repository/runtime"
)

func newRuntimeGateway(
	config proxyconfig.Config,
	process *codex.CodexProcess,
	copilotSource *copilotgateway.Source,
	claudeSource *claudegateway.Source,
	kiroSource *kirogateway.Source,
	providerCatalogs *runtimerepo.ProviderCatalogStore,
) (runtimeGateway, error) {
	client := runtimeUpstreamHTTPClient(config.UpstreamNoProxy)
	switch config.Provider.Kind {
	case "", "openai":
		return openai.NewTransportWithCommittedWebSocketIdle04382(
			config.UpstreamURL, client, process, config.WebSocketCommittedIdleTimeout,
		)
	case "copilot":
		return newCopilotRuntimeGateway(config, copilotSource, providerCatalogs, client)
	case "anthropic":
		return newAnthropicRuntimeGateway(config, claudeSource, client)
	case "deepseek":
		return deepseekgateway.NewRuntimePoolWithOptions(config.Provider.APIURL, config.ProviderCredentials, deepseekgateway.RequestOptions{
			StrictTools: config.Provider.StrictTools, WebSearchMode: config.Provider.WebSearchMode,
			BetaBaseURL: config.Provider.BetaBaseURL, SSELookaheadTimeout: config.Provider.SSELookaheadTimeout,
		}, client)
	case "gemini":
		if !config.GatewayMode {
			return geminigateway.NewLocalRuntimePool(config.Provider.APIURL, config.ProviderCredentials, client)
		}
		return geminigateway.NewRuntimePool(config.Provider.APIURL, config.ProviderCredentials, client)
	case "kiro":
		return newKiroRuntimeGateway(config, kiroSource)
	case "local":
		return localrewritegateway.NewTransport(config.Provider.APIURL, client)
	case "openai-compatible":
		return localrewritegateway.NewTransportWithAuthPolicy(config.Provider.APIURL, client, true)
	default:
		return nil, fmt.Errorf("runtime provider %q is not implemented", config.Provider.Kind)
	}
}
