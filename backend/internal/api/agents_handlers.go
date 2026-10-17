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
		Name             string `json:"name"`
		Description      string `json:"description"`
		EnableVersioning bool   `json:"enable_versioning"`
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
	sess := auth.FromContext(r.Context())
	a, err := s.Store.CreateAgent(r.Context(), sess.OrganizationID, req.Name, req.Description, provider, model, voiceID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if req.EnableVersioning {
		if _, _, err := s.Store.EnableVersioning(r.Context(), sess.OrganizationID, a.ID, sess.UserID); err == nil {
			a.VersioningEnabled = true
		}
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

// agentDetail é o Agent (Draft) + o estado de publicação e controle de versão por branches.
type agentDetail struct {
	store.Agent
	LatestVersion              *int                 `json:"latest_version"`
	LatestVersionID            *string              `json:"latest_version_id,omitempty"`
	HasUnpublishedChanges      bool                 `json:"has_unpublished_changes"`
	PublishErrors              []string             `json:"publish_errors"`
	CurrentBranch              *store.AgentBranch   `json:"current_branch,omitempty"`
	Branches                   []store.AgentBranch  `json:"branches,omitempty"`
	ViewingVersionID           *string              `json:"viewing_version_id,omitempty"`
	IsReadOnlyPerAgentSettings bool                 `json:"is_read_only_per_agent_settings"`
	ActiveProceduresSnapshot   []store.ProcedureSnapshot `json:"active_procedures_snapshot,omitempty"`
}

func applySnapshotToAgent(a *store.Agent, snap store.AgentConfig) {
	a.Provider = snap.Provider
	a.Model = snap.Model
	a.Voice = snap.Voice
	a.Language = snap.Language
	a.Timezone = snap.Timezone
	a.SystemPrompt = snap.SystemPrompt
	a.FirstMessage = snap.FirstMessage
	a.FirstMessageInterruptible = snap.FirstMessageInterruptible
	a.MaxDurationSeconds = snap.MaxDurationSeconds
	a.RecordingEnabled = snap.RecordingEnabled
	a.InitiationWebhookURL = snap.InitiationWebhookURL
	a.InitiationWebhookTimeout = snap.InitiationWebhookTimeout
	if b, err := json.Marshal(snap.Variables); err == nil {
		a.Variables = b
	}
	if b, err := json.Marshal(snap.SystemTools); err == nil {
		a.SystemTools = b
	}
	if b, err := json.Marshal(snap.SystemToolsConfig); err == nil {
		a.SystemToolsConfig = b
	}
	if b, err := json.Marshal(snap.Tools); err == nil {
		a.Tools = b
	}
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
	sess := auth.FromContext(r.Context())
	orgID := sess.OrganizationID
	d := agentDetail{Agent: a, PublishErrors: []string{}}

	q := r.URL.Query()
	versionRef := strings.TrimSpace(q.Get("version_id"))
	branchRef := strings.TrimSpace(q.Get("branch_id"))
	includeDraft := !strings.EqualFold(q.Get("include_draft"), "false")

	cfg, err := s.Store.DraftConfig(r.Context(), orgID, a)
	if err != nil {
		return d, err
	}

	if a.VersioningEnabled {
		branches, _ := s.Store.ListBranches(r.Context(), orgID, a.ID, sess.UserID, true)
		d.Branches = branches

		var selectedBranch *store.AgentBranch
		for i := range branches {
			if branchRef != "" && (branches[i].ID == branchRef || strings.EqualFold(branches[i].Name, branchRef)) {
				selectedBranch = &branches[i]
				break
			}
			if branchRef == "" && branches[i].IsMain {
				selectedBranch = &branches[i]
			}
		}
		if selectedBranch == nil && len(branches) > 0 {
			selectedBranch = &branches[0]
		}
		d.CurrentBranch = selectedBranch
		if selectedBranch != nil && !selectedBranch.IsMain {
			d.IsReadOnlyPerAgentSettings = true
		}

		if versionRef != "" {
			if ver, err := s.Store.GetVersionByRef(r.Context(), orgID, a.ID, versionRef); err == nil {
				cfg = ver.Snapshot
				applySnapshotToAgent(&d.Agent, cfg)
				d.ViewingVersionID = &ver.VersionCode
				d.ActiveProceduresSnapshot = cfg.Procedures
			}
		} else if selectedBranch != nil {
			var headSnap *store.AgentConfig
			if selectedBranch.HeadVersionID != nil {
				if hv, err := s.Store.GetVersionByRef(r.Context(), orgID, a.ID, *selectedBranch.HeadVersionID); err == nil {
					headSnap = &hv.Snapshot
					d.LatestVersion = &hv.Version
					d.LatestVersionID = &hv.VersionCode
				}
			}
			if includeDraft {
				if draftCfg, _, err := s.Store.GetBranchDraft(r.Context(), orgID, a.ID, selectedBranch.ID, sess.UserID); err == nil && draftCfg != nil {
					cfg = *draftCfg
					applySnapshotToAgent(&d.Agent, cfg)
					d.ActiveProceduresSnapshot = cfg.Procedures
				} else if headSnap != nil && !selectedBranch.IsMain {
					cfg = *headSnap
					applySnapshotToAgent(&d.Agent, cfg)
					d.ActiveProceduresSnapshot = cfg.Procedures
				}
			} else if headSnap != nil {
				cfg = *headSnap
				applySnapshotToAgent(&d.Agent, cfg)
				d.ActiveProceduresSnapshot = cfg.Procedures
			}

			if headSnap != nil {
				draftBytes, _ := json.Marshal(cfg)
				pubBytes, _ := json.Marshal(*headSnap)
				d.HasUnpublishedChanges = !bytes.Equal(draftBytes, pubBytes)
			} else {
				d.HasUnpublishedChanges = true
			}
		}
	} else {
		if versionRef != "" {
			if ver, err := s.Store.GetVersionByRef(r.Context(), orgID, a.ID, versionRef); err == nil {
				cfg = ver.Snapshot
				applySnapshotToAgent(&d.Agent, cfg)
				d.ViewingVersionID = &ver.VersionCode
				d.ActiveProceduresSnapshot = cfg.Procedures
			}
		}
		latest, err := s.Store.LatestVersion(r.Context(), orgID, a.ID)
		switch {
		case errors.Is(err, store.ErrNotFound):
			d.HasUnpublishedChanges = true
		case err != nil:
			return d, err
		default:
			d.LatestVersion = &latest.Version
			d.LatestVersionID = &latest.VersionCode
			draft, _ := json.Marshal(cfg)
			published, _ := json.Marshal(latest.Snapshot)
			d.HasUnpublishedChanges = !bytes.Equal(draft, published)
		}
	}

	if errs := agents.ValidatePublish(cfg, s.Voices); errs != nil {
		d.PublishErrors = errs
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
	sess := auth.FromContext(r.Context())
	branchRef := strings.TrimSpace(r.URL.Query().Get("branch_id"))

	if u.EnableVersioningIfNotEnabled && !a.VersioningEnabled {
		if _, _, err := s.Store.EnableVersioning(r.Context(), a.OrganizationID, a.ID, sess.UserID); err == nil {
			a.VersioningEnabled = true
		}
	}

	if a.VersioningEnabled && branchRef != "" {
		b, err := s.Store.GetBranch(r.Context(), a.OrganizationID, a.ID, branchRef, sess.UserID)
		if err != nil {
			s.notFoundOr(w, r, err)
			return
		}
		if b.Archived {
			writeError(w, http.StatusUnprocessableEntity, "não é possível editar uma branch arquivada")
			return
		}
		// Regra ElevenLabs: per-agent settings (name, description, active) só podem ser alteradas na Main.
		if !b.IsMain {
			u.Name = a.Name
			u.Description = a.Description
			u.Active = a.Active
		} else {
			updated, err := s.Store.UpdateAgent(r.Context(), a.OrganizationID, a.ID, u)
			if err != nil {
				s.notFoundOr(w, r, err)
				return
			}
			a = updated
		}

		// Preserva procedures atuais da branch/draft no snapshot salvo
		baseCfg, _ := s.Store.DraftConfig(r.Context(), a.OrganizationID, a)
		if existingDraft, _, err := s.Store.GetBranchDraft(r.Context(), a.OrganizationID, a.ID, b.ID, sess.UserID); err == nil && existingDraft != nil {
			baseCfg.Procedures = existingDraft.Procedures
		} else if b.HeadVersionID != nil {
			if hv, err := s.Store.GetVersionByRef(r.Context(), a.OrganizationID, a.ID, *b.HeadVersionID); err == nil {
				baseCfg.Procedures = hv.Snapshot.Procedures
			}
		}

		draftSnap := store.AgentConfig{
			Name:                      u.Name,
			Provider:                  u.Provider,
			Model:                     u.Model,
			Voice:                     u.Voice,
			Language:                  u.Language,
			Timezone:                  u.Timezone,
			SystemPrompt:              u.SystemPrompt,
			FirstMessage:              u.FirstMessage,
			FirstMessageInterruptible: u.FirstMessageInterruptible,
			MaxDurationSeconds:        u.MaxDurationSeconds,
			RecordingEnabled:          u.RecordingEnabled,
			Variables:                 u.Variables,
			InitiationWebhookURL:      u.InitiationWebhookURL,
			InitiationWebhookTimeout:  u.InitiationWebhookTimeout,
			SystemTools:               u.SystemTools,
			SystemToolsConfig:         u.SystemToolsConfig,
			Tools:                     u.Tools,
			Procedures:                baseCfg.Procedures,
		}
		if err := s.Store.SaveBranchDraft(r.Context(), a.OrganizationID, a.ID, b.ID, sess.UserID, draftSnap); err != nil {
			s.internalError(w, r, err)
			return
		}
		d, err := s.detail(r, a)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, d)
		return
	}

	updated, err := s.Store.UpdateAgent(r.Context(), a.OrganizationID, a.ID, u)
	if err != nil {
		s.notFoundOr(w, r, err)
		return
	}
	if updated.VersioningEnabled {
		if cfg, err := s.Store.DraftConfig(r.Context(), updated.OrganizationID, updated); err == nil {
			_ = s.Store.SaveBranchDraft(r.Context(), updated.OrganizationID, updated.ID, "Main", sess.UserID, cfg)
		}
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
	var req struct {
		BranchID      string `json:"branch_id"`
		CommitMessage string `json:"commit_message"`
	}
	if r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if req.BranchID == "" {
		req.BranchID = strings.TrimSpace(r.URL.Query().Get("branch_id"))
	}
	if strings.TrimSpace(req.CommitMessage) == "" {
		req.CommitMessage = "Nova versão publicada"
	}

	sess := auth.FromContext(r.Context())
	cfg, err := s.Store.DraftConfig(r.Context(), sess.OrganizationID, a)
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	if a.VersioningEnabled {
		targetBranch := req.BranchID
		if targetBranch == "" {
			targetBranch = "Main"
		}
		if draftCfg, _, err := s.Store.GetBranchDraft(r.Context(), sess.OrganizationID, a.ID, targetBranch, sess.UserID); err == nil && draftCfg != nil {
			cfg = *draftCfg
		} else if b, err := s.Store.GetBranch(r.Context(), sess.OrganizationID, a.ID, targetBranch, sess.UserID); err == nil && !b.IsMain && b.HeadVersionID != nil {
			if hv, err := s.Store.GetVersionByRef(r.Context(), sess.OrganizationID, a.ID, *b.HeadVersionID); err == nil {
				cfg = hv.Snapshot
			}
		}
	}

	if errs := agents.ValidatePublish(cfg, s.Voices); len(errs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "o Draft não pode ser publicado", "errors": errs})
		return
	}
	v, err := s.Store.PublishToBranch(r.Context(), sess.OrganizationID, a.ID, sess.UserID, req.BranchID, req.CommitMessage, cfg)
	if err != nil {
		if errors.Is(err, store.ErrBranchArchived) {
			writeError(w, http.StatusUnprocessableEntity, "não é possível publicar em uma branch arquivada")
			return
		}
		s.internalError(w, r, err)
		return
	}
	s.Log.Info("agent_published", "agent_id", a.ID, "version", v.Version, "version_id", v.VersionCode, "branch_id", v.BranchID, "user_id", sess.UserID)
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":             v.ID,
		"version_id":     v.VersionCode,
		"version":        v.Version,
		"branch_id":      v.BranchID,
		"commit_message": v.CommitMessage,
		"created_at":     v.CreatedAt,
	})
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
	branchFilter := strings.TrimSpace(r.URL.Query().Get("branch_id"))
	if branchFilter != "" {
		filtered := make([]store.VersionSummary, 0, len(list))
		for _, v := range list {
			if (v.BranchID != nil && *v.BranchID == branchFilter) || strings.EqualFold(v.BranchName, branchFilter) {
				filtered = append(filtered, v)
			}
		}
		list = filtered
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) enableVersioning(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadAgent(w, r)
	if !ok {
		return
	}
	sess := auth.FromContext(r.Context())
	branch, ver, err := s.Store.EnableVersioning(r.Context(), a.OrganizationID, a.ID, sess.UserID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"versioning_enabled": true,
		"main_branch":        branch,
		"initial_version":    ver,
	})
}

