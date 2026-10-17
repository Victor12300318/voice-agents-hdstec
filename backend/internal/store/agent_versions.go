package store

import (
	"context"
	"encoding/json"
	"time"

	"voiceagent/internal/variables"
)

type AgentTransferRule struct {
	AgentID                            string  `json:"agent_id"`
	AgentName                          string  `json:"agent_name,omitempty"`
	Condition                          string  `json:"condition"`
	DelayMS                            int     `json:"delay_ms"`
	TransferMessage                    *string `json:"transfer_message,omitempty"`
	EnableTransferredAgentFirstMessage bool    `json:"enable_transferred_agent_first_message"`
}

type TransferToAgentToolConfig struct {
	Type        string              `json:"type"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Transfers   []AgentTransferRule `json:"transfers"`
}

type GuardianJevConfig struct {
	Enabled                bool    `json:"enabled"`
	Provider               string  `json:"provider,omitempty"`
	Model                  string  `json:"model,omitempty"`
	APIKey                 string  `json:"api_key,omitempty"`
	CheckVoicemail         bool    `json:"check_voicemail"`
	CheckTalkingToSelf     bool    `json:"check_talking_to_self"`
	CheckHallucination     bool    `json:"check_hallucination"`
	VoicemailThreshold     float64 `json:"voicemail_threshold,omitempty"`
	TalkingToSelfThreshold float64 `json:"talking_to_self_threshold,omitempty"`
	HallucinationThreshold float64 `json:"hallucination_threshold,omitempty"`
}

type SystemToolsConfig struct {
	TransferToAgent *TransferToAgentToolConfig `json:"transfer_to_agent,omitempty"`
	GuardianJev     *GuardianJevConfig         `json:"guardian_jev,omitempty"`
}

// AgentConfig é o que uma Agent Version congela (ADR 0002): tudo que é do Agent,
// incluindo Procedures; Tools e Knowledge Bases ficam de fora.
type AgentConfig struct {
	Name                      string               `json:"name"`
	Provider                  string               `json:"provider"`
	Model                     string               `json:"model"`
	Voice                     string               `json:"voice"`
	Language                  string               `json:"language"`
	Timezone                  string               `json:"timezone"`
	SystemPrompt              string               `json:"system_prompt"`
	FirstMessage              string               `json:"first_message"`
	FirstMessageInterruptible bool                 `json:"first_message_interruptible"`
	MaxDurationSeconds        int                  `json:"max_duration_seconds"`
	RecordingEnabled          bool                 `json:"recording_enabled"`
	Variables                 []variables.Declared `json:"variables"`
	InitiationWebhookURL      string               `json:"initiation_webhook_url"`
	InitiationWebhookTimeout  int                  `json:"initiation_webhook_timeout_ms"`
	SystemTools               []string             `json:"system_tools"`
	SystemToolsConfig         SystemToolsConfig    `json:"system_tools_config"`
	Tools                     []string             `json:"tools"`
	Procedures                []ProcedureSnapshot  `json:"procedures"`
}

type ProcedureSnapshot struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Trigger    string   `json:"trigger"`
	Guardrails []string `json:"guardrails"`
	Content    string   `json:"content"`
	Active     bool     `json:"active"`
}

type AgentVersion struct {
	ID          string      `json:"id"`
	AgentID     string      `json:"agent_id"`
	Version     int         `json:"version"`
	Snapshot    AgentConfig `json:"snapshot"`
	PublishedBy *string     `json:"published_by"`
	CreatedAt   time.Time   `json:"created_at"`
}

// AgentUpdate são os campos editáveis do Draft.
type AgentUpdate struct {
	Name                      string               `json:"name"`
	Description               string               `json:"description"`
	Active                    bool                 `json:"active"`
	Provider                  string               `json:"provider"`
	Model                     string               `json:"model"`
	Voice                     string               `json:"voice"`
	Language                  string               `json:"language"`
	Timezone                  string               `json:"timezone"`
	SystemPrompt              string               `json:"system_prompt"`
	FirstMessage              string               `json:"first_message"`
	FirstMessageInterruptible bool                 `json:"first_message_interruptible"`
	MaxDurationSeconds        int                  `json:"max_duration_seconds"`
	RecordingEnabled          bool                 `json:"recording_enabled"`
	Variables                 []variables.Declared `json:"variables"`
	InitiationWebhookURL      string               `json:"initiation_webhook_url"`
	InitiationWebhookTimeout  int                  `json:"initiation_webhook_timeout_ms"`
	SystemTools               []string             `json:"system_tools"`
	SystemToolsConfig         SystemToolsConfig    `json:"system_tools_config"`
	Tools                     []string             `json:"tools"`
}

func mergeReferencedVariables(declared []variables.Declared, texts ...string) []variables.Declared {
	out := make([]variables.Declared, 0, len(declared))
	seen := make(map[string]bool, len(declared))
	for _, d := range declared {
		if d.Name != "" && !seen[d.Name] {
			seen[d.Name] = true
			out = append(out, d)
		}
	}
	for _, name := range variables.References(texts...) {
		if variables.ValidName(name) && !variables.IsSystem(name) && !seen[name] {
			seen[name] = true
			out = append(out, variables.Declared{
				Name:        name,
				Description: "Variável dinâmica extraída automaticamente",
			})
		}
	}
	return out
}

func (s *Store) UpdateAgent(ctx context.Context, orgID, id string, u AgentUpdate) (Agent, error) {
	u.Variables = mergeReferencedVariables(u.Variables, u.SystemPrompt, u.FirstMessage)
	if u.SystemTools == nil {
		u.SystemTools = []string{}
	}
	if u.Tools == nil {
		u.Tools = []string{}
	}
	vars, _ := json.Marshal(u.Variables)
	sysTools, _ := json.Marshal(u.SystemTools)
	sysToolsCfg, _ := json.Marshal(u.SystemToolsConfig)
	tools, _ := json.Marshal(u.Tools)
	a, err := scanAgent(s.Pool.QueryRow(ctx, `
		UPDATE agents SET name=$3, description=$4, active=$5, provider=$6, model=$7, voice=$8, language=$9,
			timezone=$10, system_prompt=$11, first_message=$12, first_message_interruptible=$13,
			max_duration_seconds=$14, recording_enabled=$15, variables=$16, initiation_webhook_url=$17,
			initiation_webhook_timeout_ms=$18, system_tools=$19, system_tools_config=$20, tools=$21, updated_at=now()
		WHERE organization_id=$1 AND id=$2 RETURNING `+agentCols,
		orgID, id, u.Name, u.Description, u.Active, u.Provider, u.Model, u.Voice, u.Language, u.Timezone,
		u.SystemPrompt, u.FirstMessage, u.FirstMessageInterruptible, u.MaxDurationSeconds, u.RecordingEnabled,
		vars, u.InitiationWebhookURL, u.InitiationWebhookTimeout, sysTools, sysToolsCfg, tools))
	return a, notFound(err)
}

// DraftConfig monta a configuração atual (Draft) de um Agent com seus Procedures.
func (s *Store) DraftConfig(ctx context.Context, orgID string, a Agent) (AgentConfig, error) {
	cfg := AgentConfig{
		Name: a.Name, Provider: a.Provider, Model: a.Model, Voice: a.Voice, Language: a.Language,
		Timezone: a.Timezone, SystemPrompt: a.SystemPrompt, FirstMessage: a.FirstMessage,
		FirstMessageInterruptible: a.FirstMessageInterruptible, MaxDurationSeconds: a.MaxDurationSeconds,
		RecordingEnabled: a.RecordingEnabled, InitiationWebhookURL: a.InitiationWebhookURL,
		InitiationWebhookTimeout: a.InitiationWebhookTimeout,
	}
	if err := json.Unmarshal(a.Variables, &cfg.Variables); err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(a.SystemTools, &cfg.SystemTools); err != nil {
		return cfg, err
	}
	if len(a.SystemToolsConfig) > 0 {
		_ = json.Unmarshal(a.SystemToolsConfig, &cfg.SystemToolsConfig)
	}
	if len(a.Tools) > 0 {
		_ = json.Unmarshal(a.Tools, &cfg.Tools)
	}
	if cfg.Tools == nil {
		cfg.Tools = []string{}
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, name, type, trigger, guardrails, content, active FROM procedures
		WHERE organization_id=$1 AND agent_id=$2 ORDER BY position, name`, orgID, a.ID)
	if err != nil {
		return cfg, err
	}
	defer rows.Close()
	cfg.Procedures = []ProcedureSnapshot{}
	for rows.Next() {
		var p ProcedureSnapshot
		var guard []byte
		if err := rows.Scan(&p.ID, &p.Name, &p.Type, &p.Trigger, &guard, &p.Content, &p.Active); err != nil {
			return cfg, err
		}
		if p.Type == "" {
			p.Type = "free_form"
		}
		if err := json.Unmarshal(guard, &p.Guardrails); err != nil {
			return cfg, err
		}
		cfg.Procedures = append(cfg.Procedures, p)
	}
	if err := rows.Err(); err != nil {
		return cfg, err
	}
	texts := []string{cfg.SystemPrompt, cfg.FirstMessage}
	for _, p := range cfg.Procedures {
		texts = append(texts, p.Content, p.Trigger)
		texts = append(texts, p.Guardrails...)
	}
	cfg.Variables = mergeReferencedVariables(cfg.Variables, texts...)
	return cfg, nil
}

