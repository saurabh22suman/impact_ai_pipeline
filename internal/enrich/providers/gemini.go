package providers

func NewGeminiProvider(model string) ProviderClient {
	return NewStubProvider("gemini", model)
}
