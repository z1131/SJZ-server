package welcome

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sipeed/picoclaw/internal/uiapi/dto"
	"github.com/sipeed/picoclaw/pkg/config"
)

func TestService_SaveModelSetup(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Write initial config
	initialCfg := config.DefaultConfig()
	if err := config.SaveConfig(configPath, initialCfg); err != nil {
		t.Fatalf("failed to save initial config: %v", err)
	}

	service := NewServiceWithPath(configPath)

	// Test saving model setup
	req := dto.SaveModelRequest{
		ModelID:  "gpt-5.2",
		Provider: "openai",
		AuthMode: "token",
		APIKey:   "test-api-key",
		BaseURL:  "https://api.openai.com/v1",
	}

	if err := service.SaveModelSetup(req); err != nil {
		t.Errorf("SaveModelSetup failed: %v", err)
	}

	// Verify config was saved
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Agents.Defaults.Provider != "openai" {
		t.Errorf("expected provider 'openai', got '%s'", cfg.Agents.Defaults.Provider)
	}

	if cfg.Agents.Defaults.Model != "gpt-5.2" {
		t.Errorf("expected model 'gpt-5.2', got '%s'", cfg.Agents.Defaults.Model)
	}

	// Check that model was added to ModelList
	found := false
	for _, m := range cfg.ModelList {
		if m.ModelName == "gpt-5.2" {
			found = true
			if m.APIKey != "test-api-key" {
				t.Errorf("expected API key 'test-api-key', got '%s'", m.APIKey)
			}
			break
		}
	}
	if !found {
		t.Error("model not found in ModelList")
	}
}

func TestService_SaveQQSetup(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Write initial config
	initialCfg := config.DefaultConfig()
	if err := config.SaveConfig(configPath, initialCfg); err != nil {
		t.Fatalf("failed to save initial config: %v", err)
	}

	service := NewServiceWithPath(configPath)

	// Test saving QQ setup
	req := dto.SaveQQRequest{
		AppID:     "123456789",
		AppSecret: "test-secret",
	}

	if err := service.SaveQQSetup(req); err != nil {
		t.Errorf("SaveQQSetup failed: %v", err)
	}

	// Verify config was saved
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if !cfg.Channels.QQ.Enabled {
		t.Error("expected QQ channel to be enabled")
	}

	if cfg.Channels.QQ.AppID != "123456789" {
		t.Errorf("expected AppID '123456789', got '%s'", cfg.Channels.QQ.AppID)
	}

	if cfg.Channels.QQ.AppSecret != "test-secret" {
		t.Errorf("expected AppSecret 'test-secret', got '%s'", cfg.Channels.QQ.AppSecret)
	}
}

func TestService_GetBootstrap(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Write initial config (empty)
	initialCfg := config.DefaultConfig()
	if err := config.SaveConfig(configPath, initialCfg); err != nil {
		t.Fatalf("failed to save initial config: %v", err)
	}

	service := NewServiceWithPath(configPath)

	// Test getting bootstrap (should return "model" step)
	resp, err := service.GetBootstrap()
	if err != nil {
		t.Fatalf("GetBootstrap failed: %v", err)
	}

	if resp.CurrentStep != "model" {
		t.Errorf("expected step 'model', got '%s'", resp.CurrentStep)
	}

	if len(resp.ModelOptions) == 0 {
		t.Error("expected model options, got none")
	}

	if resp.QQ.GuideURL == "" {
		t.Error("expected QQ guide URL")
	}

	if len(resp.QQ.Fields) != 2 {
		t.Errorf("expected 2 QQ fields, got %d", len(resp.QQ.Fields))
	}
}

func TestService_GetBootstrap_Completed(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Write config with both model and QQ configured
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Provider = "openai"
	cfg.Agents.Defaults.Model = "gpt-5.2"
	cfg.Channels.QQ.Enabled = true
	cfg.Channels.QQ.AppID = "123"
	cfg.Channels.QQ.AppSecret = "secret"
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	service := NewServiceWithPath(configPath)

	// Test getting bootstrap (should return "completed" step)
	resp, err := service.GetBootstrap()
	if err != nil {
		t.Fatalf("GetBootstrap failed: %v", err)
	}

	if resp.CurrentStep != "completed" {
		t.Errorf("expected step 'completed', got '%s'", resp.CurrentStep)
	}
}

func TestExtractProviderID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"openai/gpt-4", "openai"},
		{"anthropic/claude", "anthropic"},
		{"qwen/qwen-plus", "qwen"},
		{"invalid", "invalid"},
		{"", ""},
	}

	for _, tc := range tests {
		result := extractProviderID(tc.input)
		if result != tc.expected {
			t.Errorf("extractProviderID(%q) = %q, expected %q", tc.input, result, tc.expected)
		}
	}
}

func TestGetConfigPath(t *testing.T) {
	// Save original env
	originalPath := os.Getenv("PICOCLAW_CONFIG")
	defer os.Setenv("PICOCLAW_CONFIG", originalPath)

	// Test with env variable
	os.Setenv("PICOCLAW_CONFIG", "/custom/path/config.json")
	path := getConfigPath()
	if path != "/custom/path/config.json" {
		t.Errorf("expected '/custom/path/config.json', got '%s'", path)
	}

	// Test without env variable (should use home dir)
	os.Unsetenv("PICOCLAW_CONFIG")
	path = getConfigPath()
	if path == "" {
		t.Error("expected non-empty path")
	}
}
