package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

var (
	ErrBranchArchived        = errors.New("branch_archived")
	ErrCannotArchiveMain     = errors.New("cannot_archive_main")
	ErrCannotArchiveTraffic  = errors.New("cannot_archive_branch_with_traffic")
	ErrCannotRebaseMain      = errors.New("cannot_rebase_main")
	ErrBranchAlreadyUpToDate = errors.New("branch_already_up_to_date")
	ErrNoNewChangesToMerge   = errors.New("no_new_changes_to_merge")
	ErrBranchAlreadyMerged   = errors.New("branch_already_merged")
	ErrInvalidTrafficSplit   = errors.New("deployments_must_sum_to_100")
	ErrInvalidBranchName     = errors.New("invalid_branch_name")
)

var branchNameRe = regexp.MustCompile(`^[a-zA-Z0-9_()\[\]{}\-/. ]+$`)

// ValidateBranchName valida as regras da ElevenLabs: letras, números e () [] {} - / . (máx. 140 caracteres).
func ValidateBranchName(name string) error {
	name = strings.TrimSpace(name)
	n := utf8.RuneCountInString(name)
	if n == 0 || n > 140 {
		return ErrInvalidBranchName
	}
	if !branchNameRe.MatchString(name) {
		return ErrInvalidBranchName
	}
	return nil
}

type AgentBranch struct {
	ID                  string           `json:"id"`
	OrganizationID      string           `json:"organization_id"`
	AgentID             string           `json:"agent_id"`
	Name                string           `json:"name"`
	Description         string           `json:"description"`
	IsMain              bool             `json:"is_main"`
	Archived            bool             `json:"archived"`
	TrafficPercentage   int              `json:"traffic_percentage"`
	ParentVersionID     *string          `json:"parent_version_uuid,omitempty"`
	ParentVersionCode   *string          `json:"parent_version_id,omitempty"`
	HeadVersionID       *string          `json:"head_version_uuid,omitempty"`
	HeadVersionCode     *string          `json:"head_version_id,omitempty"`
	HeadVersionNumber   *int             `json:"head_version_number,omitempty"`
	BaseMainVersionID   *string          `json:"base_main_version_uuid,omitempty"`
	BaseMainVersionCode *string          `json:"base_main_version_id,omitempty"`
	MergedIntoBranchID  *string          `json:"merged_into_branch_id,omitempty"`
	MergedVersionID     *string          `json:"merged_version_id,omitempty"`
	HasDraft            bool             `json:"has_draft"`
	DraftUpdatedAt      *time.Time       `json:"draft_updated_at,omitempty"`
	AheadOfMain         bool             `json:"ahead_of_main"`
	BehindMain          bool             `json:"behind_main"`
	VersionsCount       int              `json:"versions_count"`
	Versions            []VersionSummary `json:"versions,omitempty"`
	CreatedAt           time.Time        `json:"created_at"`
	UpdatedAt           time.Time        `json:"updated_at"`
}

type TrafficAllocation struct {
	BranchID   string `json:"branch_id"`
	Percentage int    `json:"percentage"`
}

type FieldDiff struct {
	Field         string `json:"field"`
	Label         string `json:"label"`
	TargetValue   any    `json:"target_value"`
	SourceValue   any    `json:"source_value"`
	ResolvedValue any    `json:"resolved_value"`
	Winner        string `json:"winner,omitempty"` // "source" | "target"
}

type MergePreview struct {
	SourceBranchID   string      `json:"source_branch_id"`
	SourceBranchName string      `json:"source_branch_name"`
	TargetBranchID   string      `json:"target_branch_id"`
	TargetBranchName string      `json:"target_branch_name"`
	OverriddenFields []FieldDiff `json:"overridden_fields"`
	Conflicts        []FieldDiff `json:"conflicts"`
	CanMerge         bool        `json:"can_merge"`
}

type MergeResult struct {
	CreatedVersion     AgentVersion `json:"created_version"`
	SourceBranch       AgentBranch  `json:"source_branch"`
	TargetBranch       AgentBranch  `json:"target_branch"`
	TransferredTraffic int          `json:"transferred_traffic"`
}

type RebasePreview struct {
	BranchID              string      `json:"branch_id"`
	BranchName            string      `json:"branch_name"`
	MainVersionID         string      `json:"main_version_id"`
	OverriddenFields      []FieldDiff `json:"overridden_fields"`
	PreservedBranchFields []string    `json:"preserved_branch_fields"`
	CanRebase             bool        `json:"can_rebase"`
}

// syncMainTablesTx sincroniza as colunas editáveis de agents e procedures com um snapshot da Main.
func syncMainTablesTx(ctx context.Context, tx pgx.Tx, orgID, agentID string, cfg AgentConfig) error {
	vars, _ := json.Marshal(cfg.Variables)
	sysTools, _ := json.Marshal(cfg.SystemTools)
	sysToolsCfg, _ := json.Marshal(cfg.SystemToolsConfig)
	tools, _ := json.Marshal(cfg.Tools)

	_, err := tx.Exec(ctx, `
		UPDATE agents
		SET provider=$3, model=$4, voice=$5, language=$6, timezone=$7,
		    system_prompt=$8, first_message=$9, first_message_interruptible=$10,
		    max_duration_seconds=$11, recording_enabled=$12, variables=$13,
		    initiation_webhook_url=$14, initiation_webhook_timeout_ms=$15,
		    system_tools=$16, system_tools_config=$17, tools=$18, updated_at=now()
		WHERE organization_id=$1 AND id=$2`,
		orgID, agentID, cfg.Provider, cfg.Model, cfg.Voice, cfg.Language, cfg.Timezone,
		cfg.SystemPrompt, cfg.FirstMessage, cfg.FirstMessageInterruptible,
		cfg.MaxDurationSeconds, cfg.RecordingEnabled, vars,
		cfg.InitiationWebhookURL, cfg.InitiationWebhookTimeout,
		sysTools, sysToolsCfg, tools)
	if err != nil {
		return err
	}

	// Sincroniza procedures da tabela principal com o snapshot da Main
	_, err = tx.Exec(ctx, `DELETE FROM procedures WHERE organization_id=$1 AND agent_id=$2`, orgID, agentID)
	if err != nil {
		return err
	}
	for i, p := range cfg.Procedures {
		guardBytes, _ := json.Marshal(p.Guardrails)
		pType := p.Type
		if pType == "" {
			pType = "free_form"
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO procedures (organization_id, agent_id, name, type, trigger, guardrails, content, active, position)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			orgID, agentID, p.Name, pType, p.Trigger, guardBytes, p.Content, p.Active, i+1)
		if err != nil {
			return err
		}
	}
	return nil
}

