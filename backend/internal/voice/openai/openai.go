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
	{ID: "gpt-5.6-terra", Label: "GPT-5.6 Terra (Padrão Reasoner)", Description: "Padrão estilo Vapi — equilíbrio ideal entre raciocínio, CPF/procedimentos e baixa latência", Default: true},
	{ID: "gpt-6-luna", Label: "GPT-6 Luna", Description: "Ultra-rápido e econômico para alto volume"},
	{ID: "gpt-5.6-sol", Label: "GPT-5.6 Sol", Description: "Máxima precisão na série 5.6 para procedimentos complexos"},
	{ID: "gpt-5.6-luna", Label: "GPT-5.6 Luna", Description: "Baixa latência e custo reduzido (série 5.6)"},
	{ID: "gpt-6.2-sol", Label: "GPT-6.2 Sol", Description: "Modelo avançado série 6.2 para agentes em escala"},
	{ID: "gpt-6-sol", Label: "GPT-6 Sol", Description: "Especializado em fluxos agênticos e múltiplas ferramentas"},
	{ID: "gpt-6.1-terra", Label: "GPT-6.1 Terra", Description: "Rápido e preciso para procedimentos estruturados"},
	{ID: "gpt-6.2-astra", Label: "GPT-6.2 Astra", Description: "Máxima capacidade de raciocínio para regras críticas"},
	{ID: "gpt-5.4-mini", Label: "GPT-5.4 Mini", Description: "Modelo Mini ágil para subagentes e ferramentas"},
	{ID: "gpt-4.1-mini", Label: "GPT-4.1 Mini (Legado)", Description: "Compatibilidade com projetos legados"},
}

var fallbackDelegationChain = []string{
	"gpt-5.6-terra",
	"gpt-6-luna",
	"gpt-5.6-luna",
	"gpt-5.4-mini",
	"gpt-4.1-mini",
}

func resolveDelegationModel(m string) string {
	m = strings.TrimSpace(m)
	switch m {
	case "", "gpt-4.1-nano", "o4-mini", "o3-mini", "o1-mini", "gpt-5-nano", "gpt-5.4-nano":
		return "gpt-5.6-terra"
	default:
		return m
	}
}

func resolveReasoningEffort(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "none", "minimal", "low", "medium", "high", "xhigh":
		return strings.ToLower(strings.TrimSpace(effort))
	default:
		return "low"
	}
}

