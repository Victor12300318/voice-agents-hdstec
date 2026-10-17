package api

import (
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"voiceagent/internal/auth"
	"voiceagent/internal/store"
	"voiceagent/internal/tools"
	"voiceagent/internal/voice"
)

type toolPayload struct {
	Name          string              `json:"name"`
	Description   string              `json:"description"`
	Kind          string              `json:"kind"`
	Type          string              `json:"type"` // Alias ElevenLabs ("webhook" | "client" | "mcp" | "code")
	InputSchema   []voice.Param       `json:"input_schema"`
	Config        store.WebhookConfig `json:"config"`
	SecretHeaders map[string]string   `json:"secret_headers"`
	TimeoutMS     int                 `json:"timeout_ms"`
	Active        *bool               `json:"active"`

	// Campos de nível superior no formato JSON nativo da ElevenLabs
	APISchema                     *store.ToolAPISchema        `json:"api_schema,omitempty"`
	ResponseTimeoutSecs           int                         `json:"response_timeout_secs,omitempty"`
	FollowRedirects               *bool                       `json:"follow_redirects,omitempty"`
	FollowRedirectsAllowedDomains []string                    `json:"follow_redirects_allowed_domains,omitempty"`
	DynamicVariables              *store.ToolDynamicVariables `json:"dynamic_variables,omitempty"`
	Assignments                   []store.ToolAssignment      `json:"assignments,omitempty"`
	InterruptionMode              string                      `json:"interruption_mode,omitempty"`
	PreToolSpeech                 string                      `json:"pre_tool_speech,omitempty"`
	ToolCallSound                 *string                     `json:"tool_call_sound,omitempty"`
	ToolCallSoundBehavior         string                      `json:"tool_call_sound_behavior,omitempty"`
	ExecutionMode                 string                      `json:"execution_mode,omitempty"`
	ToolErrorHandlingMode         string                      `json:"tool_error_handling_mode,omitempty"`
	ResponseMocks                 []store.ToolResponseMock    `json:"response_mocks,omitempty"`
	ExpectsResponse               *bool                       `json:"expects_response,omitempty"`
	Code                          string                      `json:"code,omitempty"`
	ContextObject                 *store.ToolCodeContext      `json:"context_object,omitempty"`
}

