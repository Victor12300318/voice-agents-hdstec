package api

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"voiceagent/internal/auth"
)

type kbPayload struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type docPayload struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

func (s *Server) listKnowledgeBases(w http.ResponseWriter, r *http.Request) {
	orgID := auth.FromContext(r.Context()).OrganizationID
	list, err := s.Store.ListKnowledgeBases(r.Context(), orgID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	var req kbPayload
	if !decode(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || utf8.RuneCountInString(req.Name) > 100 {
		writeError(w, http.StatusBadRequest, "nome da base de conhecimento é obrigatório (máx. 100 caracteres)")
		return
	}
	orgID := auth.FromContext(r.Context()).OrganizationID
	kb, err := s.Store.CreateKnowledgeBase(r.Context(), orgID, req.Name, req.Description)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, kb)
}

func (s *Server) getKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isUUID(id) {
		writeError(w, http.StatusNotFound, "não encontrado")
		return
	}
	orgID := auth.FromContext(r.Context()).OrganizationID
	kb, err := s.Store.GetKnowledgeBase(r.Context(), orgID, id)
	if err != nil {
		s.notFoundOr(w, r, err)
		return
	}
	docs, err := s.Store.ListDocuments(r.Context(), orgID, id)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"knowledge_base": kb,
		"documents":      docs,
	})
}

func (s *Server) updateKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isUUID(id) {
		writeError(w, http.StatusNotFound, "não encontrado")
		return
	}
	var req kbPayload
	if !decode(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || utf8.RuneCountInString(req.Name) > 100 {
		writeError(w, http.StatusBadRequest, "nome é obrigatório")
		return
	}
	orgID := auth.FromContext(r.Context()).OrganizationID
	updated, err := s.Store.UpdateKnowledgeBase(r.Context(), orgID, id, req.Name, req.Description)
	if err != nil {
		s.notFoundOr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isUUID(id) {
		writeError(w, http.StatusNotFound, "não encontrado")
		return
	}
	orgID := auth.FromContext(r.Context()).OrganizationID
	if err := s.Store.DeleteKnowledgeBase(r.Context(), orgID, id); err != nil {
		s.notFoundOr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) addDocument(w http.ResponseWriter, r *http.Request) {
	kbID := r.PathValue("id")
	if !isUUID(kbID) {
		writeError(w, http.StatusNotFound, "não encontrado")
		return
	}
	var req docPayload
	if !decode(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "nome do documento é obrigatório")
		return
	}
	if len(req.Content) > 200_000 {
		writeError(w, http.StatusBadRequest, "documento excede o limite de tamanho para inclusão inline (máx. 200 KB)")
		return
	}
	orgID := auth.FromContext(r.Context()).OrganizationID
	doc, err := s.Store.CreateDocument(r.Context(), orgID, kbID, req.Name, req.Content)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, doc)
}

func (s *Server) deleteDocument(w http.ResponseWriter, r *http.Request) {
	kbID := r.PathValue("id")
	docID := r.PathValue("doc_id")
	if !isUUID(kbID) || !isUUID(docID) {
		writeError(w, http.StatusNotFound, "não encontrado")
		return
	}
	orgID := auth.FromContext(r.Context()).OrganizationID
	if err := s.Store.DeleteDocument(r.Context(), orgID, kbID, docID); err != nil {
		s.notFoundOr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
