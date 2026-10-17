package api

import (
	"errors"
	"net/http"
	"strings"

	"voiceagent/internal/auth"
	"voiceagent/internal/store"
)

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, r, &req) {
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "e-mail e senha são obrigatórios")
		return
	}
	u, err := s.Store.UserByEmail(r.Context(), req.Email)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !auth.CheckPassword(u.PasswordHash, req.Password)) {
		s.Log.Warn("login_failed", "email", req.Email)
		writeError(w, http.StatusUnauthorized, "e-mail ou senha inválidos")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	token, err := s.Sessions.Create(r.Context(), auth.Session{UserID: u.ID, OrganizationID: u.OrganizationID, Email: u.Email})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: auth.CookieName, Value: token, Path: "/", HttpOnly: true,
		Secure: s.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: 7 * 24 * 3600,
	})
	s.Log.Info("login", "user_id", u.ID, "organization_id", u.OrganizationID)
	writeJSON(w, http.StatusOK, map[string]string{"user_id": u.ID, "organization_id": u.OrganizationID, "email": u.Email})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.CookieName); err == nil {
		s.Sessions.Delete(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: auth.CookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	sess := auth.FromContext(r.Context())
	org, err := s.Store.Organization(r.Context(), sess.OrganizationID)
	if err != nil {
		s.notFoundOr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id": sess.UserID, "email": sess.Email, "organization": org,
	})
}
