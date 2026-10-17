package main

import (
	proxyhttp "github.com/christiandoxa/godex/internal/delivery/http/proxy"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

func runtimeProxyHTTPConfig(
	config proxymodel.Config,
	router *routingusecase.Router,
	activity *runtimeusecase.Activity,
	redactor proxyhttp.BodyRedactor,
) proxyhttp.Config {
	return proxyhttp.Config{
		Router: router, Activity: activity, Broker: config.Broker, ListenAddr: config.ListenAddr,
		SmartContextEnabled: config.SmartContextEnabled, Redactor: redactor,
		ActiveRequestLimit: config.ActiveRequestLimit, AdmissionWaitBudget: config.AdmissionWaitBudget,
		PressureAdmissionWaitBudget:      config.PressureAdmissionWaitBudget,
		LongLivedQueueWaitBudget:         config.LongLivedQueueWaitBudget,
		PressureLongLivedQueueWaitBudget: config.PressureLongLivedQueueWaitBudget,
	}
}
