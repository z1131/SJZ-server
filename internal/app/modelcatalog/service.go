package modelcatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/internal/uiapi/dto"
	"github.com/sipeed/picoclaw/pkg/auth"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
)

const (
	sourceRemote = "remote"
	sourceStatic = "static"

	// Codex backend endpoint for OpenAI OAuth models
	codexModelsEndpoint = "https://chatgpt.com/backend-api/codex/models"
)

type Service struct {
	client *http.Client
}

func NewService() *Service {
	return &Service{
		client: &http.Client{Timeout: 12 * time.Second},
	}
}

func NewServiceWithClient(client *http.Client) *Service {
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	return &Service{client: client}
}

type runtimeConfig struct {
	apiKey  string
	baseURL string
	headers map[string]string
}

type openAIModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func (s *Service) ListProviderModels(
	ctx context.Context,
	req dto.ProviderModelsRequest,
) (dto.ProviderModelsResponse, error) {
	provider := normalizeProviderID(req.Provider)
	staticModels := staticModelsForProvider(provider)
	if provider == "" {
		return dto.ProviderModelsResponse{}, fmt.Errorf("provider is required")
	}

	if provider == "openai" && strings.TrimSpace(req.AuthMode) == "oauth" {
		models, err := s.fetchCodexModels(ctx)
		if err != nil {
			staticModels := staticModelsForProvider(provider)
			if len(staticModels) > 0 {
				return dto.ProviderModelsResponse{
					Provider: provider,
					Source:   sourceStatic,
					Fallback: true,
					Reason:   fmt.Sprintf("Codex 模型列表拉取失败: %s", err.Error()),
					Models:   staticModels,
				}, nil
			}
			return dto.ProviderModelsResponse{}, fmt.Errorf("fetching codex models: %w", err)
		}
		return dto.ProviderModelsResponse{
			Provider: provider,
			Source:   sourceRemote,
			Fallback: false,
			Reason:   "模型列表来自 ChatGPT Codex backend。",
			Models:   models,
		}, nil
	}

	runtime, err := buildRuntimeConfig(provider, req)
	if err != nil {
		if len(staticModels) > 0 {
			return dto.ProviderModelsResponse{
				Provider: provider,
				Source:   sourceStatic,
				Fallback: true,
				Reason:   err.Error(),
				Models:   staticModels,
			}, nil
		}
		return dto.ProviderModelsResponse{}, err
	}

	if models, ok, err := s.fetchRemoteModels(ctx, provider, runtime); err == nil && ok {
		return dto.ProviderModelsResponse{
			Provider: provider,
			Source:   sourceRemote,
			Fallback: false,
			Models:   models,
		}, nil
	} else if err != nil && len(staticModels) == 0 {
		return dto.ProviderModelsResponse{}, err
	}

	return dto.ProviderModelsResponse{
		Provider: provider,
		Source:   sourceStatic,
		Fallback: true,
		Reason:   "远程模型列表拉取失败，已回退默认模型集合。",
		Models:   staticModels,
	}, nil
}

func (s *Service) fetchRemoteModels(
	ctx context.Context,
	provider string,
	runtime runtimeConfig,
) ([]dto.ProviderModelItem, bool, error) {
	switch provider {
	case "anthropic":
		models, err := s.fetchAnthropicModels(ctx, runtime)
		return models, true, err
	case "antigravity":
		models, err := s.fetchAntigravityModels(ctx, runtime)
		return models, true, err
	case "gemini":
		models, err := s.fetchGeminiModels(ctx, runtime)
		return models, true, err
	case "ollama":
		models, err := s.fetchOllamaModels(ctx, runtime)
		return models, true, err
	case "openai", "openrouter", "zhipu", "nvidia", "moonshot", "deepseek", "volcengine", "qwen", "other":
		models, err := s.fetchOpenAICompatibleModels(ctx, runtime)
		return models, true, err
	default:
		return nil, false, nil
	}
}