func (s *Server) duplicateAgent(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadAgent(w, r)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	sess := auth.FromContext(r.Context())
	dup, err := s.Store.DuplicateAgent(r.Context(), a.OrganizationID, a.ID, sess.UserID, req.Name)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, dup)
}

func (s *Server) listBranches(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadAgent(w, r)
	if !ok {
		return
	}
	sess := auth.FromContext(r.Context())
	includeArchived := !strings.EqualFold(r.URL.Query().Get("include_archived"), "false")
	branches, err := s.Store.ListBranches(r.Context(), a.OrganizationID, a.ID, sess.UserID, includeArchived)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"branches": branches,
	})
}

func (s *Server) getBranch(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadAgent(w, r)
	if !ok {
		return
	}
	branchID := r.PathValue("branch_id")
	sess := auth.FromContext(r.Context())
	b, err := s.Store.GetBranch(r.Context(), a.OrganizationID, a.ID, branchID, sess.UserID)
	if err != nil {
		s.notFoundOr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) createBranch(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadAgent(w, r)
	if !ok {
		return
	}
	var req struct {
		ParentVersionID string             `json:"parent_version_id"`
		Name            string             `json:"name"`
		Description     string             `json:"description"`
		Changes         *store.AgentConfig `json:"changes"`
	}
	if !decode(w, r, &req) {
		return
	}
	sess := auth.FromContext(r.Context())
	b, ver, err := s.Store.CreateBranch(r.Context(), a.OrganizationID, a.ID, sess.UserID, req.ParentVersionID, req.Name, req.Description, req.Changes)
	if err != nil {
		if errors.Is(err, store.ErrInvalidBranchName) {
			writeError(w, http.StatusBadRequest, "Nome de branch inválido. Use apenas letras, números e () [] {} - / . (máx. 140 caracteres).")
			return
		}
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "versão base não encontrada")
			return
		}
		writeError(w, http.StatusConflict, "Não foi possível criar a branch (verifique se já existe uma branch com este nome).")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"created_branch_id":  b.ID,
		"created_version_id": ver.VersionCode,
		"branch":             b,
		"version":            ver,
	})
}

