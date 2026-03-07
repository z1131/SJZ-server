package providers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/auth"
	"github.com/sipeed/picoclaw/pkg/providers/openai_compat"
)

const defaultQwenOAuthAPIBase = "https://dashscope.aliyuncs.com/compatible-mode/v1"

type QwenOAuthProvider struct {
	proxy           string
	maxTokensField  string
	requestTimeout  int
	fallbackAPIBase string
}

func NewQwenOAuthProvider(proxy, maxTokensField string, requestTimeout int, fallbackAPIBase string) *QwenOAuthProvider {
	if fallbackAPIBase == "" {
		fallbackAPIBase = defaultQwenOAuthAPIBase
	}

	return &QwenOAuthProvider{
		proxy:           proxy,
		maxTokensField:  maxTokensField,
		requestTimeout:  requestTimeout,
		fallbackAPIBase: fallbackAPIBase,
	}
}

func (p *QwenOAuthProvider) Chat(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
) (*LLMResponse, error) {
	token, apiBase, err := resolveQwenOAuthRuntimeConfig(p.fallbackAPIBase)
	if err != nil {
		return nil, err
	}

	delegate := openai_compat.NewProvider(
		token,
		apiBase,
		p.proxy,
		openai_compat.WithMaxTokensField(p.maxTokensField),
		openai_compat.WithRequestTimeout(time.Duration(p.requestTimeout)*time.Second),
		openai_compat.WithHeaders(map[string]string{
			"X-DashScope-AuthType": authTypeQwenOAuth,
		}),
	)

	return delegate.Chat(ctx, messages, tools, model, options)
}

func (p *QwenOAuthProvider) GetDefaultModel() string {
	return ""
}

const authTypeQwenOAuth = "qwen-oauth"

func resolveQwenOAuthRuntimeConfig(fallbackAPIBase string) (string, string, error) {
	cred, err := auth.GetCredential("qwen")
	if err != nil {
		return "", "", fmt.Errorf("loading qwen auth credentials: %w", err)
	}
	if cred == nil {
		return "", "", fmt.Errorf("no credentials for qwen. Use Qwen OAuth login first")
	}

	if cred.NeedsRefresh() && cred.RefreshToken != "" {
		refreshed, err := auth.RefreshQwenAccessToken(cred)
		if err != nil {
			return "", "", fmt.Errorf("refreshing qwen token: %w", err)
		}
		if err := auth.SetCredential("qwen", refreshed); err != nil {
			return "", "", fmt.Errorf("saving refreshed qwen token: %w", err)
		}
		cred = refreshed
	}

	if cred.IsExpired() {
		return "", "", fmt.Errorf("qwen credentials expired. Use Qwen OAuth login again")
	}

	return cred.AccessToken, normalizeQwenOAuthAPIBase(cred.ResourceURL, fallbackAPIBase), nil
}

func normalizeQwenOAuthAPIBase(resourceURL, fallbackAPIBase string) string {
	base := strings.TrimSpace(resourceURL)
	if base == "" {
		base = strings.TrimSpace(fallbackAPIBase)
	}
	if base == "" {
		base = defaultQwenOAuthAPIBase
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
