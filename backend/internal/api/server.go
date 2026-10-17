package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"voiceagent/internal/auth"
	"voiceagent/internal/runtime"
	"voiceagent/internal/storage"
	"voiceagent/internal/store"
	"voiceagent/internal/voice"
)

type Server struct {
	Store             *store.Store
	Sessions          *auth.Sessions
	Voices            *voice.Registry
	Runtime           *runtime.Runtime
	Storage           *storage.Storage
	HTTPClient        *http.Client
	CookieSecure      bool
	Log               *slog.Logger
	JambonzAPIURL     string
	JambonzAPIKey     string
	JambonzAccountSid string
	PublicWSURL       string
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	protected := func(h http.HandlerFunc) http.Handler { return s.Sessions.Require(h) }

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.Handle("GET /api/auth/me", protected(s.me))

	mux.Handle("GET /api/providers", protected(s.listProviders))
	mux.Handle("POST /api/voices/preview", protected(s.previewVoice))
	mux.Handle("GET /api/agents", protected(s.listAgents))
	mux.Handle("POST /api/agents", protected(s.createAgent))
	mux.Handle("GET /api/agents/{id}", protected(s.getAgent))
	mux.Handle("PUT /api/agents/{id}", protected(s.updateAgent))
	mux.Handle("POST /api/agents/{id}/publish", protected(s.publishAgent))
	mux.Handle("GET /api/agents/{id}/versions", protected(s.listVersions))
	mux.Handle("POST /api/agents/{id}/initiation-webhook/test", protected(s.testInitiationWebhook))

	// Ferramentas (Tools) da Organization
	mux.Handle("GET /api/tools", protected(s.listTools))
	mux.Handle("POST /api/tools", protected(s.createTool))
	mux.Handle("GET /api/tools/{id}", protected(s.getTool))
	mux.Handle("PUT /api/tools/{id}", protected(s.updateTool))
	mux.Handle("DELETE /api/tools/{id}", protected(s.deleteTool))
	mux.Handle("POST /api/tools/{id}/test", protected(s.testTool))

	// Procedures do Agent (Draft)
	mux.Handle("GET /api/agents/{id}/procedures", protected(s.listProcedures))
	mux.Handle("POST /api/agents/{id}/procedures", protected(s.createProcedure))
	mux.Handle("POST /api/agents/{id}/procedures/import-sop", protected(s.importSOPProcedures))
	mux.Handle("GET /api/agents/{id}/procedures/{proc_id}", protected(s.getProcedure))
	mux.Handle("PUT /api/agents/{id}/procedures/{proc_id}", protected(s.updateProcedure))
	mux.Handle("DELETE /api/agents/{id}/procedures/{proc_id}", protected(s.deleteProcedure))

	// Bases de Conhecimento (Knowledge Bases) da Organization
	mux.Handle("GET /api/knowledge-bases", protected(s.listKnowledgeBases))
	mux.Handle("POST /api/knowledge-bases", protected(s.createKnowledgeBase))
	mux.Handle("GET /api/knowledge-bases/{id}", protected(s.getKnowledgeBase))
	mux.Handle("PUT /api/knowledge-bases/{id}", protected(s.updateKnowledgeBase))
	mux.Handle("DELETE /api/knowledge-bases/{id}", protected(s.deleteKnowledgeBase))
	mux.Handle("POST /api/knowledge-bases/{id}/documents", protected(s.addDocument))
	mux.Handle("DELETE /api/knowledge-bases/{id}/documents/{doc_id}", protected(s.deleteDocument))

	// Dashboard e Histórico de Conversas
	mux.Handle("GET /api/dashboard", protected(s.getDashboard))
	mux.Handle("GET /api/conversations", protected(s.listConversations))
	mux.Handle("GET /api/conversations/{id}", protected(s.getConversation))
	mux.Handle("GET /api/conversations/{id}/audio", protected(s.getConversationAudio))

	// Gerenciamento de Números de Telefone (DIDs da Organization)
	mux.Handle("GET /api/phone-numbers", protected(s.listPhoneNumbers))
	mux.Handle("POST /api/phone-numbers", protected(s.createPhoneNumber))
	mux.Handle("POST /api/phone-numbers/twilio/available", protected(s.listTwilioAvailableNumbers))
	mux.Handle("PUT /api/phone-numbers/{id}", protected(s.updatePhoneNumber))
	mux.Handle("DELETE /api/phone-numbers/{id}", protected(s.deletePhoneNumber))

	// Telefonia SIP (Jambonz Ingress)
	mux.HandleFunc("POST /api/telephony/jambonz/webhook", s.handleJambonzWebhook)
	mux.HandleFunc("POST /api/telephony/jambonz/status", s.handleJambonzStatus)
	if s.Runtime != nil {
		mux.HandleFunc("GET /ws/telephony/jambonz/audio", s.handleJambonzAudioWS)
	}

	// Telefonia Twilio (Twilio Voice & Media Streams Ingress)
	mux.HandleFunc("POST /api/telephony/twilio/webhook", s.handleTwilioWebhook)
	mux.HandleFunc("POST /api/telephony/twilio/status", s.handleTwilioStatus)
	if s.Runtime != nil {
		mux.HandleFunc("GET /ws/telephony/twilio/audio", s.handleTwilioAudioWS)
	}

	// WebSocket de voz do navegador
	if s.Runtime != nil {
		mux.HandleFunc("GET /ws/conversations/{agent_id}", s.Runtime.ServeWS)
	}

	return s.logRequests(mux)
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		s.Log.Info("http_request", "method", r.Method, "path", r.URL.Path,
			"status", rec.status, "duration_ms", time.Since(start).Milliseconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Unwrap permite que http.ResponseController (e upgrades de WebSocket) alcancem o writer original.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	s.Log.Error("internal_error", "path", r.URL.Path, "error", err)
	writeError(w, http.StatusInternalServerError, "erro interno")
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "JSON invÃ¡lido: "+err.Error())
		return false
	}
	return true
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func isUUID(s string) bool { return uuidRe.MatchString(s) }

// notFoundOr responde 404 para ErrNotFound e 500 para o resto.
func (s *Server) notFoundOr(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "nÃ£o encontrado")
		return
	}
	s.internalError(w, r, err)
}