func (s *Server) updateBranch(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadAgent(w, r)
	if !ok {
		return
	}
	branchID := r.PathValue("branch_id")
	var req struct {
		Description *string `json:"description"`
		Archived    *bool   `json:"archived"`
	}
	if !decode(w, r, &req) {
		return
	}
	sess := auth.FromContext(r.Context())
	b, err := s.Store.UpdateBranch(r.Context(), a.OrganizationID, a.ID, branchID, sess.UserID, req.Description, req.Archived)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrCannotArchiveMain):
			writeError(w, http.StatusBadRequest, "cannot_archive_main: a branch Main não pode ser arquivada")
		case errors.Is(err, store.ErrCannotArchiveTraffic):
			writeError(w, http.StatusConflict, "cannot_archive_branch_with_traffic: remova todo o tráfego alocado antes de arquivar esta branch")
		default:
			s.notFoundOr(w, r, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) deleteBranchDraft(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadAgent(w, r)
	if !ok {
		return
	}
	branchID := r.PathValue("branch_id")
	sess := auth.FromContext(r.Context())
	if err := s.Store.DeleteBranchDraft(r.Context(), a.OrganizationID, a.ID, branchID, sess.UserID); err != nil {
		s.notFoundOr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"discarded": true})
}

func (s *Server) deployTraffic(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadAgent(w, r)
	if !ok {
		return
	}
	var req struct {
		Deployments []store.TrafficAllocation `json:"deployments"`
	}
	if !decode(w, r, &req) {
		return
	}
	sess := auth.FromContext(r.Context())
	branches, err := s.Store.DeployTraffic(r.Context(), a.OrganizationID, a.ID, sess.UserID, req.Deployments)
	if err != nil {
		if errors.Is(err, store.ErrInvalidTrafficSplit) {
			writeError(w, http.StatusUnprocessableEntity, "deployments_must_sum_to_100: a soma dos percentuais de tráfego deve ser exatamente 100%")
			return
		}
		if errors.Is(err, store.ErrBranchArchived) {
			writeError(w, http.StatusUnprocessableEntity, "não é possível alocar tráfego para uma branch arquivada")
			return
		}
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"branches": branches,
	})
}

