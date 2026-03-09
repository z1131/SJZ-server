package http

import (
	"encoding/json"
	nethttp "net/http"
	"strings"

	"github.com/sipeed/picoclaw/internal/app/skills"
)

type SkillsHandler struct {
	service *skills.Service
}

func NewSkillsHandler(service *skills.Service) *SkillsHandler {
	return &SkillsHandler{service: service}
}

func (h *SkillsHandler) Register(mux *nethttp.ServeMux) {
	mux.HandleFunc("GET /api/skills", h.handleListSkills)
	mux.HandleFunc("GET /api/skills/", h.handleGetSkillDetail)
	mux.HandleFunc("DELETE /api/skills/", h.handleDeleteSkill)
	mux.HandleFunc("POST /api/skills/upload", h.handleUploadSkill)
}

func (h *SkillsHandler) handleListSkills(w nethttp.ResponseWriter, _ *nethttp.Request) {
	resp, err := h.service.ListSkills()
	if err != nil {
		writeJSONError(w, nethttp.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *SkillsHandler) handleGetSkillDetail(w nethttp.ResponseWriter, r *nethttp.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/skills/")
	if id == "" {
		writeJSONError(w, nethttp.StatusBadRequest, "skill id is required")
		return
	}

	resp, err := h.service.GetSkillDetail(id)
	if err != nil {
		writeJSONError(w, nethttp.StatusNotFound, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *SkillsHandler) handleDeleteSkill(w nethttp.ResponseWriter, r *nethttp.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/skills/")
	if id == "" {
		writeJSONError(w, nethttp.StatusBadRequest, "skill id is required")
		return
	}

	if err := h.service.DeleteSkill(id); err != nil {
		writeJSONError(w, nethttp.StatusBadRequest, err.Error())
		return
	}

	w.WriteHeader(nethttp.StatusNoContent)
}

func (h *SkillsHandler) handleUploadSkill(w nethttp.ResponseWriter, r *nethttp.Request) {
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSONError(w, nethttp.StatusBadRequest, "file is required")
		return
	}
	defer func() { _ = file.Close() }()

	resp, err := h.service.UploadSkill(header.Filename, file)
	if err != nil {
		writeJSONError(w, nethttp.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
