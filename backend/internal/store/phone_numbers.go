package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrPhoneNumberUnassigned = errors.New("phone_number_unassigned")

type PhoneNumber struct {
	ID               string    `json:"id"`
	OrganizationID   string    `json:"organization_id"`
	E164             string    `json:"e164"`
	Label            string    `json:"label"`
	AgentID          string    `json:"agent_id"`
	Direction        string    `json:"direction"`          // inbound | outbound | both
	Provider         string    `json:"provider"`           // jambonz | twilio
	TwilioAccountSid string    `json:"twilio_account_sid"` // AC...
	TwilioAuthToken  string    `json:"-"`                  // nunca serializado na resposta JSON
	TwilioPhoneSid   string    `json:"twilio_phone_sid"`   // PN...
	HasTwilioAuth    bool      `json:"has_twilio_auth"`
	Active           bool      `json:"active"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type PhoneNumberWithAgent struct {
	PhoneNumber
	AgentName string `json:"agent_name"`
}

type CreatePhoneNumberParams struct {
	E164             string
	Label            string
	AgentID          string
	Direction        string
	Provider         string
	TwilioAccountSid string
	TwilioAuthToken  string
	TwilioPhoneSid   string
}

type UpdatePhoneNumberParams struct {
	Label            *string
	AgentID          *string
	Direction        *string
	Provider         *string
	TwilioAccountSid *string
	TwilioAuthToken  *string
	TwilioPhoneSid   *string
	Active           *bool
}

const phoneCols = `p.id, p.organization_id, p.e164, p.label, COALESCE(p.agent_id::text, '') AS agent_id, p.direction, p.provider, p.twilio_account_sid, p.twilio_auth_token, p.twilio_phone_sid, p.active, p.created_at, p.updated_at`
const phoneColsPlain = `id, organization_id, e164, label, COALESCE(agent_id::text, '') AS agent_id, direction, provider, twilio_account_sid, twilio_auth_token, twilio_phone_sid, active, created_at, updated_at`

func scanPhone(r scanner) (PhoneNumber, error) {
	var p PhoneNumber
	err := r.Scan(
		&p.ID,
		&p.OrganizationID,
		&p.E164,
		&p.Label,
		&p.AgentID,
		&p.Direction,
		&p.Provider,
		&p.TwilioAccountSid,
		&p.TwilioAuthToken,
		&p.TwilioPhoneSid,
		&p.Active,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	p.HasTwilioAuth = p.TwilioAuthToken != ""
	return p, err
}

func scanPhoneWithAgent(r scanner) (PhoneNumberWithAgent, error) {
	var item PhoneNumberWithAgent
	err := r.Scan(
		&item.ID,
		&item.OrganizationID,
		&item.E164,
		&item.Label,
		&item.AgentID,
		&item.Direction,
		&item.Provider,
		&item.TwilioAccountSid,
		&item.TwilioAuthToken,
		&item.TwilioPhoneSid,
		&item.Active,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.AgentName,
	)
	item.HasTwilioAuth = item.TwilioAuthToken != ""
	return item, err
}

func (s *Store) ListPhoneNumbers(ctx context.Context, orgID string) ([]PhoneNumberWithAgent, error) {
	query := `
		SELECT ` + phoneCols + `, COALESCE(a.name, '') AS agent_name
		FROM phone_numbers p
		LEFT JOIN agents a ON a.id = p.agent_id
		WHERE p.organization_id = $1
		ORDER BY p.created_at DESC`
	rows, err := s.Pool.Query(ctx, query, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PhoneNumberWithAgent
	for rows.Next() {
		item, err := scanPhoneWithAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetPhoneNumber(ctx context.Context, orgID, id string) (PhoneNumber, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT `+phoneCols+`
		FROM phone_numbers p
		WHERE p.organization_id = $1 AND p.id = $2`, orgID, id)
	p, err := scanPhone(row)
	if err != nil {
		return p, notFound(err)
	}
	return p, nil
}

