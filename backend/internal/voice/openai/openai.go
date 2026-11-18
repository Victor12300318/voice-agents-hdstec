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

var delegationModels = []voice.Option{
	{ID: "gpt-6-luna", Label: "GPT-6 Luna", Description: "Recomendado pela OpenAI para GPT-Live — ultra-rápido e econômico para tools e procedimentos", Default: true},
	{ID: "gpt-6.2-sol", Label: "GPT-6.2 Sol", Description: "Equilíbrio ideal entre inteligência e velocidade para agentes em escala"},
	{ID: "gpt-6-sol", Label: "GPT-6 Sol", Description: "Especializado em fluxos agênticos complexos e múltiplas ferramentas"},
	{ID: "gpt-6.1-terra", Label: "GPT-6.1 Terra", Description: "Rápido e preciso para procedimentos estruturados"},
	{ID: "gpt-6.2-astra", Label: "GPT-6.2 Astra", Description: "Máxima capacidade de raciocínio para regras críticas"},
	{ID: "gpt-5.6-luna", Label: "GPT-5.6 Luna", Description: "Baixa latência otimizado para alto volume (série 5.6)"},
	{ID: "gpt-5.6-terra", Label: "GPT-5.6 Terra", Description: "Equilibrado em inteligência e custo (série 5.6)"},
	{ID: "gpt-5.4-mini", Label: "GPT-5.4 Mini", Description: "Modelo Mini ágil para subagentes e chamada de ferramentas"},
	{ID: "gpt-5.4", Label: "GPT-5.4", Description: "Modelo completo série 5.4"},
	{ID: "gpt-4.1-mini", Label: "GPT-4.1 Mini (Legado)", Description: "Compatibilidade com projetos legados"},
}

var fallbackDelegationChain = []string{
	"gpt-6-luna",
	"gpt-5.6-luna",
	"gpt-5.4-mini",
	"gpt-4.1-mini",
}

func resolveDelegationModel(m string) string {
	m = strings.TrimSpace(m)
	switch m {
	case "", "gpt-4.1-nano", "o4-mini", "o3-mini", "o1-mini", "gpt-5-nano", "gpt-5.4-nano":
		return "gpt-6-luna"
	default:
		return m
	}
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
		DelegationModels:  delegationModels,
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
		conn:        conn,
		events:      make(chan voice.Event, 256),
		updatedCh:   make(chan struct{}),
		startedTime: time.Now(),
	}
	go sess.readLoop()

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
						"model":    "gpt-4o-transcribe",
						"language": langCode,
						"prompt":   "Conversa telefônica em português do Brasil (pt-BR). Palavras frequentes: Sim, Não, Pode sim, Já, Tudo bem, Trabalho, Dia a dia, WhatsApp, Redes sociais, CPF, Gigas, Internet, Plano, TIM.",
					},
					"turn_detection": map[string]any{
						"type":                "server_vad",
						"threshold":           0.55,
						"prefix_padding_ms":   300,
						"silence_duration_ms": 500,
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

	if fm := strings.TrimSpace(cfg.FirstMessage); fm != "" {
		select {
		case <-sess.updatedCh:
		case <-time.After(1200 * time.Millisecond):
		case <-ctx.Done():
			_ = sess.Close()
			return nil, ctx.Err()
		}
		sess.startedTime = time.Now()
		sess.firstMessagePrimed = true
		if err := sess.SendText(firstMessagePrompt(fm)); err != nil {
			sess.firstMessagePrimed = false
		}
	}
	return sess, nil
}

func firstMessagePrompt(firstMessage string) string {
	return fmt.Sprintf(
		"ATENÇÃO (PRIMEIRO TURNO DA LIGAÇÃO): Fale agora IMEDIATAMENTE e em tom natural APENAS e EXATAMENTE esta frase de saudação, sem chamar ferramentas nem delegar tarefas neste turno, e aguarde o cliente responder:\n%q",
		firstMessage,
	)
}

func liveGreetingInstruction(firstMessage, lang string) string {
	l := strings.ToLower(strings.TrimSpace(lang))
	if l == "" || strings.HasPrefix(l, "pt") {
		return fmt.Sprintf(
			"Cumprimente o cliente agora imediatamente em Português do Brasil dizendo exatamente: %q. Em seguida, faça uma pausa e escute.",
			firstMessage,
		)
	}
	return fmt.Sprintf(
		"Greet the caller now immediately in %s by saying: %q. Then pause and listen.",
		lang, firstMessage,
	)
}

