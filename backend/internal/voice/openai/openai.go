// Package openai implementa voice.Provider sobre os protocolos OpenAI Realtime GA (/v1/realtime)
// e OpenAI GPT-Live 1 (/v1/live/sessions).
package openai

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"voiceagent/internal/voice"
)

const ProviderID = "openai_realtime"

var models = []voice.Option{
	{ID: "gpt-live-1", Label: "GPT-Live 1", Description: "Modelo full-duplex de voz ultra-natural da OpenAI ($0.05/min)", Default: true},
	{ID: "gpt-realtime-2.1", Label: "GPT Realtime 2.1", Description: "Última geração OpenAI Realtime de baixa latência"},
	{ID: "gpt-realtime-2.1-mini", Label: "GPT Realtime 2.1 Mini", Description: "Ultra-rápido e econômico de nova geração"},
	{ID: "gpt-realtime", Label: "GPT Realtime", Description: "Modelo estável de produção"},
	{ID: "gpt-realtime-mini", Label: "GPT Realtime Mini", Description: "Compacto e econômico"},
	{ID: "gpt-realtime-1.5", Label: "GPT Realtime 1.5", Description: "Versão 1.5"},
	{ID: "gpt-4o-realtime-preview", Label: "GPT-4o Realtime (Legado)", Description: "Redireciona automaticamente para GPT Realtime 2.1"},
	{ID: "gpt-4o-mini-realtime-preview", Label: "GPT-4o Mini Realtime (Legado)", Description: "Redireciona automaticamente para GPT Realtime 2.1 Mini"},
}

var voices = []voice.Option{
	{ID: "bossa", Label: "Bossa (PT-BR)", Description: "Feminina brasileira natural (Nativa GPT-Live 1)", Default: true},
	{ID: "tempo", Label: "Tempo (PT-BR)", Description: "Masculina brasileira natural (Nativa GPT-Live 1)"},
	{ID: "marin", Label: "Marin", Description: "Feminina, natural e expressiva (Realtime & Live)"},
	{ID: "coral", Label: "Coral", Description: "Feminina, amigável e calorosa"},
	{ID: "sage", Label: "Sage", Description: "Feminina, calma e confiante"},
	{ID: "shimmer", Label: "Shimmer", Description: "Feminina, brilhante e clara"},
	{ID: "cedar", Label: "Cedar", Description: "Masculina, natural e firme (Realtime & Live)"},
	{ID: "quartz", Label: "Quartz", Description: "Feminina, clara e expressiva (GPT-Live 1)"},
	{ID: "gleam", Label: "Gleam", Description: "Feminina, suave e natural (GPT-Live 1)"},
	{ID: "willow", Label: "Willow", Description: "Feminina, calorosa (GPT-Live 1)"},
	{ID: "alloy", Label: "Alloy", Description: "Neutra e equilibrada"},
	{ID: "ash", Label: "Ash", Description: "Suave e relaxada"},
	{ID: "ballad", Label: "Ballad", Description: "Calorosa e expressiva"},
	{ID: "echo", Label: "Echo", Description: "Firme e direta"},
	{ID: "verse", Label: "Verse", Description: "Natural e dinâmica"},
}

var realtimeSupportedVoices = map[string]bool{
	"alloy":   true,
	"ash":     true,
	"ballad":  true,
	"coral":   true,
	"echo":    true,
	"sage":    true,
	"shimmer": true,
	"verse":   true,
	"marin":   true,
	"cedar":   true,
}

var liveOnlyVoices = map[string]bool{
	"bossa":  true,
	"tempo":  true,
	"quartz": true,
	"gleam":  true,
	"willow": true,
}

var languages = []voice.Option{
	{ID: "pt-BR", Label: "Português (Brasil)", Default: true},
	{ID: "pt-PT", Label: "Português (Portugal)"},
	{ID: "en", Label: "Inglês"},
	{ID: "es", Label: "Espanhol"},
}