func (s *Store) GetPhoneNumberWithAgent(ctx context.Context, orgID, id string) (PhoneNumberWithAgent, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT `+phoneCols+`, COALESCE(a.name, '') AS agent_name
		FROM phone_numbers p
		LEFT JOIN agents a ON a.id = p.agent_id
		WHERE p.organization_id = $1 AND p.id = $2`, orgID, id)
	p, err := scanPhoneWithAgent(row)
	if err != nil {
		return p, notFound(err)
	}
	return p, nil
}

func (s *Store) GetPhoneNumberByE164(ctx context.Context, orgID, e164 string) (PhoneNumber, error) {
	e164Norm := normalizeE164(e164)
	row := s.Pool.QueryRow(ctx, `
		SELECT `+phoneCols+`
		FROM phone_numbers p
		WHERE p.organization_id = $1 AND (p.e164 = $2 OR p.e164 = $3)
		LIMIT 1`, orgID, e164Norm, strings.TrimPrefix(e164Norm, "+"))
	p, err := scanPhone(row)
	if err != nil {
		return p, notFound(err)
	}
	return p, nil
}

func (s *Store) CreatePhoneNumber(ctx context.Context, orgID string, in CreatePhoneNumberParams) (PhoneNumberWithAgent, error) {
	if in.Direction == "" {
		in.Direction = "inbound"
	}
	if in.Provider == "" {
		in.Provider = "jambonz"
	}
	e164 := normalizeE164(in.E164)
	agentID := strings.TrimSpace(in.AgentID)
	row := s.Pool.QueryRow(ctx, `
		INSERT INTO phone_numbers (
			organization_id, e164, label, agent_id, direction,
			provider, twilio_account_sid, twilio_auth_token, twilio_phone_sid, active
		)
		VALUES ($1, $2, $3, NULLIF($4, '')::uuid, $5, $6, $7, $8, $9, true)
		ON CONFLICT (organization_id, e164) DO UPDATE
		SET label = EXCLUDED.label,
		    agent_id = EXCLUDED.agent_id,
		    direction = EXCLUDED.direction,
		    provider = EXCLUDED.provider,
		    twilio_account_sid = CASE WHEN EXCLUDED.twilio_account_sid = '' THEN phone_numbers.twilio_account_sid ELSE EXCLUDED.twilio_account_sid END,
		    twilio_auth_token = CASE WHEN EXCLUDED.twilio_auth_token = '' THEN phone_numbers.twilio_auth_token ELSE EXCLUDED.twilio_auth_token END,
		    twilio_phone_sid = CASE WHEN EXCLUDED.twilio_phone_sid = '' THEN phone_numbers.twilio_phone_sid ELSE EXCLUDED.twilio_phone_sid END,
		    active = true,
		    updated_at = now()
		RETURNING `+phoneColsPlain,
		orgID, e164, in.Label, agentID, in.Direction,
		in.Provider, strings.TrimSpace(in.TwilioAccountSid), strings.TrimSpace(in.TwilioAuthToken), strings.TrimSpace(in.TwilioPhoneSid),
	)
	created, err := scanPhone(row)
	if err != nil {
		return PhoneNumberWithAgent{}, err
	}
	return s.GetPhoneNumberWithAgent(ctx, orgID, created.ID)
}

func (s *Store) UpdatePhoneNumber(ctx context.Context, orgID, id string, in UpdatePhoneNumberParams) (PhoneNumberWithAgent, error) {
	curr, err := s.GetPhoneNumber(ctx, orgID, id)
	if err != nil {
		return PhoneNumberWithAgent{}, err
	}

	label := curr.Label
	if in.Label != nil {
		label = *in.Label
	}
	agentID := curr.AgentID
	if in.AgentID != nil {
		agentID = strings.TrimSpace(*in.AgentID)
	}
	direction := curr.Direction
	if in.Direction != nil && strings.TrimSpace(*in.Direction) != "" {
		direction = strings.TrimSpace(*in.Direction)
	}
	provider := curr.Provider
	if in.Provider != nil && strings.TrimSpace(*in.Provider) != "" {
		provider = strings.TrimSpace(*in.Provider)
	}
	twilioAccountSid := curr.TwilioAccountSid
	if in.TwilioAccountSid != nil && strings.TrimSpace(*in.TwilioAccountSid) != "" {
		twilioAccountSid = strings.TrimSpace(*in.TwilioAccountSid)
	}
	twilioAuthToken := curr.TwilioAuthToken
	if in.TwilioAuthToken != nil && strings.TrimSpace(*in.TwilioAuthToken) != "" {
		twilioAuthToken = strings.TrimSpace(*in.TwilioAuthToken)
	}
	twilioPhoneSid := curr.TwilioPhoneSid
	if in.TwilioPhoneSid != nil && strings.TrimSpace(*in.TwilioPhoneSid) != "" {
		twilioPhoneSid = strings.TrimSpace(*in.TwilioPhoneSid)
	}
	active := curr.Active
	if in.Active != nil {
		active = *in.Active
	}

	row := s.Pool.QueryRow(ctx, `
		UPDATE phone_numbers
		SET label = $3,
		    agent_id = NULLIF($4, '')::uuid,
		    direction = $5,
		    provider = $6,
		    twilio_account_sid = $7,
		    twilio_auth_token = $8,
		    twilio_phone_sid = $9,
		    active = $10,
		    updated_at = now()
		WHERE organization_id = $1 AND id = $2
		RETURNING `+phoneColsPlain,
		orgID, id, label, agentID, direction, provider,
		twilioAccountSid, twilioAuthToken, twilioPhoneSid, active)
	updated, err := scanPhone(row)
	if err != nil {
		return PhoneNumberWithAgent{}, notFound(err)
	}
	return s.GetPhoneNumberWithAgent(ctx, orgID, updated.ID)
}

func (s *Store) DeletePhoneNumber(ctx context.Context, orgID, id string) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM phone_numbers WHERE organization_id = $1 AND id = $2`, orgID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// FindAgentByPhoneNumber encontra o agente ativo associado ao número discado (DID em formato E.164).
func (s *Store) FindAgentByPhoneNumber(ctx context.Context, e164 string) (PhoneNumber, Agent, error) {
	var p PhoneNumber
	e164Norm := normalizeE164(e164)

	// Busca por correspondência exata ou removendo prefixo +
	query := `
		SELECT ` + phoneCols + `
		FROM phone_numbers p
		WHERE (p.e164 = $1 OR p.e164 = $2) AND p.active = true
		LIMIT 1`
	row := s.Pool.QueryRow(ctx, query, e164Norm, strings.TrimPrefix(e164Norm, "+"))
	p, err := scanPhone(row)
	if err != nil {
		return p, Agent{}, fmt.Errorf("phone number %s not mapped: %w", e164, notFound(err))
	}
	if p.AgentID == "" {
		return p, Agent{}, fmt.Errorf("phone number %s has no assigned agent: %w", e164, ErrPhoneNumberUnassigned)
	}

	ag, err := s.GetAgent(ctx, p.OrganizationID, p.AgentID)
	if err != nil {
		return p, Agent{}, fmt.Errorf("agent %s not found for phone %s: %w", p.AgentID, e164, err)
	}
	if !ag.Active {
		return p, ag, fmt.Errorf("agent %s is inactive", ag.Name)
	}
	return p, ag, nil
}

func normalizeE164(s string) string {
	s = strings.TrimSpace(s)
	// Remove espaços, traços, parênteses
	cleaned := strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || r == '+' {
			return r
		}
		return -1
	}, s)
	if !strings.HasPrefix(cleaned, "+") && len(cleaned) > 0 {
		cleaned = "+" + cleaned
	}
	return cleaned
}