func (s *Service) fetchOpenAICompatibleModels(
	ctx context.Context,
	runtime runtimeConfig,
) ([]dto.ProviderModelItem, error) {
	if strings.TrimSpace(runtime.baseURL) == "" {
		return nil, fmt.Errorf("base url is required")
	}
	if strings.TrimSpace(runtime.apiKey) == "" {
		return nil, fmt.Errorf("api key is required")
	}

	endpoints := candidateModelEndpoints(runtime.baseURL)
	var lastErr error

	for _, endpoint := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("creating model list request: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", "Bearer "+runtime.apiKey)
		for key, value := range runtime.headers {
			req.Header.Set(key, value)
		}

		resp, err := s.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("requesting model list: %w", err)
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("reading model list response: %w", readErr)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("model list request failed: status=%d body=%s", resp.StatusCode, string(body))
			continue
		}

		var payload openAIModelsResponse
		if err := json.Unmarshal(body, &payload); err != nil {
			lastErr = fmt.Errorf("parsing model list response: %w", err)
			continue
		}

		models := make([]dto.ProviderModelItem, 0, len(payload.Data))
		seen := make(map[string]struct{}, len(payload.Data))
		for _, item := range payload.Data {
			id := strings.TrimSpace(item.ID)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			models = append(models, dto.ProviderModelItem{
				ID:    id,
				Label: id,
			})
		}
		sort.Slice(models, func(i, j int) bool {
			return models[i].Label < models[j].Label
		})
		if len(models) == 0 {
			lastErr = fmt.Errorf("remote model list is empty")
			continue
		}
		return models, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("model list request failed")
	}
	return nil, lastErr
}

// anthropicModelsResponse represents the response from Anthropic's /v1/models API
type anthropicModelsResponse struct {
	Data []struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
		Type        string `json:"type"`
	} `json:"data"`
	HasMore bool `json:"has_more"`
}

func (s *Service) fetchAnthropicModels(
	ctx context.Context,
	runtime runtimeConfig,
) ([]dto.ProviderModelItem, error) {
	if strings.TrimSpace(runtime.apiKey) == "" {
		return nil, fmt.Errorf("api key is required")
	}

	baseURL := strings.TrimSpace(runtime.baseURL)
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	endpoint := baseURL + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating anthropic models request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-api-key", runtime.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting anthropic models: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading anthropic models response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic models request failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	var payload anthropicModelsResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parsing anthropic models response: %w", err)
	}

	models := make([]dto.ProviderModelItem, 0, len(payload.Data))
	seen := make(map[string]struct{}, len(payload.Data))
	for _, item := range payload.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}

		label := strings.TrimSpace(item.DisplayName)
		if label == "" {
			label = id
		}
		models = append(models, dto.ProviderModelItem{
			ID:    id,
			Label: label,
		})
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i].Label < models[j].Label
	})

	if len(models) == 0 {
		return nil, fmt.Errorf("anthropic models response is empty")
	}

	return models, nil
}

// geminiModelsResponse represents the response from Gemini's /v1beta/models API
type geminiModelsResponse struct {
	Models []struct {
		Name                       string   `json:"name"`
		Version                    string   `json:"version"`
		DisplayName                string   `json:"displayName"`
		Description                string   `json:"description"`
		SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
	} `json:"models"`
}

