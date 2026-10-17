package agents

import (
	"context"
	"strings"
	"testing"

	"voiceagent/internal/store"
	"voiceagent/internal/variables"
	"voiceagent/internal/voice"
)

type fakeProvider struct{}

func (fakeProvider) Catalog() voice.Catalog {
	return voice.Catalog{
		Provider: "fake", Label: "Fake", MaxSessionSeconds: 540,
		Models:    []voice.Option{{ID: "m1"}},
		Voices:    []voice.Option{{ID: "v1"}},
		Languages: []voice.Option{{ID: "pt-BR"}},
	}
}
func (fakeProvider) Connect(context.Context, voice.SessionConfig) (voice.Session, error) { return nil, nil }

var reg = voice.NewRegistry(fakeProvider{})

func valid() store.AgentConfig {
	return store.AgentConfig{
		Provider: "fake", Model: "m1", Voice: "v1", Language: "pt-BR", MaxDurationSeconds: 300,
		SystemPrompt: "Você atende {{nome}}.", FirstMessage: "{{horario}}!",
		Variables:            []variables.Declared{{Name: "nome"}},
		InitiationWebhookURL: "https://example.com/hook",
	}
}

func TestValidatePublishOK(t *testing.T) {
	if errs := ValidatePublish(valid(), reg); len(errs) > 0 {
		t.Fatal(errs)
	}
}

func TestValidatePublishBlocks(t *testing.T) {
	cases := map[string]func(*store.AgentConfig){
		"voz incompatível":       func(c *store.AgentConfig) { c.Voice = "outra" },
		"modelo incompatível":    func(c *store.AgentConfig) { c.Model = "outro" },
		"sem provider":           func(c *store.AgentConfig) { c.Provider = "" },
		"acima do limite":        func(c *store.AgentConfig) { c.MaxDurationSeconds = 600 },
		"variable não declarada": func(c *store.AgentConfig) { c.FirstMessage = "Oi {{protocolo}}" },
		"variable em procedure": func(c *store.AgentConfig) {
			c.Procedures = []store.ProcedureSnapshot{{Name: "p", Content: "use {{cpf}}"}}
		},
		"sem webhook":   func(c *store.AgentConfig) { c.InitiationWebhookURL = "" },
		"prompt vazio":  func(c *store.AgentConfig) { c.SystemPrompt = " " },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := valid()
			mutate(&c)
			if errs := ValidatePublish(c, reg); len(errs) == 0 {
				t.Fatal("esperava erro")
			} else {
				t.Log(strings.Join(errs, "; "))
			}
		})
	}
}

func TestValidateDraft(t *testing.T) {
	u := store.AgentUpdate{Name: " Livia ", Timezone: "America/Sao_Paulo", MaxDurationSeconds: 300, InitiationWebhookTimeout: 3000,
		Variables: []variables.Declared{{Name: "nome"}}}
	if errs := ValidateDraft(&u, reg); len(errs) > 0 {
		t.Fatal(errs)
	}
	if u.Name != "Livia" {
		t.Fatal("nome deveria ser aparado")
	}
	bad := store.AgentUpdate{Name: "x", Timezone: "Marte/Base", MaxDurationSeconds: 5, InitiationWebhookTimeout: 3000,
		Variables:            []variables.Declared{{Name: "horario"}, {Name: "Nome Cliente"}},
		InitiationWebhookURL: "ftp://x", SystemTools: []string{"hackear"}, Provider: "nenhum"}
	errs := ValidateDraft(&bad, reg)
	if len(errs) != 7 {
		t.Fatalf("esperava 7 erros, veio %d: %v", len(errs), errs)
	}
}
