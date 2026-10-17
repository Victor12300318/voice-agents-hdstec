package store

import (
	"context"
	"time"
)

type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type User struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Email          string    `json:"email"`
	PasswordHash   string    `json:"-"`
	CreatedAt      time.Time `json:"created_at"`
}

// CreateOrgWithAdmin cria uma Organization e seu primeiro User numa transação.
func (s *Store) CreateOrgWithAdmin(ctx context.Context, orgName, email, passwordHash string) (Organization, User, error) {
	var org Organization
	var u User
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return org, u, err
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx,
		`INSERT INTO organizations (name) VALUES ($1) RETURNING id, name, created_at`, orgName).
		Scan(&org.ID, &org.Name, &org.CreatedAt)
	if err != nil {
		return org, u, err
	}
	err = tx.QueryRow(ctx,
		`INSERT INTO users (organization_id, email, password_hash) VALUES ($1, $2, $3)
		 RETURNING id, organization_id, email, password_hash, created_at`,
		org.ID, email, passwordHash).
		Scan(&u.ID, &u.OrganizationID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return org, u, err
	}
	return org, u, tx.Commit(ctx)
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n, err
}

// ResetFirstUser troca e-mail e senha do User mais antigo (o admin do bootstrap).
func (s *Store) ResetFirstUser(ctx context.Context, email, passwordHash string) (User, error) {
	var u User
	err := s.Pool.QueryRow(ctx,
		`UPDATE users SET email = $1, password_hash = $2
		 WHERE id = (SELECT id FROM users ORDER BY created_at LIMIT 1)
		 RETURNING id, organization_id, email, password_hash, created_at`, email, passwordHash).
		Scan(&u.ID, &u.OrganizationID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	return u, notFound(err)
}

func (s *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	var u User
	err := s.Pool.QueryRow(ctx,
		`SELECT id, organization_id, email, password_hash, created_at FROM users WHERE lower(email) = lower($1)`, email).
		Scan(&u.ID, &u.OrganizationID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	return u, notFound(err)
}

func (s *Store) Organization(ctx context.Context, id string) (Organization, error) {
	var o Organization
	err := s.Pool.QueryRow(ctx,
		`SELECT id, name, created_at FROM organizations WHERE id = $1`, id).
		Scan(&o.ID, &o.Name, &o.CreatedAt)
	return o, notFound(err)
}