func (s *Server) previewMergeBranch(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadAgent(w, r)
	if !ok {
		return
	}
	sourceBranchID := r.PathValue("branch_id")
	var req struct {
		TargetBranchID string `json:"target_branch_id"`
		Force          bool   `json:"force"`
	}
	if r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	sess := auth.FromContext(r.Context())
	preview, _, err := s.Store.PreviewMerge(r.Context(), a.OrganizationID, a.ID, sess.UserID, sourceBranchID, req.TargetBranchID, req.Force)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNoNewChangesToMerge):
			writeError(w, http.StatusConflict, "no_new_changes_to_merge: a branch de origem não possui novas alterações em relação ao destino")
		case errors.Is(err, store.ErrBranchAlreadyMerged):
			writeError(w, http.StatusConflict, "branch_already_merged: esta versão da branch já foi mesclada no destino")
		default:
			s.notFoundOr(w, r, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) mergeBranch(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadAgent(w, r)
	if !ok {
		return
	}
	sourceBranchID := r.PathValue("branch_id")
	req := struct {
		TargetBranchID      string `json:"target_branch_id"`
		ArchiveSourceBranch *bool  `json:"archive_source_branch"`
		Force               bool   `json:"force"`
	}{
		TargetBranchID: "Main",
	}
	if r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	archiveSrc := true
	if req.ArchiveSourceBranch != nil {
		archiveSrc = *req.ArchiveSourceBranch
	}
	sess := auth.FromContext(r.Context())
	res, err := s.Store.MergeBranch(r.Context(), a.OrganizationID, a.ID, sess.UserID, sourceBranchID, req.TargetBranchID, archiveSrc, req.Force)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNoNewChangesToMerge):
			writeError(w, http.StatusConflict, "no_new_changes_to_merge: a branch de origem não possui novas alterações em relação ao destino")
		case errors.Is(err, store.ErrBranchAlreadyMerged):
			writeError(w, http.StatusConflict, "branch_already_merged: esta versão da branch já foi mesclada no destino")
		default:
			s.notFoundOr(w, r, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) previewRebaseBranch(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadAgent(w, r)
	if !ok {
		return
	}
	branchID := r.PathValue("branch_id")
	sess := auth.FromContext(r.Context())
	preview, _, _, err := s.Store.PreviewRebase(r.Context(), a.OrganizationID, a.ID, sess.UserID, branchID)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrCannotRebaseMain):
			writeError(w, http.StatusBadRequest, "cannot_rebase_main: apenas branches secundárias podem sofrer rebase a partir da Main")
		case errors.Is(err, store.ErrBranchAlreadyUpToDate):
			writeError(w, http.StatusConflict, "branch_already_up_to_date: esta branch já contém todas as alterações da Main")
		default:
			s.notFoundOr(w, r, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) rebaseBranch(w http.ResponseWriter, r *http.Request) {
	a, ok := s.loadAgent(w, r)
	if !ok {
		return
	}
	branchID := r.PathValue("branch_id")
	sess := auth.FromContext(r.Context())
	ver, err := s.Store.RebaseBranch(r.Context(), a.OrganizationID, a.ID, sess.UserID, branchID)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrCannotRebaseMain):
			writeError(w, http.StatusBadRequest, "cannot_rebase_main: apenas branches secundárias podem sofrer rebase a partir da Main")
		case errors.Is(err, store.ErrBranchAlreadyUpToDate):
			writeError(w, http.StatusConflict, "branch_already_up_to_date: esta branch já contém todas as alterações da Main")
		default:
			s.notFoundOr(w, r, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"rebased_version": ver,
	})
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