func (s *Service) fetchGeminiModels(
	ctx context.Context,
	runtime runtimeConfig,
) ([]dto.ProviderModelItem, error) {
	if strings.TrimSpace(runtime.apiKey) == "" {
		return nil, fmt.Errorf("api key is required")
	}

	baseURL := strings.TrimSpace(runtime.baseURL)
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	// Gemini uses API key as query parameter
	endpoint := fmt.Sprintf("%s/models?key=%s", baseURL, url.QueryEscape(runtime.apiKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating gemini models request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting gemini models: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading gemini models response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini models request failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	var payload geminiModelsResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parsing gemini models response: %w", err)
	}

	models := make([]dto.ProviderModelItem, 0, len(payload.Models))
	seen := make(map[string]struct{}, len(payload.Models))

	for _, item := range payload.Models {
		// Gemini returns model names like "models/gemini-2.0-flash-exp"
		// We need to strip the "models/" prefix
		id := strings.TrimSpace(item.Name)
		if id == "" {
			continue
		}
		id = strings.TrimPrefix(id, "models/")
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}

		// Filter models that support generateContent
		supportsChat := false
		for _, method := range item.SupportedGenerationMethods {
			if method == "generateContent" {
				supportsChat = true
				break
			}
		}
		if !supportsChat {
			continue
		}

		label := strings.TrimSpace(item.DisplayName)
		if label == "" {
			label = id
		}
		models = append(models, dto.ProviderModelItem{
			ID:    id,
			Label: label,
		})
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i].Label < models[j].Label
	})

	if len(models) == 0 {
		return nil, fmt.Errorf("gemini models response is empty or no chat models found")
	}

	return models, nil
}

func (s *Service) fetchAntigravityModels(
	ctx context.Context,
	runtime runtimeConfig,
) ([]dto.ProviderModelItem, error) {
	if strings.TrimSpace(runtime.apiKey) == "" {
		return nil, fmt.Errorf("access token is required")
	}

	projectID := runtime.headers["X-Project-ID"]
	if strings.TrimSpace(projectID) == "" {
		return nil, fmt.Errorf("project id is required")
	}

	models, err := providers.FetchAntigravityModels(runtime.apiKey, projectID)
	if err != nil {
		return nil, fmt.Errorf("fetching antigravity models: %w", err)
	}

	result := make([]dto.ProviderModelItem, 0, len(models))
	seen := make(map[string]struct{}, len(models))

	for _, m := range models {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}

		label := strings.TrimSpace(m.DisplayName)
		if label == "" {
			label = id
		}
		// Mark exhausted models in label
		if m.IsExhausted {
			label = label + " (配额已用尽)"
		}

		result = append(result, dto.ProviderModelItem{
			ID:    id,
			Label: label,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Label < result[j].Label
	})

	if len(result) == 0 {
		return nil, fmt.Errorf("antigravity models response is empty")
	}

	return result, nil
}

// ollamaModelsResponse represents the response from Ollama's /api/tags API
type ollamaModelsResponse struct {
	Models []struct {
		Name    string `json:"name"`
		Model   string `json:"model"`
		Details struct {
			Family         string   `json:"family"`
			Families       []string `json:"families"`
			ParameterSize  string   `json:"parameter_size"`
			Quantization   string   `json:"quantization_level"`
		} `json:"details"`
	} `json:"models"`
}

func (s *Service) fetchOllamaModels(
	ctx context.Context,
	runtime runtimeConfig,
) ([]dto.ProviderModelItem, error) {
	baseURL := strings.TrimSpace(runtime.baseURL)
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	// Ollama uses /api/tags endpoint for listing models
	endpoint := baseURL + "/api/tags"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating ollama models request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	// Ollama may have optional API key authentication
	if strings.TrimSpace(runtime.apiKey) != "" && runtime.apiKey != "ollama" {
		req.Header.Set("Authorization", "Bearer "+runtime.apiKey)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting ollama models: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading ollama models response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama models request failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	var payload ollamaModelsResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parsing ollama models response: %w", err)
	}

	models := make([]dto.ProviderModelItem, 0, len(payload.Models))
	seen := make(map[string]struct{}, len(payload.Models))

	for _, item := range payload.Models {
		// Use the name field as model ID
		id := strings.TrimSpace(item.Name)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}

		// Build a descriptive label with model details
		label := id
		if item.Details.ParameterSize != "" {
			label = fmt.Sprintf("%s (%s)", id, item.Details.ParameterSize)
		}

		models = append(models, dto.ProviderModelItem{
			ID:    id,
			Label: label,
		})
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i].Label < models[j].Label
	})

	if len(models) == 0 {
		return nil, fmt.Errorf("ollama models response is empty")
	}

	return models, nil
}

