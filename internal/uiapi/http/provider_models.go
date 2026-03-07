package http

import (
	"encoding/json"
	"errors"
	nethttp "net/http"

	"github.com/sipeed/picoclaw/internal/app/modelcatalog"
	"github.com/sipeed/picoclaw/internal/uiapi/dto"
)

type ProviderModelsHandler struct {
	service *modelcatalog.Service
}

func NewProviderModelsHandler(service *modelcatalog.Service) *ProviderModelsHandler {
	return &ProviderModelsHandler{service: service}
}

func (h *ProviderModelsHandler) Register(mux *nethttp.ServeMux) {
	mux.HandleFunc("POST /api/provider-models", h.handleProviderModels)
}

func (h *ProviderModelsHandler) handleProviderModels(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req dto.ProviderModelsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, nethttp.StatusBadRequest, "invalid request body")
		return
	}

	resp, err := h.service.ListProviderModels(r.Context(), req)
	if err != nil {
		status := nethttp.StatusInternalServerError
		switch {
		case errors.Is(err, nethttp.ErrNotSupported):
			status = nethttp.StatusBadRequest
		}
		writeJSONError(w, status, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func writeJSONError(w nethttp.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}
