package http

import (
	"encoding/json"
	"fmt"
	nethttp "net/http"

	"github.com/sipeed/picoclaw/internal/app/qwenauth"
	"github.com/sipeed/picoclaw/internal/uiapi/dto"
)

type QwenAuthHandler struct {
	service *qwenauth.Service
}

func NewQwenAuthHandler(service *qwenauth.Service) *QwenAuthHandler {
	return &QwenAuthHandler{service: service}
}

func (h *QwenAuthHandler) Register(mux *nethttp.ServeMux) {
	mux.HandleFunc("GET /api/auth/qwen/status", h.handleStatus)
	mux.HandleFunc("POST /api/auth/qwen/login", h.handleLogin)
	mux.HandleFunc("GET /api/auth/qwen/events", h.handleEvents)
}

func (h *QwenAuthHandler) handleStatus(w nethttp.ResponseWriter, _ *nethttp.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(mapQwenStatus(h.service.GetStatus()))
}

func (h *QwenAuthHandler) handleLogin(w nethttp.ResponseWriter, _ *nethttp.Request) {
	result, err := h.service.StartLogin()
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dto.StartQwenAuthResponse{
		Status:            result.Status,
		UserCode:          result.UserCode,
		VerifyURL:         result.VerifyURL,
		VerifyURLComplete: result.VerifyURLComplete,
		ExpiresIn:         result.ExpiresIn,
	})
}

func (h *QwenAuthHandler) handleEvents(w nethttp.ResponseWriter, r *nethttp.Request) {
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
			payload, err := json.Marshal(mapQwenStatus(status))
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

func mapQwenStatus(status qwenauth.Status) dto.QwenAuthStatusResponse {
	resp := dto.QwenAuthStatusResponse{
		Provider:   status.Provider,
		Status:     status.Status,
		Connected:  status.Connected,
		AuthMethod: status.AuthMethod,
		AccountID:  status.AccountID,
		Email:      status.Email,
		Error:      status.Error,
	}
	if status.PendingDevice != nil {
		resp.PendingDevice = &dto.QwenPendingDeviceInfo{
			UserCode:          status.PendingDevice.UserCode,
			VerifyURL:         status.PendingDevice.VerifyURL,
			VerifyURLComplete: status.PendingDevice.VerifyURLComplete,
			ExpiresIn:         status.PendingDevice.ExpiresIn,
		}
	}
	return resp
}