func supportsReasoningEffort(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(m, "gpt-5") || strings.HasPrefix(m, "gpt-6") || strings.HasPrefix(m, "o")
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

	if !cfg.DeferFirstMessage {
		select {
		case <-sess.updatedCh:
		case <-time.After(1200 * time.Millisecond):
		case <-ctx.Done():
			_ = sess.Close()
			return nil, ctx.Err()
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

// extractSpeakerPersonaFromClassicPrompt extrai apenas as seções de identidade, voz e tom
// quando o usuário ainda não preencheu um Speaker Prompt separado, garantindo que regras de
// procedimentos, fluxos passo a passo e ferramentas fiquem 100% restritos ao Reasoner.
func extractSpeakerPersonaFromClassicPrompt(classicPrompt, agentName string) string {
	trimmed := strings.TrimSpace(classicPrompt)
	if trimmed == "" {
		if agentName != "" {
			return fmt.Sprintf("Você é %s, assistente virtual de atendimento por voz. Fale com cordialidade, clareza e objetividade.", agentName)
		}
		return "Você é uma assistente virtual de atendimento por voz cordial, clara e prestativa."
	}

	lines := strings.Split(trimmed, "\n")
	type mdSection struct {
		heading string
		body    strings.Builder
	}
	var sections []mdSection
	var preamble strings.Builder
	curIdx := -1

	for _, line := range lines {
		tLine := strings.TrimSpace(line)
		if strings.HasPrefix(tLine, "#") {
			h := strings.TrimLeft(tLine, "#")
			h = strings.TrimSpace(h)
			sections = append(sections, mdSection{heading: strings.ToLower(h)})
			curIdx = len(sections) - 1
			sections[curIdx].body.WriteString(line)
			sections[curIdx].body.WriteString("\n")
			continue
		}
		if curIdx >= 0 {
			sections[curIdx].body.WriteString(line)
			sections[curIdx].body.WriteString("\n")
		} else {
			preamble.WriteString(line)
			preamble.WriteString("\n")
		}
	}

	if len(sections) > 0 {
		var kept strings.Builder
		if p := strings.TrimSpace(preamble.String()); p != "" {
			kept.WriteString(truncateUTF8Runes(p, 500))
			kept.WriteString("\n\n")
		}
		for _, sec := range sections {
			h := sec.heading
			isSpeakerSection := strings.Contains(h, "personal") ||
				strings.Contains(h, "role") ||
				strings.Contains(h, "papel") ||
				strings.Contains(h, "identidade") ||
				strings.Contains(h, "persona") ||
				strings.Contains(h, "tone") ||
				strings.Contains(h, "tom") ||
				strings.Contains(h, "style") ||
				strings.Contains(h, "estilo") ||
				strings.Contains(h, "environment") ||
				strings.Contains(h, "ambiente") ||
				strings.Contains(h, "pronunc") ||
				strings.Contains(h, "pronúnc")
			if isSpeakerSection {
				kept.WriteString(strings.TrimSpace(sec.body.String()))
				kept.WriteString("\n\n")
			}
		}
		if res := strings.TrimSpace(kept.String()); res != "" {
			return truncateUTF8Runes(res, 2500)
		}
	}

	// Se for texto corrido sem seções markdown, mantém apenas o parágrafo inicial de apresentação/persona
	firstPara := trimmed
	if idx := strings.Index(firstPara, "\n\n"); idx > 0 {
		firstPara = strings.TrimSpace(firstPara[:idx])
	}
	return truncateUTF8Runes(firstPara, 600)
}

func renderPersonalityPacks(packs []string) string {
	var sb strings.Builder
	for _, p := range packs {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "unhurried":
			sb.WriteString("\nEstilo (Unhurried): Fale em ritmo calmo e claro, articulando bem datas, valores e nomes, deixando espaço natural entre ideias.")
		case "eager-listener":
			sb.WriteString("\nEstilo (Eager Listener): Em conversas abertas (exceto durante ditado de números/CPF), demonstre escuta atenta e dê espaço para o cliente concluir.")
		case "bouncy":
			sb.WriteString("\nEstilo (Bouncy): Use entonação acolhedora, expressiva e natural, adaptando-se ao humor do cliente sem exageros.")
		case "idle-hummer":
			sb.WriteString("\nEstilo (Idle Hummer): Mantenha presença suave durante esperas mais longas de ferramentas sem repetir frases de 'ainda verificando'.")
		case "patient-numbers":
			sb.WriteString("\nEstilo (Patient Numbers): Aguarde em silêncio absoluto toda a sequência de dígitos quando o cliente ditar CPF, protocolo, telefone ou data.")
		}
	}
	return sb.String()
}

// buildLiveSpeakerInstructions constrói o prompt exclusivo do Speaker (modelo gpt-live-1),
// seguindo o padrão Vapi: APENAS condução de fala, escuta, pausas e gatilhos de delegação,
// SEM o conteúdo detalhado de procedimentos ou regras de ferramentas.
func buildLiveSpeakerInstructions(cfg voice.SessionConfig, firstMsg string) string {
	speakerBase := strings.TrimSpace(cfg.SpeakerPrompt)
	if speakerBase == "" {
		source := cfg.VoicePrompt
		if strings.TrimSpace(source) == "" {
			source = cfg.SystemPrompt
		}
		speakerBase = extractSpeakerPersonaFromClassicPrompt(source, cfg.AgentName)
	} else {
		speakerBase = truncateUTF8Runes(speakerBase, 4000)
	}

	var sb strings.Builder
	sb.WriteString(speakerBase)

	if packsText := renderPersonalityPacks(cfg.PersonalityPacks); packsText != "" {
		sb.WriteString("\n")
		sb.WriteString(packsText)
	}

	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(cfg.Language)), "pt") || cfg.Language == "" {
		sb.WriteString(`

Regras de fala e condução (Português do Brasil):
- Fale sempre em Português do Brasil (pt-BR), de forma cordial, natural e objetiva, em uma ou duas frases curtas por turno.
- Faça APENAS UMA pergunta por vez. Use os dados que o cliente já informou (como o nome dele) e nunca peça novamente o que já foi dito.
- Tags entre colchetes como [super fast] são marcações internas e jamais devem ser lidas em voz alta.
- PACIÊNCIA ABSOLUTA EM CPF E NÚMEROS: Clientes ditam CPF (11 dígitos), telefones e datas em blocos com pausas no meio (ex.: "176... 110... 240... 00"). Enquanto o cliente estiver ditando números, permaneça em SILÊNCIO TOTAL — não diga "uhum", "certo", "pode continuar" nem dê exemplos por cima da fala dele. Só responda e delegue depois que ele terminar de ditar os números e fizer uma pausa clara.
- SEPARE FALAR DE FAZER: Nunca diga "vou verificar" quando estiver apenas pedindo um dado que falta (como pedir o CPF). Se faltar o CPF, apenas peça o CPF de forma direta e aguarde em silêncio.`)
	}

	if firstMsg != "" {
		sb.WriteString(fmt.Sprintf("\n- Saudação inicial na abertura da chamada: diga apenas %q uma única vez e aguarde o cliente responder.", firstMsg))
	}

	sb.WriteString("\n\nBackchannel policy: Use light backchannels sparingly. Never produce any backchannel or interjection while the caller is dictating digits, a CPF, a name spelling, or dates.\n")
	sb.WriteString("\nInterruption policy: Stop speaking immediately when the caller interrupts. Listen until they finish.\n")

	hasRealTools := false
	for _, t := range cfg.Tools {
		if t.Name != "ativar_procedure" {
			hasRealTools = true
			break
		}
	}

	if hasRealTools || len(cfg.SkillSummaries) > 0 {
		sb.WriteString("\nDelegation policy:\nBackend tools:\n")
		for _, sk := range cfg.SkillSummaries {
			desc := truncateUTF8Runes(strings.TrimSpace(sk.Description), 200)
			sb.WriteString(fmt.Sprintf("- Procedure [%s]: %s\n", sk.Name, desc))
		}
		for _, t := range cfg.Tools {
			if t.Name == "ativar_procedure" {
				continue
			}
			desc := truncateUTF8Runes(strings.TrimSpace(t.Description), 180)
			sb.WriteString(fmt.Sprintf("- Tool [%s]: %s\n", t.Name, desc))
		}
		sb.WriteString(`
Generate <|SpawnThinking|> when:
- The caller states what they need (such as second copy of a bill/boleto, financial info, registration check, or any listed Procedure) so the reasoner can guide the exact next step.
- The reasoner asked for a detail (such as CPF, birth date, or holder/dependent confirmation) and the caller has finished providing it.
- The caller corrects a number, changes their request, asks to transfer to a human/agent, or says goodbye / asks to end the call.

Do not generate <|SpawnThinking|> when:
- Delivering the initial greeting at the start of the call before the caller speaks.
- The caller only said "só um momento / espera um instante" while looking for a document — simply say "Claro, fico aguardando" and wait quietly.
- The caller is still mid-dictation of a CPF or number sequence.

While work is running:
- Acknowledge the wait at most once briefly (e.g., "Só um instante enquanto verifico.") and give the caller quiet room. Do NOT repeat "vou verificar" twice and do NOT invent results while waiting.

Results:
- Relay the reasoner's exact question or verified facts naturally in one or two short sentences. Never claim a lookup, boleto send, or action succeeded unless the reasoner confirmed it.`)
	}

	return sb.String()
}

