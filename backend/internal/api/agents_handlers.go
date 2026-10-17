package api

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"voiceagent/internal/agents"
	"voiceagent/internal/auth"
	"voiceagent/internal/store"
	"voiceagent/internal/variables"
	"voiceagent/internal/voice"
)

var voicePreviewCache sync.Map

func (s *Server) listProviders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Voices.Catalogs())
}

func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListAgents(r.Context(), auth.FromContext(r.Context()).OrganizationID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createAgent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if !decode(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || utf8.RuneCountInString(req.Name) > 100 {
		writeError(w, http.StatusBadRequest, "nome é obrigatório (máx. 100 caracteres)")
		return
	}
	// Padrões: primeiro provider registrado, com seus modelo e voz padrão.
	var provider, model, voiceID string
	if cats := s.Voices.Catalogs(); len(cats) > 0 {
		provider, model, voiceID = cats[0].Provider, defaultOf(cats[0].Models), defaultOf(cats[0].Voices)
	}
	a, err := s.Store.CreateAgent(r.Context(), auth.FromContext(r.Context()).OrganizationID, req.Name, req.Description, provider, model, voiceID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func defaultOf(opts []voice.Option) string {
	for _, o := range opts {
		if o.Default {
			return o.ID
		}
	}
	if len(opts) > 0 {
		return opts[0].ID
	}
	return ""
}

// agentDetail é o Agent (Draft) + o estado de publicação.
type agentDetail struct {
	store.Agent
	LatestVersion         *int     `json:"latest_version"`
	HasUnpublishedChanges bool     `json:"has_unpublished_changes"`
	PublishErrors         []string `json:"publish_errors"`
}

// loadAgent resolve {id} dentro da Organization do User; responde 404 se não achar.
func (s *Server) loadAgent(w http.ResponseWriter, r *http.Request) (store.Agent, bool) {
	id := r.PathValue("id")
	if !isUUID(id) {
		writeError(w, http.StatusNotFound, "não encontrado")
		return store.Agent{}, false
	}
	a, err := s.Store.GetAgent(r.Context(), auth.FromContext(r.Context()).OrganizationID, id)
	if err != nil {
		s.notFoundOr(w, r, err)
		return a, false
	}
	return a, true
}

func (s *Server) detail(r *http.Request, a store.Agent) (agentDetail, error) {
	orgID := auth.FromContext(r.Context()).OrganizationID
	d := agentDetail{Agent: a, PublishErrors: []string{}}
	cfg, err := s.Store.DraftConfig(r.Context(), orgID, a)
	if err != nil {
		return d, err
	}
	if errs := agents.ValidatePublish(cfg, s.Voices); errs != nil {
		d.PublishErrors = errs
	}
	latest, err := s.Store.LatestVersion(r.Context(), orgID, a.ID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		d.HasUnpublishedChanges = true
	case err != nil:
		return d, err
	default:
		d.LatestVersion = &latest.Version
		draft, _ := json.Marshal(cfg)
		published, _ := json.Marshal(latest.Snapshot)
		d.HasUnpublishedChanges = !bytes.Equal(draft, published)
	}
	return d, nil
}

func (s *Server) getAgent(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadAgent(w, r)
	if !ok {
		return
	}
	d, err := s.detail(r, a)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) updateAgent(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadAgent(w, r)
	if !ok {
		return
	}
	var u store.AgentUpdate
	if !decode(w, r, &u) {
		return
	}
	if errs := agents.ValidateDraft(&u, s.Voices); len(errs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "dados inválidos", "errors": errs})
		return
	}
	updated, err := s.Store.UpdateAgent(r.Context(), a.OrganizationID, a.ID, u)
	if err != nil {
		s.notFoundOr(w, r, err)
		return
	}
	d, err := s.detail(r, updated)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) publishAgent(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadAgent(w, r)
	if !ok {
		return
	}
	sess := auth.FromContext(r.Context())
	cfg, err := s.Store.DraftConfig(r.Context(), sess.OrganizationID, a)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if errs := agents.ValidatePublish(cfg, s.Voices); len(errs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "o Draft não pode ser publicado", "errors": errs})
		return
	}
	v, err := s.Store.Publish(r.Context(), sess.OrganizationID, a.ID, sess.UserID, cfg)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.Log.Info("agent_published", "agent_id", a.ID, "version", v.Version, "user_id", sess.UserID)
	writeJSON(w, http.StatusCreated, map[string]any{"version": v.Version, "created_at": v.CreatedAt})
}

