package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/coder/websocket"
	"voiceagent/internal/runtime"
	"voiceagent/internal/store"
	"voiceagent/internal/telephony"
)

func (s *Server) resolveBaseWSURL(r *http.Request) string {
	baseWS := s.PublicWSURL
	if baseWS == "" {
		host := r.Host
		scheme := "ws"
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "wss"
		}
		baseWS = fmt.Sprintf("%s://%s", scheme, host)
	}
	return strings.TrimSuffix(baseWS, "/")
}

func (s *Server) handleJambonzWebhook(w http.ResponseWriter, r *http.Request) {
	var req telephony.JambonzWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.Log.Warn("jambonz_webhook_invalid_json", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}

	s.Log.Info("jambonz_inbound_call",
		"call_sid", req.CallSid,
		"from", req.From,
		"to", req.To,
		"originating_ip", req.OriginatingSIPIP)

	// Busca o agente ativo configurado para este DID telefônico (E.164)
	phone, agent, err := s.Store.FindAgentByPhoneNumber(r.Context(), req.To)
	if err != nil {
		s.Log.Warn("jambonz_did_not_found", "to", req.To, "error", err)
		rejectApp := telephony.BuildRejectApp("O número chamado não está associado a nenhum atendente de voz ativo no momento.")
		writeJSON(w, http.StatusOK, rejectApp)
		return
	}

	if cfg, _, err := s.resolveAgentConfig(r.Context(), phone.OrganizationID, agent); err == nil {
		s.Runtime.PrefetchInitiation(req.CallSid, agent.ID, req.From, req.To, cfg)
	}

	baseWS := s.resolveBaseWSURL(r)

	audioWSURL := fmt.Sprintf("%s/ws/telephony/jambonz/audio?call_sid=%s&did=%s&from=%s&agent_id=%s&org_id=%s",
		baseWS,
		url.QueryEscape(req.CallSid),
		url.QueryEscape(req.To),
		url.QueryEscape(req.From),
		phone.AgentID,
		phone.OrganizationID,
	)

	listenApp := telephony.BuildListenApp(audioWSURL)
	s.Log.Info("jambonz_routing_call", "agent", agent.Name, "ws_url", audioWSURL)
	writeJSON(w, http.StatusOK, listenApp)
}

func (s *Server) handleJambonzStatus(w http.ResponseWriter, r *http.Request) {
	var payload map[string]any
	_ = json.NewDecoder(r.Body).Decode(&payload)
	s.Log.Debug("jambonz_call_status", "payload", payload)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleJambonzAudioWS(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	callSid := q.Get("call_sid")
	did := q.Get("did")
	from := q.Get("from")
	agentID := q.Get("agent_id")
	orgID := q.Get("org_id")

	if agentID == "" || orgID == "" {
		http.Error(w, "missing agent_id or org_id", http.StatusBadRequest)
		return
	}

	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		s.Log.Error("jambonz_ws_accept_error", "error", err)
		return
	}
	defer ws.Close(websocket.StatusInternalError, "fechando conexão")
	ws.SetReadLimit(10 * 1024 * 1024)

	ctx := r.Context()

	ag, err := s.Store.GetAgent(ctx, orgID, agentID)
	if err != nil {
		s.Log.Error("jambonz_agent_not_found", "agent_id", agentID, "error", err)
		_ = ws.Close(websocket.StatusPolicyViolation, "agent not found")
		return
	}

	cfg, versionID, err := s.resolveAgentConfig(ctx, orgID, ag)
	if err != nil {
		s.Log.Error("jambonz_resolve_config_error", "agent_id", agentID, "error", err)
		_ = ws.Close(websocket.StatusInternalError, "config error")
		return
	}

	prov, err := s.Voices.Get(cfg.Provider)
	if err != nil {
		s.Log.Error("jambonz_provider_error", "provider", cfg.Provider, "error", err)
		_ = ws.Close(websocket.StatusInternalError, "provider error")
		return
	}

	cat := prov.Catalog()
	tr := telephony.NewJambonzTransport(ws, cat.InputSampleRate, cat.OutputSampleRate, callSid, s.JambonzAccountSid, s.JambonzAPIURL, s.JambonzAPIKey)

	opts := runtime.SessionOptions{
		OrgID:      orgID,
		AgentID:    agentID,
		VersionID:  versionID,
		IsTest:     false,
		Caller:     from,
		Direction:  "inbound",
		FromNumber: from,
		ToNumber:   did,
		CallSid:    callSid,
		Transport:  tr,
	}

	s.Log.Info("jambonz_session_starting", "call_sid", callSid, "from", from, "to", did, "agent", ag.Name)
	s.Runtime.RunSessionWithOptions(ctx, opts, cfg, prov)
}

// ============================================================================
// Twilio Voice & Media Streams Ingress
// ============================================================================