// buildLiveReasonerInstructions constrói o prompt exclusivo do Reasoner (delegation.responses),
// contendo todas as instruções de tarefa, procedimentos completos (Skills) e regras de ferramentas.
func buildLiveReasonerInstructions(cfg voice.SessionConfig) string {
	reasonerBody := strings.TrimSpace(cfg.ReasonerPrompt)
	if reasonerBody == "" {
		reasonerBody = strings.TrimSpace(cfg.SystemPrompt)
	}

	var sb strings.Builder
	sb.WriteString(`## Papel do Reasoner (Backend de Raciocínio, Procedimentos e Ferramentas)
Você é o Reasoner por trás da assistente de voz em uma ligação telefônica em Português do Brasil (pt-BR). Você NÃO fala diretamente com o cliente: o resultado que você retornar será falado pelo Speaker.

Regras essenciais do Reasoner:
1. Trabalhe sempre a partir da solicitação mais recente na transcrição da conversa. Transcrições de voz chegam fragmentadas, com vírgulas, pausas ("...") ou números por extenso em português ("meia" = 6, "zero" = 0, "um/uma" = 1, "dois/duas" = 2, "três" = 3, "quatro" = 4, "cinco" = 5, "sete" = 7, "oito" = 8, "nove" = 9, "dez" = 10, "onze" = 11, "vinte" = 20, "trinta" = 30, "quarenta" = 40, "cinquenta" = 50).
2. Reconstrução de CPF: junte os dígitos ditados pelo cliente (inclusive se vierem divididos em turnos seguidos como "176" e depois "11, 02, 40, 00" -> "17611024000"), removendo pontos, espaços, vírgulas e traços. Assim que houver 11 dígitos, CHAME IMEDIATAMENTE a ferramenta de consulta correspondente neste mesmo turno!
3. Se um dado obrigatório para a ferramenta ainda não foi informado (ex.: o cliente pediu 2ª via de boleto mas ainda não disse o CPF), retorne APENAS uma instrução curta e direta dizendo qual pergunta o Speaker deve fazer agora (ex.: "Peça ao cliente os 11 números do CPF do beneficiário."), sem dizer "vou verificar".
4. Se o CPF transcrito tiver menos de 11 dígitos (ex.: cortou um dígito na linha), peça de forma simples para o cliente repetir pausadamente os 11 números do CPF, sem dar exemplos longos.
5. Retorne SEMPRE fatos curtos, diretos e prontos para fala natural — SEM Markdown, SEM listas com asteriscos, SEM blocos técnicos. Nunca confirme uma ação ou envio que a ferramenta não tenha confirmado com sucesso.

`)
	sb.WriteString(reasonerBody)
	return sb.String()
}

