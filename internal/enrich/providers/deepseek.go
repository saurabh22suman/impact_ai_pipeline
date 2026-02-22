package providers

func NewDeepSeekProvider(model string) ProviderClient {
	return NewStubProvider("deepseek", model)
}
