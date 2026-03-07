package welcome

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sipeed/picoclaw/internal/uiapi/dto"
	"github.com/sipeed/picoclaw/pkg/config"
)

// Service handles welcome page business logic
type Service struct {
	configPath string
}

// NewService creates a new welcome service
func NewService() *Service {
	return &Service{
		configPath: getConfigPath(),
	}
}

// NewServiceWithPath creates a service with custom config path (for testing)
func NewServiceWithPath(path string) *Service {
	return &Service{
		configPath: path,
	}
}

// GetBootstrap returns the initial data for the welcome page
func (s *Service) GetBootstrap() (*dto.BootstrapResponse, error) {
	cfg, err := config.LoadConfig(s.configPath)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	// Determine current step
	currentStep := s.determineCurrentStep(cfg)

	// Build model options from default model list
	modelOptions := s.buildModelOptions(cfg)

	// Build QQ config
	qqConfig := dto.QQBootstrapConfig{
		GuideURL: "https://q.qq.com/qqbot/openclaw/login.html",
		Fields: []dto.QQConfigField{
			{
				Key:         "app_id",
				Label:       "App ID",
				Type:        "text",
				Required:    true,
				Placeholder: "输入 QQ 开放平台生成的 App ID",
			},
			{
				Key:         "app_secret",
				Label:       "App Secret",
				Type:        "password",
				Required:    true,
				Placeholder: "输入 QQ 开放平台生成的 App Secret",
			},
		},
	}

	return &dto.BootstrapResponse{
		CurrentStep:  currentStep,
		ModelOptions: modelOptions,
		QQ:           qqConfig,
	}, nil
}