func openAILiveTools(tools []voice.ToolSpec) []map[string]any {
	filtered := make([]voice.ToolSpec, 0, len(tools))
	for _, t := range tools {
		// No GPT-Live todos os procedimentos já estão carregados no Reasoner; remover ativar_procedure
		// evita gastar um round-trip extra antes de chamar a ferramenta real (ex: consultar_beneficiario).
		if t.Name == "ativar_procedure" {
			continue
		}
		filtered = append(filtered, t)
	}
	return openAITools(filtered)
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
	reasoningEffort := resolveReasoningEffort(cfg.ReasoningEffort)
	liveTools := openAILiveTools(cfg.Tools)

	seededGreeting := !cfg.DeferFirstMessage && firstMsg != ""

	sess := &openAILiveSession{
		conn:                    conn,
		events:                  make(chan voice.Event, 256),
		startedCh:               make(chan struct{}),
		stopCh:                  make(chan struct{}),
		startedTime:             time.Now(),
		hasTools:                len(liveTools) > 0,
		lang:                    cfg.Language,
		seededDeveloperGreeting: seededGreeting,
		activeDelegationModel:   delegationModel,
	}
	// Começa zerado para o relógio de áudio disparar imediatamente na primeira fala antes do microfone conectar
	sess.lastAudioAt.Store(0)

	speakerPrompt := buildLiveSpeakerInstructions(cfg, firstMsg)
	reasonerPrompt := buildLiveReasonerInstructions(cfg)
	voiceID := resolveLiveVoice(cfg.Voice)

	responsesCfg := map[string]any{
		"model":               delegationModel,
		"instructions":        reasonerPrompt,
		"tools":               liveTools,
		"tool_choice":         "auto",
		"parallel_tool_calls": false,
	}
	if supportsReasoningEffort(delegationModel) {
		responsesCfg["reasoning"] = map[string]any{
			"effort": reasoningEffort,
		}
	}

	sessionObj := map[string]any{
		"model":        "gpt-live-1",
		"instructions": speakerPrompt,
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
			"type":                     "responses",
			"acknowledgement_delay_ms": 600,
			"responses":                responsesCfg,
		},
	}

	// Guia oficial "Greet before the caller speaks": injeta a instrução de saudação em session.input
	// apenas quando a chamada já está conectada (não durante o PrewarmCall de telefonia enquanto o telefone toca).
	if seededGreeting {
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
	if !cfg.DeferFirstMessage && firstMsg == "" {
		sess.ensureKeepalive()
	}
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
	keepaliveOnce            sync.Once
	writeMu                  sync.Mutex
	mu                       sync.Mutex
	closed                   bool
	hasTools                 bool
	lang                     string
	seededDeveloperGreeting  bool
	activeDelegationModel    string
	fallbackAttempt          int
	firstMessagePrimed       bool
	startedTime              time.Time
	firstOutputAudioReceived atomic.Bool
	eventSeq                 atomic.Int64
	lastAudioAt              atomic.Int64
	nextKeepaliveAt          atomic.Int64
	lastOutputAt             atomic.Int64
	lastInterruptAt          atomic.Int64
	lastDelegationAt         atomic.Int64
	agentSpeaking            atomic.Bool
	silentTailSent           atomic.Int32
	turnTimer                *time.Timer
	speechTimer              *time.Timer
	inRem                    []byte
	prevInSample             int32
	hasPrevSample            bool
}

