package store

import (
	"context"
	"encoding/json"
	"time"
)

type Procedure struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	AgentID        string    `json:"agent_id"`
	Name           string    `json:"name"`
	Type           string    `json:"type"` // free_form | deterministic
	Trigger        string    `json:"trigger"`
	Guardrails     []string  `json:"guardrails"`
	Content        string    `json:"content"`
	Active         bool      `json:"active"`
	Position       int       `json:"position"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

const procCols = `id, organization_id, agent_id, name, type, trigger, guardrails, content, active, position, created_at, updated_at`

func scanProc(r scanner) (Procedure, error) {
	var p Procedure
	var guardBytes []byte
	err := r.Scan(&p.ID, &p.OrganizationID, &p.AgentID, &p.Name, &p.Type, &p.Trigger, &guardBytes,
		&p.Content, &p.Active, &p.Position, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return p, err
	}
	if p.Type == "" {
		p.Type = "free_form"
	}
	_ = json.Unmarshal(guardBytes, &p.Guardrails)
	if p.Guardrails == nil {
		p.Guardrails = []string{}
	}
	return p, nil
}

func (s *Store) CreateProcedure(ctx context.Context, orgID, agentID string, p Procedure) (Procedure, error) {
	if p.Guardrails == nil {
		p.Guardrails = []string{}
	}
	if p.Type == "" {
		p.Type = "free_form"
	}
	guardBytes, _ := json.Marshal(p.Guardrails)
	row := s.Pool.QueryRow(ctx, `
		INSERT INTO procedures (organization_id, agent_id, name, type, trigger, guardrails, content, active, position)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, (SELECT coalesce(max(position), 0) + 1 FROM procedures WHERE agent_id = $2))
		RETURNING `+procCols,
		orgID, agentID, p.Name, p.Type, p.Trigger, guardBytes, p.Content, p.Active)
	return scanProc(row)
}

func (s *Store) UpdateProcedure(ctx context.Context, orgID, agentID, id string, p Procedure) (Procedure, error) {
	if p.Guardrails == nil {
		p.Guardrails = []string{}
	}
	if p.Type == "" {
		p.Type = "free_form"
	}
	guardBytes, _ := json.Marshal(p.Guardrails)
	row := s.Pool.QueryRow(ctx, `
		UPDATE procedures
		SET name = $4, type = $5, trigger = $6, guardrails = $7, content = $8, active = $9, updated_at = now()
		WHERE organization_id = $1 AND agent_id = $2 AND id = $3
		RETURNING `+procCols,
		orgID, agentID, id, p.Name, p.Type, p.Trigger, guardBytes, p.Content, p.Active)
	res, err := scanProc(row)
	return res, notFound(err)
}

func (s *Store) GetProcedure(ctx context.Context, orgID, agentID, id string) (Procedure, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT `+procCols+` FROM procedures
		WHERE organization_id = $1 AND agent_id = $2 AND id = $3`, orgID, agentID, id)
	res, err := scanProc(row)
	return res, notFound(err)
}

func (s *Store) ListProcedures(ctx context.Context, orgID, agentID string) ([]Procedure, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT `+procCols+` FROM procedures
		WHERE organization_id = $1 AND agent_id = $2
		ORDER BY position ASC, name ASC`, orgID, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Procedure{}
	for rows.Next() {
		p, err := scanProc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) DeleteProcedure(ctx context.Context, orgID, agentID, id string) error {
	cmd, err := s.Pool.Exec(ctx, `
		DELETE FROM procedures
		WHERE organization_id = $1 AND agent_id = $2 AND id = $3`, orgID, agentID, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
