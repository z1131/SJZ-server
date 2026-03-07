package dto

type ProviderCatalogItem struct {
	ID                string              `json:"id"`
	Label             string              `json:"label"`
	Aliases           []string            `json:"aliases,omitempty"`
	DefaultBaseURL    string              `json:"defaultBaseUrl,omitempty"`
	AuthModes         []string            `json:"authModes"`
	RecommendedModels []ProviderModelItem `json:"recommendedModels,omitempty"`
}

type ProviderModelItem struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type ProviderCatalogResponse struct {
	Providers []ProviderCatalogItem `json:"providers"`
}
