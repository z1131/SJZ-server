package catalog

import (
	"sort"
	"strings"

	"github.com/sipeed/picoclaw/internal/uiapi/dto"
	"github.com/sipeed/picoclaw/pkg/config"
)

type Service struct{}

func NewService() *Service {
	return &Service{}
}

type providerMeta struct {
	label     string
	aliases   []string
	authModes []string
}

var providerIDs = []string{
	"anthropic",
	"openai",
	"openrouter",
	"zhipu",
	"gemini",
	"nvidia",
	"ollama",
	"moonshot",
	"deepseek",
	"volcengine",
	"github_copilot",
	"antigravity",
	"qwen",
	"other",
}

var providerMetaMap = map[string]providerMeta{
	"openai": {
		label:     "OpenAI（GPT）",
		aliases:   []string{"gpt"},
		authModes: []string{"oauth", "token"},
	},
	"anthropic": {
		label:     "Anthropic（Claude）",
		aliases:   []string{"claude"},
		authModes: []string{"token"},
	},
	"openrouter": {
		label:     "OpenRouter",
		authModes: []string{"token"},
	},
	"zhipu": {
		label:     "Zhipu（GLM）",
		aliases:   []string{"glm"},
		authModes: []string{"token"},
	},
	"gemini": {
		label:     "Gemini（Google）",
		aliases:   []string{"google"},
		authModes: []string{"token"},
	},
	"nvidia": {
		label:     "NVIDIA",
		authModes: []string{"token"},
	},
	"ollama": {
		label:     "Ollama",
		authModes: []string{"token"},
	},
	"moonshot": {
		label:     "Moonshot（Kimi）",
		aliases:   []string{"kimi"},
		authModes: []string{"token"},
	},
	"deepseek": {
		label:     "DeepSeek",
		authModes: []string{"token"},
	},
	"volcengine": {
		label:     "Volcengine（Doubao）",
		aliases:   []string{"doubao"},
		authModes: []string{"token"},
	},
	"github_copilot": {
		label:     "GitHub Copilot（Copilot）",
		aliases:   []string{"copilot"},
		authModes: []string{"oauth"},
	},
	"antigravity": {
		label:     "Google Antigravity",
		authModes: []string{"oauth"},
	},
	"qwen": {
		label:     "Qwen（Tongyi）",
		aliases:   []string{"tongyi"},
		authModes: []string{"oauth", "token"},
	},
	"other": {
		label:     "其他",
		authModes: []string{"token"},
	},
}

func (s *Service) ListProviders() dto.ProviderCatalogResponse {
	defaultCfg := config.DefaultConfig()
	defaultsByProvider := make(map[string][]config.ModelConfig)

	for _, modelCfg := range defaultCfg.ModelList {
		providerID := extractProviderID(modelCfg.Model)
		if providerID == "" {
			continue
		}
		defaultsByProvider[providerID] = append(defaultsByProvider[providerID], modelCfg)
	}

	items := make([]dto.ProviderCatalogItem, 0, len(providerIDs))
	for _, providerID := range providerIDs {
		meta := providerMetaFor(providerID)
		item := dto.ProviderCatalogItem{
			ID:        providerID,
			Label:     meta.label,
			Aliases:   append([]string{}, meta.aliases...),
			AuthModes: append([]string{}, meta.authModes...),
		}

		if modelConfigs, ok := defaultsByProvider[providerID]; ok {
			item.DefaultBaseURL = modelConfigs[0].APIBase
			item.RecommendedModels = make([]dto.ProviderModelItem, 0, len(modelConfigs))
			for _, modelCfg := range modelConfigs {
				item.RecommendedModels = append(item.RecommendedModels, dto.ProviderModelItem{
					ID:    modelCfg.ModelName,
					Label: modelCfg.ModelName,
				})
			}
		}

		if providerID == "qwen" {
			item.RecommendedModels = []dto.ProviderModelItem{
				{ID: "coder-model", Label: "coder-model"},
				{ID: "vision-model", Label: "vision-model"},
			}
		}

		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Label < items[j].Label
	})

	return dto.ProviderCatalogResponse{Providers: items}
}

func extractProviderID(model string) string {
	parts := strings.SplitN(model, "/", 2)
	if len(parts) == 0 {
		return ""
	}
	return normalizeProviderID(parts[0])
}

func normalizeProviderID(providerID string) string {
	switch strings.ToLower(providerID) {
	case "github-copilot":
		return "github_copilot"
	default:
		return strings.ToLower(providerID)
	}
}

func providerMetaFor(providerID string) providerMeta {
	if meta, ok := providerMetaMap[providerID]; ok {
		return meta
	}
	if providerID == "" {
		return providerMeta{
			label:     "",
			authModes: []string{"token"},
		}
	}
	return providerMeta{
		label:     formatProviderLabel(providerID),
		authModes: []string{"token"},
	}
}

func formatProviderLabel(providerID string) string {
	switch providerID {
	case "github_copilot":
		return "GitHub Copilot"
	default:
		parts := strings.Fields(strings.NewReplacer("_", " ", "-", " ").Replace(providerID))
		for i, part := range parts {
			if part == "" {
				continue
			}
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
		return strings.Join(parts, " ")
	}
}