// Publish grava uma nova Agent Version numerada com o snapshot informado.
func (s *Store) Publish(ctx context.Context, orgID, agentID, userID string, cfg AgentConfig) (AgentVersion, error) {
	var v AgentVersion
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return v, err
	}
	defer tx.Rollback(ctx)
	// Trava a linha do Agent para serializar a numeração de versões.
	var locked string
	if err := tx.QueryRow(ctx, `SELECT id FROM agents WHERE organization_id=$1 AND id=$2 FOR UPDATE`, orgID, agentID).Scan(&locked); err != nil {
		return v, notFound(err)
	}
	snap, _ := json.Marshal(cfg)
	var raw []byte
	err = tx.QueryRow(ctx, `
		INSERT INTO agent_versions (organization_id, agent_id, version, snapshot, published_by)
		VALUES ($1, $2, (SELECT coalesce(max(version), 0) + 1 FROM agent_versions WHERE agent_id=$2), $3, $4)
		RETURNING id, agent_id, version, snapshot, published_by, created_at`,
		orgID, agentID, snap, userID).Scan(&v.ID, &v.AgentID, &v.Version, &raw, &v.PublishedBy, &v.CreatedAt)
	if err != nil {
		return v, err
	}
	if err := json.Unmarshal(raw, &v.Snapshot); err != nil {
		return v, err
	}
	return v, tx.Commit(ctx)
}

// LatestVersion devolve a Agent Version publicada mais recente, ou ErrNotFound.
func (s *Store) LatestVersion(ctx context.Context, orgID, agentID string) (AgentVersion, error) {
	var v AgentVersion
	var raw []byte
	err := s.Pool.QueryRow(ctx, `
		SELECT id, agent_id, version, snapshot, published_by, created_at FROM agent_versions
		WHERE organization_id=$1 AND agent_id=$2 ORDER BY version DESC LIMIT 1`, orgID, agentID).
		Scan(&v.ID, &v.AgentID, &v.Version, &raw, &v.PublishedBy, &v.CreatedAt)
	if err != nil {
		return v, notFound(err)
	}
	return v, json.Unmarshal(raw, &v.Snapshot)
}

type VersionSummary struct {
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) ListVersions(ctx context.Context, orgID, agentID string) ([]VersionSummary, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT version, created_at FROM agent_versions
		WHERE organization_id=$1 AND agent_id=$2 ORDER BY version DESC`, orgID, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []VersionSummary{}
	for rows.Next() {
		var v VersionSummary
		if err := rows.Scan(&v.Version, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