func truncateUTF8Runes(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes])
}

func buildLiveFrontendInstructions(cfg voice.SessionConfig, firstMsg string) string {
	base := strings.TrimSpace(cfg.VoicePrompt)
	if base == "" {
		base = strings.TrimSpace(cfg.SystemPrompt)
	}
	base = truncateUTF8Runes(base, 10000)

	var sb strings.Builder
	if firstMsg != "" {
		sb.WriteString(fmt.Sprintf(
			" Saudação inicial obrigatória ao iniciar a chamada: diga exatamente %q uma única vez e aguarde o cliente responder.\n\n",
			firstMsg,
		))
	}
	sb.WriteString(base)

	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(cfg.Language)), "pt") || cfg.Language == "" {
		sb.WriteString("\n\nATENÇÃO DE ESCUTA E TRANSCRIÇÃO (PT-BR): O cliente fala em Português do Brasil (pt-BR) por telefone. Interprete respostas curtas e números ditados em português do Brasil ('meia' = 6, 'zero' = 0, 'Sim', 'Não', 'Pode sim', CPF em blocos). Nunca mude para espanhol ou inglês.")
	}

	// Estrutura oficial do guia Prompting GPT-Live (developers.openai.com/api/docs/guides/live-prompting):
	// Para o gpt-live-1, a delegação DEVE ser solicitada via "Generate <|SpawnThinking|>".
	sb.WriteString("\n\nBackchannel policy: Use brief, natural listening cues sparingly without competing with the caller or interrupting numbers/CPF dictation.\n")
	sb.WriteString("\nInterruption policy: Stop speaking when the user interrupts and listen until they finish.\n")

	if len(cfg.Tools) > 0 {
		sb.WriteString("\nDelegation policy:\nBackend tools:\n")
		for _, t := range cfg.Tools {
			if t.Name == "ativar_procedure" {
				continue
			}
			desc := truncateUTF8Runes(strings.TrimSpace(t.Description), 220)
			sb.WriteString(fmt.Sprintf("- %s: %s\n", t.Name, desc))
		}
		sb.WriteString(`
Generate <|SpawnThinking|> when:
- The user provides a CPF, document number, birth date, verification answer, or any data needed to look up or validate their account/registration/boleto.
- The user asks for a second copy of a bill/boleto, invoice details, registration lookup, transfer, or any action handled by Backend tools.
- A correction or additional detail changes a lookup or task already requested.
- Whenever you tell the user you will check, verify, consult, transfer, or end the call ("vou verificar", "só um instante", "já consulto"), you MUST generate <|SpawnThinking|> immediately.

Do not generate <|SpawnThinking|> when:
- Delivering the opening greeting before the user has spoken.
- Asking the user for a missing piece of information (for example, asking them to say their CPF before they have provided it).

Generate <|SpawnThinking|> before giving an answer that depends on backend work.
Do not guess the result while waiting.`)
	}

	return sb.String()
}

func buildLiveBackendInstructions(cfg voice.SessionConfig) string {
	var sb strings.Builder
	sb.WriteString(`## Voice conversation context
You are the reasoning and tool-execution backend for a live telephone voice assistant in Brazilian Portuguese (pt-BR).
- Spoken transcripts arrive in fragments and may contain pauses, punctuation ("..."), or Brazilian Portuguese number words: "meia" = 6, "zero" = 0, "um/uma" = 1, "dois/duas" = 2, "três" = 3, "quatro" = 4, "cinco" = 5, "sete" = 7, "oito" = 8, "nove" = 9, "dez" = 10, "onze" = 11, "vinte" = 20, "trinta" = 30, "quarenta" = 40, "cinquenta" = 50, "sessenta" = 60, "setenta" = 70, "oitenta" = 80, "noventa" = 90, "cem" = 100 (for example: "1 7 meia ... 11 ... 0 2 ... 40 00" = "17611024000").
- Always reconstruct and strip spaces, dots, and hyphens from spoken CPF digits before calling any lookup tool. If the reconstructed CPF has 11 digits, call the lookup tool IMMEDIATELY in this response!
- All agent procedures are already expanded below: call the required business/webhook/MCP tool DIRECTLY (do NOT waste a turn calling ativar_procedure first).
- Return concise, factual results and the exact next step or question for the voice assistant to speak naturally to the caller.

`)
	sb.WriteString(cfg.SystemPrompt)
	return sb.String()
}

