package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"voiceagent/internal/auth"
	"voiceagent/internal/store"
	"voiceagent/internal/telephony"
)

type createPhoneNumberRequest struct {
	E164                string `json:"e164"`
	Label               string `json:"label"`
	AgentID             string `json:"agent_id"`
	Direction           string `json:"direction"`
	Provider            string `json:"provider"`
	TwilioAccountSid    string `json:"twilio_account_sid"`
	TwilioAuthToken     string `json:"twilio_auth_token"`
	TwilioPhoneSid      string `json:"twilio_phone_sid"`
	ConfigureWebhookURL string `json:"configure_webhook_url"`
}

type updatePhoneNumberRequest struct {
	Label            string `json:"label"`
	AgentID          string `json:"agent_id"`
	Direction        string `json:"direction"`
	Provider         string `json:"provider"`
	TwilioAccountSid string `json:"twilio_account_sid"`
	TwilioAuthToken  string `json:"twilio_auth_token"`
	TwilioPhoneSid   string `json:"twilio_phone_sid"`
	Active           bool   `json:"active"`
}

type twilioAvailableRequest struct {
	AccountSid string `json:"account_sid"`
	AuthToken  string `json:"auth_token"`
}

func (s *Server) listPhoneNumbers(w http.ResponseWriter, r *http.Request) {
	orgID := auth.FromContext(r.Context()).OrganizationID
	list, err := s.Store.ListPhoneNumbers(r.Context(), orgID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if list == nil {
		list = []store.PhoneNumberWithAgent{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) listTwilioAvailableNumbers(w http.ResponseWriter, r *http.Request) {
	var req twilioAvailableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if strings.TrimSpace(req.AccountSid) == "" || strings.TrimSpace(req.AuthToken) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Account SID e Auth Token da Twilio são obrigatórios"})
		return
	}

	numbers, err := telephony.ListTwilioIncomingNumbers(r.Context(), s.HTTPClient, req.AccountSid, req.AuthToken)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, numbers)
}

func (s *Server) createPhoneNumber(w http.ResponseWriter, r *http.Request) {
	orgID := auth.FromContext(r.Context()).OrganizationID
	var req createPhoneNumberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}

	req.E164 = strings.TrimSpace(req.E164)
	if req.E164 == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "e164 é obrigatório"})
		return
	}
	if req.AgentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_id é obrigatório"})
		return
	}

	// Verifica se o agente pertence à organização
	if _, err := s.Store.GetAgent(r.Context(), orgID, req.AgentID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agente não encontrado"})
		return
	}

	// Se o usuário pediu para configurar automaticamente o VoiceUrl na Twilio
	if req.Provider == "twilio" && req.ConfigureWebhookURL != "" && req.TwilioAccountSid != "" && req.TwilioAuthToken != "" && req.TwilioPhoneSid != "" {
		if err := telephony.ConfigureTwilioNumberWebhook(
			r.Context(),
			s.HTTPClient,
			req.TwilioAccountSid,
			req.TwilioAuthToken,
			req.TwilioPhoneSid,
			req.ConfigureWebhookURL,
		); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}

	created, err := s.Store.CreatePhoneNumber(r.Context(), orgID, store.CreatePhoneNumberParams{
		E164:             req.E164,
		Label:            req.Label,
		AgentID:          req.AgentID,
		Direction:        req.Direction,
		Provider:         req.Provider,
		TwilioAccountSid: req.TwilioAccountSid,
		TwilioAuthToken:  req.TwilioAuthToken,
		TwilioPhoneSid:   req.TwilioPhoneSid,
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) updatePhoneNumber(w http.ResponseWriter, r *http.Request) {
	orgID := auth.FromContext(r.Context()).OrganizationID
	id := r.PathValue("id")

	var req updatePhoneNumberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}

	if req.AgentID != "" {
		if _, err := s.Store.GetAgent(r.Context(), orgID, req.AgentID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agente não encontrado"})
			return
		}
	}

	updated, err := s.Store.UpdatePhoneNumber(
		r.Context(), orgID, id,
		req.Label, req.AgentID, req.Direction, req.Provider,
		req.TwilioAccountSid, req.TwilioAuthToken, req.TwilioPhoneSid,
		req.Active,
	)
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deletePhoneNumber(w http.ResponseWriter, r *http.Request) {
	orgID := auth.FromContext(r.Context()).OrganizationID
	id := r.PathValue("id")

	if err := s.Store.DeletePhoneNumber(r.Context(), orgID, id); err != nil {
		s.internalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
