package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	HTTPAddr      string
	DatabaseURL   string
	RedisURL      string
	AdminEmail    string
	AdminPassword string
	OrgName       string
	CookieSecure   bool
	GeminiAPIKey   string
	OpenAIAPIKey   string
	MinioEndpoint  string
	MinioAccessKey string
	MinioSecretKey string
	MinioBucket       string
	LogLevel          string
	JambonzAPIURL     string
	JambonzAPIKey     string
	JambonzAccountSid string
	PublicWSURL       string
	OpenRouterAPIKey  string
	OpenRouterModel   string
	TypeSafeAPIKey    string
}

func Load() (Config, error) {
	c := Config{
		HTTPAddr:          env("HTTP_ADDR", ":8080"),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		RedisURL:          os.Getenv("REDIS_URL"),
		AdminEmail:        os.Getenv("ADMIN_EMAIL"),
		AdminPassword:     os.Getenv("ADMIN_PASSWORD"),
		OrgName:           env("ORG_NAME", "Minha Organização"),
		CookieSecure:      strings.EqualFold(os.Getenv("COOKIE_SECURE"), "true"),
		GeminiAPIKey:      os.Getenv("GEMINI_API_KEY"),
		OpenAIAPIKey:      os.Getenv("OPENAI_API_KEY"),
		MinioEndpoint:     env("MINIO_ENDPOINT", "minio:9000"),
		MinioAccessKey:    env("MINIO_ACCESS_KEY", "voiceagent"),
		MinioSecretKey:    os.Getenv("MINIO_SECRET_KEY"),
		MinioBucket:       env("MINIO_BUCKET", "recordings"),
		LogLevel:          env("LOG_LEVEL", "info"),
		JambonzAPIURL:     os.Getenv("JAMBONZ_API_URL"),
		JambonzAPIKey:     os.Getenv("JAMBONZ_API_KEY"),
		JambonzAccountSid: os.Getenv("JAMBONZ_ACCOUNT_SID"),
		PublicWSURL:       os.Getenv("PUBLIC_WS_URL"),
		OpenRouterAPIKey:  os.Getenv("OPENROUTER_API_KEY"),
		OpenRouterModel:   env("OPENROUTER_MODEL", "typesafe/jev-router"),
		TypeSafeAPIKey:    os.Getenv("TYPESAFE_API_KEY"),
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL é obrigatório")
	}
	if c.RedisURL == "" {
		return c, fmt.Errorf("REDIS_URL é obrigatório")
	}
	return c, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