func staticModelsForProvider(provider string) []dto.ProviderModelItem {
	if provider == "openai" {
		return []dto.ProviderModelItem{
			{ID: "gpt-5.3-codex", Label: "gpt-5.3-codex"},
			{ID: "gpt-5.2-codex", Label: "gpt-5.2-codex"},
			{ID: "gpt-5.1-codex", Label: "gpt-5.1-codex"},
			{ID: "codex-mini-latest", Label: "codex-mini-latest"},
		}
	}

	if provider == "qwen" {
		return []dto.ProviderModelItem{
			{ID: "coder-model", Label: "coder-model"},
			{ID: "vision-model", Label: "vision-model"},
		}
	}

	if provider == "gemini" {
		return []dto.ProviderModelItem{
			{ID: "gemini-2.0-flash", Label: "Gemini 2.0 Flash"},
			{ID: "gemini-2.0-flash-lite", Label: "Gemini 2.0 Flash-Lite"},
			{ID: "gemini-2.5-flash", Label: "Gemini 2.5 Flash"},
			{ID: "gemini-2.5-pro", Label: "Gemini 2.5 Pro"},
			{ID: "gemini-1.5-flash", Label: "Gemini 1.5 Flash"},
			{ID: "gemini-1.5-pro", Label: "Gemini 1.5 Pro"},
		}
	}

	if provider == "ollama" {
		return []dto.ProviderModelItem{
			{ID: "llama3", Label: "Llama 3"},
			{ID: "llama3.1", Label: "Llama 3.1"},
			{ID: "llama3.2", Label: "Llama 3.2"},
			{ID: "qwen2.5", Label: "Qwen 2.5"},
			{ID: "mistral", Label: "Mistral"},
			{ID: "codellama", Label: "CodeLlama"},
			{ID: "deepseek-coder", Label: "DeepSeek Coder"},
		}
	}

	if provider == "antigravity" {
		return []dto.ProviderModelItem{
			{ID: "gemini-3-flash", Label: "Gemini 3 Flash"},
			{ID: "claude-sonnet-4.6", Label: "Claude Sonnet 4.6"},
			{ID: "claude-opus-4.6", Label: "Claude Opus 4.6"},
		}
	}

	defaultCfg := config.DefaultConfig()
	models := make([]dto.ProviderModelItem, 0)
	seen := make(map[string]struct{})

	for _, modelCfg := range defaultCfg.ModelList {
		if extractProviderID(modelCfg.Model) != provider {
			continue
		}
		if _, ok := seen[modelCfg.ModelName]; ok {
			continue
		}
		seen[modelCfg.ModelName] = struct{}{}
		models = append(models, dto.ProviderModelItem{
			ID:    modelCfg.ModelName,
			Label: modelCfg.ModelName,
		})
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i].Label < models[j].Label
	})
	return models
}

func buildRuntimeConfig(provider string, req dto.ProviderModelsRequest) (runtimeConfig, error) {
	authMode := strings.TrimSpace(req.AuthMode)
	switch authMode {
	case "token":
		return runtimeConfig{
			apiKey:  strings.TrimSpace(req.APIKey),
			baseURL: strings.TrimSpace(req.BaseURL),
		}, nil
	case "oauth":
		return buildOAuthRuntimeConfig(provider, strings.TrimSpace(req.BaseURL))
	default:
		return runtimeConfig{}, fmt.Errorf("unsupported auth mode: %s", authMode)
	}
}