func promisedBackendLookup(spoken string) bool {
	s := strings.ToLower(strings.TrimSpace(spoken))
	if s == "" {
		return false
	}
	for _, kw := range []string{
		"vou verificar",
		"já vou verificar",
		"vou consultar",
		"já vou consultar",
		"vou checar",
		"vou buscar",
		"já te retorno",
		"só um instante",
		"so um instante",
		"aguarde um instante",
		"um momento enquanto",
	} {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
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

	firstMsg := strings.TrimSpace(cfg.FirstMessage)
	delegationModel := resolveDelegationModel(cfg.DelegationModel)

	sess := &openAILiveSession{
		conn:                  conn,
		events:                make(chan voice.Event, 256),
		startedCh:             make(chan struct{}),
		stopCh:                make(chan struct{}),
		startedTime:           time.Now(),
		hasTools:              len(cfg.Tools) > 0,
		activeDelegationModel: delegationModel,
	}
	// Começa zerado para o relógio de áudio disparar imediatamente na primeira fala antes do microfone conectar
	sess.lastAudioAt.Store(0)

	livePrompt := buildLiveFrontendInstructions(cfg, firstMsg)
	backendPrompt := buildLiveBackendInstructions(cfg)
	voiceID := resolveLiveVoice(cfg.Voice)

	sessionObj := map[string]any{
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
				"model":               delegationModel,
				"instructions":        backendPrompt,
				"tools":               openAITools(cfg.Tools),
				"tool_choice":         "auto",
				"parallel_tool_calls": false,
			},
		},
	}

	// Guia oficial "Greet before the caller speaks": injeta a instrução de saudação em session.input
	// (role: "developer") para estar disponível já no instante t=0ms do startup.
	if firstMsg != "" {
		sessionObj["input"] = []map[string]any{
			{
				"type": "message",
				"role": "developer",
				"content": []map[string]any{
					{
						"type": "input_text",
						"text": liveGreetingInstruction(firstMsg, cfg.Language),
					},
				},
			},
		}
	}

	sessionStart := map[string]any{
		"type":     "session.start",
		"event_id": "ev_start_1",
		"session":  sessionObj,
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

	sess.startedTime = time.Now()
	if firstMsg != "" {
		sess.firstMessagePrimed = true
		if err := sess.triggerFirstMessage(firstMsg, cfg.Language); err != nil {
			sess.firstMessagePrimed = false
		}
	}

	go sess.silenceKeepaliveLoop()
	return sess, nil
}

func openAITools(tools []voice.ToolSpec) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		var schema map[string]any
		if len(t.ParametersSchema) > 0 {
			schema = voice.SanitizeToolJSONSchema(t.ParametersSchema)
		} else {
			props := map[string]any{}
			required := []string{}
			for _, p := range t.Parameters {
				props[p.Name] = map[string]any{"type": p.Type, "description": p.Description}
				if p.Required {
					required = append(required, p.Name)
				}
			}
			schema = map[string]any{
				"type": "object", "properties": props, "required": required,
			}
		}
		out = append(out, map[string]any{
			"type":        "function",
			"name":        t.Name,
			"description": t.Description,
			"parameters":  schema,
		})
	}
	return out
}

// ============================================================================
// Sessão GPT-Live 1 (wss://api.openai.com/v1/live/sessions)
// ============================================================================

