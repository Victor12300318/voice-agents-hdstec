// Package gemini implementa voice.Provider sobre a Gemini Live API usando o SDK
// oficial google.golang.org/genai.
package gemini

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"google.golang.org/genai"

	"voiceagent/internal/voice"
)

const ProviderID = "gemini_live"

// Modelos Live com saída de áudio nativa (docs: ai.google.dev/gemini-api/docs/live-api/capabilities).
var models = []voice.Option{
	{ID: "gemini-3.8-live", Label: "Gemini 3.8 Live", Description: "Padrão recomendado para voz de baixa latência", Default: true},
	{ID: "gemini-3.8-live-extended-thinking", Label: "Gemini 3.8 Live Extended Thinking", Description: "Raciocínio contínuo em segundo plano (thinking) para interações e tools complexas"},
	{ID: "gemini-3.1-flash-live-preview", Label: "Gemini 3.1 Flash Live (preview, legado)"},
}

// Vozes pré-configuradas (docs: ai.google.dev/gemini-api/docs/speech-generation#voices).
var voices = []voice.Option{
	{ID: "Kore", Label: "Kore", Description: "Firme", Default: true},
	{ID: "Zephyr", Label: "Zephyr", Description: "Brilhante"},
	{ID: "Puck", Label: "Puck", Description: "Animada"},
	{ID: "Charon", Label: "Charon", Description: "Informativa"},
	{ID: "Fenrir", Label: "Fenrir", Description: "Empolgada"},
	{ID: "Leda", Label: "Leda", Description: "Jovem"},
	{ID: "Orus", Label: "Orus", Description: "Firme"},
	{ID: "Aoede", Label: "Aoede", Description: "Leve"},
	{ID: "Callirrhoe", Label: "Callirrhoe", Description: "Tranquila"},
	{ID: "Autonoe", Label: "Autonoe", Description: "Brilhante"},
	{ID: "Enceladus", Label: "Enceladus", Description: "Sussurrada"},
	{ID: "Iapetus", Label: "Iapetus", Description: "Clara"},
	{ID: "Umbriel", Label: "Umbriel", Description: "Tranquila"},
	{ID: "Algieba", Label: "Algieba", Description: "Suave"},
	{ID: "Despina", Label: "Despina", Description: "Suave"},
	{ID: "Erinome", Label: "Erinome", Description: "Clara"},
	{ID: "Algenib", Label: "Algenib", Description: "Rouca"},
	{ID: "Rasalgethi", Label: "Rasalgethi", Description: "Informativa"},
	{ID: "Laomedeia", Label: "Laomedeia", Description: "Animada"},
	{ID: "Achernar", Label: "Achernar", Description: "Macia"},
	{ID: "Alnilam", Label: "Alnilam", Description: "Firme"},
	{ID: "Schedar", Label: "Schedar", Description: "Equilibrada"},
	{ID: "Gacrux", Label: "Gacrux", Description: "Madura"},
	{ID: "Pulcherrima", Label: "Pulcherrima", Description: "Direta"},
	{ID: "Achird", Label: "Achird", Description: "Amigável"},
	{ID: "Zubenelgenubi", Label: "Zubenelgenubi", Description: "Casual"},
	{ID: "Vindemiatrix", Label: "Vindemiatrix", Description: "Gentil"},
	{ID: "Sadachbia", Label: "Sadachbia", Description: "Vivaz"},
	{ID: "Sadaltager", Label: "Sadaltager", Description: "Conhecedora"},
	{ID: "Sulafat", Label: "Sulafat", Description: "Calorosa"},
}

// Modelos de áudio nativo escolhem o idioma sozinhos (não aceitam language code);
// o runtime instrui o idioma pelo System Prompt.
var languages = []voice.Option{
	{ID: "pt-BR", Label: "Português (Brasil)", Default: true},
	{ID: "pt-PT", Label: "Português (Portugal)"},
	{ID: "en", Label: "Inglês"},
	{ID: "es", Label: "Espanhol"},
}

type Provider struct {
	client *genai.Client
}

