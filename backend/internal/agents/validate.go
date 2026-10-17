// Package agents concentra as regras de domínio do Agent: o que é aceito ao salvar
// o Draft e o que é exigido para Publish.
package agents

import (
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"voiceagent/internal/store"
	"voiceagent/internal/variables"
	"voiceagent/internal/voice"
)

// SystemTools disponíveis na plataforma.
var SystemTools = []string{
	"encerrar_conversa",
	"end_call",
	"transferir_numero",
	"transferir_agente",
	"transfer_to_agent",
	"detectar_idioma",
	"pular_vez",
	"atualizar_estado",
	"reproduzir_dtmf",
	"detectar_correio_voz",
	"guardiao_jev",
	"jev_guardian",
}

// ValidateDraft checa formato ao salvar. Compatibilidade com o provider é checada só no Publish,
// para permitir trocar o provider e ajustar modelo/voz depois.
func ValidateDraft(u *store.AgentUpdate, reg *voice.Registry) []string {
	var errs []string
	add := func(f string, a ...any) { errs = append(errs, fmt.Sprintf(f, a...)) }

	u.Name = strings.TrimSpace(u.Name)
	if u.Name == "" || utf8.RuneCountInString(u.Name) > 100 {
		add("nome é obrigatório (máx. 100 caracteres)")
	}
	if utf8.RuneCountInString(u.Description) > 500 {
		add("descrição: máx. 500 caracteres")
	}
	if u.Provider != "" {
		if _, err := reg.Get(u.Provider); err != nil {
			add("provider desconhecido: %s", u.Provider)
		}
	}
	if _, err := time.LoadLocation(u.Timezone); err != nil || u.Timezone == "" {
		add("fuso horário inválido: %q", u.Timezone)
	}
	if len(u.SystemPrompt) > 100_000 {
		add("System Prompt: máx. 100.000 caracteres")
	}
	if utf8.RuneCountInString(u.FirstMessage) > 2000 {
		add("First Message: máx. 2.000 caracteres")
	}
	if u.MaxDurationSeconds < 30 || u.MaxDurationSeconds > 3600 {
		add("duração máxima deve estar entre 30 e 3600 segundos")
	}
	seen := map[string]bool{}
	for i := range u.Variables {
		v := &u.Variables[i]
		v.Name = strings.TrimSpace(v.Name)
		switch {
		case !variables.ValidName(v.Name):
			add("Variable %q: use letras minúsculas, números e _ (começando por letra ou _)", v.Name)
		case variables.IsSystem(v.Name):
			add("Variable %q é uma System Variable reservada", v.Name)
		case seen[v.Name]:
			add("Variable %q declarada mais de uma vez", v.Name)
		}
		seen[v.Name] = true
	}
	if u.InitiationWebhookURL != "" {
		if p, err := url.Parse(u.InitiationWebhookURL); err != nil || (p.Scheme != "http" && p.Scheme != "https") || p.Host == "" {
			add("Initiation Webhook: URL http(s) inválida")
		}
	}
	if u.InitiationWebhookTimeout == 0 {
		u.InitiationWebhookTimeout = 3000
	} else if u.InitiationWebhookTimeout < 500 || u.InitiationWebhookTimeout > 10_000 {
		add("timeout do Initiation Webhook deve estar entre 500 e 10000 ms")
	}
	for _, t := range u.SystemTools {
		if !contains(SystemTools, t) {
			add("System Tool desconhecida: %s", t)
		}
	}
	return errs
}

// ValidatePublish lista o que impede transformar o Draft numa Agent Version.
func ValidatePublish(cfg store.AgentConfig, reg *voice.Registry) []string {
	var errs []string
	add := func(f string, a ...any) { errs = append(errs, fmt.Sprintf(f, a...)) }

	if cfg.Provider == "" {
		add("escolha um Voice Provider")
	} else if p, err := reg.Get(cfg.Provider); err != nil {
		add("Voice Provider indisponível: %s", cfg.Provider)
	} else {
		cat := p.Catalog()
		if !cat.HasModel(cfg.Model) {
			add("modelo %q não existe em %s", cfg.Model, cat.Label)
		}
		if !cat.HasVoice(cfg.Voice) {
			add("voz %q não existe em %s", cfg.Voice, cat.Label)
		}
		if !cat.HasLanguage(cfg.Language) {
			add("idioma %q não suportado por %s", cfg.Language, cat.Label)
		}
		if cfg.MaxDurationSeconds > cat.MaxSessionSeconds {
			add("duração máxima acima do limite de %s (%d s)", cat.Label, cat.MaxSessionSeconds)
		}
	}
	if strings.TrimSpace(cfg.SystemPrompt) == "" {
		add("System Prompt é obrigatório")
	}
	declared := map[string]bool{}
	for _, v := range cfg.Variables {
		declared[v.Name] = true
	}
	texts := []string{cfg.SystemPrompt, cfg.FirstMessage}
	for _, p := range cfg.Procedures {
		texts = append(texts, p.Content, p.Trigger)
		texts = append(texts, p.Guardrails...)
	}
	for _, name := range variables.References(texts...) {
		if !variables.IsSystem(name) && !declared[name] {
			add("{{%s}} é usada mas não foi declarada como Variable", name)
		}
	}
	if len(cfg.Variables) > 0 && cfg.InitiationWebhookURL == "" {
		add("há Dynamic Variables declaradas, mas nenhum Initiation Webhook configurado")
	}
	return errs
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