func (s *Server) listVersions(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadAgent(w, r)
	if !ok {
		return
	}
	list, err := s.Store.ListVersions(r.Context(), a.OrganizationID, a.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// testInitiationWebhook chama o Initiation Webhook do Draft como no início de uma Conversation (padrão ElevenLabs Twilio Personalization).
func (s *Server) testInitiationWebhook(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadAgent(w, r)
	if !ok {
		return
	}
	var req struct {
		Caller       string         `json:"caller"`
		CallerID     string         `json:"caller_id"`
		CalledNumber string         `json:"called_number"`
		CallSid      string         `json:"call_sid"`
		Metadata     map[string]any `json:"metadata"`
	}
	if !decode(w, r, &req) {
		return
	}
	callerID := req.CallerID
	if callerID == "" {
		callerID = req.Caller
	}
	if req.CalledNumber == "" {
		req.CalledNumber = "+551140041234"
	}
	if req.CallSid == "" {
		req.CallSid = "CA_test_twilio_personalization"
	}

	cfg, err := s.Store.DraftConfig(r.Context(), a.OrganizationID, a)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if cfg.InitiationWebhookURL == "" {
		writeError(w, http.StatusUnprocessableEntity, "nenhum Initiation Webhook configurado")
		return
	}
	start := time.Now()
	initRes, err := variables.FetchInitiationData(
		r.Context(),
		s.HTTPClient,
		cfg.InitiationWebhookURL,
		time.Duration(cfg.InitiationWebhookTimeout)*time.Millisecond,
		variables.InitiationRequest{
			CallerID:       callerID,
			AgentID:        a.ID,
			CalledNumber:   req.CalledNumber,
			CallSid:        req.CallSid,
			ConversationID: "conv_test_initiation",
			Caller:         callerID,
			Metadata:       req.Metadata,
		},
		cfg.Variables,
	)
	ms := time.Since(start).Milliseconds()
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error(), "duration_ms": ms})
		return
	}
	if len(initRes.MissingVariables) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":                        "Variables declaradas não devolvidas: " + strings.Join(initRes.MissingVariables, ", "),
			"dynamic_variables":            initRes.DynamicVariables,
			"conversation_config_override": initRes.ConversationConfigOverride,
			"missing_variables":            initRes.MissingVariables,
			"duration_ms":                  ms,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"type":                         initRes.Type,
		"dynamic_variables":            initRes.DynamicVariables,
		"conversation_config_override": initRes.ConversationConfigOverride,
		"duration_ms":                  ms,
	})
}

func (s *Server) previewVoice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
		Voice    string `json:"voice"`
		Language string `json:"language"`
	}
	if !decode(w, r, &req) {
		return
	}
	req.Voice = strings.TrimSpace(req.Voice)
	if req.Voice == "" {
		writeError(w, http.StatusBadRequest, "voice é obrigatório")
		return
	}
	if req.Language == "" {
		req.Language = "pt-BR"
	}

	var prov voice.Provider
	var err error
	if req.Provider != "" {
		prov, err = s.Voices.Get(req.Provider)
	}
	if prov == nil || err != nil {
		cats := s.Voices.Catalogs()
		if len(cats) == 0 {
			writeError(w, http.StatusServiceUnavailable, "nenhum provedor de voz disponível")
			return
		}
		prov, err = s.Voices.Get(cats[0].Provider)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
	}

	cat := prov.Catalog()
	cacheKey := fmt.Sprintf("%s:%s:%s", cat.Provider, req.Voice, req.Language)
	if cached, ok := voicePreviewCache.Load(cacheKey); ok {
		wavBytes := cached.([]byte)
		w.Header().Set("Content-Type", "audio/wav")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(wavBytes)
		return
	}

	modelID := defaultOf(cat.Models)
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	sess, err := prov.Connect(ctx, voice.SessionConfig{
		Model:        modelID,
		Voice:        req.Voice,
		Language:     req.Language,
		SystemPrompt: "Você é uma assistente de voz brasileira demonstrando sua voz. Fale em Português do Brasil (pt-BR) com ritmo natural, claro e cordial.",
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "falha ao conectar provedor para amostra: "+err.Error())
		return
	}
	defer sess.Close()

	prompt := fmt.Sprintf("Diga apenas esta frase curta em português do Brasil: \"Olá! Eu sou a voz %s, pronta para atender seus clientes com clareza e naturalidade.\"", req.Voice)
	if err := sess.SendText(prompt); err != nil {
		writeError(w, http.StatusBadGateway, "falha ao solicitar amostra de voz: "+err.Error())
		return
	}

	var pcmBuf bytes.Buffer
	for {
		select {
		case ev, ok := <-sess.Events():
			if !ok {
				goto done
			}
			if ev.Type == voice.EventAudio && len(ev.Audio) > 0 {
				pcmBuf.Write(ev.Audio)
			} else if ev.Type == voice.EventTurnComplete || ev.Type == voice.EventClosed {
				goto done
			}
		case <-ctx.Done():
			goto done
		}
	}

done:
	if pcmBuf.Len() == 0 {
		writeError(w, http.StatusGatewayTimeout, "nenhum áudio retornado para a amostra")
		return
	}

	sampleRate := cat.OutputSampleRate
	if sampleRate <= 0 {
		sampleRate = 24000
	}
	wavBytes := buildPCM16WAV(pcmBuf.Bytes(), sampleRate)
	voicePreviewCache.Store(cacheKey, wavBytes)

	w.Header().Set("Content-Type", "audio/wav")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(wavBytes)
}

func buildPCM16WAV(pcm []byte, sampleRate int) []byte {
	dataLen := uint32(len(pcm))
	totalLen := 36 + dataLen
	buf := bytes.NewBuffer(make([]byte, 0, 44+len(pcm)))
	buf.WriteString("RIFF")
	_ = binary.Write(buf, binary.LittleEndian, totalLen)
	buf.WriteString("WAVEfmt ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1)) // PCM
	_ = binary.Write(buf, binary.LittleEndian, uint16(1)) // Mono
	_ = binary.Write(buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(buf, binary.LittleEndian, uint32(sampleRate*2))
	_ = binary.Write(buf, binary.LittleEndian, uint16(2))
	_ = binary.Write(buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	_ = binary.Write(buf, binary.LittleEndian, dataLen)
	buf.Write(pcm)
	return buf.Bytes()
}
