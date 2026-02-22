package providers

func NewAnthropicProvider(model string) ProviderClient {
	return NewStubProvider("anthropic", model)
}