func buildOAuthRuntimeConfig(provider string, fallbackBaseURL string) (runtimeConfig, error) {
	switch provider {
	case "qwen":
		cred, err := auth.GetCredential("qwen")
		if err != nil {
			return runtimeConfig{}, fmt.Errorf("loading qwen oauth credentials: %w", err)
		}
		if cred == nil || strings.TrimSpace(cred.AccessToken) == "" {
			return runtimeConfig{}, fmt.Errorf("qwen oauth credentials not found")
		}
		if cred.NeedsRefresh() && cred.RefreshToken != "" {
			refreshed, err := auth.RefreshQwenAccessToken(cred)
			if err != nil {
				return runtimeConfig{}, fmt.Errorf("refreshing qwen oauth token: %w", err)
			}
			if err := auth.SetCredential("qwen", refreshed); err != nil {
				return runtimeConfig{}, fmt.Errorf("saving refreshed qwen oauth token: %w", err)
			}
			cred = refreshed
		}
		if cred.IsExpired() {
			return runtimeConfig{}, fmt.Errorf("qwen oauth credentials expired")
		}
		return runtimeConfig{
			apiKey:  cred.AccessToken,
			baseURL: normalizeQwenOAuthAPIBase(cred.ResourceURL, fallbackBaseURL),
			headers: map[string]string{
				"X-DashScope-AuthType": "qwen-oauth",
			},
		}, nil
	case "openai":
		cred, err := auth.GetCredential("openai")
		if err != nil {
			return runtimeConfig{}, fmt.Errorf("loading openai oauth credentials: %w", err)
		}
		if cred == nil || strings.TrimSpace(cred.AccessToken) == "" {
			return runtimeConfig{}, fmt.Errorf("openai oauth credentials not found")
		}
		if cred.AuthMethod == "oauth" && cred.NeedsRefresh() && cred.RefreshToken != "" {
			refreshed, err := auth.RefreshAccessToken(cred, auth.OpenAIOAuthConfig())
			if err != nil {
				return runtimeConfig{}, fmt.Errorf("refreshing openai oauth token: %w", err)
			}
			if err := auth.SetCredential("openai", refreshed); err != nil {
				return runtimeConfig{}, fmt.Errorf("saving refreshed openai oauth token: %w", err)
			}
			cred = refreshed
		}
		if cred.IsExpired() {
			return runtimeConfig{}, fmt.Errorf("openai oauth credentials expired")
		}
		if strings.TrimSpace(fallbackBaseURL) == "" {
			fallbackBaseURL = "https://api.openai.com/v1"
		}
		return runtimeConfig{
			apiKey:  cred.AccessToken,
			baseURL: strings.TrimSpace(fallbackBaseURL),
		}, nil
	case "antigravity":
		cred, err := auth.GetCredential("google-antigravity")
		if err != nil {
			return runtimeConfig{}, fmt.Errorf("loading antigravity oauth credentials: %w", err)
		}
		if cred == nil || strings.TrimSpace(cred.AccessToken) == "" {
			return runtimeConfig{}, fmt.Errorf("antigravity oauth credentials not found")
		}
		if cred.NeedsRefresh() && cred.RefreshToken != "" {
			oauthCfg := auth.GoogleAntigravityOAuthConfig()
			refreshed, err := auth.RefreshAccessToken(cred, oauthCfg)
			if err != nil {
				return runtimeConfig{}, fmt.Errorf("refreshing antigravity oauth token: %w", err)
			}
			refreshed.Email = cred.Email
			if refreshed.ProjectID == "" {
				refreshed.ProjectID = cred.ProjectID
			}
			if err := auth.SetCredential("google-antigravity", refreshed); err != nil {
				return runtimeConfig{}, fmt.Errorf("saving refreshed antigravity oauth token: %w", err)
			}
			cred = refreshed
		}
		if cred.IsExpired() {
			return runtimeConfig{}, fmt.Errorf("antigravity oauth credentials expired")
		}
		// For antigravity, we need to use a special fetcher that calls Cloud Code Assist API
		// Return empty runtime and handle in fetchRemoteModels
		return runtimeConfig{
			apiKey:  cred.AccessToken,
			baseURL: "", // Special marker for antigravity
			headers: map[string]string{
				"X-Project-ID": cred.ProjectID,
			},
		}, nil
	default:
		return runtimeConfig{}, fmt.Errorf("oauth model listing not supported for provider: %s", provider)
	}
}

