package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

const (
	CookieName = "va_session"
	sessionTTL = 7 * 24 * time.Hour
)

var ErrNoSession = errors.New("no session")

type Session struct {
	UserID         string `json:"user_id"`
	OrganizationID string `json:"organization_id"`
	Email          string `json:"email"`
}

func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// Sessions guarda sessões de login do painel no Redis.
type Sessions struct {
	Redis *redis.Client
}

// NewToken gera um token aleatório de 64 caracteres hex.
func NewToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func (s *Sessions) Create(ctx context.Context, sess Session) (string, error) {
	token, err := NewToken()
	if err != nil {
		return "", err
	}
	data, _ := json.Marshal(sess)
	if err := s.Redis.Set(ctx, key(token), data, sessionTTL).Err(); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Sessions) Get(ctx context.Context, token string) (Session, error) {
	var sess Session
	if token == "" {
		return sess, ErrNoSession
	}
	data, err := s.Redis.Get(ctx, key(token)).Bytes()
	if errors.Is(err, redis.Nil) {
		return sess, ErrNoSession
	}
	if err != nil {
		return sess, err
	}
	return sess, json.Unmarshal(data, &sess)
}

func (s *Sessions) Delete(ctx context.Context, token string) error {
	return s.Redis.Del(ctx, key(token)).Err()
}

func key(token string) string { return "session:" + token }

type ctxKey struct{}

func WithSession(ctx context.Context, s Session) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

// FromContext devolve a sessão autenticada; só chame em rotas protegidas por Require.
func FromContext(ctx context.Context) Session {
	s, _ := ctx.Value(ctxKey{}).(Session)
	return s
}

// Require exige uma sessão válida e a coloca no contexto.
func (s *Sessions) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(CookieName)
		if err != nil {
			unauthorized(w)
			return
		}
		sess, err := s.Get(r.Context(), c.Value)
		if err != nil {
			unauthorized(w)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithSession(r.Context(), sess)))
	})
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`{"error":"não autenticado"}`))
}
