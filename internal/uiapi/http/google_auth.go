package http

import (
	"encoding/json"
	"fmt"
	nethttp "net/http"

	"github.com/sipeed/picoclaw/internal/app/googleauth"
	"github.com/sipeed/picoclaw/internal/uiapi/dto"
)

// GoogleAuthHandler handles Google OAuth endpoints for the UI.
type GoogleAuthHandler struct {
	service *googleauth.Service
}

// NewGoogleAuthHandler creates a new Google auth handler.
func NewGoogleAuthHandler(service *googleauth.Service) *GoogleAuthHandler {
	return &GoogleAuthHandler{service: service}
}

// Register registers the Google auth endpoints on the mux.
// Uses "antigravity" as the provider ID to match the catalog.
func (h *GoogleAuthHandler) Register(mux *nethttp.ServeMux) {
	mux.HandleFunc("GET /api/auth/antigravity/status", h.handleStatus)
	mux.HandleFunc("POST /api/auth/antigravity/login", h.handleLogin)
	mux.HandleFunc("GET /api/auth/antigravity/events", h.handleEvents)
	mux.HandleFunc("POST /api/auth/antigravity/models", h.handleModels)
}

func (h *GoogleAuthHandler) handleStatus(w nethttp.ResponseWriter, _ *nethttp.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(mapGoogleStatus(h.service.GetStatus()))
}

func (h *GoogleAuthHandler) handleLogin(w nethttp.ResponseWriter, _ *nethttp.Request) {
	result, err := h.service.StartLogin()
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dto.StartGoogleAuthResponse{
		Status:    result.Status,
		VerifyURL: result.VerifyURL,
	})
}

func (h *GoogleAuthHandler) handleEvents(w nethttp.ResponseWriter, r *nethttp.Request) {
	flusher, ok := w.(nethttp.Flusher)
	if !ok {
		nethttp.Error(w, "streaming unsupported", nethttp.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := h.service.Subscribe()
	defer h.service.Unsubscribe(ch)

	for {
		select {
		case <-r.Context().Done():
			return
		case status, ok := <-ch:
			if !ok {
				return
			}
			payload, err := json.Marshal(mapGoogleStatus(status))
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "event: status\ndata: %s\n\n", payload); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (h *GoogleAuthHandler) handleModels(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req dto.GoogleModelsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		nethttp.Error(w, "invalid request body", nethttp.StatusBadRequest)
		return
	}

	if req.AccessToken == "" || req.ProjectID == "" {
		nethttp.Error(w, "access_token and project_id are required", nethttp.StatusBadRequest)
		return
	}

	models, err := googleauth.FetchModels(req.AccessToken, req.ProjectID)
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dto.GoogleModelsResponse{
		Provider: "google",
		Source:   "remote",
		Models:   mapGoogleModels(models),
	})
}

func mapGoogleStatus(status googleauth.Status) dto.GoogleAuthStatusResponse {
	return dto.GoogleAuthStatusResponse{
		Provider:   status.Provider,
		Status:     status.Status,
		Connected:  status.Connected,
		AuthMethod: status.AuthMethod,
		Email:      status.Email,
		ProjectID:  status.ProjectID,
		Error:      status.Error,
	}
}

func mapGoogleModels(models []googleauth.ModelInfo) []dto.GoogleModelItem {
	result := make([]dto.GoogleModelItem, len(models))
	for i, m := range models {
		result[i] = dto.GoogleModelItem{
			ID:          m.ID,
			Label:       m.DisplayName,
			IsExhausted: m.IsExhausted,
		}
	}
	return result
}