func candidateModelEndpoints(baseURL string) []string {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		return nil
	}

	parsed, err := url.Parse(base)
	if err != nil {
		return []string{strings.TrimRight(base, "/") + "/models"}
	}

	normalized := strings.TrimRight(parsed.String(), "/")
	endpoints := []string{normalized + "/models"}

	if !strings.HasSuffix(parsed.Path, "/v1") && !strings.HasSuffix(parsed.Path, "/models") {
		trimmed := strings.TrimRight(normalized, "/")
		endpoints = append(endpoints, trimmed+"/v1/models")
	}

	return uniqueStrings(endpoints)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
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

func normalizeQwenOAuthAPIBase(resourceURL, fallbackAPIBase string) string {
	base := strings.TrimSpace(resourceURL)
	if base == "" {
		base = strings.TrimSpace(fallbackAPIBase)
	}
	if base == "" {
		base = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "https://" + base
	}
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/v1") {
		return base
	}
	return base + "/v1"
}

// codexModelInfo represents a model entry from the Codex backend /models API
type codexModelInfo struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
	Visibility  string `json:"visibility"`
	Priority    int    `json:"priority"`
}

type codexModelsResponse struct {
	Models []codexModelInfo `json:"models"`
}

// fetchCodexModels fetches available models from the ChatGPT Codex backend.
// This is used for OpenAI OAuth authentication mode.
func (s *Service) fetchCodexModels(ctx context.Context) ([]dto.ProviderModelItem, error) {
	cred, err := auth.GetCredential("openai")
	if err != nil {
		return nil, fmt.Errorf("loading openai oauth credentials: %w", err)
	}
	if cred == nil || strings.TrimSpace(cred.AccessToken) == "" {
		return nil, fmt.Errorf("openai oauth credentials not found")
	}

	// Refresh token if needed
	if cred.AuthMethod == "oauth" && cred.NeedsRefresh() && cred.RefreshToken != "" {
		refreshed, err := auth.RefreshAccessToken(cred, auth.OpenAIOAuthConfig())
		if err != nil {
			return nil, fmt.Errorf("refreshing openai oauth token: %w", err)
		}
		if refreshed.AccountID == "" {
			refreshed.AccountID = cred.AccountID
		}
		if err := auth.SetCredential("openai", refreshed); err != nil {
			return nil, fmt.Errorf("saving refreshed openai oauth token: %w", err)
		}
		cred = refreshed
	}

	if cred.IsExpired() {
		return nil, fmt.Errorf("openai oauth credentials expired")
	}

	// Codex backend requires client_version query parameter
	// Use a recent version to ensure all models are available
	endpoint := codexModelsEndpoint + "?client_version=0.98.0"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating codex models request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+cred.AccessToken)
	req.Header.Set("originator", "codex_cli_rs")
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	if cred.AccountID != "" {
		req.Header.Set("Chatgpt-Account-Id", cred.AccountID)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting codex models: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading codex models response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("codex models request failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	var payload codexModelsResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parsing codex models response: %w", err)
	}

	models := make([]dto.ProviderModelItem, 0, len(payload.Models))
	seen := make(map[string]struct{}, len(payload.Models))

	for _, item := range payload.Models {
		id := strings.TrimSpace(item.Slug)
		if id == "" {
			continue
		}
		// Filter out hidden models
		if item.Visibility == "hide" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}

		label := strings.TrimSpace(item.DisplayName)
		if label == "" {
			label = id
		}
		models = append(models, dto.ProviderModelItem{
			ID:    id,
			Label: label,
		})
	}

	// Sort by priority (lower priority value = higher precedence)
	sort.Slice(models, func(i, j int) bool {
		return models[i].Label < models[j].Label
	})

	if len(models) == 0 {
		return nil, fmt.Errorf("codex models response is empty")
	}

	return models, nil
}