func New(ctx context.Context, apiKey string) (*Provider, error) {
	if apiKey == "" {
		return nil, errors.New("GEMINI_API_KEY não definido")
	}
	c, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey, Backend: genai.BackendGeminiAPI})
	if err != nil {
		return nil, err
	}
	return &Provider{client: c}, nil
}

func (p *Provider) Catalog() voice.Catalog {
	return voice.Catalog{
		Provider: ProviderID,
		Label:    "Gemini Live",
		Models:   models,
		Voices:   voices,
		// Conexões Live duram ~10 min e não usamos session resumption no MVP.
		MaxSessionSeconds: 540,
		Languages:         languages,
		InputSampleRate:   16000,
		OutputSampleRate:  24000,
	}
}

func (p *Provider) Connect(ctx context.Context, cfg voice.SessionConfig) (voice.Session, error) {
	conf := &genai.LiveConnectConfig{
		ResponseModalities: []genai.Modality{genai.ModalityAudio},
		SpeechConfig: &genai.SpeechConfig{VoiceConfig: &genai.VoiceConfig{
			PrebuiltVoiceConfig: &genai.PrebuiltVoiceConfig{VoiceName: cfg.Voice},
		}},
		SystemInstruction:        &genai.Content{Parts: []*genai.Part{{Text: cfg.SystemPrompt}}},
		InputAudioTranscription:  &genai.AudioTranscriptionConfig{},
		OutputAudioTranscription: &genai.AudioTranscriptionConfig{},
	}
	if strings.Contains(cfg.Model, "extended-thinking") {
		level := genai.ThinkingLevelMedium
		switch strings.ToUpper(cfg.ThinkingLevel) {
		case "LOW":
			level = genai.ThinkingLevelLow
		case "HIGH":
			level = genai.ThinkingLevelHigh
		}
		conf.ThinkingConfig = &genai.ThinkingConfig{
			ThinkingLevel: level,
		}
	}
	if len(cfg.Tools) > 0 {
		conf.Tools = []*genai.Tool{{FunctionDeclarations: declarations(cfg.Model, cfg.Tools)}}
	}
	s, err := p.client.Live.Connect(ctx, cfg.Model, conf)
	if err != nil {
		return nil, fmt.Errorf("gemini live connect: %w", err)
	}
	sess := &session{s: s, events: make(chan voice.Event, 256)}
	go sess.readLoop()
	return sess, nil
}

func declarations(model string, tools []voice.ToolSpec) []*genai.FunctionDeclaration {
	out := make([]*genai.FunctionDeclaration, 0, len(tools))
	for _, t := range tools {
		props := map[string]any{}
		required := []string{}
		for _, p := range t.Parameters {
			props[p.Name] = map[string]any{"type": p.Type, "description": p.Description}
			if p.Required {
				required = append(required, p.Name)
			}
		}
		d := &genai.FunctionDeclaration{
			Name:        t.Name,
			Description: t.Description,
			ParametersJsonSchema: map[string]any{
				"type": "object", "properties": props, "required": required,
			},
		}
		// Gemini 3.8 Live usa chamadas assíncronas por padrão; pedimos o modo bloqueante
		// para que o Agent espere o resultado da Tool antes de responder.
		// Porém, gemini-3.8-live-extended-thinking exige NON_BLOCKING (chamadas assíncronas).
		isTerminalSystemTool := t.Name == "transfer_to_agent" ||
			t.Name == "transferir_agente" ||
			t.Name == "encerrar_conversa" ||
			t.Name == "end_call" ||
			t.Name == "transferir_numero"
		if strings.HasPrefix(model, "gemini-3.8-live") && !strings.Contains(model, "extended-thinking") && !isTerminalSystemTool {
			d.Behavior = genai.BehaviorBlocking
		}
		out = append(out, d)
	}
	return out
}

type session struct {
	s      *genai.Session
	events chan voice.Event

	mu     sync.Mutex
	closed bool
}

func (s *session) SendAudio(pcm []byte) error {
	return s.s.SendRealtimeInput(genai.LiveRealtimeInput{
		Audio: &genai.Blob{Data: pcm, MIMEType: "audio/pcm;rate=16000"},
	})
}

