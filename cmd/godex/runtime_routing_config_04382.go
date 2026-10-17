package main

import (
	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

func runtimeRoutingConfig(
	config proxyconfig.Config,
	dependencies runtimeRouterDependencies,
	gateway runtimeGateway,
) routingusecase.Config {
	routingConfig := routingusecase.Config{
		Gateway: gateway, Accounts: runtimeAccountSource(config.Accounts, gateway),
		PreferredAccount: config.PreferredAccount, Bindings: dependencies.bindings,
		ProfileInflightHardLimit: config.ProfileInflightHardLimit,
		PrecommitWatchdogBudget:  config.PrecommitWatchdogBudget,
		RoutingState:             dependencies.bindings,
		Activity:                 dependencies.activity,
		AutoRedeem:               config.AutoRedeem, Redeemer: dependencies.autoRedeemer,
	}
	if !config.SkipQuotaPreflight {
		routingConfig.QuotaPreflight = dependencies.quota
	}
	return routingConfig
}
