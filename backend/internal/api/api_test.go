package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"

	"voiceagent/internal/auth"
	"voiceagent/internal/store"
	"voiceagent/internal/voice"
)

type fakeProvider struct{}

func (fakeProvider) Catalog() voice.Catalog {
	return voice.Catalog{
		Provider: "fake", Label: "Fake", MaxSessionSeconds: 540,
		Models:    []voice.Option{{ID: "m1", Default: true}, {ID: "m2"}},
		Voices:    []voice.Option{{ID: "v1", Default: true}},
		Languages: []voice.Option{{ID: "pt-BR", Default: true}},
	}
}
func (fakeProvider) Connect(context.Context, voice.SessionConfig) (voice.Session, error) {
	return nil, nil
}

// newTestServer sobe o servidor contra Postgres/Redis reais (docker compose --profile test).
func newTestServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	dbURL, redisURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_REDIS_URL")
	if dbURL == "" || redisURL == "" {
		t.Skip("TEST_DATABASE_URL/TEST_REDIS_URL não definidos")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(opt)
	t.Cleanup(func() { rdb.Close() })

	srv := &Server{Store: st, Sessions: &auth.Sessions{Redis: rdb}, Voices: voice.NewRegistry(fakeProvider{}),
		HTTPClient: http.DefaultClient, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, st
}

type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newClient(t *testing.T, base string) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: base, http: &http.Client{Jar: jar}}
}

func (c *client) do(method, path string, body any, out any) int {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func createOrg(t *testing.T, st *store.Store, name, email, pw string) {
	t.Helper()
	hash, _ := auth.HashPassword(pw)
	if _, _, err := st.CreateOrgWithAdmin(context.Background(), name, email, hash); err != nil {
		t.Fatal(err)
	}
}

func TestLoginAndOrganizationIsolation(t *testing.T) {
	ts, st := newTestServer(t)
	suffix := randSuffix()
	createOrg(t, st, "Org A", "a-"+suffix+"@test.local", "senha-a-123")
	createOrg(t, st, "Org B", "b-"+suffix+"@test.local", "senha-b-123")

	anon := newClient(t, ts.URL)
	if code := anon.do("GET", "/api/agents", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("sem login: esperado 401, veio %d", code)
	}

	a := newClient(t, ts.URL)
	if code := a.do("POST", "/api/auth/login", map[string]string{"email": "a-" + suffix + "@test.local", "password": "errada"}, nil); code != http.StatusUnauthorized {
		t.Fatalf("senha errada: esperado 401, veio %d", code)
	}
	if code := a.do("POST", "/api/auth/login", map[string]string{"email": "a-" + suffix + "@test.local", "password": "senha-a-123"}, nil); code != http.StatusOK {
		t.Fatalf("login A: %d", code)
	}
	var agent store.Agent
	if code := a.do("POST", "/api/agents", map[string]string{"name": "Livia"}, &agent); code != http.StatusCreated {
		t.Fatalf("criar agent: %d", code)
	}
	if code := a.do("GET", "/api/agents/"+agent.ID, nil, nil); code != http.StatusOK {
		t.Fatalf("A lendo o próprio agent: %d", code)
	}

	b := newClient(t, ts.URL)
	if code := b.do("POST", "/api/auth/login", map[string]string{"email": "b-" + suffix + "@test.local", "password": "senha-b-123"}, nil); code != http.StatusOK {
		t.Fatalf("login B: %d", code)
	}
	if code := b.do("GET", "/api/agents/"+agent.ID, nil, nil); code != http.StatusNotFound {
		t.Fatalf("B lendo agent de A: esperado 404, veio %d", code)
	}
	var list []store.Agent
	b.do("GET", "/api/agents", nil, &list)
	for _, x := range list {
		if x.ID == agent.ID {
			t.Fatal("agent de A apareceu na lista de B")
		}
	}

	if code := b.do("POST", "/api/auth/logout", nil, nil); code != http.StatusNoContent {
		t.Fatalf("logout: %d", code)
	}
	if code := b.do("GET", "/api/auth/me", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("após logout: esperado 401, veio %d", code)
	}
}

func randSuffix() string {
	tok, _ := auth.NewToken()
	return tok[:12]
}

func getTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("TEST_REDIS_URL não definido")
	}
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(opt)
	t.Cleanup(func() { rdb.Close() })
	return rdb
}
