package gatepasses

import "github.com/go-chi/chi/v5"

// RegisterRoutes mounts this module under the API router.
func (h *Handler) RegisterRoutes(r chi.Router) { h.registerRoutes(r) }