// EnableVersioning habilita permanentemente o controle de versão no agente e cria a branch "Main" com a versão inicial.
func (s *Store) EnableVersioning(ctx context.Context, orgID, agentID, userID string) (AgentBranch, AgentVersion, error) {
	var b AgentBranch
	var v AgentVersion

	a, err := s.GetAgent(ctx, orgID, agentID)
	if err != nil {
		return b, v, err
	}
	cfg, err := s.DraftConfig(ctx, orgID, a)
	if err != nil {
		return b, v, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return b, v, err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `UPDATE agents SET versioning_enabled=true, updated_at=now() WHERE organization_id=$1 AND id=$2`, orgID, agentID)
	if err != nil {
		return b, v, err
	}

	// Verifica se a branch Main já existe
	err = tx.QueryRow(ctx, `
		SELECT id, organization_id, agent_id, name, description, is_main, archived, traffic_percentage,
		       parent_version_id, head_version_id, base_main_version_id, created_at, updated_at
		FROM agent_branches
		WHERE organization_id=$1 AND agent_id=$2 AND is_main=true`, orgID, agentID).
		Scan(&b.ID, &b.OrganizationID, &b.AgentID, &b.Name, &b.Description, &b.IsMain, &b.Archived,
			&b.TrafficPercentage, &b.ParentVersionID, &b.HeadVersionID, &b.BaseMainVersionID, &b.CreatedAt, &b.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `
			INSERT INTO agent_branches (organization_id, agent_id, name, description, is_main, archived, traffic_percentage)
			VALUES ($1, $2, 'Main', 'Branch principal de produção', true, false, 100)
			RETURNING id, organization_id, agent_id, name, description, is_main, archived, traffic_percentage,
			          parent_version_id, head_version_id, base_main_version_id, created_at, updated_at`,
			orgID, agentID).
			Scan(&b.ID, &b.OrganizationID, &b.AgentID, &b.Name, &b.Description, &b.IsMain, &b.Archived,
				&b.TrafficPercentage, &b.ParentVersionID, &b.HeadVersionID, &b.BaseMainVersionID, &b.CreatedAt, &b.UpdatedAt)
	}
	if err != nil {
		return b, v, err
	}

	// Associa versões antigas sem branch_id à branch Main
	_, _ = tx.Exec(ctx, `UPDATE agent_versions SET branch_id=$1 WHERE agent_id=$2 AND branch_id IS NULL`, b.ID, agentID)

	// Busca a versão mais recente da Main; se não houver nenhuma, cria a versão inicial v1.
	var raw []byte
	err = tx.QueryRow(ctx, `
		SELECT id, version_code, agent_id, branch_id, parent_version_id, version, commit_message, snapshot, published_by, created_at
		FROM agent_versions
		WHERE organization_id=$1 AND agent_id=$2 AND branch_id=$3
		ORDER BY version DESC LIMIT 1`, orgID, agentID, b.ID).
		Scan(&v.ID, &v.VersionCode, &v.AgentID, &v.BranchID, &v.ParentVersionID, &v.Version, &v.CommitMessage, &raw, &v.PublishedBy, &v.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		snap, _ := json.Marshal(cfg)
		var uid *string
		if userID != "" {
			uid = &userID
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO agent_versions (organization_id, agent_id, branch_id, version, commit_message, snapshot, published_by)
			VALUES ($1, $2, $3, 1, 'Versão inicial ao habilitar controle de versão', $4, $5)
			RETURNING id, version_code, agent_id, branch_id, parent_version_id, version, commit_message, snapshot, published_by, created_at`,
			orgID, agentID, b.ID, snap, uid).
			Scan(&v.ID, &v.VersionCode, &v.AgentID, &v.BranchID, &v.ParentVersionID, &v.Version, &v.CommitMessage, &raw, &v.PublishedBy, &v.CreatedAt)
	}
	if err != nil {
		return b, v, err
	}
	_ = json.Unmarshal(raw, &v.Snapshot)

	_, err = tx.Exec(ctx, `
		UPDATE agent_branches
		SET head_version_id=$1, base_main_version_id=$1, updated_at=now()
		WHERE id=$2`, v.ID, b.ID)
	if err != nil {
		return b, v, err
	}
	b.HeadVersionID = &v.ID
	b.HeadVersionCode = &v.VersionCode
	b.HeadVersionNumber = &v.Version
	b.BaseMainVersionID = &v.ID
	b.BaseMainVersionCode = &v.VersionCode

	if err := tx.Commit(ctx); err != nil {
		return b, v, err
	}
	return b, v, nil
}

// GetVersionByRef localiza uma versão por version_code (agtvrsn_xxxx), UUID (id) ou número inteiro da versão.
func (s *Store) GetVersionByRef(ctx context.Context, orgID, agentID, ref string) (AgentVersion, error) {
	var v AgentVersion
	var raw []byte
	ref = strings.TrimSpace(ref)
	if num, err := strconv.Atoi(ref); err == nil {
		err = s.Pool.QueryRow(ctx, `
			SELECT id, version_code, agent_id, branch_id, parent_version_id, version, commit_message, snapshot, published_by, created_at
			FROM agent_versions
			WHERE organization_id=$1 AND agent_id=$2 AND version=$3`, orgID, agentID, num).
			Scan(&v.ID, &v.VersionCode, &v.AgentID, &v.BranchID, &v.ParentVersionID, &v.Version, &v.CommitMessage, &raw, &v.PublishedBy, &v.CreatedAt)
		if err != nil {
			return v, notFound(err)
		}
		return v, json.Unmarshal(raw, &v.Snapshot)
	}

	err := s.Pool.QueryRow(ctx, `
		SELECT id, version_code, agent_id, branch_id, parent_version_id, version, commit_message, snapshot, published_by, created_at
		FROM agent_versions
		WHERE organization_id=$1 AND agent_id=$2 AND (version_code=$3 OR id::text=$3)`, orgID, agentID, ref).
		Scan(&v.ID, &v.VersionCode, &v.AgentID, &v.BranchID, &v.ParentVersionID, &v.Version, &v.CommitMessage, &raw, &v.PublishedBy, &v.CreatedAt)
	if err != nil {
		return v, notFound(err)
	}
	return v, json.Unmarshal(raw, &v.Snapshot)
}

func (s *Store) ListBranches(ctx context.Context, orgID, agentID, userID string, includeArchived bool) ([]AgentBranch, error) {
	query := `
		SELECT b.id, b.organization_id, b.agent_id, b.name, b.description, b.is_main, b.archived, b.traffic_percentage,
		       b.parent_version_id, pv.version_code,
		       b.head_version_id, hv.version_code, hv.version,
		       b.base_main_version_id, bmv.version_code,
		       b.merged_into_branch_id, b.merged_version_id,
		       b.created_at, b.updated_at
		FROM agent_branches b
		LEFT JOIN agent_versions pv ON pv.id = b.parent_version_id
		LEFT JOIN agent_versions hv ON hv.id = b.head_version_id
		LEFT JOIN agent_versions bmv ON bmv.id = b.base_main_version_id
		WHERE b.organization_id=$1 AND b.agent_id=$2 `
	if !includeArchived {
		query += `AND b.archived = false `
	}
	query += `ORDER BY b.is_main DESC, b.archived ASC, b.created_at ASC`

	rows, err := s.Pool.Query(ctx, query, orgID, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []AgentBranch{}
	var mainHeadID string
	for rows.Next() {
		var b AgentBranch
		if err := rows.Scan(
			&b.ID, &b.OrganizationID, &b.AgentID, &b.Name, &b.Description, &b.IsMain, &b.Archived, &b.TrafficPercentage,
			&b.ParentVersionID, &b.ParentVersionCode,
			&b.HeadVersionID, &b.HeadVersionCode, &b.HeadVersionNumber,
			&b.BaseMainVersionID, &b.BaseMainVersionCode,
			&b.MergedIntoBranchID, &b.MergedVersionID,
			&b.CreatedAt, &b.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if b.IsMain && b.HeadVersionID != nil {
			mainHeadID = *b.HeadVersionID
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Carrega versões por branch e drafts do usuário
	allVers, _ := s.ListVersions(ctx, orgID, agentID)
	versByBranch := map[string][]VersionSummary{}
	for _, v := range allVers {
		if v.BranchID != nil {
			versByBranch[*v.BranchID] = append(versByBranch[*v.BranchID], v)
		}
	}

	draftsByBranch := map[string]time.Time{}
	if userID != "" {
		dRows, err := s.Pool.Query(ctx, `
			SELECT branch_id, updated_at FROM agent_branch_drafts
			WHERE organization_id=$1 AND agent_id=$2 AND user_id=$3`, orgID, agentID, userID)
		if err == nil {
			defer dRows.Close()
			for dRows.Next() {
				var bid string
				var ts time.Time
				if dRows.Scan(&bid, &ts) == nil {
					draftsByBranch[bid] = ts
				}
			}
		}
	}

	for i := range out {
		b := &out[i]
		b.Versions = versByBranch[b.ID]
		if b.Versions == nil {
			b.Versions = []VersionSummary{}
		}
		b.VersionsCount = len(b.Versions)
		if ts, ok := draftsByBranch[b.ID]; ok {
			b.HasDraft = true
			tCopy := ts
			b.DraftUpdatedAt = &tCopy
		}
		if !b.IsMain {
			if mainHeadID != "" && (b.BaseMainVersionID == nil || *b.BaseMainVersionID != mainHeadID) {
				b.BehindMain = true
			}
			if b.HeadVersionID != nil && (b.ParentVersionID == nil || *b.HeadVersionID != *b.ParentVersionID || b.VersionsCount > 0) {
				if b.MergedVersionID == nil || *b.MergedVersionID != *b.HeadVersionID {
					b.AheadOfMain = true
				}
			}
		}
	}
	return out, nil
}

func (s *Store) GetBranch(ctx context.Context, orgID, agentID, branchRef, userID string) (AgentBranch, error) {
	list, err := s.ListBranches(ctx, orgID, agentID, userID, true)
	if err != nil {
		return AgentBranch{}, err
	}
	for _, b := range list {
		if b.ID == branchRef || strings.EqualFold(b.Name, branchRef) {
			return b, nil
		}
	}
	return AgentBranch{}, ErrNotFound
}

// CreateBranch cria uma nova branch a partir de qualquer versão de qualquer branch existente.
func (s *Store) CreateBranch(ctx context.Context, orgID, agentID, userID, parentVersionRef, name, description string, overrides *AgentConfig) (AgentBranch, AgentVersion, error) {
	var createdBranch AgentBranch
	var createdVersion AgentVersion

	name = strings.TrimSpace(name)
	if err := ValidateBranchName(name); err != nil {
		return createdBranch, createdVersion, err
	}

	// Garante que o controle de versão está habilitado
	a, err := s.GetAgent(ctx, orgID, agentID)
	if err != nil {
		return createdBranch, createdVersion, err
	}
	if !a.VersioningEnabled {
		if _, _, err := s.EnableVersioning(ctx, orgID, agentID, userID); err != nil {
			return createdBranch, createdVersion, err
		}
	}

	// Resolve a versão pai (ou usa o head da Main se omitido)
	var parentVer AgentVersion
	if strings.TrimSpace(parentVersionRef) == "" {
		mainB, err := s.GetBranch(ctx, orgID, agentID, "Main", userID)
		if err != nil || mainB.HeadVersionID == nil {
			return createdBranch, createdVersion, ErrNotFound
		}
		parentVer, err = s.GetVersionByRef(ctx, orgID, agentID, *mainB.HeadVersionID)
		if err != nil {
			return createdBranch, createdVersion, err
		}
	} else {
		parentVer, err = s.GetVersionByRef(ctx, orgID, agentID, parentVersionRef)
		if err != nil {
			return createdBranch, createdVersion, err
		}
	}

	// Determina base_main_version_id
	baseMainVerID := parentVer.ID
	if parentVer.BranchID != nil {
		if pb, err := s.GetBranch(ctx, orgID, agentID, *parentVer.BranchID, userID); err == nil {
			if pb.IsMain {
				baseMainVerID = parentVer.ID
			} else if pb.BaseMainVersionID != nil {
				baseMainVerID = *pb.BaseMainVersionID
			}
		}
	}

	initialSnap := parentVer.Snapshot
	if overrides != nil {
		initialSnap = *overrides
		initialSnap.Name = a.Name // per-agent setting preservado
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return createdBranch, createdVersion, err
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx, `
		INSERT INTO agent_branches (organization_id, agent_id, name, description, is_main, archived, traffic_percentage, parent_version_id, base_main_version_id)
		VALUES ($1, $2, $3, $4, false, false, 0, $5, $6)
		RETURNING id, organization_id, agent_id, name, description, is_main, archived, traffic_percentage,
		          parent_version_id, base_main_version_id, created_at, updated_at`,
		orgID, agentID, name, strings.TrimSpace(description), parentVer.ID, baseMainVerID).
		Scan(&createdBranch.ID, &createdBranch.OrganizationID, &createdBranch.AgentID, &createdBranch.Name,
			&createdBranch.Description, &createdBranch.IsMain, &createdBranch.Archived, &createdBranch.TrafficPercentage,
			&createdBranch.ParentVersionID, &createdBranch.BaseMainVersionID, &createdBranch.CreatedAt, &createdBranch.UpdatedAt)
	if err != nil {
		return createdBranch, createdVersion, err
	}

	snapBytes, _ := json.Marshal(initialSnap)
	var raw []byte
	var uid *string
	if userID != "" {
		uid = &userID
	}
	commitMsg := fmt.Sprintf("Branch criada a partir de %s (v%d)", parentVer.VersionCode, parentVer.Version)
	err = tx.QueryRow(ctx, `
		INSERT INTO agent_versions (organization_id, agent_id, branch_id, parent_version_id, version, commit_message, snapshot, published_by)
		VALUES ($1, $2, $3, $4, (SELECT coalesce(max(version), 0) + 1 FROM agent_versions WHERE agent_id=$2), $5, $6, $7)
		RETURNING id, version_code, agent_id, branch_id, parent_version_id, version, commit_message, snapshot, published_by, created_at`,
		orgID, agentID, createdBranch.ID, parentVer.ID, commitMsg, snapBytes, uid).
		Scan(&createdVersion.ID, &createdVersion.VersionCode, &createdVersion.AgentID, &createdVersion.BranchID,
			&createdVersion.ParentVersionID, &createdVersion.Version, &createdVersion.CommitMessage, &raw, &createdVersion.PublishedBy, &createdVersion.CreatedAt)
	if err != nil {
		return createdBranch, createdVersion, err
	}
	_ = json.Unmarshal(raw, &createdVersion.Snapshot)

	_, err = tx.Exec(ctx, `UPDATE agent_branches SET head_version_id=$1, updated_at=now() WHERE id=$2`, createdVersion.ID, createdBranch.ID)
	if err != nil {
		return createdBranch, createdVersion, err
	}
	createdBranch.HeadVersionID = &createdVersion.ID
	createdBranch.HeadVersionCode = &createdVersion.VersionCode
	createdBranch.HeadVersionNumber = &createdVersion.Version
	createdBranch.ParentVersionCode = &parentVer.VersionCode

	if err := tx.Commit(ctx); err != nil {
		return createdBranch, createdVersion, err
	}
	return createdBranch, createdVersion, nil
}

// UpdateBranch atualiza descrição e/ou estado arquivado de uma branch.
func (s *Store) UpdateBranch(ctx context.Context, orgID, agentID, branchID, userID string, description *string, archived *bool) (AgentBranch, error) {
	b, err := s.GetBranch(ctx, orgID, agentID, branchID, userID)
	if err != nil {
		return b, err
	}
	if archived != nil && *archived {
		if b.IsMain {
			return b, ErrCannotArchiveMain
		}
		if b.TrafficPercentage > 0 {
			return b, ErrCannotArchiveTraffic
		}
	}
	newDesc := b.Description
	if description != nil {
		newDesc = strings.TrimSpace(*description)
	}
	newArch := b.Archived
	if archived != nil {
		newArch = *archived
	}
	_, err = s.Pool.Exec(ctx, `
		UPDATE agent_branches SET description=$1, archived=$2, updated_at=now()
		WHERE organization_id=$3 AND agent_id=$4 AND id=$5`,
		newDesc, newArch, orgID, agentID, b.ID)
	if err != nil {
		return b, err
	}
	return s.GetBranch(ctx, orgID, agentID, b.ID, userID)
}

// SaveBranchDraft salva um rascunho por usuário e por branch.
func (s *Store) SaveBranchDraft(ctx context.Context, orgID, agentID, branchRef, userID string, cfg AgentConfig) error {
	b, err := s.GetBranch(ctx, orgID, agentID, branchRef, userID)
	if err != nil {
		return err
	}
	if b.Archived {
		return ErrBranchArchived
	}
	if !b.IsMain {
		if a, err := s.GetAgent(ctx, orgID, agentID); err == nil {
			cfg.Name = a.Name
		}
	}
	snap, _ := json.Marshal(cfg)
	_, err = s.Pool.Exec(ctx, `
		INSERT INTO agent_branch_drafts (organization_id, agent_id, branch_id, user_id, snapshot, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (agent_id, branch_id, user_id)
		DO UPDATE SET snapshot = EXCLUDED.snapshot, updated_at = now()`,
		orgID, agentID, b.ID, userID, snap)
	return err
}

func (s *Store) GetBranchDraft(ctx context.Context, orgID, agentID, branchRef, userID string) (*AgentConfig, *time.Time, error) {
	b, err := s.GetBranch(ctx, orgID, agentID, branchRef, userID)
	if err != nil {
		return nil, nil, err
	}
	var raw []byte
	var updatedAt time.Time
	err = s.Pool.QueryRow(ctx, `
		SELECT snapshot, updated_at FROM agent_branch_drafts
		WHERE organization_id=$1 AND agent_id=$2 AND branch_id=$3 AND user_id=$4`,
		orgID, agentID, b.ID, userID).Scan(&raw, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var cfg AgentConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, nil, err
	}
	return &cfg, &updatedAt, nil
}

func (s *Store) DeleteBranchDraft(ctx context.Context, orgID, agentID, branchRef, userID string) error {
	b, err := s.GetBranch(ctx, orgID, agentID, branchRef, userID)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `
		DELETE FROM agent_branch_drafts
		WHERE organization_id=$1 AND agent_id=$2 AND branch_id=$3 AND user_id=$4`,
		orgID, agentID, b.ID, userID)
	return err
}

// DeployTraffic distribui o tráfego percentual entre as branches (a soma deve ser exatamente 100%).
func (s *Store) DeployTraffic(ctx context.Context, orgID, agentID, userID string, allocations []TrafficAllocation) ([]AgentBranch, error) {
	total := 0
	byBranch := map[string]int{}
	for _, al := range allocations {
		if al.Percentage < 0 || al.Percentage > 100 {
			return nil, ErrInvalidTrafficSplit
		}
		total += al.Percentage
		byBranch[strings.TrimSpace(al.BranchID)] = al.Percentage
	}
	if total != 100 {
		return nil, ErrInvalidTrafficSplit
	}

	branches, err := s.ListBranches(ctx, orgID, agentID, userID, true)
	if err != nil {
		return nil, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	matchedTotal := 0
	for _, b := range branches {
		pct := 0
		if v, ok := byBranch[b.ID]; ok {
			pct = v
		} else if v, ok := byBranch[b.Name]; ok {
			pct = v
		}
		if pct > 0 && b.Archived {
			return nil, ErrBranchArchived
		}
		matchedTotal += pct
		if _, err := tx.Exec(ctx, `
			UPDATE agent_branches SET traffic_percentage=$1, updated_at=now()
			WHERE organization_id=$2 AND agent_id=$3 AND id=$4`,
			pct, orgID, agentID, b.ID); err != nil {
			return nil, err
		}
	}
	if matchedTotal != 100 {
		return nil, ErrInvalidTrafficSplit
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.ListBranches(ctx, orgID, agentID, userID, true)
}

// RouteVersionForConversation faz o roteamento determinístico baseado no conversation_id / call_sid.
func (s *Store) RouteVersionForConversation(ctx context.Context, orgID string, ag Agent, routingKey string) (AgentConfig, *string, *string, error) {
	if ag.VersioningEnabled {
		branches, err := s.ListBranches(ctx, orgID, ag.ID, "", false)
		if err == nil && len(branches) > 0 {
			h := fnv.New32a()
			if routingKey == "" {
				routingKey = ag.ID
			}
			_, _ = h.Write([]byte(routingKey))
			bucket := int(h.Sum32() % 100)

			cum := 0
			var chosen *AgentBranch
			for i := range branches {
				if branches[i].TrafficPercentage <= 0 {
					continue
				}
				cum += branches[i].TrafficPercentage
				if bucket < cum {
					chosen = &branches[i]
					break
				}
			}
			if chosen == nil {
				for i := range branches {
					if branches[i].IsMain {
						chosen = &branches[i]
						break
					}
				}
			}
			if chosen != nil && chosen.HeadVersionID != nil {
				if ver, err := s.GetVersionByRef(ctx, orgID, ag.ID, *chosen.HeadVersionID); err == nil {
					bID := chosen.ID
					return ver.Snapshot, &ver.ID, &bID, nil
				}
			}
		}
	}

	v, err := s.LatestVersion(ctx, orgID, ag.ID)
	if err == nil {
		return v.Snapshot, &v.ID, v.BranchID, nil
	}
	cfg, err := s.DraftConfig(ctx, orgID, ag)
	return cfg, nil, nil, err
}

type versionedFieldAccessor struct {
	Key   string
	Label string
	Get   func(c AgentConfig) any
	Set   func(c *AgentConfig, src AgentConfig)
}

var versionedFields = []versionedFieldAccessor{
	{Key: "system_prompt", Label: "System prompt", Get: func(c AgentConfig) any { return c.SystemPrompt }, Set: func(c *AgentConfig, s AgentConfig) { c.SystemPrompt = s.SystemPrompt }},
	{Key: "first_message", Label: "Primeira mensagem", Get: func(c AgentConfig) any { return c.FirstMessage }, Set: func(c *AgentConfig, s AgentConfig) { c.FirstMessage = s.FirstMessage }},
	{Key: "first_message_interruptible", Label: "Primeira mensagem interrompível", Get: func(c AgentConfig) any { return c.FirstMessageInterruptible }, Set: func(c *AgentConfig, s AgentConfig) { c.FirstMessageInterruptible = s.FirstMessageInterruptible }},
	{Key: "provider", Label: "Provedor de Voz/LLM", Get: func(c AgentConfig) any { return c.Provider }, Set: func(c *AgentConfig, s AgentConfig) { c.Provider = s.Provider }},
	{Key: "model", Label: "Modelo LLM", Get: func(c AgentConfig) any { return c.Model }, Set: func(c *AgentConfig, s AgentConfig) { c.Model = s.Model }},
	{Key: "voice", Label: "Voz TTS", Get: func(c AgentConfig) any { return c.Voice }, Set: func(c *AgentConfig, s AgentConfig) { c.Voice = s.Voice }},
	{Key: "language", Label: "Idioma", Get: func(c AgentConfig) any { return c.Language }, Set: func(c *AgentConfig, s AgentConfig) { c.Language = s.Language }},
	{Key: "timezone", Label: "Fuso horário", Get: func(c AgentConfig) any { return c.Timezone }, Set: func(c *AgentConfig, s AgentConfig) { c.Timezone = s.Timezone }},
	{Key: "max_duration_seconds", Label: "Duração máxima (s)", Get: func(c AgentConfig) any { return c.MaxDurationSeconds }, Set: func(c *AgentConfig, s AgentConfig) { c.MaxDurationSeconds = s.MaxDurationSeconds }},
	{Key: "recording_enabled", Label: "Gravação de áudio", Get: func(c AgentConfig) any { return c.RecordingEnabled }, Set: func(c *AgentConfig, s AgentConfig) { c.RecordingEnabled = s.RecordingEnabled }},
	{Key: "variables", Label: "Variáveis dinâmicas", Get: func(c AgentConfig) any { return c.Variables }, Set: func(c *AgentConfig, s AgentConfig) { c.Variables = s.Variables }},
	{Key: "initiation_webhook_url", Label: "Webhook de inicialização", Get: func(c AgentConfig) any { return c.InitiationWebhookURL }, Set: func(c *AgentConfig, s AgentConfig) { c.InitiationWebhookURL = s.InitiationWebhookURL }},
	{Key: "initiation_webhook_timeout_ms", Label: "Timeout do webhook (ms)", Get: func(c AgentConfig) any { return c.InitiationWebhookTimeout }, Set: func(c *AgentConfig, s AgentConfig) { c.InitiationWebhookTimeout = s.InitiationWebhookTimeout }},
	{Key: "tools", Label: "Ferramentas vinculadas", Get: func(c AgentConfig) any { return c.Tools }, Set: func(c *AgentConfig, s AgentConfig) { c.Tools = s.Tools }},
	{Key: "system_tools", Label: "Ferramentas de sistema", Get: func(c AgentConfig) any { return c.SystemTools }, Set: func(c *AgentConfig, s AgentConfig) { c.SystemTools = s.SystemTools }},
	{Key: "system_tools_config", Label: "Configurações de segurança e transferência", Get: func(c AgentConfig) any { return c.SystemToolsConfig }, Set: func(c *AgentConfig, s AgentConfig) { c.SystemToolsConfig = s.SystemToolsConfig }},
	{Key: "procedures", Label: "Fluxo de procedimentos (Workflow)", Get: func(c AgentConfig) any { return c.Procedures }, Set: func(c *AgentConfig, s AgentConfig) { c.Procedures = s.Procedures }},
}

func jsonEqual(a, b any) bool {
	ba, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return bytes.Equal(ba, bb)
}

func computeMerge(base, source, target AgentConfig, sourceNewer, force bool) (AgentConfig, []FieldDiff, []FieldDiff) {
	merged := target
	overridden := []FieldDiff{}
	conflicts := []FieldDiff{}

	for _, f := range versionedFields {
		bVal := f.Get(base)
		sVal := f.Get(source)
		tVal := f.Get(target)

		if jsonEqual(sVal, tVal) {
			continue
		}
		sourceChanged := !jsonEqual(sVal, bVal)
		targetChanged := !jsonEqual(tVal, bVal)

		if sourceChanged && !targetChanged {
			f.Set(&merged, source)
			overridden = append(overridden, FieldDiff{
				Field:         f.Key,
				Label:         f.Label,
				TargetValue:   tVal,
				SourceValue:   sVal,
				ResolvedValue: sVal,
				Winner:        "source",
			})
		} else if sourceChanged && targetChanged {
			winner := "target"
			resolved := tVal
			if force || sourceNewer {
				f.Set(&merged, source)
				winner = "source"
				resolved = sVal
			}
			diff := FieldDiff{
				Field:         f.Key,
				Label:         f.Label,
				TargetValue:   tVal,
				SourceValue:   sVal,
				ResolvedValue: resolved,
				Winner:        winner,
			}
			conflicts = append(conflicts, diff)
			overridden = append(overridden, diff)
		}
	}
	return merged, overridden, conflicts
}

func (s *Store) PreviewMerge(ctx context.Context, orgID, agentID, userID, sourceBranchRef, targetBranchRef string, force bool) (MergePreview, AgentConfig, error) {
	var preview MergePreview
	var mergedCfg AgentConfig

	srcB, err := s.GetBranch(ctx, orgID, agentID, sourceBranchRef, userID)
	if err != nil {
		return preview, mergedCfg, err
	}
	if targetBranchRef == "" {
		targetBranchRef = "Main"
	}
	tgtB, err := s.GetBranch(ctx, orgID, agentID, targetBranchRef, userID)
	if err != nil {
		return preview, mergedCfg, err
	}

	preview.SourceBranchID = srcB.ID
	preview.SourceBranchName = srcB.Name
	preview.TargetBranchID = tgtB.ID
	preview.TargetBranchName = tgtB.Name
	preview.OverriddenFields = []FieldDiff{}
	preview.Conflicts = []FieldDiff{}

	if srcB.HeadVersionID == nil || tgtB.HeadVersionID == nil {
		return preview, mergedCfg, ErrNoNewChangesToMerge
	}
	if srcB.MergedIntoBranchID != nil && *srcB.MergedIntoBranchID == tgtB.ID &&
		srcB.MergedVersionID != nil && *srcB.MergedVersionID == *srcB.HeadVersionID {
		return preview, mergedCfg, ErrBranchAlreadyMerged
	}

	srcVer, err := s.GetVersionByRef(ctx, orgID, agentID, *srcB.HeadVersionID)
	if err != nil {
		return preview, mergedCfg, err
	}
	tgtVer, err := s.GetVersionByRef(ctx, orgID, agentID, *tgtB.HeadVersionID)
	if err != nil {
		return preview, mergedCfg, err
	}

	baseVer := tgtVer
	if srcB.BaseMainVersionID != nil && tgtB.IsMain {
		if bv, err := s.GetVersionByRef(ctx, orgID, agentID, *srcB.BaseMainVersionID); err == nil {
			baseVer = bv
		}
	} else if srcB.ParentVersionID != nil {
		if pv, err := s.GetVersionByRef(ctx, orgID, agentID, *srcB.ParentVersionID); err == nil {
			baseVer = pv
		}
	}

	// Se não há alterações da source em relação à base nem em relação ao target
	if jsonEqual(srcVer.Snapshot, baseVer.Snapshot) || jsonEqual(srcVer.Snapshot, tgtVer.Snapshot) {
		return preview, mergedCfg, ErrNoNewChangesToMerge
	}

	sourceNewer := srcVer.CreatedAt.After(tgtVer.CreatedAt) || srcVer.Version > tgtVer.Version
	mergedCfg, overridden, conflicts := computeMerge(baseVer.Snapshot, srcVer.Snapshot, tgtVer.Snapshot, sourceNewer, force)
	if len(overridden) == 0 && len(conflicts) == 0 {
		return preview, mergedCfg, ErrNoNewChangesToMerge
	}

	preview.OverriddenFields = overridden
	preview.Conflicts = conflicts
	preview.CanMerge = true
	return preview, mergedCfg, nil
}

// MergeBranch realiza o merge de sourceBranch para targetBranch conforme a especificação ElevenLabs.
func (s *Store) MergeBranch(ctx context.Context, orgID, agentID, userID, sourceBranchRef, targetBranchRef string, archiveSource, force bool) (MergeResult, error) {
	var res MergeResult
	preview, mergedCfg, err := s.PreviewMerge(ctx, orgID, agentID, userID, sourceBranchRef, targetBranchRef, force)
	if err != nil {
		return res, err
	}

	srcB, err := s.GetBranch(ctx, orgID, agentID, preview.SourceBranchID, userID)
	if err != nil {
		return res, err
	}
	tgtB, err := s.GetBranch(ctx, orgID, agentID, preview.TargetBranchID, userID)
	if err != nil {
		return res, err
	}

	// Regra ElevenLabs: per-agent settings (name) pertencem ao agente / Main
	if a, err := s.GetAgent(ctx, orgID, agentID); err == nil {
		mergedCfg.Name = a.Name
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return res, err
	}
	defer tx.Rollback(ctx)

	snapBytes, _ := json.Marshal(mergedCfg)
	var raw []byte
	var uid *string
	if userID != "" {
		uid = &userID
	}
	commitMsg := fmt.Sprintf("Merge da branch '%s' (%s) em '%s'", srcB.Name, srcB.ID, tgtB.Name)

	var newVer AgentVersion
	err = tx.QueryRow(ctx, `
		INSERT INTO agent_versions (organization_id, agent_id, branch_id, parent_version_id, version, commit_message, snapshot, published_by)
		VALUES ($1, $2, $3, $4, (SELECT coalesce(max(version), 0) + 1 FROM agent_versions WHERE agent_id=$2), $5, $6, $7)
		RETURNING id, version_code, agent_id, branch_id, parent_version_id, version, commit_message, snapshot, published_by, created_at`,
		orgID, agentID, tgtB.ID, tgtB.HeadVersionID, commitMsg, snapBytes, uid).
		Scan(&newVer.ID, &newVer.VersionCode, &newVer.AgentID, &newVer.BranchID,
			&newVer.ParentVersionID, &newVer.Version, &newVer.CommitMessage, &raw, &newVer.PublishedBy, &newVer.CreatedAt)
	if err != nil {
		return res, err
	}
	_ = json.Unmarshal(raw, &newVer.Snapshot)

	// Transfere automaticamente o tráfego da source branch para a target branch
	transferredTraffic := srcB.TrafficPercentage
	newTargetTraffic := tgtB.TrafficPercentage + transferredTraffic

	if tgtB.IsMain {
		_, err = tx.Exec(ctx, `
			UPDATE agent_branches
			SET head_version_id=$1, base_main_version_id=$1, traffic_percentage=$2, updated_at=now()
			WHERE id=$3`, newVer.ID, newTargetTraffic, tgtB.ID)
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE agent_branches
			SET head_version_id=$1, traffic_percentage=$2, updated_at=now()
			WHERE id=$3`, newVer.ID, newTargetTraffic, tgtB.ID)
	}
	if err != nil {
		return res, err
	}

	shouldArchiveSrc := archiveSource && !srcB.IsMain
	_, err = tx.Exec(ctx, `
		UPDATE agent_branches
		SET traffic_percentage=0, archived=$1, merged_into_branch_id=$2, merged_version_id=$3, updated_at=now()
		WHERE id=$4`,
		shouldArchiveSrc, tgtB.ID, srcB.HeadVersionID, srcB.ID)
	if err != nil {
		return res, err
	}

	// Descarta todos os drafts na branch de destino (regra ElevenLabs: "Drafts are also discarded when merging into a branch")
	_, _ = tx.Exec(ctx, `DELETE FROM agent_branch_drafts WHERE agent_id=$1 AND branch_id=$2`, agentID, tgtB.ID)

	if tgtB.IsMain {
		if err := syncMainTablesTx(ctx, tx, orgID, agentID, mergedCfg); err != nil {
			return res, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return res, err
	}

	res.CreatedVersion = newVer
	res.TransferredTraffic = transferredTraffic
	res.SourceBranch, _ = s.GetBranch(ctx, orgID, agentID, srcB.ID, userID)
	res.TargetBranch, _ = s.GetBranch(ctx, orgID, agentID, tgtB.ID, userID)
	return res, nil
}

// PreviewRebase calcula quais alterações da Main serão puxadas para a branch sem sobrescrever as edições da própria branch.
func (s *Store) PreviewRebase(ctx context.Context, orgID, agentID, userID, branchRef string) (RebasePreview, AgentConfig, string, error) {
	var preview RebasePreview
	var rebasedCfg AgentConfig

	b, err := s.GetBranch(ctx, orgID, agentID, branchRef, userID)
	if err != nil {
		return preview, rebasedCfg, "", err
	}
	if b.IsMain {
		return preview, rebasedCfg, "", ErrCannotRebaseMain
	}
	mainB, err := s.GetBranch(ctx, orgID, agentID, "Main", userID)
	if err != nil || mainB.HeadVersionID == nil || b.HeadVersionID == nil {
		return preview, rebasedCfg, "", ErrBranchAlreadyUpToDate
	}

	if b.BaseMainVersionID != nil && *b.BaseMainVersionID == *mainB.HeadVersionID {
		return preview, rebasedCfg, "", ErrBranchAlreadyUpToDate
	}

	mainVer, err := s.GetVersionByRef(ctx, orgID, agentID, *mainB.HeadVersionID)
	if err != nil {
		return preview, rebasedCfg, "", err
	}
	branchVer, err := s.GetVersionByRef(ctx, orgID, agentID, *b.HeadVersionID)
	if err != nil {
		return preview, rebasedCfg, "", err
	}

	baseVer := mainVer
	if b.BaseMainVersionID != nil {
		if bv, err := s.GetVersionByRef(ctx, orgID, agentID, *b.BaseMainVersionID); err == nil {
			baseVer = bv
		}
	} else if b.ParentVersionID != nil {
		if pv, err := s.GetVersionByRef(ctx, orgID, agentID, *b.ParentVersionID); err == nil {
			baseVer = pv
		}
	}

	preview.BranchID = b.ID
	preview.BranchName = b.Name
	preview.MainVersionID = mainVer.VersionCode
	preview.OverriddenFields = []FieldDiff{}
	preview.PreservedBranchFields = []string{}

	rebasedCfg = branchVer.Snapshot
	rebasedCfg.Name = mainVer.Snapshot.Name

	for _, f := range versionedFields {
		baseVal := f.Get(baseVer.Snapshot)
		mainVal := f.Get(mainVer.Snapshot)
		branchVal := f.Get(branchVer.Snapshot)

		mainChanged := !jsonEqual(mainVal, baseVal)
		branchChanged := !jsonEqual(branchVal, baseVal)

		if mainChanged && !branchChanged && !jsonEqual(mainVal, branchVal) {
			f.Set(&rebasedCfg, mainVer.Snapshot)
			preview.OverriddenFields = append(preview.OverriddenFields, FieldDiff{
				Field:         f.Key,
				Label:         f.Label,
				TargetValue:   branchVal,
				SourceValue:   mainVal,
				ResolvedValue: mainVal,
				Winner:        "main",
			})
		} else if mainChanged && branchChanged {
			// Regra ElevenLabs: "Preserves the branch's own changes: if a setting was edited on both the branch and main, the branch's value is always kept"
			preview.PreservedBranchFields = append(preview.PreservedBranchFields, f.Label)
		}
	}

	preview.CanRebase = true
	return preview, rebasedCfg, mainVer.ID, nil
}

// RebaseBranch puxa as alterações mais recentes da Main para uma branch secundária preservando as alterações da branch.
func (s *Store) RebaseBranch(ctx context.Context, orgID, agentID, userID, branchRef string) (AgentVersion, error) {
	var newVer AgentVersion
	preview, rebasedCfg, mainHeadUUID, err := s.PreviewRebase(ctx, orgID, agentID, userID, branchRef)
	if err != nil {
		return newVer, err
	}

	b, err := s.GetBranch(ctx, orgID, agentID, preview.BranchID, userID)
	if err != nil {
		return newVer, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return newVer, err
	}
	defer tx.Rollback(ctx)

	snapBytes, _ := json.Marshal(rebasedCfg)
	var raw []byte
	var uid *string
	if userID != "" {
		uid = &userID
	}
	commitMsg := fmt.Sprintf("Rebase com a branch Main (%s)", preview.MainVersionID)

	err = tx.QueryRow(ctx, `
		INSERT INTO agent_versions (organization_id, agent_id, branch_id, parent_version_id, version, commit_message, snapshot, published_by)
		VALUES ($1, $2, $3, $4, (SELECT coalesce(max(version), 0) + 1 FROM agent_versions WHERE agent_id=$2), $5, $6, $7)
		RETURNING id, version_code, agent_id, branch_id, parent_version_id, version, commit_message, snapshot, published_by, created_at`,
		orgID, agentID, b.ID, b.HeadVersionID, commitMsg, snapBytes, uid).
		Scan(&newVer.ID, &newVer.VersionCode, &newVer.AgentID, &newVer.BranchID,
			&newVer.ParentVersionID, &newVer.Version, &newVer.CommitMessage, &raw, &newVer.PublishedBy, &newVer.CreatedAt)
	if err != nil {
		return newVer, err
	}
	_ = json.Unmarshal(raw, &newVer.Snapshot)

	_, err = tx.Exec(ctx, `
		UPDATE agent_branches
		SET head_version_id=$1, base_main_version_id=$2, updated_at=now()
		WHERE id=$3`, newVer.ID, mainHeadUUID, b.ID)
	if err != nil {
		return newVer, err
	}

	_, _ = tx.Exec(ctx, `DELETE FROM agent_branch_drafts WHERE agent_id=$1 AND branch_id=$2`, agentID, b.ID)

	if err := tx.Commit(ctx); err != nil {
		return newVer, err
	}
	return newVer, nil
}

// DuplicateAgent duplica um agente seguindo a especificação ElevenLabs:
// - Se o agente possui controle de versão habilitado, copia todas as branches não arquivadas (Main + secundárias),
//   cada uma iniciando com uma única versão correspondente à última versão daquela branch.
// - Branches arquivadas e drafts não são copiados.
// - 100% do tráfego vai para a branch Main.
func (s *Store) DuplicateAgent(ctx context.Context, orgID, sourceAgentID, userID, newName string) (Agent, error) {
	srcAgent, err := s.GetAgent(ctx, orgID, sourceAgentID)
	if err != nil {
		return Agent{}, err
	}
	newName = strings.TrimSpace(newName)
	if newName == "" {
		newName = srcAgent.Name + " (Cópia)"
	}

	newAgent, err := s.CreateAgent(ctx, orgID, newName, srcAgent.Description, srcAgent.Provider, srcAgent.Model, srcAgent.Voice)
	if err != nil {
		return Agent{}, err
	}

	mainCfg, err := s.DraftConfig(ctx, orgID, srcAgent)
	if err != nil {
		return Agent{}, err
	}
	mainCfg.Name = newName

	if !srcAgent.VersioningEnabled {
		tx, err := s.Pool.Begin(ctx)
		if err != nil {
			return Agent{}, err
		}
		defer tx.Rollback(ctx)
		if err := syncMainTablesTx(ctx, tx, orgID, newAgent.ID, mainCfg); err != nil {
			return Agent{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Agent{}, err
		}
		return s.GetAgent(ctx, orgID, newAgent.ID)
	}

	// Agente com versioning habilitado
	srcBranches, err := s.ListBranches(ctx, orgID, sourceAgentID, userID, false)
	if err != nil {
		return Agent{}, err
	}

	// Encontra o snapshot mais recente da Main de origem
	for _, b := range srcBranches {
		if b.IsMain && b.HeadVersionID != nil {
			if v, err := s.GetVersionByRef(ctx, orgID, sourceAgentID, *b.HeadVersionID); err == nil {
				mainCfg = v.Snapshot
				mainCfg.Name = newName
			}
			break
		}
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Agent{}, err
	}
	defer tx.Rollback(ctx)

	if err := syncMainTablesTx(ctx, tx, orgID, newAgent.ID, mainCfg); err != nil {
		return Agent{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE agents SET versioning_enabled=true WHERE id=$1`, newAgent.ID)
	if err != nil {
		return Agent{}, err
	}

	var uid *string
	if userID != "" {
		uid = &userID
	}

	// 1. Cria a branch Main no agente duplicado com 100% do tráfego e 1 versão inicial
	var newMainBranchID string
	err = tx.QueryRow(ctx, `
		INSERT INTO agent_branches (organization_id, agent_id, name, description, is_main, archived, traffic_percentage)
		VALUES ($1, $2, 'Main', 'Branch principal de produção', true, false, 100)
		RETURNING id`, orgID, newAgent.ID).Scan(&newMainBranchID)
	if err != nil {
		return Agent{}, err
	}

	mainSnapBytes, _ := json.Marshal(mainCfg)
	var newMainVerID string
	err = tx.QueryRow(ctx, `
		INSERT INTO agent_versions (organization_id, agent_id, branch_id, version, commit_message, snapshot, published_by)
		VALUES ($1, $2, $3, 1, 'Versão inicial (duplicada da Main)', $4, $5)
		RETURNING id`, orgID, newAgent.ID, newMainBranchID, mainSnapBytes, uid).Scan(&newMainVerID)
	if err != nil {
		return Agent{}, err
	}

	_, err = tx.Exec(ctx, `
		UPDATE agent_branches SET head_version_id=$1, base_main_version_id=$1 WHERE id=$2`,
		newMainVerID, newMainBranchID)
	if err != nil {
		return Agent{}, err
	}

	// 2. Recria cada outra branch não arquivada com 0% de tráfego e 1 única versão inicial
	verNum := 1
	for _, sb := range srcBranches {
		if sb.IsMain || sb.Archived {
			continue
		}
		branchSnap := mainCfg
		if sb.HeadVersionID != nil {
			if sv, err := s.GetVersionByRef(ctx, orgID, sourceAgentID, *sb.HeadVersionID); err == nil {
				branchSnap = sv.Snapshot
				branchSnap.Name = newName
			}
		}
		var newBranchID string
		err := tx.QueryRow(ctx, `
			INSERT INTO agent_branches (organization_id, agent_id, name, description, is_main, archived, traffic_percentage, parent_version_id, base_main_version_id)
			VALUES ($1, $2, $3, $4, false, false, 0, $5, $5)
			RETURNING id`,
			orgID, newAgent.ID, sb.Name, sb.Description, newMainVerID).Scan(&newBranchID)
		if err != nil {
			return Agent{}, err
		}
		verNum++
		bSnapBytes, _ := json.Marshal(branchSnap)
		var newBranchVerID string
		err = tx.QueryRow(ctx, `
			INSERT INTO agent_versions (organization_id, agent_id, branch_id, parent_version_id, version, commit_message, snapshot, published_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id`,
			orgID, newAgent.ID, newBranchID, newMainVerID, verNum,
			fmt.Sprintf("Versão inicial (duplicada da branch %s)", sb.Name), bSnapBytes, uid).Scan(&newBranchVerID)
		if err != nil {
			return Agent{}, err
		}
		_, err = tx.Exec(ctx, `UPDATE agent_branches SET head_version_id=$1 WHERE id=$2`, newBranchVerID, newBranchID)
		if err != nil {
			return Agent{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Agent{}, err
	}
	return s.GetAgent(ctx, orgID, newAgent.ID)
}
