package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

var (
	ErrNotFound = errors.New("not found")
	migrateMu   sync.Mutex
)

type Store struct {
	Pool *pgxpool.Pool
}

// Open aplica as migrations pendentes e abre o pool de conexões.
func Open(ctx context.Context, url string) (*Store, error) {
	if err := migrate(url); err != nil {
		return nil, fmt.Errorf("migrations: %w", err)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{Pool: pool}, nil
}

func (s *Store) Close() { s.Pool.Close() }

func migrate(url string) error {
	migrateMu.Lock()
	defer migrateMu.Unlock()

	db, err := sql.Open("pgx", url)
	if err != nil {
		return err
	}
	defer db.Close()

	// Trava consultiva no Postgres para impedir corrida entre processos/testes concorrentes
	if _, err := db.Exec("SELECT pg_advisory_lock(982347123)"); err != nil {
		return err
	}
	defer func() { _, _ = db.Exec("SELECT pg_advisory_unlock(982347123)") }()

	goose.SetBaseFS(migrations)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.Up(db, "migrations")
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