func normalizeToolPayload(p *toolPayload) {
	if p.Kind == "" && p.Type != "" {
		p.Kind = p.Type
	}
	if p.Kind == "" {
		p.Kind = "webhook"
	}

	// Mescla campos de nível superior do JSON ElevenLabs dentro de p.Config
	if p.APISchema != nil && p.Config.APISchema == nil {
		p.Config.APISchema = p.APISchema
	}
	if p.ResponseTimeoutSecs > 0 && p.Config.ResponseTimeoutSecs == 0 {
		p.Config.ResponseTimeoutSecs = p.ResponseTimeoutSecs
	}
	if p.FollowRedirects != nil {
		p.Config.FollowRedirects = *p.FollowRedirects
	}
	if len(p.FollowRedirectsAllowedDomains) > 0 && len(p.Config.FollowRedirectsAllowedDomains) == 0 {
		p.Config.FollowRedirectsAllowedDomains = p.FollowRedirectsAllowedDomains
	}
	if p.DynamicVariables != nil && p.Config.DynamicVariables == nil {
		p.Config.DynamicVariables = p.DynamicVariables
	}
	if len(p.Assignments) > 0 && len(p.Config.Assignments) == 0 {
		p.Config.Assignments = p.Assignments
	}
	if p.InterruptionMode != "" && p.Config.InterruptionMode == "" {
		p.Config.InterruptionMode = p.InterruptionMode
	}
	if p.PreToolSpeech != "" && p.Config.PreToolSpeech == "" {
		p.Config.PreToolSpeech = p.PreToolSpeech
	}
	if p.ToolCallSound != nil && p.Config.ToolCallSound == nil {
		p.Config.ToolCallSound = p.ToolCallSound
	}
	if p.ToolCallSoundBehavior != "" && p.Config.ToolCallSoundBehavior == "" {
		p.Config.ToolCallSoundBehavior = p.ToolCallSoundBehavior
	}
	if p.ExecutionMode != "" && p.Config.ExecutionMode == "" {
		p.Config.ExecutionMode = p.ExecutionMode
	}
	if p.ToolErrorHandlingMode != "" && p.Config.ToolErrorHandlingMode == "" {
		p.Config.ToolErrorHandlingMode = p.ToolErrorHandlingMode
	}
	if len(p.ResponseMocks) > 0 && len(p.Config.ResponseMocks) == 0 {
		p.Config.ResponseMocks = p.ResponseMocks
	}
	if p.ExpectsResponse != nil {
		p.Config.ExpectsResponse = *p.ExpectsResponse
	}
	if p.Code != "" && p.Config.Code == "" {
		p.Config.Code = p.Code
	}
	if p.ContextObject != nil && p.Config.ContextObject == nil {
		p.Config.ContextObject = p.ContextObject
	}

	// Sincroniza URL e Method entre Config e Config.APISchema
	if p.Config.APISchema != nil {
		if p.Config.URL == "" && p.Config.APISchema.URL != "" {
			p.Config.URL = strings.TrimSpace(p.Config.APISchema.URL)
		} else if p.Config.APISchema.URL == "" && p.Config.URL != "" {
			p.Config.APISchema.URL = strings.TrimSpace(p.Config.URL)
		}
		if p.Config.Method == "" && p.Config.APISchema.Method != "" {
			p.Config.Method = strings.ToUpper(strings.TrimSpace(p.Config.APISchema.Method))
		} else if p.Config.APISchema.Method == "" && p.Config.Method != "" {
			p.Config.APISchema.Method = strings.ToUpper(strings.TrimSpace(p.Config.Method))
		}
		if p.Config.APISchema.ContentType == "" {
			p.Config.APISchema.ContentType = "application/json"
		}
	}

	if p.Config.Method == "" {
		if p.Kind == "webhook" && p.Config.APISchema != nil && p.Config.APISchema.Method != "" {
			p.Config.Method = p.Config.APISchema.Method
		} else {
			p.Config.Method = "POST"
		}
	}

	// Sincroniza timeout_ms e response_timeout_secs
	if p.Config.ResponseTimeoutSecs > 0 && p.TimeoutMS == 0 {
		p.TimeoutMS = p.Config.ResponseTimeoutSecs * 1000
	} else if p.TimeoutMS > 0 && p.Config.ResponseTimeoutSecs == 0 {
		p.Config.ResponseTimeoutSecs = p.TimeoutMS / 1000
	}
	if p.TimeoutMS == 0 {
		p.TimeoutMS = 20000
		p.Config.ResponseTimeoutSecs = 20
	}

	// Sincroniza InputSchema para o LLM a partir de path_params_schema, query_params_schema e request_body_schema
	if p.Config.APISchema != nil && len(p.InputSchema) == 0 {
		seen := map[string]bool{}
		addParam := func(ps store.ToolParamSchema) {
			name := strings.TrimSpace(ps.Name)
			if name == "" || seen[name] {
				return
			}
			// Apenas parâmetros coletados pelo LLM (ou sem value_type explícito) vão para a assinatura de função do LLM
			if ps.ValueType != "" && ps.ValueType != "llm_prompt" {
				return
			}
			seen[name] = true
			t := ps.Type
			if t == "" {
				t = "string"
			}
			p.InputSchema = append(p.InputSchema, voice.Param{
				Name:        name,
				Type:        t,
				Description: ps.Description,
				Required:    ps.Required,
			})
		}
		for _, pp := range p.Config.APISchema.PathParamsSchema {
			pp.Required = true
			addParam(pp)
		}
		for _, qp := range p.Config.APISchema.QueryParamsSchema {
			addParam(qp)
		}
		if p.Config.APISchema.RequestBodySchema != nil {
			for _, bp := range p.Config.APISchema.RequestBodySchema.Properties {
				addParam(bp)
			}
		}
	}
}

func validateTool(p *toolPayload) []string {
	normalizeToolPayload(p)

	var errs []string
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || utf8.RuneCountInString(p.Name) > 64 {
		errs = append(errs, "nome é obrigatório (máx. 64 caracteres)")
	}
	if p.Kind != "webhook" && p.Kind != "mcp" && p.Kind != "client" && p.Kind != "code" {
		errs = append(errs, "tipo de ferramenta inválido (use: webhook, mcp, client ou code)")
	}

	if p.Kind == "webhook" || p.Kind == "mcp" {
		if p.Config.URL == "" {
			errs = append(errs, "URL é obrigatória para ferramentas webhook e integração MCP")
		} else {
			// Permite placeholders {{var}} e {param} na URL durante a validação
			cleanURL := strings.ReplaceAll(p.Config.URL, "{{", "")
			cleanURL = strings.ReplaceAll(cleanURL, "}}", "")
			cleanURL = strings.ReplaceAll(cleanURL, "{", "")
			cleanURL = strings.ReplaceAll(cleanURL, "}", "")
			if u, err := url.Parse(cleanURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				errs = append(errs, "URL deve começar com http:// ou https://")
			}
		}
	}

	if p.TimeoutMS < 500 || p.TimeoutMS > 60000 {
		errs = append(errs, "timeout deve estar entre 500 e 60000 ms (1 a 60s)")
	}

	seen := map[string]bool{}
	for _, param := range p.InputSchema {
		name := strings.TrimSpace(param.Name)
		if name == "" {
			errs = append(errs, "parâmetro com nome vazio")
		} else if seen[name] {
			errs = append(errs, "parâmetro duplicado: "+name)
		}
		seen[name] = true
	}
	return errs
}

