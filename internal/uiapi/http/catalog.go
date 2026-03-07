package http

import (
	"encoding/json"
	nethttp "net/http"

	"github.com/sipeed/picoclaw/internal/app/catalog"
)

type CatalogHandler struct {
	service *catalog.Service
}

func NewCatalogHandler(service *catalog.Service) *CatalogHandler {
	return &CatalogHandler{service: service}
}

func (h *CatalogHandler) Register(mux *nethttp.ServeMux) {
	mux.HandleFunc("GET /api/provider-catalog", h.handleProviderCatalog)
}

func (h *CatalogHandler) handleProviderCatalog(w nethttp.ResponseWriter, _ *nethttp.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.service.ListProviders())
}
