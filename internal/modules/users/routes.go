package users

import "github.com/go-chi/chi/v5"

// RegisterRoutes mounts user-management endpoints. Authentication routes are
// registered separately by AuthService because they are not tenant CRUD.
func (h *Handler) RegisterRoutes(r chi.Router) { h.registerRoutes(r) }

func (h *Handler) RegisterMeRoute(r chi.Router) { r.Get("/auth/me", h.Me) }