type Provider struct {
	apiKey string
}

func New(apiKey string) *Provider {
	return &Provider{apiKey: strings.TrimSpace(apiKey)}
}

func (p *Provider) Catalog() voice.Catalog {
	return voice.Catalog{
		Provider:          ProviderID,
		Label:             "OpenAI Realtime & GPT-Live",
		Models:            models,
		Voices:            voices,
		Languages:         languages,
		MaxSessionSeconds: 1800,
		InputSampleRate:   16000,
		OutputSampleRate:  24000,
	}
}

func resolveRealtimeModel(m string) string {
	m = strings.TrimSpace(m)
	switch m {
	case "", "gpt-4o-realtime-preview":
		return "gpt-realtime-2.1"
	case "gpt-4o-mini-realtime-preview":
		return "gpt-realtime-2.1-mini"
	default:
		return m
	}
}

func resolveRealtimeVoice(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if realtimeSupportedVoices[v] {
		return v
	}
	if v == "tempo" {
		return "cedar"
	}
	return "marin"
}

func resolveLiveVoice(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	for _, opt := range voices {
		if opt.ID == v {
			return v
		}
	}
	return "bossa"
}

func resolveTranscriptionLang(lang string) string {
	l := strings.ToLower(strings.TrimSpace(lang))
	if strings.HasPrefix(l, "pt") || l == "" {
		return "pt"
	}
	if strings.HasPrefix(l, "en") {
		return "en"
	}
	if strings.HasPrefix(l, "es") {
		return "es"
	}
	return "pt"
}

func (p *Provider) Connect(ctx context.Context, cfg voice.SessionConfig) (voice.Session, error) {
	if p.apiKey == "" {
		return nil, errors.New("OPENAI_API_KEY não configurada")
	}

	modelTrim := strings.ToLower(strings.TrimSpace(cfg.Model))
	voiceTrim := strings.ToLower(strings.TrimSpace(cfg.Voice))
	// Usa o protocolo GPT-Live (/v1/live/sessions) quando o modelo for gpt-live-1
	// ou quando estiver gerando amostra de uma voz exclusiva do GPT-Live (ex: bossa, tempo)
	if strings.HasPrefix(modelTrim, "gpt-live") || (modelTrim == "" && liveOnlyVoices[voiceTrim]) {
		return p.connectLive(ctx, cfg)
	}

	modelID := resolveRealtimeModel(cfg.Model)
	voiceID := resolveRealtimeVoice(cfg.Voice)
	langCode := resolveTranscriptionLang(cfg.Language)

	wsURL := fmt.Sprintf("wss://api.openai.com/v1/realtime?model=%s", modelID)
	header := http.Header{}
	header.Set("Authorization", "Bearer "+p.apiKey)

	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		return nil, fmt.Errorf("openai realtime dial: %w", err)
	}

	conn.SetReadLimit(16 * 1024 * 1024)

	sess := &openAISession{
		conn:   conn,
		events: make(chan voice.Event, 256),
	}

	// Configuração da sessão na API GA do OpenAI Realtime (/v1/realtime)
	sessionUpdate := map[string]any{
		"type": "session.update",
		"session": map[string]any{
			"type":              "realtime",
			"output_modalities": []string{"audio"},
			"instructions":      cfg.SystemPrompt,
			"tools":             openAITools(cfg.Tools),
			"tool_choice":       "auto",
			"audio": map[string]any{
				"input": map[string]any{
					"format": map[string]any{
						"type": "audio/pcm",
						"rate": 24000,
					},
					"transcription": map[string]any{
						"model":    "gpt-4o-mini-transcribe",
						"language": langCode,
					},
					"turn_detection": map[string]any{
						"type":                "server_vad",
						"threshold":           0.5,
						"prefix_padding_ms":   300,
						"silence_duration_ms": 550,
						"create_response":     true,
						"interrupt_response":  true,
					},
				},
				"output": map[string]any{
					"format": map[string]any{
						"type": "audio/pcm",
						"rate": 24000,
					},
					"voice": voiceID,
				},
			},
		},
	}

	updateBytes, _ := json.Marshal(sessionUpdate)
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := conn.Write(writeCtx, websocket.MessageText, updateBytes); err != nil {
		_ = conn.Close(websocket.StatusInternalError, "erro no setup")
		return nil, err
	}

	go sess.readLoop()
	return sess, nil
}

