package api

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"

	"voiceagent/internal/auth"
	"voiceagent/internal/store"
)

type DashboardMetrics struct {
	TotalAgents         int                    `json:"total_agents"`
	ActiveConversations int                    `json:"active_conversations"`
	TotalConversations  int                    `json:"total_conversations"`
	FailedConversations int                    `json:"failed_conversations"`
	AvgDurationMS       int64                  `json:"avg_duration_ms"`
	AvgLatencyMS        int64                  `json:"avg_latency_ms"`
	TotalCostUSD        float64                `json:"total_cost_usd"`
	TotalCostBRL        float64                `json:"total_cost_brl"`
	RecentConversations []ConversationListItem `json:"recent_conversations"`
}

type ConversationListItem struct {
	store.Conversation
	AgentName string `json:"agent_name"`
}

func (s *Server) getDashboard(w http.ResponseWriter, r *http.Request) {
	orgID := auth.FromContext(r.Context()).OrganizationID
	ctx := r.Context()

	var metrics DashboardMetrics
	// 1. Total de agentes
	_ = s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM agents WHERE organization_id = $1`, orgID).
		Scan(&metrics.TotalAgents)

	// 2. Métricas de conversas da organização
	_ = s.Store.Pool.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE status IN ('active', 'starting') AND started_at > now() - interval '2 hours'),
		       count(*) FILTER (WHERE status = 'failed'),
		       coalesce(round(avg(duration_ms))::bigint, 0),
		       coalesce(sum(cost_usd), 0),
		       coalesce(sum(cost_brl), 0)
		FROM conversations
		WHERE organization_id = $1`, orgID).
		Scan(&metrics.TotalConversations, &metrics.ActiveConversations, &metrics.FailedConversations, &metrics.AvgDurationMS, &metrics.TotalCostUSD, &metrics.TotalCostBRL)

	// 3. Média de Response Latency (sem chamadas de tool para não misturar lentidão externa)
	_ = s.Store.Pool.QueryRow(ctx, `
		SELECT coalesce(round(avg((payload->>'latency_ms')::numeric))::bigint, 0)
		FROM conversation_events
		WHERE organization_id = $1
		  AND type = 'response_latency'
		  AND (payload->>'had_tool_call')::boolean = false`, orgID).
		Scan(&metrics.AvgLatencyMS)

	// 4. Últimas conversas
	list, _ := s.queryConversations(ctx, orgID, true, 20)
	metrics.RecentConversations = list

	writeJSON(w, http.StatusOK, metrics)
}

func (s *Server) listConversations(w http.ResponseWriter, r *http.Request) {
	orgID := auth.FromContext(r.Context()).OrganizationID
	includeTest := strings.EqualFold(r.URL.Query().Get("include_test"), "true")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	list, err := s.queryConversations(r.Context(), orgID, includeTest, limit)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) queryConversations(ctx context.Context, orgID string, includeTest bool, limit int) ([]ConversationListItem, error) {
	query := `
		SELECT c.id, c.organization_id, c.agent_id, c.agent_version_id, c.is_test, c.session_id, c.provider, c.model,
		       c.status, c.ended_reason, c.started_at, c.ended_at, c.duration_ms, c.config_snapshot, c.variables, c.metadata, c.recording_key,
		       c.cost_usd, c.cost_brl, c.tokens_usage,
		       coalesce(a.name, 'Agente Excluído') as agent_name
		FROM conversations c
		LEFT JOIN agents a ON a.id = c.agent_id
		WHERE c.organization_id = $1 `
	if !includeTest {
		query += `AND c.is_test = false `
	}
	query += `ORDER BY c.started_at DESC LIMIT $2`

	rows, err := s.Store.Pool.Query(ctx, query, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ConversationListItem{}
	for rows.Next() {
		var item ConversationListItem
		err := rows.Scan(&item.ID, &item.OrganizationID, &item.AgentID, &item.AgentVersionID, &item.IsTest,
			&item.SessionID, &item.Provider, &item.Model, &item.Status, &item.EndedReason,
			&item.StartedAt, &item.EndedAt, &item.DurationMs, &item.ConfigSnapshot, &item.Variables,
			&item.Metadata, &item.RecordingKey,
			&item.CostUSD, &item.CostBRL, &item.TokensUsage,
			&item.AgentName)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Server) getConversation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isUUID(id) {
		writeError(w, http.StatusNotFound, "não encontrado")
		return
	}
	orgID := auth.FromContext(r.Context()).OrganizationID
	conv, events, err := s.Store.GetConversation(r.Context(), orgID, id)
	if err != nil {
		s.notFoundOr(w, r, err)
		return
	}

	execs, _ := s.Store.ListToolExecutions(r.Context(), orgID, id)
	agent, _ := s.Store.GetAgent(r.Context(), orgID, conv.AgentID)

	writeJSON(w, http.StatusOK, map[string]any{
		"conversation":    conv,
		"agent":           agent,
		"events":          events,
		"tool_executions": execs,
	})
}

func (s *Server) getConversationAudio(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isUUID(id) {
		writeError(w, http.StatusNotFound, "não encontrado")
		return
	}
	orgID := auth.FromContext(r.Context()).OrganizationID
	conv, _, err := s.Store.GetConversation(r.Context(), orgID, id)
	if err != nil {
		s.notFoundOr(w, r, err)
		return
	}

	if conv.RecordingKey == "" || s.Storage == nil {
		writeError(w, http.StatusNotFound, "gravação não encontrada para esta conversa")
		return
	}

	reader, size, err := s.Storage.GetRecording(r.Context(), conv.RecordingKey)
	if err != nil {
		s.Log.Warn("get_recording_error", "key", conv.RecordingKey, "error", err)
		writeError(w, http.StatusNotFound, "arquivo de áudio não encontrado no armazenamento")
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.Header().Set("Accept-Ranges", "bytes")
	_, _ = io.Copy(w, reader)
}
