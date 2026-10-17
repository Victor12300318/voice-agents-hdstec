package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"voiceagent/internal/api"
	"voiceagent/internal/auth"
	"voiceagent/internal/config"
	"voiceagent/internal/guardian"
	"voiceagent/internal/runtime"
	"voiceagent/internal/storage"
	"voiceagent/internal/store"
	"voiceagent/internal/voice"
	"voiceagent/internal/voice/gemini"
	"voiceagent/internal/voice/openai"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}
	log := newLogger(cfg.LogLevel)
	slog.SetDefault(log)

	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "serve":
		err = serve(cfg, log)
	case "create-org":
		err = createOrg(cfg, os.Args[2:])
	case "reset-admin":
		err = resetAdmin(cfg)
	default:
		err = fmt.Errorf("comando desconhecido %q (use: serve | create-org | reset-admin)", cmd)
	}
	if err != nil {
		log.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func serve(cfg config.Config, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer st.Close()

	rdb, err := openRedis(ctx, cfg.RedisURL)
	if err != nil {
		return fmt.Errorf("redis: %w", err)
	}
	defer rdb.Close()

	if err := bootstrap(ctx, st, cfg, log); err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}

	var provList []voice.Provider
	gem, err := gemini.New(ctx, cfg.GeminiAPIKey)
	if err != nil {
		return fmt.Errorf("gemini: %w", err)
	}
	provList = append(provList, gem)

	if cfg.OpenAIAPIKey != "" {
		oai := openai.New(cfg.OpenAIAPIKey)
		provList = append(provList, oai)
		log.Info("provider_registered", "provider", openai.ProviderID)
	}

	voices := voice.NewRegistry(provList...)

	sessions := &auth.Sessions{Redis: rdb}
	httpClient := &http.Client{Timeout: 10 * time.Second}

	minioStorage, err := storage.New(ctx, cfg.MinioEndpoint, cfg.MinioAccessKey, cfg.MinioSecretKey, cfg.MinioBucket, log)
	if err != nil {
		log.Warn("minio_init_warning", "error", err)
	}

	rt := &runtime.Runtime{
		Store:      st,
		Voices:     voices,
		Sessions:   sessions,
		Storage:    minioStorage,
		HTTPClient: httpClient,
		Guardian: &guardian.Evaluator{
			HTTPClient:       httpClient,
			OpenAIAPIKey:     cfg.OpenAIAPIKey,
			OpenRouterAPIKey: cfg.OpenRouterAPIKey,
			OpenRouterModel:  cfg.OpenRouterModel,
			TypeSafeAPIKey:   cfg.TypeSafeAPIKey,
		},
		Log: log,
	}

	srv := &api.Server{
		Store:             st,
		Sessions:          sessions,
		Voices:            voices,
		Runtime:           rt,
		Storage:           minioStorage,
		HTTPClient:        httpClient,
		CookieSecure:      cfg.CookieSecure,
		Log:               log,
		JambonzAPIURL:     cfg.JambonzAPIURL,
		JambonzAPIKey:     cfg.JambonzAPIKey,
		JambonzAccountSid: cfg.JambonzAccountSid,
		PublicWSURL:       cfg.PublicWSURL,
	}
	httpSrv := &http.Server{Addr: cfg.HTTPAddr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		httpSrv.Shutdown(shutdownCtx)
	}()

	log.Info("server_started", "addr", cfg.HTTPAddr)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	log.Info("server_stopped")
	return nil
}

// bootstrap cria a primeira Organization e o User admin se ainda não existir nenhum User.
func bootstrap(ctx context.Context, st *store.Store, cfg config.Config, log *slog.Logger) error {
	n, err := st.CountUsers(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		if cfg.AdminEmail != "" {
			if _, err := st.UserByEmail(ctx, cfg.AdminEmail); errors.Is(err, store.ErrNotFound) {
				log.Warn("bootstrap_admin_mismatch",
					"reason", "ADMIN_EMAIL não existe no banco; o bootstrap só roda na primeira subida",
					"fix", "docker compose exec backend /server reset-admin")
			}
		}
		return nil
	}
	if cfg.AdminEmail == "" || cfg.AdminPassword == "" {
		log.Warn("bootstrap_skipped", "reason", "nenhum User existe e ADMIN_EMAIL/ADMIN_PASSWORD não definidos")
		return nil
	}
	hash, err := auth.HashPassword(cfg.AdminPassword)
	if err != nil {
		return err
	}
	org, u, err := st.CreateOrgWithAdmin(ctx, cfg.OrgName, cfg.AdminEmail, hash)
	if err != nil {
		return err
	}
	log.Info("bootstrap_created", "organization_id", org.ID, "user_id", u.ID, "email", u.Email)
	return nil
}

func createOrg(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("create-org", flag.ContinueOnError)
	name := fs.String("name", "", "nome da Organization")
	email := fs.String("email", "", "e-mail do User admin")
	password := fs.String("password", "", "senha do User admin")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*name) == "" || *email == "" || len(*password) < 8 {
		return errors.New("uso: create-org -name NOME -email EMAIL -password SENHA(min 8)")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()
	hash, err := auth.HashPassword(*password)
	if err != nil {
		return err
	}
	org, u, err := st.CreateOrgWithAdmin(ctx, *name, *email, hash)
	if err != nil {
		return err
	}
	fmt.Printf("Organization %s (%s) criada com User %s\n", org.Name, org.ID, u.Email)
	return nil
}

// resetAdmin aplica ADMIN_EMAIL/ADMIN_PASSWORD ao admin criado no bootstrap.
func resetAdmin(cfg config.Config) error {
	if cfg.AdminEmail == "" || len(cfg.AdminPassword) < 8 {
		return errors.New("defina ADMIN_EMAIL e ADMIN_PASSWORD (mín. 8 caracteres) no .env")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()
	hash, err := auth.HashPassword(cfg.AdminPassword)
	if err != nil {
		return err
	}
	u, err := st.ResetFirstUser(ctx, cfg.AdminEmail, hash)
	if err != nil {
		return err
	}
	fmt.Printf("Admin atualizado: %s\n", u.Email)
	return nil
}

func openRedis(ctx context.Context, url string) (*redis.Client, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	c := redis.NewClient(opt)
	if err := c.Ping(ctx).Err(); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func newLogger(level string) *slog.Logger {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		l = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l}))
}
