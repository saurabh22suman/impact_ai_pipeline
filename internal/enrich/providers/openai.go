package providers

func NewOpenAIProvider(model string) ProviderClient {
	return NewStubProvider("openai", model)
}