// connectLive estabelece uma sessão WebSocket no protocolo GPT-Live 1 (wss://api.openai.com/v1/live/sessions).
func (p *Provider) connectLive(ctx context.Context, cfg voice.SessionConfig) (voice.Session, error) {
	wsURL := "wss://api.openai.com/v1/live/sessions"
	header := http.Header{}
	header.Set("Authorization", "Bearer "+p.apiKey)

	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		return nil, fmt.Errorf("openai gpt-live-1 dial: %w", err)
	}
	conn.SetReadLimit(16 * 1024 * 1024)

	sess := &openAILiveSession{
		conn:      conn,
		events:    make(chan voice.Event, 256),
		startedCh: make(chan struct{}),
		stopCh:    make(chan struct{}),
	}
	sess.lastAudioAt.Store(time.Now().UnixMilli())

	livePrompt := cfg.SystemPrompt
	if len(livePrompt) > 28000 {
		livePrompt = livePrompt[:28000]
	}
	if len(cfg.Tools) > 0 {
		livePrompt += "\n\nQuando precisar executar qualquer ferramenta (como encerrar_conversa, transfer_to_agent ou webhooks), delegue imediatamente ao backend."
	}

	voiceID := resolveLiveVoice(cfg.Voice)
	sessionStart := map[string]any{
		"type":     "session.start",
		"event_id": "ev_start_1",
		"session": map[string]any{
			"model":        "gpt-live-1",
			"instructions": livePrompt,
			"audio": map[string]any{
				"format": map[string]any{
					"type": "audio/pcm",
					"rate": 24000,
				},
				"output": map[string]any{
					"voice": voiceID,
				},
			},
			"delegation": map[string]any{
				"type": "responses",
				"responses": map[string]any{
					"model":        "gpt-4.1-mini",
					"instructions": cfg.SystemPrompt,
					"tools":        openAITools(cfg.Tools),
					"tool_choice":  "auto",
				},
			},
		},
	}

	if err := sess.writeJSON(sessionStart); err != nil {
		_ = conn.Close(websocket.StatusInternalError, "erro no session.start")
		return nil, err
	}

	go sess.readLoop()

	// Aguarda session.started antes de retornar para garantir que SendText/SendAudio funcionem imediatamente
	select {
	case <-sess.startedCh:
	case <-time.After(5 * time.Second):
	case <-ctx.Done():
		_ = sess.Close()
		return nil, ctx.Err()
	}

	go sess.silenceKeepaliveLoop()
	return sess, nil
}

func openAITools(tools []voice.ToolSpec) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		props := map[string]any{}
		required := []string{}
		for _, p := range t.Parameters {
			props[p.Name] = map[string]any{"type": p.Type, "description": p.Description}
			if p.Required {
				required = append(required, p.Name)
			}
		}
		out = append(out, map[string]any{
			"type":        "function",
			"name":        t.Name,
			"description": t.Description,
			"parameters": map[string]any{
				"type": "object", "properties": props, "required": required,
			},
		})
	}
	return out
}

// ============================================================================
// Sessão GPT-Live 1 (wss://api.openai.com/v1/live/sessions)
// ============================================================================