func (s *openAILiveSession) FirstMessagePrimed() bool { return s.firstMessagePrimed }

func (s *openAILiveSession) ensureKeepalive() {
	s.keepaliveOnce.Do(func() {
		go s.silenceKeepaliveLoop()
	})
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

func (s *openAILiveSession) TriggerFirstMessage(firstMessage string) error {
	fm := strings.TrimSpace(firstMessage)
	if fm == "" {
		s.ensureKeepalive()
		return nil
	}
	s.startedTime = time.Now()
	s.firstMessagePrimed = true
	s.lastAudioAt.Store(0)

	eventType := "session.instructions.append"
	if s.seededDeveloperGreeting {
		eventType = "session.commentary.append"
	}
	if err := s.writeJSON(map[string]any{
		"type":          eventType,
		"event_id":      s.nextEventID("greeting"),
		"delegation_id": nil,
		"content":       liveGreetingInstruction(fm, s.lang),
	}); err != nil {
		s.firstMessagePrimed = false
		return err
	}
	s.primeAudioClock(6)
	s.ensureKeepalive()
	return nil
}

func (s *openAILiveSession) SendAudio(pcm16k []byte) error {
	s.ensureKeepalive()
	s.mu.Lock()
	combined := append(s.inRem, pcm16k...)
	usableBytes := (len(combined) / 4) * 4
	var pcm24k []byte
	if usableBytes > 0 {
		pcm24k = upsample16to24Stateful(combined[:usableBytes], &s.prevInSample, &s.hasPrevSample)
	}
	s.inRem = append(s.inRem[:0], combined[usableBytes:]...)
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
	s.ensureKeepalive()
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
			// Só emite EventInterrupted com debounce de 800ms se o agente estiver realmente falando no momento
			nowMS := time.Now().UnixMilli()
			if nowMS-s.lastOutputAt.Load() < 350 && nowMS-s.lastInterruptAt.Load() > 800 && s.agentSpeaking.CompareAndSwap(true, false) {
				s.lastInterruptAt.Store(nowMS)
				s.events <- voice.Event{Type: voice.EventInterrupted}
			}
			s.events <- voice.Event{Type: voice.EventCallerTranscript, Text: ev.Delta}
			s.mu.Lock()
			if s.speechTimer != nil {
				s.speechTimer.Stop()
			}
			s.speechTimer = time.AfterFunc(550*time.Millisecond, func() {
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
			// Se o modelo selecionado não suportar reasoning.effort, remove reasoning mantendo o modelo
			if strings.Contains(errEv.Error.Param, "reasoning") {
				_ = s.writeJSON(map[string]any{
					"type":     "session.update",
					"event_id": s.nextEventID("del_noreason"),
					"session": map[string]any{
						"delegation": map[string]any{
							"type": "responses",
							"responses": map[string]any{
								"reasoning": nil,
							},
						},
					},
				})
				return
			}
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
					respPatch := map[string]any{
						"model": nextModel,
					}
					if !supportsReasoningEffort(nextModel) {
						respPatch["reasoning"] = nil
					}
					_ = s.writeJSON(map[string]any{
						"type":     "session.update",
						"event_id": s.nextEventID("del_fallback"),
						"session": map[string]any{
							"delegation": map[string]any{
								"type":      "responses",
								"responses": respPatch,
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
	inRem                    []byte
	prevInSample             int32
	hasPrevSample            bool
}

func (s *openAISession) FirstMessagePrimed() bool { return s.firstMessagePrimed }

func (s *openAISession) TriggerFirstMessage(firstMessage string) error {
	fm := strings.TrimSpace(firstMessage)
	if fm == "" {
		return nil
	}
	if s.updatedCh != nil {
		select {
		case <-s.updatedCh:
		case <-time.After(800 * time.Millisecond):
		}
	}
	s.startedTime = time.Now()
	s.firstMessagePrimed = true
	if err := s.SendText(firstMessagePrompt(fm)); err != nil {
		s.firstMessagePrimed = false
		return err
	}
	return nil
}

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
	combined := append(s.inRem, pcm16k...)
	usableBytes := (len(combined) / 4) * 4
	var pcm24k []byte
	if usableBytes > 0 {
		pcm24k = upsample16to24Stateful(combined[:usableBytes], &s.prevInSample, &s.hasPrevSample)
	}
	s.inRem = append(s.inRem[:0], combined[usableBytes:]...)
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
