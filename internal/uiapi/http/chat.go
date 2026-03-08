package http

import (
	"encoding/json"
	nethttp "net/http"

	"github.com/sipeed/picoclaw/internal/app/chat"
	"github.com/sipeed/picoclaw/internal/uiapi/dto"
)

type ChatHandler struct {
	service *chat.Service
}

func NewChatHandler(service *chat.Service) *ChatHandler {
	return &ChatHandler{service: service}
}

func (h *ChatHandler) Register(mux *nethttp.ServeMux) {
	mux.HandleFunc("GET /api/chat/sessions", h.handleListSessions)
	mux.HandleFunc("POST /api/chat/sessions", h.handleCreateSession)
	mux.HandleFunc("GET /api/chat/sessions/{sessionId}/messages", h.handleGetMessages)
	mux.HandleFunc("POST /api/chat/sessions/{sessionId}/messages", h.handleSendMessage)
}

func (h *ChatHandler) handleListSessions(w nethttp.ResponseWriter, _ *nethttp.Request) {
	resp, err := h.service.ListSessions()
	if err != nil {
		writeJSONError(w, nethttp.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *ChatHandler) handleCreateSession(w nethttp.ResponseWriter, r *nethttp.Request) {
	var req dto.CreateChatSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		writeJSONError(w, nethttp.StatusBadRequest, "invalid request body")
		return
	}

	resp, err := h.service.CreateSession(req.Title)
	if err != nil {
		writeJSONError(w, nethttp.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *ChatHandler) handleGetMessages(w nethttp.ResponseWriter, r *nethttp.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		writeJSONError(w, nethttp.StatusBadRequest, "sessionId is required")
		return
	}

	resp, err := h.service.GetMessages(sessionID)
	if err != nil {
		writeJSONError(w, nethttp.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *ChatHandler) handleSendMessage(w nethttp.ResponseWriter, r *nethttp.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		writeJSONError(w, nethttp.StatusBadRequest, "sessionId is required")
		return
	}

	var req dto.SendChatMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, nethttp.StatusBadRequest, "invalid request body")
		return
	}

	resp, err := h.service.SendMessage(r.Context(), sessionID, req.Content)
	if err != nil {
		writeJSONError(w, nethttp.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
