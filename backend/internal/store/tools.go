package store

import (
	"context"
	"encoding/json"
	"time"

	"voiceagent/internal/voice"
)

type ToolParamSchema struct {
	Name            string `json:"name"`
	Type            string `json:"type"` // string | number | integer | boolean | array | object
	Description     string `json:"description,omitempty"`
	Required        bool   `json:"required"`
	ValueType       string `json:"value_type,omitempty"` // llm_prompt | constant | dynamic_variable
	ConstantValue   any    `json:"constant_value,omitempty"`
	DynamicVariable string `json:"dynamic_variable,omitempty"`
}

type ToolHeaderSchema struct {
	Type  string `json:"type"` // value | secret | dynamic_variable
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ToolRequestBodySchema struct {
	Type        string            `json:"type"` // "object"
	Description string            `json:"description,omitempty"`
	Properties  []ToolParamSchema `json:"properties,omitempty"`
}

type ToolAPISchema struct {
	URL                string                 `json:"url"`
	Method             string                 `json:"method"` // GET | POST | PUT | PATCH | DELETE
	PathParamsSchema   []ToolParamSchema      `json:"path_params_schema"`
	QueryParamsSchema  []ToolParamSchema      `json:"query_params_schema"`
	RequestBodySchema  *ToolRequestBodySchema `json:"request_body_schema"`
	RequestHeaders     []ToolHeaderSchema     `json:"request_headers"`
	ContentType        string                 `json:"content_type"` // application/json | application/x-www-form-urlencoded
	AuthConnection     any                    `json:"auth_connection"`
	MTLSAuthConnection any                    `json:"mtls_auth_connection"`
	ResponseFilter     any                    `json:"response_filter"`
}

type ToolDynamicVariables struct {
	DynamicVariablePlaceholders map[string]any `json:"dynamic_variable_placeholders"`
}

type ToolAssignment struct {
	Source          string `json:"source,omitempty"` // "response"
	DynamicVariable string `json:"dynamic_variable"`
	ValuePath       string `json:"value_path"`
}

type ToolResponseMock struct {
	ID             string `json:"id,omitempty"`
	Name           string `json:"name,omitempty"`
	ConditionType  string `json:"condition_type,omitempty"` // "always" | "parameter_equals"
	ParameterName  string `json:"parameter_name,omitempty"`
	ParameterValue string `json:"parameter_value,omitempty"`
	Response       string `json:"response"`
	Enabled        bool   `json:"enabled"`
}

type ToolCodeContext struct {
	Config          map[string]string `json:"config,omitempty"`
	Secrets         []string          `json:"secrets,omitempty"`
	AuthConnections []string          `json:"auth_connections,omitempty"`
}

type WebhookConfig struct {
	URL                           string                `json:"url"`
	Method                        string                `json:"method"` // GET | POST | PUT | PATCH | DELETE
	APISchema                     *ToolAPISchema        `json:"api_schema,omitempty"`
	ResponseTimeoutSecs           int                   `json:"response_timeout_secs,omitempty"`
	FollowRedirects               bool                  `json:"follow_redirects"`
	FollowRedirectsAllowedDomains []string              `json:"follow_redirects_allowed_domains,omitempty"`
	DynamicVariables              *ToolDynamicVariables `json:"dynamic_variables,omitempty"`
	Assignments                   []ToolAssignment      `json:"assignments,omitempty"`
	InterruptionMode              string                `json:"interruption_mode,omitempty"`
	PreToolSpeech                 string                `json:"pre_tool_speech,omitempty"`
	ToolCallSound                 *string               `json:"tool_call_sound"`
	ToolCallSoundBehavior         string                `json:"tool_call_sound_behavior,omitempty"`
	ExecutionMode                 string                `json:"execution_mode,omitempty"`
	ToolErrorHandlingMode         string                `json:"tool_error_handling_mode,omitempty"`
	ResponseMocks                 []ToolResponseMock    `json:"response_mocks,omitempty"`

	// Campos específicos de Client Tool (type = "client")
	ExpectsResponse bool `json:"expects_response,omitempty"`

	// Campos específicos de Code Tool (type = "code")
	Code          string           `json:"code,omitempty"`
	ContextObject *ToolCodeContext `json:"context_object,omitempty"`

	// Campos específicos de MCP Tool (type = "mcp")
	Transport      string `json:"transport,omitempty"`       // SSE | STREAMABLE_HTTP
	ApprovalPolicy string `json:"approval_policy,omitempty"` // always_ask | fine_grained | no_approval
}

type Tool struct {
	ID             string            `json:"id"`
	OrganizationID string            `json:"organization_id"`
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	Kind           string            `json:"kind"` // webhook | mcp
	InputSchema    []voice.Param     `json:"input_schema"`
	Config         WebhookConfig     `json:"config"`
	SecretHeaders  map[string]string `json:"secret_headers,omitempty"`
	HasSecrets     bool              `json:"has_secrets"` // true se secret_headers não estiver vazio
	TimeoutMS      int               `json:"timeout_ms"`
	Active         bool              `json:"active"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

type ToolWithStats struct {
	Tool
	TotalExecutions int     `json:"total_executions"`
	SuccessRate     float64 `json:"success_rate"`
}

type ToolExecution struct {
	ID              string          `json:"id"`
	OrganizationID  string          `json:"organization_id"`
	AgentID         string          `json:"agent_id"`
	ConversationID  string          `json:"conversation_id"`
	ToolID          *string         `json:"tool_id"`
	ToolName        string          `json:"tool_name"`
	ProcedureID     *string         `json:"procedure_id"`
	Request         json.RawMessage `json:"request"`
	Response        json.RawMessage `json:"response"`
	Status          string          `json:"status"` // success | error | rejected
	ExecutionTimeMS int             `json:"execution_time_ms"`
	CreatedAt       time.Time       `json:"created_at"`
}

const toolCols = `id, organization_id, name, description, kind, input_schema, config, secret_headers, timeout_ms, active, created_at, updated_at`

func scanTool(r scanner) (Tool, error) {
	var t Tool
	var schemaBytes, configBytes, headerBytes []byte
	err := r.Scan(&t.ID, &t.OrganizationID, &t.Name, &t.Description, &t.Kind,
		&schemaBytes, &configBytes, &headerBytes, &t.TimeoutMS, &t.Active, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return t, err
	}
	_ = json.Unmarshal(schemaBytes, &t.InputSchema)
	_ = json.Unmarshal(configBytes, &t.Config)
	_ = json.Unmarshal(headerBytes, &t.SecretHeaders)
	if t.InputSchema == nil {
		t.InputSchema = []voice.Param{}
	}
	if t.SecretHeaders == nil {
		t.SecretHeaders = map[string]string{}
	}
	t.HasSecrets = len(t.SecretHeaders) > 0
	return t, nil
}

func (s *Store) CreateTool(ctx context.Context, orgID string, t Tool) (Tool, error) {
	if t.Kind == "" {
		t.Kind = "webhook"
	}
	if t.Config.Method == "" {
		t.Config.Method = "POST"
	}
	if t.TimeoutMS <= 0 {
		t.TimeoutMS = 10000
	}
	if t.InputSchema == nil {
		t.InputSchema = []voice.Param{}
	}
	if t.SecretHeaders == nil {
		t.SecretHeaders = map[string]string{}
	}

	schemaBytes, _ := json.Marshal(t.InputSchema)
	configBytes, _ := json.Marshal(t.Config)
	headerBytes, _ := json.Marshal(t.SecretHeaders)

	row := s.Pool.QueryRow(ctx, `
		INSERT INTO tools (organization_id, name, description, kind, input_schema, config, secret_headers, timeout_ms, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING `+toolCols,
		orgID, t.Name, t.Description, t.Kind, schemaBytes, configBytes, headerBytes, t.TimeoutMS, t.Active)
	return scanTool(row)
}

func (s *Store) UpdateTool(ctx context.Context, orgID, id string, t Tool, updateSecrets bool) (Tool, error) {
	if t.Kind == "" {
		t.Kind = "webhook"
	}
	if t.Config.Method == "" {
		t.Config.Method = "POST"
	}
	if t.TimeoutMS <= 0 {
		t.TimeoutMS = 10000
	}
	if t.InputSchema == nil {
		t.InputSchema = []voice.Param{}
	}

	schemaBytes, _ := json.Marshal(t.InputSchema)
	configBytes, _ := json.Marshal(t.Config)

	var row scanner
	if updateSecrets {
		if t.SecretHeaders == nil {
			t.SecretHeaders = map[string]string{}
		}
		headerBytes, _ := json.Marshal(t.SecretHeaders)
		row = s.Pool.QueryRow(ctx, `
			UPDATE tools SET name=$3, description=$4, kind=$5, input_schema=$6, config=$7, secret_headers=$8, timeout_ms=$9, active=$10, updated_at=now()
			WHERE organization_id=$1 AND id=$2 RETURNING `+toolCols,
			orgID, id, t.Name, t.Description, t.Kind, schemaBytes, configBytes, headerBytes, t.TimeoutMS, t.Active)
	} else {
		row = s.Pool.QueryRow(ctx, `
			UPDATE tools SET name=$3, description=$4, kind=$5, input_schema=$6, config=$7, timeout_ms=$8, active=$9, updated_at=now()
			WHERE organization_id=$1 AND id=$2 RETURNING `+toolCols,
			orgID, id, t.Name, t.Description, t.Kind, schemaBytes, configBytes, t.TimeoutMS, t.Active)
	}
	res, err := scanTool(row)
	return res, notFound(err)
}

func (s *Store) GetTool(ctx context.Context, orgID, id string) (Tool, error) {
	t, err := scanTool(s.Pool.QueryRow(ctx, `SELECT `+toolCols+` FROM tools WHERE organization_id=$1 AND id=$2`, orgID, id))
	return t, notFound(err)
}

func (s *Store) GetToolByName(ctx context.Context, orgID, name string) (Tool, error) {
	t, err := scanTool(s.Pool.QueryRow(ctx, `SELECT `+toolCols+` FROM tools WHERE organization_id=$1 AND lower(name)=lower($2)`, orgID, name))
	return t, notFound(err)
}

func (s *Store) ListTools(ctx context.Context, orgID string) ([]ToolWithStats, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT t.id, t.organization_id, t.name, t.description, t.kind, t.input_schema, t.config, t.secret_headers, t.timeout_ms, t.active, t.created_at, t.updated_at,
		       coalesce(stats.total, 0) as total_execs,
		       coalesce(stats.success_count, 0) as success_execs
		FROM tools t
		LEFT JOIN (
			SELECT tool_id, count(*) as total, count(*) FILTER (WHERE status = 'success') as success_count
			FROM tool_executions
			GROUP BY tool_id
		) stats ON stats.tool_id = t.id
		WHERE t.organization_id = $1
		ORDER BY t.name ASC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ToolWithStats{}
	for rows.Next() {
		var ts ToolWithStats
		var schemaBytes, configBytes, headerBytes []byte
		var total, successes int
		err := rows.Scan(&ts.ID, &ts.OrganizationID, &ts.Name, &ts.Description, &ts.Kind,
			&schemaBytes, &configBytes, &headerBytes, &ts.TimeoutMS, &ts.Active, &ts.CreatedAt, &ts.UpdatedAt,
			&total, &successes)
		if err != nil {
			return nil, err
		}
		_ = json.Unmarshal(schemaBytes, &ts.InputSchema)
		_ = json.Unmarshal(configBytes, &ts.Config)
		_ = json.Unmarshal(headerBytes, &ts.SecretHeaders)
		if ts.InputSchema == nil {
			ts.InputSchema = []voice.Param{}
		}
		ts.HasSecrets = len(ts.SecretHeaders) > 0
		ts.SecretHeaders = nil // Mascara segredos na listagem
		ts.TotalExecutions = total
		if total > 0 {
			ts.SuccessRate = float64(successes) / float64(total) * 100.0
		} else {
			ts.SuccessRate = 100.0 // Padrão visual quando não há execuções
		}
		out = append(out, ts)
	}
	return out, rows.Err()
}

func (s *Store) DeleteTool(ctx context.Context, orgID, id string) error {
	cmd, err := s.Pool.Exec(ctx, `DELETE FROM tools WHERE organization_id=$1 AND id=$2`, orgID, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RecordToolExecution(ctx context.Context, exec ToolExecution) error {
	reqJSON, err := json.Marshal(exec.Request)
	if err != nil {
		reqJSON = []byte("{}")
	}
	respJSON, err := json.Marshal(exec.Response)
	if err != nil {
		respJSON = []byte("{}")
	}
	_, err = s.Pool.Exec(ctx, `
		INSERT INTO tool_executions (organization_id, agent_id, conversation_id, tool_id, tool_name, procedure_id, request, response, status, execution_time_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		exec.OrganizationID, exec.AgentID, exec.ConversationID, exec.ToolID, exec.ToolName, exec.ProcedureID, reqJSON, respJSON, exec.Status, exec.ExecutionTimeMS)
	return err
}

func (s *Store) ListToolExecutions(ctx context.Context, orgID, convID string) ([]ToolExecution, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, organization_id, agent_id, conversation_id, tool_id, tool_name, procedure_id, request, response, status, execution_time_ms, created_at
		FROM tool_executions
		WHERE organization_id=$1 AND conversation_id=$2
		ORDER BY created_at ASC`, orgID, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ToolExecution{}
	for rows.Next() {
		var e ToolExecution
		if err := rows.Scan(&e.ID, &e.OrganizationID, &e.AgentID, &e.ConversationID, &e.ToolID, &e.ToolName, &e.ProcedureID,
			&e.Request, &e.Response, &e.Status, &e.ExecutionTimeMS, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
