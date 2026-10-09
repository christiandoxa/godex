package runtime

func (runner *Runner) SetProviderCredentialResolver(resolver providerCredentialResolver) {
	runner.credentials = resolver
}

func (runner *Runner) SetAutoRedeem(enabled bool) {
	if runner != nil {
		runner.autoRedeem = enabled
	}
}
