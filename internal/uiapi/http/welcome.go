package http

import (
	"encoding/json"
	"net/http"

	"github.com/sipeed/picoclaw/internal/app/welcome"
	"github.com/sipeed/picoclaw/internal/uiapi/dto"
)

// WelcomeHandler handles welcome page endpoints
type WelcomeHandler struct {
	service *welcome.Service
}

// NewWelcomeHandler creates a new welcome handler
func NewWelcomeHandler(service *welcome.Service) *WelcomeHandler {
	return &WelcomeHandler{service: service}
}

// Register registers the welcome endpoints on the mux
func (h *WelcomeHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/bootstrap", h.handleBootstrap)
	mux.HandleFunc("POST /api/welcome/model", h.handleSaveModel)
	mux.HandleFunc("POST /api/welcome/qq", h.handleSaveQQ)
}

func (h *WelcomeHandler) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	resp, err := h.service.GetBootstrap()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *WelcomeHandler) handleSaveModel(w http.ResponseWriter, r *http.Request) {
	var req dto.SaveModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.service.SaveModelSetup(req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dto.SaveModelResponse{
		NextStep: "qq",
	})
}

func (h *WelcomeHandler) handleSaveQQ(w http.ResponseWriter, r *http.Request) {
	var req dto.SaveQQRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.service.SaveQQSetup(req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dto.SaveQQResponse{
		Completed: true,
	})
}