func (s *Server) listTools(w http.ResponseWriter, r *http.Request) {
	orgID := auth.FromContext(r.Context()).OrganizationID
	list, err := s.Store.ListTools(r.Context(), orgID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createTool(w http.ResponseWriter, r *http.Request) {
	var req toolPayload
	if !decode(w, r, &req) {
		return
	}
	if errs := validateTool(&req); len(errs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "dados inválidos", "errors": errs})
		return
	}
	orgID := auth.FromContext(r.Context()).OrganizationID
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	created, err := s.Store.CreateTool(r.Context(), orgID, store.Tool{
		Name:          req.Name,
		Description:   req.Description,
		Kind:          req.Kind,
		InputSchema:   req.InputSchema,
		Config:        req.Config,
		SecretHeaders: req.SecretHeaders,
		TimeoutMS:     req.TimeoutMS,
		Active:        active,
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	created.SecretHeaders = nil
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) getTool(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isUUID(id) {
		writeError(w, http.StatusNotFound, "não encontrado")
		return
	}
	orgID := auth.FromContext(r.Context()).OrganizationID
	t, err := s.Store.GetTool(r.Context(), orgID, id)
	if err != nil {
		s.notFoundOr(w, r, err)
		return
	}
	t.SecretHeaders = nil
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) updateTool(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isUUID(id) {
		writeError(w, http.StatusNotFound, "não encontrado")
		return
	}
	var req toolPayload
	if !decode(w, r, &req) {
		return
	}
	if errs := validateTool(&req); len(errs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "dados inválidos", "errors": errs})
		return
	}
	orgID := auth.FromContext(r.Context()).OrganizationID
	updateSecrets := len(req.SecretHeaders) > 0
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	updated, err := s.Store.UpdateTool(r.Context(), orgID, id, store.Tool{
		Name:          req.Name,
		Description:   req.Description,
		Kind:          req.Kind,
		InputSchema:   req.InputSchema,
		Config:        req.Config,
		SecretHeaders: req.SecretHeaders,
		TimeoutMS:     req.TimeoutMS,
		Active:        active,
	}, updateSecrets)
	if err != nil {
		s.notFoundOr(w, r, err)
		return
	}
	updated.SecretHeaders = nil
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteTool(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isUUID(id) {
		writeError(w, http.StatusNotFound, "não encontrado")
		return
	}
	orgID := auth.FromContext(r.Context()).OrganizationID
	if err := s.Store.DeleteTool(r.Context(), orgID, id); err != nil {
		s.notFoundOr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) testTool(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isUUID(id) {
		writeError(w, http.StatusNotFound, "não encontrado")
		return
	}
	var req struct {
		Args             map[string]any    `json:"args"`
		Arguments        map[string]any    `json:"arguments"`
		DynamicVariables map[string]string `json:"dynamic_variables"`
	}
	if !decode(w, r, &req) {
		return
	}
	callArgs := req.Args
	if len(callArgs) == 0 && len(req.Arguments) > 0 {
		callArgs = req.Arguments
	}
	orgID := auth.FromContext(r.Context()).OrganizationID
	t, err := s.Store.GetTool(r.Context(), orgID, id)
	if err != nil {
		s.notFoundOr(w, r, err)
		return
	}
	executor := tools.NewExecutor(s.HTTPClient)
	res := executor.ExecuteWithVars(r.Context(), t, callArgs, req.DynamicVariables)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":        res.Status,
		"normalized":    res.Result,
		"raw_response":  res.RawResponse,
		"duration_ms":   res.DurationMS,
		"error":         res.ErrorMessage,
		"assigned_vars": res.AssignedVars,
		"logs":          res.Logs,
	})
}