type openAILiveSession struct {
	conn                     *websocket.Conn
	events                   chan voice.Event
	startedCh                chan struct{}
	startedOnce              sync.Once
	stopCh                   chan struct{}
	stopOnce                 sync.Once
	writeMu                  sync.Mutex
	mu                       sync.Mutex
	closed                   bool
	hasTools                 bool
	activeDelegationModel    string
	fallbackAttempt          int
	firstMessagePrimed       bool
	startedTime              time.Time
	firstOutputAudioReceived atomic.Bool
	eventSeq                 atomic.Int64
	lastAudioAt              atomic.Int64
	nextKeepaliveAt          atomic.Int64
	lastOutputAt             atomic.Int64
	lastDelegationAt         atomic.Int64
	agentSpeaking            atomic.Bool
	silentTailSent           atomic.Int32
	turnTranscript           strings.Builder
	turnTimer                *time.Timer
	speechTimer              *time.Timer
	firstMsgRetryTimer       *time.Timer
	prevInSample             int32
	hasPrevSample            bool
}

func (s *openAILiveSession) FirstMessagePrimed() bool { return s.firstMessagePrimed }

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

var silenceFrame100msB64 = base64.StdEncoding.EncodeToString(make([]byte, 4800)) // 100ms em 24kHz PCM16

// silenceKeepaliveLoop mantém o relógio de áudio do GPT-Live 1 avançando apenas quando não há
// stream de microfone ativo há >= 150ms (ex: no preview de voz ou antes do cliente conectar).
func (s *openAILiveSession) silenceKeepaliveLoop() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			nowMS := time.Now().UnixMilli()
			if nowMS < s.nextKeepaliveAt.Load() {
				continue
			}
			if time.Since(time.UnixMilli(s.lastAudioAt.Load())) >= 150*time.Millisecond {
				_ = s.writeJSON(map[string]string{
					"type":  "session.input_audio.append",
					"audio": silenceFrame100msB64,
				})
			}
		}
	}
}

func (s *openAILiveSession) primeAudioClock(frames int) {
	if frames <= 0 {
		frames = 2
	}
	for i := 0; i < frames; i++ {
		_ = s.writeJSON(map[string]string{
			"type":  "session.input_audio.append",
			"audio": silenceFrame100msB64,
		})
	}
	if frames > 2 {
		catchupMS := int64(frames-1) * 85
		s.nextKeepaliveAt.Store(time.Now().Add(time.Duration(catchupMS) * time.Millisecond).UnixMilli())
	}
}

func (s *openAILiveSession) triggerFirstMessage(firstMsg, lang string) error {
	// Guia oficial "Greet before the caller speaks": após session.started, envia session.commentary.append
	// com delegation_id: null e mantém o stream de áudio ativo.
	if err := s.writeJSON(map[string]any{
		"type":          "session.commentary.append",
		"event_id":      s.nextEventID("greeting"),
		"delegation_id": nil,
		"content":       liveGreetingInstruction(firstMsg, lang),
	}); err != nil {
		return err
	}
	s.primeAudioClock(6)

	// Watchdog: caso o modelo ainda não tenha emitido áudio após 950ms, envia session.instructions.append
	// (disclosure exato conforme guia "Deliver a disclosure") + avanço de relógio.
	s.mu.Lock()
	if s.firstMsgRetryTimer != nil {
		s.firstMsgRetryTimer.Stop()
	}
	s.firstMsgRetryTimer = time.AfterFunc(950*time.Millisecond, func() {
		if s.firstOutputAudioReceived.Load() {
			return
		}
		s.mu.Lock()
		cl := s.closed
		s.mu.Unlock()
		if cl {
			return
		}
		_ = s.writeJSON(map[string]any{
			"type":          "session.instructions.append",
			"event_id":      s.nextEventID("greeting_retry"),
			"delegation_id": nil,
			"content":       fmt.Sprintf("Immediately say the following greeting in full and then wait for the caller: %s", firstMsg),
		})
		s.primeAudioClock(4)
	})
	s.mu.Unlock()
	return nil
}

func (s *openAILiveSession) SendAudio(pcm16k []byte) error {
	s.mu.Lock()
	pcm24k := upsample16to24Stateful(pcm16k, &s.prevInSample, &s.hasPrevSample)
	s.mu.Unlock()
	if len(pcm24k) == 0 {
		return nil
	}
	// Enquanto a primeira mensagem ainda não começou a sair em áudio, envia silêncio digital puro
	// para impedir que cliques de microfone ou ruído de linha façam o VAD do GPT-Live cancelar a saudação.
	if s.firstMessagePrimed && !s.firstOutputAudioReceived.Load() && time.Since(s.startedTime) < 2500*time.Millisecond {
		pcm24k = make([]byte, len(pcm24k))
	}
	s.lastAudioAt.Store(time.Now().UnixMilli())
	b64 := base64.StdEncoding.EncodeToString(pcm24k)
	return s.writeJSON(map[string]string{
		"type":  "session.input_audio.append",
		"audio": b64,
	})
}