type openAILiveSession struct {
	conn           *websocket.Conn
	events         chan voice.Event
	startedCh      chan struct{}
	startedOnce    sync.Once
	stopCh         chan struct{}
	stopOnce       sync.Once
	writeMu        sync.Mutex
	mu             sync.Mutex
	closed         bool
	eventSeq       atomic.Int64
	lastAudioAt    atomic.Int64
	lastOutputAt   atomic.Int64
	agentSpeaking  atomic.Bool
	silentTailSent atomic.Int32
	turnTimer      *time.Timer
	speechTimer    *time.Timer
}

func isAudiblePCM16(pcm []byte) bool {
	num := len(pcm) / 2
	for i := 0; i < num; i++ {
		s := int(int16(binary.LittleEndian.Uint16(pcm[i*2:])))
		if s < 0 {
			s = -s
		}
		if s > 32 {
			return true
		}
	}
	return false
}

func (s *openAILiveSession) nextEventID(prefix string) string {
	n := s.eventSeq.Add(1)
	return fmt.Sprintf("%s_%d", prefix, n)
}

func (s *openAILiveSession) writeJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	writeCtx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	return s.conn.Write(writeCtx, websocket.MessageText, data)
}

// silenceKeepaliveLoop mantém o relógio de áudio do GPT-Live 1 avançando apenas quando não há
// stream de microfone ativo há >= 600ms (ex: no preview de voz ou antes do cliente conectar).
func (s *openAILiveSession) silenceKeepaliveLoop() {
	silence100ms := base64.StdEncoding.EncodeToString(make([]byte, 4800)) // 100ms em 24kHz PCM16
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			if time.Since(time.UnixMilli(s.lastAudioAt.Load())) >= 600*time.Millisecond {
				_ = s.writeJSON(map[string]string{
					"type":  "session.input_audio.append",
					"audio": silence100ms,
				})
			}
		}
	}
}

func (s *openAILiveSession) SendAudio(pcm16k []byte) error {
	pcm24k := upsample16to24Bytes(pcm16k)
	if len(pcm24k) == 0 {
		return nil
	}
	s.lastAudioAt.Store(time.Now().UnixMilli())
	b64 := base64.StdEncoding.EncodeToString(pcm24k)
	return s.writeJSON(map[string]string{
		"type":  "session.input_audio.append",
		"audio": b64,
	})
}

func (s *openAILiveSession) SendText(text string) error {
	return s.writeJSON(map[string]any{
		"type":          "session.instructions.append",
		"event_id":      s.nextEventID("inst"),
		"delegation_id": nil,
		"content":       text,
	})
}

func (s *openAILiveSession) SendToolResult(call voice.ToolCall, result map[string]any) error {
	raw, _ := json.Marshal(result)
	itemMsg := map[string]any{
		"type":     "response.item.create",
		"event_id": s.nextEventID("tool_res"),
		"item": map[string]any{
			"type":    "function_call_output",
			"call_id": call.ID,
			"output":  string(raw),
		},
	}
	if err := s.writeJSON(itemMsg); err != nil {
		return err
	}
	return s.writeJSON(map[string]any{
		"type":     "response.create",
		"event_id": s.nextEventID("cont"),
	})
}

func (s *openAILiveSession) Events() <-chan voice.Event { return s.events }

func (s *openAILiveSession) Close() error {
	s.stopOnce.Do(func() { close(s.stopCh) })
	s.mu.Lock()
	s.closed = true
	if s.turnTimer != nil {
		s.turnTimer.Stop()
	}
	if s.speechTimer != nil {
		s.speechTimer.Stop()
	}
	s.mu.Unlock()
	_ = s.writeJSON(map[string]string{"type": "session.close"})
	return s.conn.Close(websocket.StatusNormalClosure, "sessão encerrada")
}

func (s *openAILiveSession) readLoop() {
	defer func() {
		s.stopOnce.Do(func() { close(s.stopCh) })
		close(s.events)
	}()
	for {
		msgType, data, err := s.conn.Read(context.Background())
		if err != nil {
			s.mu.Lock()
			cl := s.closed
			s.mu.Unlock()
			if cl {
				s.events <- voice.Event{Type: voice.EventClosed}
			} else {
				s.events <- voice.Event{Type: voice.EventClosed, Err: err}
			}
			return
		}
		if msgType == websocket.MessageText {
			s.dispatch(data)
		}
	}
}

