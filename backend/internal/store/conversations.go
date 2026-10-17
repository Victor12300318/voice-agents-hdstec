package store

import (
	"context"
	"encoding/json"
	"time"
)

type Conversation struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organization_id"`
	AgentID        string          `json:"agent_id"`
	AgentVersionID *string         `json:"agent_version_id"`
	IsTest         bool            `json:"is_test"`
	SessionID      string          `json:"session_id"`
	Provider       string          `json:"provider"`
	Model          string          `json:"model"`
	Status         string          `json:"status"`       // starting | active | completed | failed
	EndedReason    string          `json:"ended_reason"` // caller_hangup | agent_ended | provider_error | initiation_failed | max_duration | internal_error
	StartedAt      time.Time       `json:"started_at"`
	EndedAt        *time.Time      `json:"ended_at"`
	DurationMs     *int64          `json:"duration_ms"`
	ConfigSnapshot json.RawMessage `json:"config_snapshot"`
	Variables      json.RawMessage `json:"variables"`
	Metadata       json.RawMessage `json:"metadata"`
	RecordingKey   string          `json:"recording_key"`
	CostUSD        float64         `json:"cost_usd"`
	CostBRL        float64         `json:"cost_brl"`
	TokensUsage    json.RawMessage `json:"tokens_usage"`
	Direction      string          `json:"direction"`   // web | inbound | outbound
	FromNumber     string          `json:"from_number"` // telefone do Caller ou vazio para web
	ToNumber       string          `json:"to_number"`   // DID do Agent ou vazio para web
}

type ConversationEvent struct {
	ID             int64           `json:"id"`
	ConversationID string          `json:"conversation_id"`
	Type           string          `json:"type"`
	Payload        json.RawMessage `json:"payload"`
	CreatedAt      time.Time       `json:"created_at"`
}

const convCols = `id, organization_id, agent_id, agent_version_id, is_test, session_id, provider, model,
	status, ended_reason, started_at, ended_at, duration_ms, config_snapshot, variables, metadata, recording_key,
	cost_usd, cost_brl, tokens_usage, direction, from_number, to_number`

func scanConv(r scanner) (Conversation, error) {
	var c Conversation
	err := r.Scan(&c.ID, &c.OrganizationID, &c.AgentID, &c.AgentVersionID, &c.IsTest, &c.SessionID,
		&c.Provider, &c.Model, &c.Status, &c.EndedReason, &c.StartedAt, &c.EndedAt, &c.DurationMs,
		&c.ConfigSnapshot, &c.Variables, &c.Metadata, &c.RecordingKey,
		&c.CostUSD, &c.CostBRL, &c.TokensUsage, &c.Direction, &c.FromNumber, &c.ToNumber)
	return c, err
}

func (s *Store) CreateConversation(
	ctx context.Context,
	orgID, agentID string,
	versionID *string,
	isTest bool,
	provider, model string,
	snapshot AgentConfig,
	vars map[string]string,
	direction, fromNumber, toNumber string,
) (Conversation, error) {
	if direction == "" {
		direction = "web"
	}
	snap, _ := json.Marshal(snapshot)
	rawVars, _ := json.Marshal(vars)
	row := s.Pool.QueryRow(ctx, `
		INSERT INTO conversations (organization_id, agent_id, agent_version_id, is_test, provider, model, status, config_snapshot, variables, direction, from_number, to_number)
		VALUES ($1, $2, $3, $4, $5, $6, 'starting', $7, $8, $9, $10, $11)
		RETURNING `+convCols,
		orgID, agentID, versionID, isTest, provider, model, snap, rawVars, direction, fromNumber, toNumber)
	return scanConv(row)
}

func (s *Store) MarkConversationActive(ctx context.Context, orgID, id string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE conversations SET status = 'active'
		WHERE organization_id = $1 AND id = $2 AND status = 'starting'`, orgID, id)
	return err
}

func (s *Store) EndConversation(ctx context.Context, orgID, id, status, reason string, durationMs int64, costUSD, costBRL float64, tokensUsage any) error {
	rawUsage, err := json.Marshal(tokensUsage)
	if err != nil {
		rawUsage = []byte("{}")
	}
	_, err = s.Pool.Exec(ctx, `
		UPDATE conversations
		SET status = $3, ended_reason = $4, ended_at = now(), duration_ms = $5,
		    cost_usd = $6, cost_brl = $7, tokens_usage = $8
		WHERE organization_id = $1 AND id = $2`,
		orgID, id, status, reason, durationMs, costUSD, costBRL, rawUsage)
	return err
}

func (s *Store) RecordEvent(ctx context.Context, orgID, convID, eventType string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		raw = []byte("{}")
	}
	_, err = s.Pool.Exec(ctx, `
		INSERT INTO conversation_events (organization_id, conversation_id, type, payload)
		VALUES ($1, $2, $3, $4)`, orgID, convID, eventType, raw)
	return err
}

func (s *Store) GetConversation(ctx context.Context, orgID, id string) (Conversation, []ConversationEvent, error) {
	c, err := scanConv(s.Pool.QueryRow(ctx, `SELECT `+convCols+` FROM conversations WHERE organization_id = $1 AND id = $2`, orgID, id))
	if err != nil {
		return c, nil, notFound(err)
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, conversation_id, type, payload, created_at
		FROM conversation_events
		WHERE organization_id = $1 AND conversation_id = $2
		ORDER BY id ASC`, orgID, id)
	if err != nil {
		return c, nil, err
	}
	defer rows.Close()
	events := []ConversationEvent{}
	for rows.Next() {
		var ev ConversationEvent
		if err := rows.Scan(&ev.ID, &ev.ConversationID, &ev.Type, &ev.Payload, &ev.CreatedAt); err != nil {
			return c, nil, err
		}
		events = append(events, ev)
	}
	return c, events, rows.Err()
}

func (s *Store) ListConversations(ctx context.Context, orgID string, includeTest bool, limit int) ([]Conversation, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query := `SELECT ` + convCols + ` FROM conversations WHERE organization_id = $1 `
	if !includeTest {
		query += `AND is_test = false `
	}
	query += `ORDER BY started_at DESC LIMIT $2`
	rows, err := s.Pool.Query(ctx, query, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Conversation{}
	for rows.Next() {
		c, err := scanConv(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