func (s *openAILiveSession) SendText(text string) error {
	if err := s.writeJSON(map[string]any{
		"type":          "session.instructions.append",
		"event_id":      s.nextEventID("inst"),
		"delegation_id": nil,
		"content":       text,
	}); err != nil {
		return err
	}
	// Se ainda não há áudio do microfone/telefonia chegando, avança imediatamente o relógio de áudio do GPT-Live
	if time.Since(time.UnixMilli(s.lastAudioAt.Load())) >= 100*time.Millisecond {
		s.primeAudioClock(6)
	}
	return nil
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
	if s.firstMsgRetryTimer != nil {
		s.firstMsgRetryTimer.Stop()
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
					if s.silentTailSent.Add(1) > 12 {
						return
					}
				} else {
					s.firstOutputAudioReceived.Store(true)
					s.silentTailSent.Store(0)
					s.agentSpeaking.Store(true)
					s.lastOutputAt.Store(time.Now().UnixMilli())
				}

				s.events <- voice.Event{Type: voice.EventAudio, Audio: raw}
				s.mu.Lock()
				if s.turnTimer != nil {
					s.turnTimer.Stop()
				}
				s.turnTimer = time.AfterFunc(650*time.Millisecond, func() {
					s.agentSpeaking.Store(false)
					s.mu.Lock()
					cl := s.closed
					spokenTurn := s.turnTranscript.String()
					s.turnTranscript.Reset()
					s.mu.Unlock()
					if !cl {
						// Safety-net: se o GPT-Live disse ao cliente "vou verificar / só um instante" mas esqueceu
						// de emitir <|SpawnThinking|> (nenhuma delegação nos últimos 4s), dispara response.create no backend!
						if s.hasTools && promisedBackendLookup(spokenTurn) && time.Since(time.UnixMilli(s.lastDelegationAt.Load())) > 4*time.Second {
							s.lastDelegationAt.Store(time.Now().UnixMilli())
							_ = s.writeJSON(map[string]any{
								"type":     "response.create",
								"event_id": s.nextEventID("auto_del"),
							})
						}
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
			s.mu.Lock()
			s.turnTranscript.WriteString(ev.Delta)
			s.mu.Unlock()
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
				if !cl && !s.agentSpeaking.Load() {
					s.events <- voice.Event{Type: voice.EventCallerSpeechEnd}
				}
			})
			s.mu.Unlock()
		}

	case "session.delegation.created":
		s.lastDelegationAt.Store(time.Now().UnixMilli())

	case "error":
		var errEv struct {
			Error struct {
				Code          string `json:"code"`
				Message       string `json:"message"`
				Param         string `json:"param"`
				ClientEventID string `json:"client_event_id"`
			} `json:"error"`
		}
		if json.Unmarshal(data, &errEv) == nil {
			// Auto-recuperação caso o modelo configurado em delegation.responses não esteja habilitado na conta/tier
			if errEv.Error.Code == "responses_validation_failed" || strings.HasPrefix(errEv.Error.Param, "session.delegation.responses") {
				s.mu.Lock()
				var nextModel string
				for s.fallbackAttempt < len(fallbackDelegationChain) {
					cand := fallbackDelegationChain[s.fallbackAttempt]
					s.fallbackAttempt++
					if cand != s.activeDelegationModel {
						nextModel = cand
						s.activeDelegationModel = cand
						break
					}
				}
				s.mu.Unlock()
				if nextModel != "" {
					_ = s.writeJSON(map[string]any{
						"type":     "session.update",
						"event_id": s.nextEventID("del_fallback"),
						"session": map[string]any{
							"delegation": map[string]any{
								"type": "responses",
								"responses": map[string]any{
									"model": nextModel,
								},
							},
						},
					})
				}
			}
		}

	case "response.event":
		s.lastDelegationAt.Store(time.Now().UnixMilli())
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
				Response struct {
					Usage struct {
						TotalTokens       int64 `json:"total_tokens"`
						InputTokens       int64 `json:"input_tokens"`
						OutputTokens      int64 `json:"output_tokens"`
						InputTokenDetails struct {
							CachedTokens int64 `json:"cached_tokens"`
						} `json:"input_token_details"`
					} `json:"usage"`
				} `json:"response"`
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
			} else if env.Event.Type == "response.completed" && env.Event.Response.Usage.TotalTokens > 0 {
				u := env.Event.Response.Usage
				s.events <- voice.Event{
					Type: voice.EventUsage,
					Usage: &voice.Usage{
						TotalTokens:      u.TotalTokens,
						PromptTokens:     u.InputTokens,
						ResponseTokens:   u.OutputTokens,
						InputTextTokens:  u.InputTokens,
						OutputTextTokens: u.OutputTokens,
						CachedTokens:     u.InputTokenDetails.CachedTokens,
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
	conn                     *websocket.Conn
	events                   chan voice.Event
	updatedCh                chan struct{}
	updatedOnce              sync.Once
	writeMu                  sync.Mutex
	mu                       sync.Mutex
	closed                   bool
	firstMessagePrimed       bool
	startedTime              time.Time
	firstOutputAudioReceived atomic.Bool
	prevInSample             int32
	hasPrevSample            bool
}

func (s *openAISession) FirstMessagePrimed() bool { return s.firstMessagePrimed }

// upsample16to24Bytes converte PCM16 LE de 16000 Hz para 24000 Hz (razão 2 -> 3).
func upsample16to24Bytes(pcm16k []byte) []byte {
	var prev int32
	var hasPrev bool
	return upsample16to24Stateful(pcm16k, &prev, &hasPrev)
}

// upsample16to24Stateful converte PCM16 LE de 16000 Hz para 24000 Hz preservando a última amostra
// do chunk anterior (sPrev) para espaçamento exato de 2/3 sem descontinuidade nas fronteiras de pacote.
func upsample16to24Stateful(pcm16k []byte, prevSample *int32, hasPrev *bool) []byte {
	num16 := len(pcm16k) / 2
	if num16 == 0 {
		return nil
	}
	numPairs := num16 / 2
	if numPairs == 0 {
		return pcm16k
	}
	out := make([]byte, numPairs*3*2)
	sPrev := int32(0)
	if prevSample != nil && hasPrev != nil && *hasPrev {
		sPrev = *prevSample
	} else {
		sPrev = int32(int16(binary.LittleEndian.Uint16(pcm16k[0:])))
	}

	for i := 0; i < numPairs; i++ {
		s0 := int32(int16(binary.LittleEndian.Uint16(pcm16k[(i*2)*2:])))
		s1 := int32(int16(binary.LittleEndian.Uint16(pcm16k[(i*2+1)*2:])))

		o0 := int16((sPrev + 2*s0) / 3)
		o1 := int16((2*s0 + s1) / 3)
		o2 := int16(s1)
		sPrev = s1

		base := i * 3 * 2
		binary.LittleEndian.PutUint16(out[base:], uint16(o0))
		binary.LittleEndian.PutUint16(out[base+2:], uint16(o1))
		binary.LittleEndian.PutUint16(out[base+4:], uint16(o2))
	}

	if prevSample != nil && hasPrev != nil {
		*prevSample = sPrev
		*hasPrev = true
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
	s.mu.Lock()
	pcm24k := upsample16to24Stateful(pcm16k, &s.prevInSample, &s.hasPrevSample)
	s.mu.Unlock()
	if len(pcm24k) == 0 {
		return nil
	}
	if s.firstMessagePrimed && !s.firstOutputAudioReceived.Load() && time.Since(s.startedTime) < 2500*time.Millisecond {
		pcm24k = make([]byte, len(pcm24k))
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
	case "session.updated":
		s.updatedOnce.Do(func() {
			if s.updatedCh != nil {
				close(s.updatedCh)
			}
		})

	case "response.output_audio.delta", "response.audio.delta":
		var ev struct {
			Delta string `json:"delta"`
		}
		if json.Unmarshal(data, &ev) == nil && ev.Delta != "" {
			raw, err := base64.StdEncoding.DecodeString(ev.Delta)
			if err == nil {
				s.firstOutputAudioReceived.Store(true)
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
