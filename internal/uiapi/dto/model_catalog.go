package dto

type ProviderModelsRequest struct {
	Provider string `json:"provider"`
	AuthMode string `json:"authMode"`
	APIKey   string `json:"apiKey,omitempty"`
	BaseURL  string `json:"baseUrl,omitempty"`
}

type ProviderModelsResponse struct {
	Provider string              `json:"provider"`
	Source   string              `json:"source"`
	Fallback bool                `json:"fallback"`
	Reason   string              `json:"reason,omitempty"`
	Models   []ProviderModelItem `json:"models"`
}