func (s *session) SendText(text string) error {
	complete := true
	return s.s.SendClientContent(genai.LiveClientContentInput{
		Turns:        []*genai.Content{{Role: "user", Parts: []*genai.Part{{Text: text}}}},
		TurnComplete: &complete,
	})
}

func (s *session) SendToolResult(call voice.ToolCall, result map[string]any) error {
	return s.s.SendToolResponse(genai.LiveToolResponseInput{
		FunctionResponses: []*genai.FunctionResponse{{ID: call.ID, Name: call.Name, Response: result}},
	})
}

func (s *session) Events() <-chan voice.Event { return s.events }

func (s *session) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return s.s.Close()
}

func (s *session) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *session) readLoop() {
	defer close(s.events)
	for {
		msg, err := s.s.Receive()
		if err != nil {
			if s.isClosed() {
				s.events <- voice.Event{Type: voice.EventClosed}
			} else {
				s.events <- voice.Event{Type: voice.EventClosed, Err: err}
			}
			return
		}
		s.dispatch(msg)
	}
}

func (s *session) dispatch(msg *genai.LiveServerMessage) {
	if va := msg.VoiceActivity; va != nil && va.VoiceActivityType == genai.VoiceActivityTypeActivityEnd {
		s.events <- voice.Event{Type: voice.EventCallerSpeechEnd}
	}
	if sc := msg.ServerContent; sc != nil {
		// Um evento pode trazer várias partes ao mesmo tempo (áudio + transcrição).
		if sc.InputTranscription != nil && sc.InputTranscription.Text != "" {
			s.events <- voice.Event{Type: voice.EventCallerTranscript, Text: sc.InputTranscription.Text}
		}
		if sc.ModelTurn != nil {
			for _, part := range sc.ModelTurn.Parts {
				if part.InlineData != nil && len(part.InlineData.Data) > 0 {
					s.events <- voice.Event{Type: voice.EventAudio, Audio: part.InlineData.Data}
				}
			}
		}
		if sc.OutputTranscription != nil && sc.OutputTranscription.Text != "" {
			s.events <- voice.Event{Type: voice.EventAgentTranscript, Text: sc.OutputTranscription.Text}
		}
		if sc.Interrupted {
			s.events <- voice.Event{Type: voice.EventInterrupted}
		}
		if sc.TurnComplete {
			// Se o servidor enviou interactionStatus (como no extended-thinking),
			// só encerra o turno se o status for IDLE.
			if sc.InteractionStatus == "" || sc.InteractionStatus == genai.InteractionStatusIdle {
				s.events <- voice.Event{Type: voice.EventTurnComplete}
			}
		} else if sc.InteractionStatus == genai.InteractionStatusIdle {
			s.events <- voice.Event{Type: voice.EventTurnComplete}
		}
	}
	if tc := msg.ToolCall; tc != nil {
		for _, fc := range tc.FunctionCalls {
			s.events <- voice.Event{Type: voice.EventToolCall, ToolCall: &voice.ToolCall{ID: fc.ID, Name: fc.Name, Args: fc.Args}}
		}
	}
	if c := msg.ToolCallCancellation; c != nil {
		s.events <- voice.Event{Type: voice.EventToolCallCancelled, CallIDs: c.IDs}
	}
	if um := msg.UsageMetadata; um != nil {
		u := voice.Usage{
			PromptTokens:   int64(um.PromptTokenCount),
			ResponseTokens: int64(um.ResponseTokenCount),
			TotalTokens:    int64(um.TotalTokenCount),
		}
		for _, d := range um.PromptTokensDetails {
			if d.Modality == genai.MediaModalityAudio {
				u.InputAudioTokens += int64(d.TokenCount)
			} else if d.Modality == genai.MediaModalityText {
				u.InputTextTokens += int64(d.TokenCount)
			}
		}
		for _, d := range um.ResponseTokensDetails {
			if d.Modality == genai.MediaModalityAudio {
				u.OutputAudioTokens += int64(d.TokenCount)
			} else if d.Modality == genai.MediaModalityText {
				u.OutputTextTokens += int64(d.TokenCount)
			}
		}
		s.events <- voice.Event{Type: voice.EventUsage, Usage: &u}
	}
}