func (s *Server) handleTwilioWebhook(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	callSid := r.FormValue("CallSid")
	from := r.FormValue("From")
	to := r.FormValue("To")

	// Suporte também a payload JSON caso testado manualmente via curl/insomnia
	if to == "" && strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var body struct {
			CallSid string `json:"CallSid"`
			From    string `json:"From"`
			To      string `json:"To"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			callSid = body.CallSid
			from = body.From
			to = body.To
		}
	}

	s.Log.Info("twilio_inbound_call", "call_sid", callSid, "from", from, "to", to)

	phone, agent, err := s.Store.FindAgentByPhoneNumber(r.Context(), to)
	if err != nil {
		s.Log.Warn("twilio_did_not_found", "to", to, "error", err)
		twiml := telephony.BuildTwilioRejectTwiML("O número chamado não está associado a nenhum atendente de voz ativo no momento.")
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(twiml))
		return
	}

	// Dispara em paralelo o Initiation Webhook durante o ringing/conexão da Twilio (padrão ElevenLabs Twilio Personalization)
	if cfg, _, err := s.resolveAgentConfig(r.Context(), phone.OrganizationID, agent); err == nil {
		s.Runtime.PrefetchInitiation(callSid, agent.ID, from, to, cfg)
	}

	baseWS := s.resolveBaseWSURL(r)
	audioWSURL := fmt.Sprintf("%s/ws/telephony/twilio/audio", baseWS)

	twiml := telephony.BuildTwilioStreamTwiML(audioWSURL, map[string]string{
		"call_sid": callSid,
		"did":      to,
		"from":     from,
		"agent_id": phone.AgentID,
		"org_id":   phone.OrganizationID,
		"phone_id": phone.ID,
	})

	s.Log.Info("twilio_routing_call", "agent", agent.Name, "ws_url", audioWSURL)
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(twiml))
}

func (s *Server) handleTwilioStatus(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	s.Log.Debug("twilio_call_status", "call_sid", r.FormValue("CallSid"), "status", r.FormValue("CallStatus"))
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleTwilioAudioWS(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		s.Log.Error("twilio_ws_accept_error", "error", err)
		return
	}
	defer ws.Close(websocket.StatusInternalError, "fechando conexão")
	ws.SetReadLimit(10 * 1024 * 1024)

	ctx := r.Context()
	q := r.URL.Query()
	callSid := q.Get("call_sid")
	did := q.Get("did")
	from := q.Get("from")
	agentID := q.Get("agent_id")
	orgID := q.Get("org_id")
	phoneID := q.Get("phone_id")
	streamSid := ""

	// A Twilio envia os parâmetros customizados de <Stream><Parameter .../></Stream>
	// dentro da mensagem JSON "start" logo após abrir o WebSocket.
	if agentID == "" || orgID == "" {
		hsStreamSid, hsCallSid, customParams, err := telephony.ReadTwilioHandshake(ctx, ws)
		if err != nil {
			s.Log.Error("twilio_ws_handshake_error", "error", err)
			_ = ws.Close(websocket.StatusPolicyViolation, "invalid handshake")
			return
		}
		streamSid = hsStreamSid
		if callSid == "" {
			callSid = hsCallSid
		}
		if v := customParams["call_sid"]; v != "" && callSid == "" {
			callSid = v
		}
		if v := customParams["did"]; v != "" {
			did = v
		}
		if v := customParams["from"]; v != "" {
			from = v
		}
		if v := customParams["agent_id"]; v != "" {
			agentID = v
		}
		if v := customParams["org_id"]; v != "" {
			orgID = v
		}
		if v := customParams["phone_id"]; v != "" {
			phoneID = v
		}
	}

	if agentID == "" || orgID == "" {
		s.Log.Error("twilio_ws_missing_params", "agent_id", agentID, "org_id", orgID)
		_ = ws.Close(websocket.StatusPolicyViolation, "missing agent_id or org_id")
		return
	}

	var accountSid, authToken string
	if phoneID != "" {
		if p, err := s.Store.GetPhoneNumber(ctx, orgID, phoneID); err == nil {
			accountSid = p.TwilioAccountSid
			authToken = p.TwilioAuthToken
		}
	} else if did != "" {
		if p, _, err := s.Store.FindAgentByPhoneNumber(ctx, did); err == nil {
			accountSid = p.TwilioAccountSid
			authToken = p.TwilioAuthToken
		}
	}

	ag, err := s.Store.GetAgent(ctx, orgID, agentID)
	if err != nil {
		s.Log.Error("twilio_agent_not_found", "agent_id", agentID, "error", err)
		_ = ws.Close(websocket.StatusPolicyViolation, "agent not found")
		return
	}

	cfg, versionID, err := s.resolveAgentConfig(ctx, orgID, ag)
	if err != nil {
		s.Log.Error("twilio_resolve_config_error", "agent_id", agentID, "error", err)
		_ = ws.Close(websocket.StatusInternalError, "config error")
		return
	}

	prov, err := s.Voices.Get(cfg.Provider)
	if err != nil {
		s.Log.Error("twilio_provider_error", "provider", cfg.Provider, "error", err)
		_ = ws.Close(websocket.StatusInternalError, "provider error")
		return
	}

	cat := prov.Catalog()
	tr := telephony.NewTwilioTransport(ws, cat.InputSampleRate, cat.OutputSampleRate, streamSid, callSid, accountSid, authToken)

	opts := runtime.SessionOptions{
		OrgID:      orgID,
		AgentID:    agentID,
		VersionID:  versionID,
		IsTest:     false,
		Caller:     from,
		Direction:  "inbound",
		FromNumber: from,
		ToNumber:   did,
		CallSid:    callSid,
		Transport:  tr,
	}

	s.Log.Info("twilio_session_starting", "call_sid", callSid, "stream_sid", streamSid, "from", from, "to", did, "agent", ag.Name)
	s.Runtime.RunSessionWithOptions(ctx, opts, cfg, prov)
}

func (s *Server) resolveAgentConfig(ctx context.Context, orgID string, ag store.Agent) (store.AgentConfig, *string, error) {
	v, err := s.Store.LatestVersion(ctx, orgID, ag.ID)
	if err == nil {
		return v.Snapshot, &v.ID, nil
	}
	// Fallback para o Draft
	cfg, err := s.Store.DraftConfig(ctx, orgID, ag)
	return cfg, nil, err
}
