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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		if recovered := s.recoverProceduresFromVersions(ctx, orgID, agentID); len(recovered) > 0 {
			return recovered, nil
		}
	}
	return out, nil
}

// recoverProceduresFromVersions restaura procedures a partir do último snapshot de agent_branch_drafts ou agent_versions
// caso um publish anterior com branch draft desatualizado tenha limpado a tabela procedures.
func (s *Store) recoverProceduresFromVersions(ctx context.Context, orgID, agentID string) []Procedure {
	var raw []byte
	err := s.Pool.QueryRow(ctx, `
		SELECT snapshot FROM (
			SELECT snapshot, updated_at AS ts, 1 AS prio
			FROM agent_branch_drafts
			WHERE organization_id = $1 AND agent_id = $2
			  AND jsonb_typeof(snapshot->'procedures') = 'array'
			  AND jsonb_array_length(snapshot->'procedures') > 0
			UNION ALL
			SELECT snapshot, created_at AS ts, 2 AS prio
			FROM agent_versions
			WHERE organization_id = $1 AND agent_id = $2
			  AND jsonb_typeof(snapshot->'procedures') = 'array'
			  AND jsonb_array_length(snapshot->'procedures') > 0
		) sub
		ORDER BY prio ASC, ts DESC
		LIMIT 1`, orgID, agentID).Scan(&raw)
	if err != nil || len(raw) == 0 {
		return nil
	}
	var snap AgentConfig
	if err := json.Unmarshal(raw, &snap); err != nil || len(snap.Procedures) == 0 {
		return nil
	}
	var restored []Procedure
	for i, sp := range snap.Procedures {
		guardBytes, _ := json.Marshal(sp.Guardrails)
		pType := sp.Type
		if pType == "" {
			pType = "free_form"
		}
		var p Procedure
		var scanErr error
		if sp.ID != "" {
			row := s.Pool.QueryRow(ctx, `
				INSERT INTO procedures (id, organization_id, agent_id, name, type, trigger, guardrails, content, active, position)
				VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10)
				ON CONFLICT (id) DO UPDATE SET
					name = EXCLUDED.name,
					type = EXCLUDED.type,
					trigger = EXCLUDED.trigger,
					guardrails = EXCLUDED.guardrails,
					content = EXCLUDED.content,
					active = EXCLUDED.active,
					position = EXCLUDED.position,
					updated_at = now()
				WHERE procedures.agent_id = EXCLUDED.agent_id
				RETURNING `+procCols,
				sp.ID, orgID, agentID, sp.Name, pType, sp.Trigger, guardBytes, sp.Content, sp.Active, i+1)
			p, scanErr = scanProc(row)
		}
		if sp.ID == "" || scanErr != nil {
			row := s.Pool.QueryRow(ctx, `
				INSERT INTO procedures (organization_id, agent_id, name, type, trigger, guardrails, content, active, position)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
				RETURNING `+procCols,
				orgID, agentID, sp.Name, pType, sp.Trigger, guardBytes, sp.Content, sp.Active, i+1)
			p, scanErr = scanProc(row)
		}
		if scanErr == nil {
			restored = append(restored, p)
		}
	}
	return restored
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
