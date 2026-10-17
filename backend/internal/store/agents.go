package store

import (
	"context"
	"encoding/json"
	"time"
)

// Agent representa o Draft de um Agent (colunas editáveis).
type Agent struct {
	ID                        string          `json:"id"`
	OrganizationID            string          `json:"organization_id"`
	Name                      string          `json:"name"`
	Description               string          `json:"description"`
	Active                    bool            `json:"active"`
	Provider                  string          `json:"provider"`
	Model                     string          `json:"model"`
	Voice                     string          `json:"voice"`
	Language                  string          `json:"language"`
	Timezone                  string          `json:"timezone"`
	SystemPrompt              string          `json:"system_prompt"`
	FirstMessage              string          `json:"first_message"`
	FirstMessageInterruptible bool            `json:"first_message_interruptible"`
	MaxDurationSeconds        int             `json:"max_duration_seconds"`
	RecordingEnabled          bool            `json:"recording_enabled"`
	Variables                 json.RawMessage `json:"variables"`
	InitiationWebhookURL      string          `json:"initiation_webhook_url"`
	InitiationWebhookTimeout  int             `json:"initiation_webhook_timeout_ms"`
	SystemTools               json.RawMessage `json:"system_tools"`
	SystemToolsConfig         json.RawMessage `json:"system_tools_config"`
	Tools                     json.RawMessage `json:"tools"`
	VersioningEnabled         bool            `json:"versioning_enabled"`
	CreatedAt                 time.Time       `json:"created_at"`
	UpdatedAt                 time.Time       `json:"updated_at"`
}

const agentCols = `id, organization_id, name, description, active, provider, model, voice, language, timezone,
	system_prompt, first_message, first_message_interruptible, max_duration_seconds, recording_enabled,
	variables, initiation_webhook_url, initiation_webhook_timeout_ms, system_tools, system_tools_config, tools, versioning_enabled, created_at, updated_at`

type scanner interface{ Scan(dest ...any) error }

func scanAgent(r scanner) (Agent, error) {
	var a Agent
	err := r.Scan(&a.ID, &a.OrganizationID, &a.Name, &a.Description, &a.Active, &a.Provider, &a.Model,
		&a.Voice, &a.Language, &a.Timezone, &a.SystemPrompt, &a.FirstMessage, &a.FirstMessageInterruptible,
		&a.MaxDurationSeconds, &a.RecordingEnabled, &a.Variables, &a.InitiationWebhookURL,
		&a.InitiationWebhookTimeout, &a.SystemTools, &a.SystemToolsConfig, &a.Tools, &a.VersioningEnabled, &a.CreatedAt, &a.UpdatedAt)
	return a, err
}

func (s *Store) CreateAgent(ctx context.Context, orgID, name, description, provider, model, voice string) (Agent, error) {
	row := s.Pool.QueryRow(ctx,
		`INSERT INTO agents (organization_id, name, description, provider, model, voice)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+agentCols,
		orgID, name, description, provider, model, voice)
	return scanAgent(row)
}

func (s *Store) ListAgents(ctx context.Context, orgID string) ([]Agent, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+agentCols+` FROM agents WHERE organization_id = $1 ORDER BY name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Agent{}
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GetAgent(ctx context.Context, orgID, id string) (Agent, error) {
	a, err := scanAgent(s.Pool.QueryRow(ctx,
		`SELECT `+agentCols+` FROM agents WHERE organization_id = $1 AND id = $2`, orgID, id))
	return a, notFound(err)
}