func (s *openAILiveSession) dispatch(data []byte) {
	var base struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &base); err != nil {
		return
	}

	switch base.Type {
	case "session.started":
		s.startedOnce.Do(func() { close(s.startedCh) })

	case "session.output_audio.delta":
		var ev struct {
			Delta string `json:"delta"`
		}
		if json.Unmarshal(data, &ev) == nil && ev.Delta != "" {
			raw, err := base64.StdEncoding.DecodeString(ev.Delta)
			if err == nil {
				// No GPT-Live 1, o servidor transmite frames de silêncio (zeros) continuamente mesmo quando
				// o assistente está apenas ouvindo. Ignoramos frames silenciosos fora da fala do agente
				// para não bloquear o microfone do Caller nem impedir o EventTurnComplete.
				if !isAudiblePCM16(raw) {
					if !s.agentSpeaking.Load() {
						return
					}
					if s.silentTailSent.Add(1) > 2 {
						return
					}
				} else {
					s.silentTailSent.Store(0)
					s.agentSpeaking.Store(true)
					s.lastOutputAt.Store(time.Now().UnixMilli())
				}

				s.events <- voice.Event{Type: voice.EventAudio, Audio: raw}
				s.mu.Lock()
				if s.turnTimer != nil {
					s.turnTimer.Stop()
				}
				s.turnTimer = time.AfterFunc(550*time.Millisecond, func() {
					s.agentSpeaking.Store(false)
					s.mu.Lock()
					cl := s.closed
					s.mu.Unlock()
					if !cl {
						s.events <- voice.Event{Type: voice.EventTurnComplete}
					}
				})
				s.mu.Unlock()
			}
		}

	case "session.output_transcript.delta":
		var ev struct {
			Delta string `json:"delta"`
		}
		if json.Unmarshal(data, &ev) == nil && ev.Delta != "" {
			s.events <- voice.Event{Type: voice.EventAgentTranscript, Text: ev.Delta}
		}

	case "session.input_transcript.delta":
		var ev struct {
			Delta string `json:"delta"`
		}
		if json.Unmarshal(data, &ev) == nil && ev.Delta != "" {
			// Só emite EventInterrupted uma única vez por turno se o agente estiver realmente falando no momento
			if time.Since(time.UnixMilli(s.lastOutputAt.Load())) < 350*time.Millisecond && s.agentSpeaking.CompareAndSwap(true, false) {
				s.events <- voice.Event{Type: voice.EventInterrupted}
			}
			s.events <- voice.Event{Type: voice.EventCallerTranscript, Text: ev.Delta}
			s.mu.Lock()
			if s.speechTimer != nil {
				s.speechTimer.Stop()
			}
			s.speechTimer = time.AfterFunc(450*time.Millisecond, func() {
				s.mu.Lock()
				cl := s.closed
				s.mu.Unlock()
				if !cl {
					s.events <- voice.Event{Type: voice.EventCallerSpeechEnd}
				}
			})
			s.mu.Unlock()
		}

	case "response.event":
		var env struct {
			DelegationID string `json:"delegation_id"`
			Event        struct {
				Type string `json:"type"`
				Item struct {
					Type      string `json:"type"`
					CallID    string `json:"call_id"`
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"item"`
			} `json:"event"`
		}
		if json.Unmarshal(data, &env) == nil {
			if env.Event.Type == "response.output_item.done" && env.Event.Item.Type == "function_call" {
				var args map[string]any
				_ = json.Unmarshal([]byte(env.Event.Item.Arguments), &args)
				s.events <- voice.Event{
					Type: voice.EventToolCall,
					ToolCall: &voice.ToolCall{
						ID:   env.Event.Item.CallID,
						Name: env.Event.Item.Name,
						Args: args,
					},
				}
			}
		}
	}
}

// ============================================================================
// Sessão OpenAI Realtime GA (wss://api.openai.com/v1/realtime)
// ============================================================================

type openAISession struct {
	conn    *websocket.Conn
	events  chan voice.Event
	writeMu sync.Mutex
	mu      sync.Mutex
	closed  bool
}

// upsample16to24Bytes converte PCM16 LE de 16000 Hz para 24000 Hz (razão 2 -> 3)
// exigido pelo OpenAI Realtime e GPT-Live (`audio/pcm` rate 24000).
func upsample16to24Bytes(pcm16k []byte) []byte {
	num16 := len(pcm16k) / 2
	if num16 == 0 {
		return nil
	}
	numPairs := num16 / 2
	if numPairs == 0 {
		return pcm16k
	}
	out := make([]byte, numPairs*3*2)
	for i := 0; i < numPairs; i++ {
		s0 := int32(int16(binary.LittleEndian.Uint16(pcm16k[(i*2)*2:])))
		s1 := int32(int16(binary.LittleEndian.Uint16(pcm16k[(i*2+1)*2:])))
		var s2 int32
		if i*2+2 < num16 {
			s2 = int32(int16(binary.LittleEndian.Uint16(pcm16k[(i*2+2)*2:])))
		} else {
			s2 = s1
		}
		o0 := int16(s0)
		o1 := int16((s0 + 2*s1) / 3)
		o2 := int16((2*s1 + s2) / 3)

		base := i * 3 * 2
		binary.LittleEndian.PutUint16(out[base:], uint16(o0))
		binary.LittleEndian.PutUint16(out[base+2:], uint16(o1))
		binary.LittleEndian.PutUint16(out[base+4:], uint16(o2))
	}
	return out
}

func (s *openAISession) writeJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	writeCtx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	return s.conn.Write(writeCtx, websocket.MessageText, data)
}

func (s *openAISession) SendAudio(pcm16k []byte) error {
	pcm24k := upsample16to24Bytes(pcm16k)
	if len(pcm24k) == 0 {
		return nil
	}
	b64 := base64.StdEncoding.EncodeToString(pcm24k)
	return s.writeJSON(map[string]string{
		"type":  "input_audio_buffer.append",
		"audio": b64,
	})
}

func (s *openAISession) SendText(text string) error {
	item := map[string]any{
		"type": "conversation.item.create",
		"item": map[string]any{
			"type": "message",
			"role": "user",
			"content": []map[string]any{
				{"type": "input_text", "text": text},
			},
		},
	}
	if err := s.writeJSON(item); err != nil {
		return err
	}
	return s.writeJSON(map[string]string{"type": "response.create"})
}

func (s *openAISession) SendToolResult(call voice.ToolCall, result map[string]any) error {
	raw, _ := json.Marshal(result)
	item := map[string]any{
		"type": "conversation.item.create",
		"item": map[string]any{
			"type":    "function_call_output",
			"call_id": call.ID,
			"output":  string(raw),
		},
	}
	if err := s.writeJSON(item); err != nil {
		return err
	}
	return s.writeJSON(map[string]string{"type": "response.create"})
}

func (s *openAISession) Events() <-chan voice.Event { return s.events }

func (s *openAISession) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return s.conn.Close(websocket.StatusNormalClosure, "sessão encerrada")
}

func (s *openAISession) readLoop() {
	defer close(s.events)
	for {
		msgType, data, err := s.conn.Read(context.Background())
		if err != nil {
			s.mu.Lock()
			cl := s.closed
			s.mu.Unlock()
			if cl {
				s.events <- voice.Event{Type: voice.EventClosed}
			} else {
				s.events <- voice.Event{Type: voice.EventClosed, Err: err}
			}
			return
		}

		if msgType == websocket.MessageText {
			s.dispatch(data)
		}
	}
}

func (s *openAISession) dispatch(data []byte) {
	var base struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &base); err != nil {
		return
	}

	switch base.Type {
	case "response.output_audio.delta", "response.audio.delta":
		var ev struct {
			Delta string `json:"delta"`
		}
		if json.Unmarshal(data, &ev) == nil && ev.Delta != "" {
			raw, err := base64.StdEncoding.DecodeString(ev.Delta)
			if err == nil {
				s.events <- voice.Event{Type: voice.EventAudio, Audio: raw}
			}
		}

	case "response.output_audio_transcript.delta", "response.audio_transcript.delta":
		var ev struct {
			Delta string `json:"delta"`
		}
		if json.Unmarshal(data, &ev) == nil && ev.Delta != "" {
			s.events <- voice.Event{Type: voice.EventAgentTranscript, Text: ev.Delta}
		}

	case "conversation.item.input_audio_transcription.completed":
		var ev struct {
			Transcript string `json:"transcript"`
		}
		if json.Unmarshal(data, &ev) == nil && strings.TrimSpace(ev.Transcript) != "" {
			s.events <- voice.Event{Type: voice.EventCallerTranscript, Text: ev.Transcript}
		}

	case "input_audio_buffer.speech_stopped":
		s.events <- voice.Event{Type: voice.EventCallerSpeechEnd}

	case "input_audio_buffer.speech_started":
		s.events <- voice.Event{Type: voice.EventInterrupted}

	case "response.function_call_arguments.done":
		var ev struct {
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}
		if json.Unmarshal(data, &ev) == nil {
			var args map[string]any
			_ = json.Unmarshal([]byte(ev.Arguments), &args)
			s.events <- voice.Event{
				Type:     voice.EventToolCall,
				ToolCall: &voice.ToolCall{ID: ev.CallID, Name: ev.Name, Args: args},
			}
		}

	case "response.done":
		var ev struct {
			Response struct {
				Status string `json:"status"`
				Output []struct {
					Type string `json:"type"`
				} `json:"output"`
				Usage struct {
					TotalTokens       int64 `json:"total_tokens"`
					InputTokens       int64 `json:"input_tokens"`
					OutputTokens      int64 `json:"output_tokens"`
					InputTokenDetails struct {
						TextTokens   int64 `json:"text_tokens"`
						AudioTokens  int64 `json:"audio_tokens"`
						CachedTokens int64 `json:"cached_tokens"`
					} `json:"input_token_details"`
					OutputTokenDetails struct {
						TextTokens  int64 `json:"text_tokens"`
						AudioTokens int64 `json:"audio_tokens"`
					} `json:"output_token_details"`
				} `json:"usage"`
			} `json:"response"`
		}
		hasMessage := false
		hasFunctionCall := false
		if json.Unmarshal(data, &ev) == nil {
			for _, item := range ev.Response.Output {
				if item.Type == "message" {
					hasMessage = true
				} else if item.Type == "function_call" {
					hasFunctionCall = true
				}
			}
			if ev.Response.Usage.TotalTokens > 0 {
				u := ev.Response.Usage
				s.events <- voice.Event{
					Type: voice.EventUsage,
					Usage: &voice.Usage{
						TotalTokens:       u.TotalTokens,
						PromptTokens:      u.InputTokens,
						ResponseTokens:    u.OutputTokens,
						InputAudioTokens:  u.InputTokenDetails.AudioTokens,
						InputTextTokens:   u.InputTokenDetails.TextTokens,
						OutputAudioTokens: u.OutputTokenDetails.AudioTokens,
						OutputTextTokens:  u.OutputTokenDetails.TextTokens,
						CachedTokens:      u.InputTokenDetails.CachedTokens,
					},
				}
			}
		}
		if hasFunctionCall && !hasMessage {
			return
		}
		s.events <- voice.Event{Type: voice.EventTurnComplete}
	}
}
