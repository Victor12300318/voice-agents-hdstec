package store

import (
	"context"
	"strings"
	"time"
)

type KnowledgeBase struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	DocumentsCount int       `json:"documents_count"`
	TotalBytes     int       `json:"total_bytes"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type KnowledgeDocument struct {
	ID              string    `json:"id"`
	OrganizationID  string    `json:"organization_id"`
	KnowledgeBaseID string    `json:"knowledge_base_id"`
	Name            string    `json:"name"`
	Content         string    `json:"content"`
	CreatedAt       time.Time `json:"created_at"`
}

const kbCols = `id, organization_id, name, description, created_at, updated_at`

func (s *Store) CreateKnowledgeBase(ctx context.Context, orgID, name, description string) (KnowledgeBase, error) {
	var kb KnowledgeBase
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO knowledge_bases (organization_id, name, description)
		VALUES ($1, $2, $3)
		RETURNING `+kbCols, orgID, name, description).
		Scan(&kb.ID, &kb.OrganizationID, &kb.Name, &kb.Description, &kb.CreatedAt, &kb.UpdatedAt)
	return kb, err
}

func (s *Store) UpdateKnowledgeBase(ctx context.Context, orgID, id, name, description string) (KnowledgeBase, error) {
	var kb KnowledgeBase
	err := s.Pool.QueryRow(ctx, `
		UPDATE knowledge_bases SET name = $3, description = $4, updated_at = now()
		WHERE organization_id = $1 AND id = $2
		RETURNING `+kbCols, orgID, id, name, description).
		Scan(&kb.ID, &kb.OrganizationID, &kb.Name, &kb.Description, &kb.CreatedAt, &kb.UpdatedAt)
	return kb, notFound(err)
}

func (s *Store) GetKnowledgeBase(ctx context.Context, orgID, id string) (KnowledgeBase, error) {
	var kb KnowledgeBase
	err := s.Pool.QueryRow(ctx, `
		SELECT k.id, k.organization_id, k.name, k.description, k.created_at, k.updated_at,
		       count(d.id) as docs_count,
		       coalesce(sum(octet_length(d.content)), 0) as total_bytes
		FROM knowledge_bases k
		LEFT JOIN knowledge_documents d ON d.knowledge_base_id = k.id
		WHERE k.organization_id = $1 AND k.id = $2
		GROUP BY k.id`, orgID, id).
		Scan(&kb.ID, &kb.OrganizationID, &kb.Name, &kb.Description, &kb.CreatedAt, &kb.UpdatedAt,
			&kb.DocumentsCount, &kb.TotalBytes)
	return kb, notFound(err)
}

func (s *Store) ListKnowledgeBases(ctx context.Context, orgID string) ([]KnowledgeBase, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT k.id, k.organization_id, k.name, k.description, k.created_at, k.updated_at,
		       count(d.id) as docs_count,
		       coalesce(sum(octet_length(d.content)), 0) as total_bytes
		FROM knowledge_bases k
		LEFT JOIN knowledge_documents d ON d.knowledge_base_id = k.id
		WHERE k.organization_id = $1
		GROUP BY k.id
		ORDER BY k.name ASC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []KnowledgeBase{}
	for rows.Next() {
		var kb KnowledgeBase
		if err := rows.Scan(&kb.ID, &kb.OrganizationID, &kb.Name, &kb.Description, &kb.CreatedAt, &kb.UpdatedAt,
			&kb.DocumentsCount, &kb.TotalBytes); err != nil {
			return nil, err
		}
		out = append(out, kb)
	}
	return out, rows.Err()
}

func (s *Store) DeleteKnowledgeBase(ctx context.Context, orgID, id string) error {
	cmd, err := s.Pool.Exec(ctx, `DELETE FROM knowledge_bases WHERE organization_id = $1 AND id = $2`, orgID, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateDocument(ctx context.Context, orgID, kbID, name, content string) (KnowledgeDocument, error) {
	var doc KnowledgeDocument
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO knowledge_documents (organization_id, knowledge_base_id, name, content)
		VALUES ($1, $2, $3, $4)
		RETURNING id, organization_id, knowledge_base_id, name, content, created_at`,
		orgID, kbID, name, content).
		Scan(&doc.ID, &doc.OrganizationID, &doc.KnowledgeBaseID, &doc.Name, &doc.Content, &doc.CreatedAt)
	return doc, err
}

func (s *Store) ListDocuments(ctx context.Context, orgID, kbID string) ([]KnowledgeDocument, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, organization_id, knowledge_base_id, name, content, created_at
		FROM knowledge_documents
		WHERE organization_id = $1 AND knowledge_base_id = $2
		ORDER BY created_at ASC`, orgID, kbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []KnowledgeDocument{}
	for rows.Next() {
		var doc KnowledgeDocument
		if err := rows.Scan(&doc.ID, &doc.OrganizationID, &doc.KnowledgeBaseID, &doc.Name, &doc.Content, &doc.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, doc)
	}
	return out, rows.Err()
}

func (s *Store) DeleteDocument(ctx context.Context, orgID, kbID, docID string) error {
	cmd, err := s.Pool.Exec(ctx, `
		DELETE FROM knowledge_documents
		WHERE organization_id = $1 AND knowledge_base_id = $2 AND id = $3`, orgID, kbID, docID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetKnowledgeBaseFullText concatena todos os documentos em texto para inclusão inline (Pergunta 7).
func (s *Store) GetKnowledgeBaseFullText(ctx context.Context, orgID, kbID string) (string, error) {
	docs, err := s.ListDocuments(ctx, orgID, kbID)
	if err != nil {
		return "", err
	}
	if len(docs) == 0 {
		return "", nil
	}
	var b strings.Builder
	for _, d := range docs {
		b.WriteString("\n--- DOCUMENTO: ")
		b.WriteString(d.Name)
		b.WriteString(" ---\n")
		b.WriteString(d.Content)
		b.WriteString("\n")
	}
	return b.String(), nil
}