// SaveModelSetup saves the model configuration from welcome page
func (s *Service) SaveModelSetup(req dto.SaveModelRequest) error {
	cfg, err := config.LoadConfig(s.configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Validate required fields
	if req.ModelID == "" || req.Provider == "" {
		return fmt.Errorf("modelId and provider are required")
	}

	// Build ModelConfig
	modelCfg := config.ModelConfig{
		ModelName:  req.ModelID,
		Model:      req.Provider + "/" + req.ModelID,
		APIKey:     req.APIKey,
		APIBase:    req.BaseURL,
		AuthMethod: req.AuthMode,
	}

	// Update or add to ModelList
	updated := false
	for i, m := range cfg.ModelList {
		if extractProviderID(m.Model) == req.Provider {
			cfg.ModelList[i] = modelCfg
			updated = true
			break
		}
	}
	if !updated {
		cfg.ModelList = append([]config.ModelConfig{modelCfg}, cfg.ModelList...)
	}

	// Update default agent settings
	cfg.Agents.Defaults.Provider = req.Provider
	cfg.Agents.Defaults.Model = req.ModelID

	// Update ProviderConfig for token auth mode
	if req.AuthMode == "token" && req.APIKey != "" {
		s.updateProviderConfig(cfg, req.Provider, req.APIKey, req.BaseURL)
	}

	// Save config
	if err := config.SaveConfig(s.configPath, cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	return nil
}

// SaveQQSetup saves the QQ channel configuration
func (s *Service) SaveQQSetup(req dto.SaveQQRequest) error {
	cfg, err := config.LoadConfig(s.configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Validate
	if req.AppID == "" || req.AppSecret == "" {
		return fmt.Errorf("appId and appSecret are required")
	}

	// Update QQ config
	cfg.Channels.QQ = config.QQConfig{
		Enabled:            true,
		AppID:              req.AppID,
		AppSecret:          req.AppSecret,
		AllowFrom:          []string{}, // Allow all by default
		GroupTrigger:       config.GroupTriggerConfig{MentionOnly: false},
		ReasoningChannelID: "",
	}

	// Save config
	if err := config.SaveConfig(s.configPath, cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	return nil
}

// determineCurrentStep checks which step user should be on
func (s *Service) determineCurrentStep(cfg *config.Config) string {
	// Check if model is configured
	modelConfigured := cfg.Agents.Defaults.Provider != "" && cfg.Agents.Defaults.Model != ""

	// Check if QQ is configured
	qqConfigured := cfg.Channels.QQ.Enabled &&
		cfg.Channels.QQ.AppID != "" &&
		cfg.Channels.QQ.AppSecret != ""

	if !modelConfigured {
		return "model"
	}
	if !qqConfigured {
		return "qq"
	}
	return "completed"
}

// buildModelOptions builds model options from default config
func (s *Service) buildModelOptions(cfg *config.Config) []dto.ModelOption {
	// Use default config as template for available options
	defaultCfg := config.DefaultConfig()

	options := make([]dto.ModelOption, 0)
	seen := make(map[string]bool)

	for _, m := range defaultCfg.ModelList {
		provider := extractProviderID(m.Model)
		if provider == "" || seen[provider] {
			continue
		}
		seen[provider] = true

		// Determine auth mode from provider
		authMode := "token"
		if m.AuthMethod != "" {
			authMode = m.AuthMethod
		}

		// Mark as recommended if it's the currently configured one
		recommended := cfg.Agents.Defaults.Provider == provider

		options = append(options, dto.ModelOption{
			ID:          m.ModelName,
			Label:       m.ModelName,
			Provider:    provider,
			AuthMode:    authMode,
			Recommended: recommended,
			Description: getProviderDescription(provider),
		})
	}

	return options
}

// updateProviderConfig updates the provider-specific config
func (s *Service) updateProviderConfig(cfg *config.Config, provider, apiKey, baseURL string) {
	switch provider {
	case "openai":
		cfg.Providers.OpenAI.APIKey = apiKey
		if baseURL != "" {
			cfg.Providers.OpenAI.APIBase = baseURL
		}
	case "anthropic":
		cfg.Providers.Anthropic.APIKey = apiKey
		if baseURL != "" {
			cfg.Providers.Anthropic.APIBase = baseURL
		}
	case "gemini":
		cfg.Providers.Gemini.APIKey = apiKey
		if baseURL != "" {
			cfg.Providers.Gemini.APIBase = baseURL
		}
	case "qwen":
		cfg.Providers.Qwen.APIKey = apiKey
		if baseURL != "" {
			cfg.Providers.Qwen.APIBase = baseURL
		}
	case "deepseek":
		cfg.Providers.DeepSeek.APIKey = apiKey
		if baseURL != "" {
			cfg.Providers.DeepSeek.APIBase = baseURL
		}
	case "zhipu":
		cfg.Providers.Zhipu.APIKey = apiKey
		if baseURL != "" {
			cfg.Providers.Zhipu.APIBase = baseURL
		}
	case "moonshot":
		cfg.Providers.Moonshot.APIKey = apiKey
		if baseURL != "" {
			cfg.Providers.Moonshot.APIBase = baseURL
		}
	case "openrouter":
		cfg.Providers.OpenRouter.APIKey = apiKey
		if baseURL != "" {
			cfg.Providers.OpenRouter.APIBase = baseURL
		}
	case "volcengine":
		cfg.Providers.VolcEngine.APIKey = apiKey
		if baseURL != "" {
			cfg.Providers.VolcEngine.APIBase = baseURL
		}
	case "ollama":
		cfg.Providers.Ollama.APIKey = apiKey
		if baseURL != "" {
			cfg.Providers.Ollama.APIBase = baseURL
		}
	}
}

// Helper functions

func getConfigPath() string {
	if path := os.Getenv("PICOCLAW_CONFIG"); path != "" {
		return path
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".picoclaw", "config.json")
}

func extractProviderID(model string) string {
	parts := strings.SplitN(model, "/", 2)
	if len(parts) == 0 {
		return ""
	}
	return strings.ToLower(parts[0])
}

func getProviderDescription(provider string) string {
	descriptions := map[string]string{
		"openai":       "OpenAI GPT 模型，优先推荐",
		"anthropic":    "Claude 模型，偏稳定推理",
		"gemini":       "Google Gemini，响应速度快",
		"qwen":         "通义千问，国内优化",
		"deepseek":     "DeepSeek，性价比高",
		"zhipu":        "智谱 GLM，中文优化",
		"moonshot":     "月之暗面 Kimi，长文本支持",
		"openrouter":   "OpenRouter，多模型聚合",
		"antigravity":  "Google Cloud Code Assist",
		"ollama":       "本地模型，隐私保护",
	}
	if desc, ok := descriptions[provider]; ok {
		return desc
	}
	return "AI 模型"
}
