package modelcatalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sipeed/picoclaw/internal/uiapi/dto"
)

func TestListProviderModels_RemoteOpenAICompatible(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected authorization header: %s", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{
				{"id": "qwen-plus"},
				{"id": "qwen-max"},
			},
		})
	}))
	defer server.Close()

	service := NewServiceWithClient(server.Client())
	result, err := service.ListProviderModels(context.Background(), dto.ProviderModelsRequest{
		Provider: "qwen",
		AuthMode: "token",
		APIKey:   "test-key",
		BaseURL:  server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("ListProviderModels() error = %v", err)
	}
	if result.Source != sourceRemote {
		t.Fatalf("expected source %q, got %q", sourceRemote, result.Source)
	}
	if result.Fallback {
		t.Fatal("expected fallback to be false")
	}
	if len(result.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(result.Models))
	}
}

func TestListProviderModels_FallbackToStatic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer server.Close()

	service := NewServiceWithClient(server.Client())
	result, err := service.ListProviderModels(context.Background(), dto.ProviderModelsRequest{
		Provider: "qwen",
		AuthMode: "token",
		APIKey:   "test-key",
		BaseURL:  server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("ListProviderModels() error = %v", err)
	}
	if result.Source != sourceStatic {
		t.Fatalf("expected source %q, got %q", sourceStatic, result.Source)
	}
	if !result.Fallback {
		t.Fatal("expected fallback to be true")
	}
	if len(result.Models) == 0 {
		t.Fatal("expected static models to be returned")
	}
}

func TestCandidateModelEndpoints(t *testing.T) {
	got := candidateModelEndpoints("https://api.example.com")
	want := []string{
		"https://api.example.com/models",
		"https://api.example.com/v1/models",
	}

	if len(got) != len(want) {
		t.Fatalf("expected %d endpoints, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected endpoint %q at index %d, got %q", want[i], i, got[i])
		}
	}
}
