// Package voice define o contrato entre o runtime e qualquer Voice Provider.
// Nada fora das implementações (voice/gemini, futuramente voice/openai) conhece
// detalhes de um provider específico.
package voice

import (
	"context"
	"errors"
)

// Catalog descreve o que um Voice Provider oferece; a UI só exibe isto.
type Catalog struct {
	Provider string   `json:"provider"`
	Label    string   `json:"label"`
	Models   []Option `json:"models"`
	Voices   []Option `json:"voices"`
	// Languages que o painel oferece. Se o provider não aceita um código de idioma,
	// o runtime instrui o idioma pelo System Prompt.
	Languages         []Option `json:"languages"`
	MaxSessionSeconds int      `json:"max_session_seconds"`
	// Taxas de amostragem PCM16 mono little-endian.
	InputSampleRate  int `json:"input_sample_rate"`
	OutputSampleRate int `json:"output_sample_rate"`
}

type Option struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Default     bool   `json:"default,omitempty"`
}

func (c Catalog) HasModel(id string) bool { return has(c.Models, id) }
func (c Catalog) HasVoice(id string) bool { return has(c.Voices, id) }
func (c Catalog) HasLanguage(id string) bool { return has(c.Languages, id) }

func has(opts []Option, id string) bool {
	for _, o := range opts {
		if o.ID == id {
			return true
		}
	}
	return false
}

// SessionConfig é tudo que o provider precisa para abrir uma sessão, já resolvido
// (Variables substituídas, idioma e índice de Procedures embutidos no prompt).
type SessionConfig struct {
	Model         string
	Voice         string
	Language      string
	SystemPrompt  string
	ThinkingLevel string // "low" | "medium" | "high"
	Tools         []ToolSpec
}

// ToolSpec é a declaração provider-agnóstica de uma função chamável pelo modelo.
type ToolSpec struct {
	Name        string
	Description string
	Parameters  []Param
}

type Param struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // string | number | integer | boolean
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

var ParamTypes = []string{"string", "number", "integer", "boolean"}

type EventType string

const (
	// Áudio do Agent (PCM16 na OutputSampleRate).
	EventAudio EventType = "audio"
	// Trecho de transcrição; Final=true encerra o texto do turno.
	EventCallerTranscript EventType = "caller_transcript"
	EventAgentTranscript  EventType = "agent_transcript"
	// Caller parou de falar (base da Response Latency).
	EventCallerSpeechEnd EventType = "caller_speech_end"
	// Caller interrompeu: o áudio pendente do Agent deve ser descartado.
	EventInterrupted  EventType = "interrupted"
	EventTurnComplete EventType = "turn_complete"
	EventToolCall     EventType = "tool_call"
	// Chamadas de Tool canceladas pelo provider (ex.: por Interruption).
	EventToolCallCancelled EventType = "tool_call_cancelled"
	// Sessão encerrada pelo provider; Err preenchido se foi por erro.
	EventClosed EventType = "closed"
	// Métricas de tokens/uso de modelo retornadas pelo provider.
	EventUsage EventType = "usage"
)

type Usage struct {
	TotalTokens       int64 `json:"total_tokens"`
	PromptTokens      int64 `json:"prompt_tokens"`
	ResponseTokens    int64 `json:"response_tokens"`
	InputAudioTokens  int64 `json:"input_audio_tokens"`
	OutputAudioTokens int64 `json:"output_audio_tokens"`
	InputTextTokens   int64 `json:"input_text_tokens"`
	OutputTextTokens  int64 `json:"output_text_tokens"`
	CachedTokens      int64 `json:"cached_tokens"`
}

type Event struct {
	Type     EventType
	Audio    []byte
	Text     string
	ToolCall *ToolCall
	CallIDs  []string
	Err      error
	Usage    *Usage
}

type ToolCall struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

// Session é uma conversa de voz aberta no provider.
type Session interface {
	// SendAudio envia áudio do Caller (PCM16 mono na InputSampleRate).
	SendAudio(pcm []byte) error
	// SendText envia uma instrução em texto como turno do usuário e pede resposta
	// (usado para a First Message).
	SendText(text string) error
	SendToolResult(call ToolCall, result map[string]any) error
	// Events é fechado após um EventClosed.
	Events() <-chan Event
	Close() error
}

type Provider interface {
	Catalog() Catalog
	Connect(ctx context.Context, cfg SessionConfig) (Session, error)
}

var ErrUnknownProvider = errors.New("voice provider desconhecido")

// Registry reúne os providers disponíveis nesta instalação.
type Registry struct {
	providers map[string]Provider
	order     []string
}

func NewRegistry(ps ...Provider) *Registry {
	r := &Registry{providers: map[string]Provider{}}
	for _, p := range ps {
		id := p.Catalog().Provider
		r.providers[id] = p
		r.order = append(r.order, id)
	}
	return r
}

func (r *Registry) Get(id string) (Provider, error) {
	p, ok := r.providers[id]
	if !ok {
		return nil, ErrUnknownProvider
	}
	return p, nil
}

func (r *Registry) Catalogs() []Catalog {
	out := make([]Catalog, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.providers[id].Catalog())
	}
	return out
}
