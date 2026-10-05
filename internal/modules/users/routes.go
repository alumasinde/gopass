package users

import "github.com/go-chi/chi/v5"

func (h *Handler) RegisterRoutes(r chi.Router) { h.registerRoutes(r) }
func (h *Handler) RegisterAuthRoutes(r chi.Router) { h.registerAuthRoutes(r) }
func (h *Handler) RegisterMeRoute(r chi.Router) { r.Get("/auth/me", h.Me) }