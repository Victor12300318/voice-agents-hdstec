package api

import (
	"context"
	"net/http"
	"strings"
	"unicode/utf8"

	"voiceagent/internal/auth"
	"voiceagent/internal/procedures"
	"voiceagent/internal/store"
)

type procPayload struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"` // free_form | deterministic
	Trigger    string   `json:"trigger"`
	Guardrails []string `json:"guardrails"`
	Content    string   `json:"content"`
	Active     bool     `json:"active"`
}

type ProcedureResponse struct {
	store.Procedure
	procedures.ProcedureMetadata
}

type importSOPRequest struct {
	Document string `json:"document"`
}

func validateProc(p *procPayload) []string {
	var errs []string
	p.Name = strings.TrimSpace(p.Name)
	if p.Type == "" {
		p.Type = "free_form"
	}
	if p.Type != "free_form" && p.Type != "deterministic" {
		errs = append(errs, "tipo de procedure inválido (use free_form ou deterministic)")
	}
	if p.Name == "" || utf8.RuneCountInString(p.Name) > 100 {
		errs = append(errs, "nome do procedimento é obrigatório (máx. 100 caracteres)")
	}
	if len(p.Content) > 50_000 {
		errs = append(errs, "conteúdo do procedimento excede o limite de 50.000 caracteres")
	}
	return errs
}

func (s *Server) enrichProcedures(ctx context.Context, orgID string, list []store.Procedure) []ProcedureResponse {
	allProcs := make(map[string]store.ProcedureSnapshot, len(list))
	for _, p := range list {
		allProcs[p.ID] = store.ProcedureSnapshot{
			ID:         p.ID,
			Name:       p.Name,
			Type:       p.Type,
			Trigger:    p.Trigger,
			Guardrails: p.Guardrails,
			Content:    p.Content,
			Active:     p.Active,
		}
	}

	toolsByID := map[string]store.Tool{}
	if tl, err := s.Store.ListTools(ctx, orgID); err == nil {
		for _, t := range tl {
			toolsByID[t.ID] = t.Tool
		}
	}

	kbsByID := map[string]store.KnowledgeBase{}
	if kbs, err := s.Store.ListKnowledgeBases(ctx, orgID); err == nil {
		for _, kb := range kbs {
			kbsByID[kb.ID] = kb
		}
	}

	out := make([]ProcedureResponse, 0, len(list))
	for _, p := range list {
		meta := procedures.InspectProcedure(p, allProcs, toolsByID, kbsByID)
		out = append(out, ProcedureResponse{
			Procedure:         p,
			ProcedureMetadata: meta,
		})
	}
	return out
}

func (s *Server) listProcedures(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.loadAgent(w, r)
	if !ok {
		return
	}
	orgID := auth.FromContext(r.Context()).OrganizationID
	list, err := s.Store.ListProcedures(r.Context(), orgID, agent.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.enrichProcedures(r.Context(), orgID, list))
}

func (s *Server) createProcedure(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.loadAgent(w, r)
	if !ok {
		return
	}
	var req procPayload
	if !decode(w, r, &req) {
		return
	}
	if errs := validateProc(&req); len(errs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "dados inválidos", "errors": errs})
		return
	}
	orgID := auth.FromContext(r.Context()).OrganizationID
	created, err := s.Store.CreateProcedure(r.Context(), orgID, agent.ID, store.Procedure{
		Name:       req.Name,
		Type:       req.Type,
		Trigger:    req.Trigger,
		Guardrails: req.Guardrails,
		Content:    req.Content,
		Active:     req.Active,
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	all, _ := s.Store.ListProcedures(r.Context(), orgID, agent.ID)
	enriched := s.enrichProcedures(r.Context(), orgID, all)
	for _, item := range enriched {
		if item.ID == created.ID {
			writeJSON(w, http.StatusCreated, item)
			return
		}
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) importSOPProcedures(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.loadAgent(w, r)
	if !ok {
		return
	}
	var req importSOPRequest
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Document) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "documento SOP está vazio"})
		return
	}

	parsed := procedures.ParseSOPDocument(req.Document)
	if len(parsed) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "nenhum procedimento identificado no documento"})
		return
	}

	orgID := auth.FromContext(r.Context()).OrganizationID
	var createdIDs []string
	for _, item := range parsed {
		c, err := s.Store.CreateProcedure(r.Context(), orgID, agent.ID, store.Procedure{
			Name:       item.Name,
			Type:       "free_form",
			Trigger:    item.Trigger,
			Guardrails: []string{},
			Content:    item.Content,
			Active:     true,
		})
		if err == nil {
			createdIDs = append(createdIDs, c.ID)
		}
	}

	all, err := s.Store.ListProcedures(r.Context(), orgID, agent.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"imported_count": len(createdIDs),
		"procedures":     s.enrichProcedures(r.Context(), orgID, all),
	})
}

func (s *Server) getProcedure(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.loadAgent(w, r)
	if !ok {
		return
	}
	procID := r.PathValue("proc_id")
	if !isUUID(procID) {
		writeError(w, http.StatusNotFound, "não encontrado")
		return
	}
	orgID := auth.FromContext(r.Context()).OrganizationID
	p, err := s.Store.GetProcedure(r.Context(), orgID, agent.ID, procID)
	if err != nil {
		s.notFoundOr(w, r, err)
		return
	}
	all, _ := s.Store.ListProcedures(r.Context(), orgID, agent.ID)
	enriched := s.enrichProcedures(r.Context(), orgID, all)
	for _, item := range enriched {
		if item.ID == p.ID {
			writeJSON(w, http.StatusOK, item)
			return
		}
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) updateProcedure(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.loadAgent(w, r)
	if !ok {
		return
	}
	procID := r.PathValue("proc_id")
	if !isUUID(procID) {
		writeError(w, http.StatusNotFound, "não encontrado")
		return
	}
	var req procPayload
	if !decode(w, r, &req) {
		return
	}
	if errs := validateProc(&req); len(errs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "dados inválidos", "errors": errs})
		return
	}
	orgID := auth.FromContext(r.Context()).OrganizationID
	updated, err := s.Store.UpdateProcedure(r.Context(), orgID, agent.ID, procID, store.Procedure{
		Name:       req.Name,
		Type:       req.Type,
		Trigger:    req.Trigger,
		Guardrails: req.Guardrails,
		Content:    req.Content,
		Active:     req.Active,
	})
	if err != nil {
		s.notFoundOr(w, r, err)
		return
	}
	all, _ := s.Store.ListProcedures(r.Context(), orgID, agent.ID)
	enriched := s.enrichProcedures(r.Context(), orgID, all)
	for _, item := range enriched {
		if item.ID == updated.ID {
			writeJSON(w, http.StatusOK, item)
			return
		}
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteProcedure(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.loadAgent(w, r)
	if !ok {
		return
	}
	procID := r.PathValue("proc_id")
	if !isUUID(procID) {
		writeError(w, http.StatusNotFound, "não encontrado")
		return
	}
	orgID := auth.FromContext(r.Context()).OrganizationID
	if err := s.Store.DeleteProcedure(r.Context(), orgID, agent.ID, procID); err != nil {
		s.notFoundOr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
