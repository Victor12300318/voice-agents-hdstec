package runtime

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"voiceagent/internal/billing"
	"voiceagent/internal/guardian"
	"voiceagent/internal/procedures"
	"voiceagent/internal/recording"
	"voiceagent/internal/store"
	"voiceagent/internal/tools"
	"voiceagent/internal/variables"
	"voiceagent/internal/voice"
)

func (rt *Runtime) runSession(
	ctx context.Context,
	ws *websocket.Conn,
	orgID, agentID string,
	versionID *string,
	isTest bool,
	cfg store.AgentConfig,
	prov voice.Provider,
	caller string,
) {
	opts := SessionOptions{
		OrgID:      orgID,
		AgentID:    agentID,
		VersionID:  versionID,
		IsTest:     isTest,
		Caller:     caller,
		Direction:  "web",
		FromNumber: caller,
		ToNumber:   "",
		Transport:  NewWebSocketTransport(ws),
	}
	rt.RunSessionWithOptions(ctx, opts, cfg, prov)
}

func (rt *Runtime) RunSessionWithOptions(
	ctx context.Context,
	opts SessionOptions,
	cfg store.AgentConfig,
	prov voice.Provider,
) {
	cat := prov.Catalog()
	startedAt := time.Now()

	rec := recording.NewSessionRecorder()
	rec.SetStartTime(startedAt)

	var lastCallerSpeechEnd time.Time
	var turnHadToolCall bool

	var sessionUsage voice.Usage
	var sessionUsageMu sync.Mutex

	tr := opts.Transport
	orgID := opts.OrgID
	agentID := opts.AgentID
	versionID := opts.VersionID
	isTest := opts.IsTest
	caller := opts.Caller

	sendJSON := func(v serverMessage) error {
		return tr.SendMessage(ctx, v)
	}

	sendBinary := func(data []byte) error {
		return tr.SendAudio(ctx, data)
	}

	// 1. Cria a Conversation no banco
	direction := opts.Direction
	if direction == "" {
		direction = "web"
	}
	conv, err := rt.Store.CreateConversation(ctx, orgID, agentID, versionID, isTest, cfg.Provider, cfg.Model, cfg, nil, direction, opts.FromNumber, opts.ToNumber)
	if err != nil {
		rt.Log.Error("create_conversation_error", "error", err)
		_ = sendJSON(serverMessage{Type: "error", Message: "erro ao registrar conversa"})
		return
	}

	endCall := func(status, reason string) billing.CostResult {
		dur := time.Since(startedAt).Milliseconds()
		sessionUsageMu.Lock()
		finalUsage := sessionUsage
		sessionUsageMu.Unlock()

		calcRes := billing.Calculate(cfg.Provider, cfg.Model, finalUsage, dur)

		_ = rt.Store.EndConversation(context.Background(), orgID, conv.ID, status, reason, dur, calcRes.TotalCostUSD, calcRes.TotalCostBRL, calcRes)
		_ = rt.Store.RecordEvent(context.Background(), orgID, conv.ID, "ended", map[string]any{
			"status": status, "reason": reason, "duration_ms": dur,
			"cost_usd": calcRes.TotalCostUSD, "cost_brl": calcRes.TotalCostBRL,
			"tokens_usage": calcRes,
		})
		rt.Log.Info("conversation_ended", "conversation_id", conv.ID, "status", status, "reason", reason, "duration_ms", dur, "cost_usd", calcRes.TotalCostUSD, "cost_brl", calcRes.TotalCostBRL)
		return calcRes
	}

	// 2. Resolve variáveis
	allVars := map[string]string{}
	for _, decl := range cfg.Variables {
		allVars[decl.Name] = ""
	}

	callerID := opts.FromNumber
	if callerID == "" {
		callerID = caller
	}

	// Dynamic variables + Overrides via Initiation Webhook (padrão ElevenLabs Twilio Personalization)
	if cfg.InitiationWebhookURL != "" {
		var initRes variables.InitiationResult
		var initErr error

		if prefRes, ok, prefErr := rt.consumePrefetchedInitiation(ctx, opts.CallSid); ok {
			initRes, initErr = prefRes, prefErr
		} else {
			timeout := time.Duration(cfg.InitiationWebhookTimeout) * time.Millisecond
			if timeout <= 0 {
				timeout = 3 * time.Second
			}
			initRes, initErr = variables.FetchInitiationData(
				ctx,
				rt.HTTPClient,
				cfg.InitiationWebhookURL,
				timeout,
				variables.InitiationRequest{
					CallerID:       callerID,
					AgentID:        agentID,
					CalledNumber:   opts.ToNumber,
					CallSid:        opts.CallSid,
					ConversationID: conv.ID,
					Caller:         callerID,
					Metadata:       map[string]any{},
				},
				cfg.Variables,
			)
		}

		if initErr != nil {
			rt.Log.Warn("initiation_webhook_failed", "conversation_id", conv.ID, "caller_id", callerID, "error", initErr)
			_ = rt.Store.RecordEvent(ctx, orgID, conv.ID, "initiation_webhook_warning", map[string]any{
				"phase": "initiation",
				"error": initErr.Error(),
			})
		} else {
			for k, v := range initRes.DynamicVariables {
				allVars[k] = v
			}
			if len(initRes.MissingVariables) > 0 {
				rt.Log.Warn("initiation_webhook_missing_vars", "conversation_id", conv.ID, "missing", initRes.MissingVariables)
			}
			// Aplica conversation_config_override se retornado pelo webhook (padrão ElevenLabs)
			if ov := initRes.ConversationConfigOverride; ov != nil {
				if ov.Agent != nil {
					if ov.Agent.Prompt != nil {
						if strings.TrimSpace(ov.Agent.Prompt.Prompt) != "" {
							cfg.SystemPrompt = ov.Agent.Prompt.Prompt
						}
						if strings.TrimSpace(ov.Agent.Prompt.LLM) != "" {
							cfg.Model = ov.Agent.Prompt.LLM
						}
					}
					if strings.TrimSpace(ov.Agent.FirstMessage) != "" {
						cfg.FirstMessage = ov.Agent.FirstMessage
					}
					if strings.TrimSpace(ov.Agent.Language) != "" {
						cfg.Language = ov.Agent.Language
					}
				}
				if ov.TTS != nil && strings.TrimSpace(ov.TTS.VoiceID) != "" {
					cfg.Voice = ov.TTS.VoiceID
				}
			}
			_ = rt.Store.RecordEvent(ctx, orgID, conv.ID, "initiation_webhook_loaded", map[string]any{
				"dynamic_variables": initRes.DynamicVariables,
				"has_overrides":     initRes.ConversationConfigOverride != nil,
			})
		}
	}

	// System variables no fuso do agente (incluindo system__caller_id, system__called_number, system__call_sid, etc.)
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		loc = time.UTC
	}
	sysVars := variables.SystemForCall(startedAt, loc, conv.ID, agentID, callerID, opts.ToNumber, opts.CallSid, cfg.Timezone)
	for k, v := range sysVars {
		allVars[k] = v
	}

	// Alias inteligente priNome <-> primNome e fallback a partir da primeira palavra de "nome"
	if allVars["primNome"] == "" && allVars["priNome"] != "" {
		allVars["primNome"] = allVars["priNome"]
	}
	if allVars["priNome"] == "" && allVars["primNome"] != "" {
		allVars["priNome"] = allVars["primNome"]
	}
	if allVars["primNome"] == "" && allVars["nome"] != "" {
		fields := strings.Fields(allVars["nome"])
		if len(fields) > 0 {
			allVars["primNome"] = fields[0]
			allVars["priNome"] = fields[0]
		}
	}

	// Atualiza snapshot das variáveis resolvidas na conversa
	rawVars, _ := json.Marshal(allVars)
	_, _ = rt.Store.Pool.Exec(ctx, `UPDATE conversations SET variables = $3 WHERE organization_id = $1 AND id = $2`, orgID, conv.ID, rawVars)

	// 3. Renderiza prompts
	firstMessage := variables.Render(cfg.FirstMessage, allVars)
	systemPrompt := variables.Render(cfg.SystemPrompt, allVars)
	if cfg.Language != "" {
		systemPrompt += fmt.Sprintf("\n\nDIRETRIZ DE IDIOMA: Responda obrigatoriamente no idioma configurado (%s). Fale em ritmo ágil, natural e fluido. Tags entre colchetes como [super fast] são marcações de ritmo de fala e jamais devem ser lidas em voz alta.", cfg.Language)
	}
	if strings.TrimSpace(firstMessage) != "" {
		systemPrompt += fmt.Sprintf("\n\nREGRA OBRIGATÓRIA DE PRIMEIRA MENSAGEM: Na sua PRIMEIRA fala ao iniciar a conversa, você DEVE dizer EXCLUSIVAMENTE a frase de abertura configurada (%q) e PARAR de falar para aguardar a resposta do cliente. Só inicie o passo 1 de # Actions DEPOIS que o cliente responder à sua primeira mensagem.", firstMessage)
	}

	// 4. Carrega Procedures e monta índice de Triggers (ADR 0001 + Sub-procedures ElevenLabs)
	procsMap := make(map[string]store.ProcedureSnapshot)
	var activeProcedures []store.ProcedureSnapshot
	var entryProcedures []store.ProcedureSnapshot
	for _, p := range cfg.Procedures {
		if p.Active {
			procsMap[p.ID] = p
			activeProcedures = append(activeProcedures, p)
			// Sub-procedures têm Trigger vazio e só podem ser acionados por referência de outro Procedure
			if strings.TrimSpace(p.Trigger) != "" {
				entryProcedures = append(entryProcedures, p)
			}
		}
	}

	var toolSpecs []voice.ToolSpec
	registeredTools := make(map[string]store.Tool)
	addedToolIDs := make(map[string]bool)

	for _, st := range cfg.SystemTools {
		if st == "encerrar_conversa" {
			toolSpecs = append(toolSpecs,
				voice.ToolSpec{
					Name:        "encerrar_conversa",
					Description: "Encerra a conversa com o Caller após se despedir cordialmente.",
					Parameters:  []voice.Param{},
				},
				voice.ToolSpec{
					Name:        "end_call",
					Description: "Encerra a chamada telefônica imediatamente após a despedida cordial.",
					Parameters:  []voice.Param{},
				},
			)
		} else if st == "transferir_numero" {
			toolSpecs = append(toolSpecs, voice.ToolSpec{
				Name:        "transferir_numero",
				Description: "Transfere a chamada telefônica para outro número ou ramal após avisar o Caller.",
				Parameters: []voice.Param{
					{Name: "numero", Type: "string", Description: "Número de telefone de destino no formato E.164 ou ramal (ex: +5511999998888)", Required: true},
					{Name: "motivo", Type: "string", Description: "Motivo ou departamento da transferência (ex: financeiro, suporte humano)", Required: false},
				},
			})
		} else if st == "transferir_agente" || st == "transfer_to_agent" {
			transferDesc := "Transfere a conversa em andamento para outro agente de IA especializado quando a condição correspondente for atendida."
			var rulesPrompt strings.Builder
			if cfg.SystemToolsConfig.TransferToAgent != nil {
				if customDesc := strings.TrimSpace(cfg.SystemToolsConfig.TransferToAgent.Description); customDesc != "" {
					transferDesc = customDesc
				}
				if len(cfg.SystemToolsConfig.TransferToAgent.Transfers) > 0 {
					rulesPrompt.WriteString("\n\n## REGRAS DE TRANSFERÊNCIA ENTRE AGENTES (transfer_to_agent):\nQuando a solicitação do usuário atender a uma das condições abaixo, diga a frase de encaminhamento e CHAME IMEDIATAMENTE NO MESMO TURNO a ferramenta \"transfer_to_agent\" passando \"agent_number\" (indexado a partir de 0) e \"reason\":\n")
					for idx, rule := range cfg.SystemToolsConfig.TransferToAgent.Transfers {
						targetLabel := rule.AgentName
						if targetLabel == "" {
							if ta, err := rt.Store.GetAgent(ctx, orgID, rule.AgentID); err == nil {
								targetLabel = ta.Name
							} else {
								targetLabel = rule.AgentID
							}
						}
						rulesPrompt.WriteString(fmt.Sprintf("- agent_number = %d (Agente %q): %s\n", idx, targetLabel, rule.Condition))
					}
					transferDesc += " " + rulesPrompt.String()
					systemPrompt += rulesPrompt.String()
				}
			}
			transferParams := []voice.Param{
				{Name: "agent_number", Type: "integer", Description: "Número indexado a partir de zero (0, 1, 2...) do agente para o qual transferir com base nas regras configuradas", Required: true},
				{Name: "reason", Type: "string", Description: "O motivo da transferência do agente", Required: false},
			}
			toolSpecs = append(toolSpecs,
				voice.ToolSpec{
					Name:        "transfer_to_agent",
					Description: transferDesc,
					Parameters:  transferParams,
				},
				voice.ToolSpec{
					Name:        "transferir_agente",
					Description: transferDesc,
					Parameters:  transferParams,
				},
			)
		}
	}

	if len(entryProcedures) > 0 {
		toolSpecs = append(toolSpecs, voice.ToolSpec{
			Name:        "ativar_procedure",
			Description: "Ativa um procedimento de negócio quando o assunto do Caller corresponder ao seu gatilho. Retorna o passo a passo e libera as ferramentas necessárias.",
			Parameters: []voice.Param{
				{Name: "nome", Type: "string", Description: "Nome do procedimento a ativar", Required: true},
			},
		})

		var triggerIndex strings.Builder
		triggerIndex.WriteString("\n\n## PROCEDIMENTOS DISPONÍVEIS:\nQuando a solicitação do usuário corresponder a um dos tópicos abaixo, chame a ferramenta \"ativar_procedure\" passando o nome do procedimento:\n")
		for _, p := range entryProcedures {
			triggerIndex.WriteString(fmt.Sprintf("- \"%s\": %s\n", p.Name, p.Trigger))
		}
		systemPrompt += triggerIndex.String()
	}

	executor := tools.NewExecutor(rt.HTTPClient)
	procExpander := procedures.NewExpander(rt.Store)

	registerToolByID := func(toolID string) {
		if toolID == "" || addedToolIDs[toolID] {
			return
		}
		t, err := rt.Store.GetTool(ctx, orgID, toolID)
		if err == nil && t.Active {
			addedToolIDs[toolID] = true
			toolSpecs = append(toolSpecs, voice.ToolSpec{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			})
			registeredTools[t.Name] = t
		}
	}

	for _, toolID := range cfg.Tools {
		registerToolByID(toolID)
	}

	// Auto-attach: Tools referenciadas inline ou em Structured Procedures são registradas automaticamente (estilo ElevenLabs)
	for _, p := range activeProcedures {
		meta := procedures.InspectProcedure(store.Procedure{
			ID:      p.ID,
			Name:    p.Name,
			Type:    p.Type,
			Trigger: p.Trigger,
			Content: p.Content,
		}, procsMap, nil, nil)
		for _, refToolID := range meta.ReferencedToolIDs {
			registerToolByID(refToolID)
		}
	}

	// 5. Conecta com o Voice Provider
	provSess, err := prov.Connect(ctx, voice.SessionConfig{
		Model:        cfg.Model,
		Voice:        cfg.Voice,
		Language:     cfg.Language,
		SystemPrompt: systemPrompt,
		Tools:        toolSpecs,
	})
	if err != nil {
		rt.Log.Error("provider_connect_failed", "conversation_id", conv.ID, "provider", cfg.Provider, "error", err)
		_ = rt.Store.RecordEvent(ctx, orgID, conv.ID, "error", map[string]any{"phase": "connect", "error": err.Error()})
		endCall("failed", "provider_error")
		sendJSON(serverMessage{Type: "error", Message: fmt.Sprintf("falha ao conectar com o Voice Provider: %v", err)})
		return
	}
	var provSessMu sync.RWMutex
	getProvSess := func() voice.Session {
		provSessMu.RLock()
		defer provSessMu.RUnlock()
		return provSess
	}
	var transcriptHistory []string

	defer func() {
		if s := getProvSess(); s != nil {
			s.Close()
		}
	}()

	_ = rt.Store.MarkConversationActive(ctx, orgID, conv.ID)

	// Envia mensagem ready para o navegador
	_ = sendJSON(serverMessage{
		Type:             "ready",
		ConversationID:   conv.ID,
		InputSampleRate:  cat.InputSampleRate,
		OutputSampleRate: cat.OutputSampleRate,
		Variables:        allVars,
	})

	// Controle de ignorar áudio do usuário durante primeira mensagem não-interrompível
	var ignoringCallerAudio atomic.Bool
	if firstMessage != "" && !cfg.FirstMessageInterruptible {
		ignoringCallerAudio.Store(true)
		time.AfterFunc(5*time.Second, func() {
			ignoringCallerAudio.Store(false)
		})
	}

	// 5. Inicia First Message se configurada
	if firstMessage != "" {
		_ = rt.Store.RecordEvent(ctx, orgID, conv.ID, "first_message_sent", map[string]any{"text": firstMessage})
		instruction := fmt.Sprintf("ATENÇÃO (PRIMEIRO TURNO DA LIGAÇÃO): Diga agora APENAS e EXATAMENTE a seguinte frase de saudação e aguarde em silêncio o cliente responder (NÃO fale a apresentação da empresa ainda neste turno):\n\"%s\"", firstMessage)
		if err := provSess.SendText(instruction); err != nil {
			rt.Log.Warn("first_message_send_failed", "conversation_id", conv.ID, "error", err)
		}
	}

	// Timer de duração máxima
	maxDur := time.Duration(cfg.MaxDurationSeconds) * time.Second
	if maxDur <= 0 || maxDur > time.Duration(cat.MaxSessionSeconds)*time.Second {
		maxDur = time.Duration(cat.MaxSessionSeconds) * time.Second
	}
	maxTimer := time.NewTimer(maxDur)
	defer maxTimer.Stop()

	sessionCtx, cancelSession := context.WithCancel(ctx)
	defer cancelSession()

	var endOnce sync.Once
	finalStatus := "completed"
	finalReason := "caller_hangup"
	pendingHangupAfterTurn := false
	pendingTransferTarget := ""
	var callEnding atomic.Bool
	var turnAgentAudioBytes int
	var turnFirstAudioAt time.Time
	var currentTurnAgentText strings.Builder
	var playbackMu sync.Mutex
	var agentPlaybackEndsAt time.Time

	var guardianMu sync.Mutex
	var currentCallerTurnText strings.Builder
	var lastCallerTurnText string
	var lastCallerTranscriptAt time.Time
	var totalCallerTurns int
	var consecutiveAgentTurnsWithoutCaller int
	var muteWarnings int
	var hallucinationWarnings int
	var silenceTimer *time.Timer

	// Estado dos Procedures ativos durante a chamada (ADR 0001 + regra dos 5 mais recentes da ElevenLabs)
	var activeProcedureName string
	var activeProcedureID *string
	var recentActivatedProcs []procedures.ActivatedProcedure
	activeAllowedTools := make(map[string]bool)
	activeToolPolicies := make(map[string]procedures.ToolExecutionPolicy)
	// Se o agente não possui procedures de entrada, todas as tools registradas ficam permitidas
	if len(entryProcedures) == 0 {
		for name := range registeredTools {
			activeAllowedTools[name] = true
		}
	} else {
		// Ferramentas anexadas globalmente ao agente ficam sempre permitidas, além das anexadas por cada procedure
		for _, globalToolID := range cfg.Tools {
			if t, err := rt.Store.GetTool(ctx, orgID, globalToolID); err == nil {
				activeAllowedTools[t.Name] = true
			}
		}
	}

	terminate := func(status, reason string) {
		endOnce.Do(func() {
			guardianMu.Lock()
			if silenceTimer != nil {
				silenceTimer.Stop()
			}
			guardianMu.Unlock()
			finalStatus = status
			finalReason = reason
			cancelSession()
		})
	}

	resolveGuardianConfig := func() store.GuardianJevConfig {
		g := store.GuardianJevConfig{
			Enabled:                false,
			Provider:               "openrouter",
			Model:                  guardian.DefaultModel,
			CheckVoicemail:         true,
			CheckTalkingToSelf:     true,
			CheckHallucination:     true,
			VoicemailThreshold:     0.80,
			TalkingToSelfThreshold: 0.78,
			HallucinationThreshold: 0.75,
		}
		for _, st := range cfg.SystemTools {
			if st == "guardiao_jev" || st == "jev_guardian" || st == "detectar_correio_voz" {
				g.Enabled = true
			}
		}
		if cfg.SystemToolsConfig.GuardianJev != nil {
			custom := *cfg.SystemToolsConfig.GuardianJev
			g.Enabled = custom.Enabled
			if custom.Provider != "" {
				g.Provider = custom.Provider
			}
			if custom.Model != "" {
				g.Model = custom.Model
			}
			if custom.APIKey != "" {
				g.APIKey = custom.APIKey
			}
			g.CheckVoicemail = custom.CheckVoicemail
			g.CheckTalkingToSelf = custom.CheckTalkingToSelf
			g.CheckHallucination = custom.CheckHallucination
			if custom.VoicemailThreshold > 0 {
				g.VoicemailThreshold = custom.VoicemailThreshold
			}
			if custom.TalkingToSelfThreshold > 0 {
				g.TalkingToSelfThreshold = custom.TalkingToSelfThreshold
			}
			if custom.HallucinationThreshold > 0 {
				g.HallucinationThreshold = custom.HallucinationThreshold
			}
		} else if rt.Guardian != nil && rt.Guardian.IsConfigured("") {
			// Se houver chave global (OPENROUTER_API_KEY / TYPESAFE_API_KEY), ativa por padrão
			g.Enabled = true
		}
		return g
	}

	// Goroutine A: Provider -> Navegador
	go func() {
		defer cancelSession()
		for {
			curSess := getProvSess()
			select {
			case <-sessionCtx.Done():
				return
			case ev, ok := <-curSess.Events():
				if !ok {
					if getProvSess() != curSess {
						continue
					}
					return
				}
				switch ev.Type {
				case voice.EventAudio:
					if callEnding.Load() {
						continue
					}
					if turnAgentAudioBytes == 0 {
						turnFirstAudioAt = time.Now()
					}
					turnAgentAudioBytes += len(ev.Audio)
					playbackMu.Lock()
					nowPlay := time.Now()
					if nowPlay.After(agentPlaybackEndsAt) {
						agentPlaybackEndsAt = nowPlay
					}
					bPerSec := cat.OutputSampleRate * 2
					if bPerSec <= 0 {
						bPerSec = 48000
					}
					chunkDur := time.Duration(float64(len(ev.Audio)) / float64(bPerSec) * float64(time.Second))
					agentPlaybackEndsAt = agentPlaybackEndsAt.Add(chunkDur)
					playbackMu.Unlock()
					rec.AddAgentAudio(ev.Audio)
					if !lastCallerSpeechEnd.IsZero() {
						latencyMS := time.Since(lastCallerSpeechEnd).Milliseconds()
						_ = rt.Store.RecordEvent(sessionCtx, orgID, conv.ID, "response_latency", map[string]any{
							"latency_ms": latencyMS, "had_tool_call": turnHadToolCall,
						})
						lastCallerSpeechEnd = time.Time{}
					}
					if err := sendBinary(ev.Audio); err != nil {
						return
					}

				case voice.EventCallerSpeechEnd:
					lastCallerSpeechEnd = time.Now()
					turnAgentAudioBytes = 0
					currentTurnAgentText.Reset()

				case voice.EventCallerTranscript:
					if callEnding.Load() {
						continue
					}
					trimmedCaller := strings.TrimSpace(ev.Text)
					if trimmedCaller != "" {
						transcriptHistory = append(transcriptHistory, fmt.Sprintf("Usuário: %s", trimmedCaller))
						guardianMu.Lock()
						if silenceTimer != nil {
							silenceTimer.Stop()
						}
						currentCallerTurnText.WriteString(" ")
						currentCallerTurnText.WriteString(trimmedCaller)
						lastCallerTurnText = strings.TrimSpace(currentCallerTurnText.String())
						lastCallerTranscriptAt = time.Now()
						callerSnapshot := lastCallerTurnText
						turnsSnapshot := totalCallerTurns
						guardianMu.Unlock()

						// Checagem instantânea de Caixa Postal nas primeiras falas do Caller
						gCfg := resolveGuardianConfig()
						if gCfg.Enabled && gCfg.CheckVoicemail && turnsSnapshot <= 3 {
							if hit, reason := guardian.FastVoicemailHeuristic(callerSnapshot); hit {
								callEnding.Store(true)
								_ = tr.SendMessage(sessionCtx, serverMessage{Type: "interrupted"})
								alertPayload := map[string]any{
									"kind":         "voicemail",
									"action":       "hangup",
									"is_voicemail": 0.99,
									"reason":       reason,
									"model":        "jev-fast-voicemail",
								}
								_ = rt.Store.RecordEvent(sessionCtx, orgID, conv.ID, "guardian_alert", alertPayload)
								terminate("completed", "voicemail_detected")
								return
							}
						}
					}
					_ = sendJSON(serverMessage{Type: "caller_transcript", Text: ev.Text})
					_ = rt.Store.RecordEvent(sessionCtx, orgID, conv.ID, "caller_transcript", map[string]any{"text": ev.Text})

				case voice.EventAgentTranscript:
					if callEnding.Load() {
						continue
					}
					// Filtra frases automáticas em inglês ou vazamento de raciocínio markdown do modelo
					lowerTxt := strings.ToLower(strings.TrimSpace(ev.Text))
					if strings.Contains(lowerTxt, "call has been terminated") ||
						strings.Contains(lowerTxt, "call is terminated") ||
						strings.Contains(lowerTxt, "ending the call") ||
						strings.Contains(ev.Text, "**") ||
						strings.Contains(ev.Text, "*   ") {
						continue
					}
					if strings.TrimSpace(ev.Text) != "" {
						currentTurnAgentText.WriteString(" ")
						currentTurnAgentText.WriteString(strings.TrimSpace(ev.Text))
						transcriptHistory = append(transcriptHistory, fmt.Sprintf("Agente (%s): %s", cfg.Name, strings.TrimSpace(ev.Text)))
					}
					_ = sendJSON(serverMessage{Type: "agent_transcript", Text: ev.Text})
					_ = rt.Store.RecordEvent(sessionCtx, orgID, conv.ID, "agent_transcript", map[string]any{"text": ev.Text})

				case voice.EventInterrupted:
					if callEnding.Load() || pendingHangupAfterTurn {
						continue
					}
					playbackMu.Lock()
					agentPlaybackEndsAt = time.Now()
					playbackMu.Unlock()
					rec.TruncateAgentOnInterruption()
					_ = sendJSON(serverMessage{Type: "interrupted"})
					_ = rt.Store.RecordEvent(sessionCtx, orgID, conv.ID, "interruption", map[string]any{})

				case voice.EventUsage:
					if ev.Usage != nil {
						sessionUsageMu.Lock()
						// No Gemini Live, o input (prompt/áudio caller) é cumulativo na janela da sessão,
						// mas o output (áudio e texto do agente) é emitido por turno!
						if cfg.Provider == "gemini_live" || strings.Contains(strings.ToLower(cfg.Model), "gemini") {
							if ev.Usage.InputAudioTokens > sessionUsage.InputAudioTokens {
								sessionUsage.InputAudioTokens = ev.Usage.InputAudioTokens
							}
							if ev.Usage.InputTextTokens > sessionUsage.InputTextTokens {
								sessionUsage.InputTextTokens = ev.Usage.InputTextTokens
							}
							if ev.Usage.PromptTokens > sessionUsage.PromptTokens {
								sessionUsage.PromptTokens = ev.Usage.PromptTokens
							}
							if ev.Usage.CachedTokens > sessionUsage.CachedTokens {
								sessionUsage.CachedTokens = ev.Usage.CachedTokens
							}

							// Output acumula a fala de todos os turnos
							sessionUsage.OutputAudioTokens += ev.Usage.OutputAudioTokens
							sessionUsage.OutputTextTokens += ev.Usage.OutputTextTokens
							sessionUsage.ResponseTokens += ev.Usage.ResponseTokens
							sessionUsage.TotalTokens = sessionUsage.PromptTokens + sessionUsage.ResponseTokens
						} else {
							// OpenAI Realtime emite métricas por response.done
							sessionUsage.TotalTokens += ev.Usage.TotalTokens
							sessionUsage.PromptTokens += ev.Usage.PromptTokens
							sessionUsage.ResponseTokens += ev.Usage.ResponseTokens
							sessionUsage.InputAudioTokens += ev.Usage.InputAudioTokens
							sessionUsage.OutputAudioTokens += ev.Usage.OutputAudioTokens
							sessionUsage.InputTextTokens += ev.Usage.InputTextTokens
							sessionUsage.OutputTextTokens += ev.Usage.OutputTextTokens
							sessionUsage.CachedTokens += ev.Usage.CachedTokens
						}
						sessionUsageMu.Unlock()
						_ = rt.Store.RecordEvent(sessionCtx, orgID, conv.ID, "usage", ev.Usage)
					}

				case voice.EventTurnComplete:
					rec.EndAgentTurn()
					hadToolInTurn := turnHadToolCall
					rawAgentTurn := strings.TrimSpace(currentTurnAgentText.String())
					spokenInTurn := strings.ToLower(rawAgentTurn)
					turnHadToolCall = false
					currentTurnAgentText.Reset()
					lastCallerSpeechEnd = time.Time{}
					ignoringCallerAudio.Store(false)
					_ = sendJSON(serverMessage{Type: "turn_complete"})
					if pendingTransferTarget != "" {
						time.Sleep(300 * time.Millisecond)
						_ = tr.OnTransfer(sessionCtx, pendingTransferTarget)
						terminate("completed", "transferred")
						return
					}
					if pendingHangupAfterTurn {
						bytesPerSec := cat.OutputSampleRate * 2
						if bytesPerSec <= 0 {
							bytesPerSec = 48000
						}
						totalAudioDur := time.Duration(float64(turnAgentAudioBytes) / float64(bytesPerSec) * float64(time.Second))
						elapsed := time.Since(turnFirstAudioAt)
						waitDur := totalAudioDur - elapsed + 650*time.Millisecond
						if waitDur < 500*time.Millisecond {
							waitDur = 500 * time.Millisecond
						}
						if waitDur > 8*time.Second {
							waitDur = 8 * time.Second
						}
						callEnding.Store(true)
						time.Sleep(waitDur)
						terminate("completed", "agent_ended")
						return
					}
					// Se o agente anunciou a frase final de transferência mas esqueceu de emitir a tool call no mesmo turno,
					// aciona o gatilho imediato da ferramenta transfer_to_agent sem esperar o cliente falar "Alô?"
					if !hadToolInTurn && cfg.SystemToolsConfig.TransferToAgent != nil && len(cfg.SystemToolsConfig.TransferToAgent.Transfers) > 0 {
						if strings.Contains(spokenInTurn, "aguarde um instante na linha") ||
							strings.Contains(spokenInTurn, "finalizando a transferência") ||
							strings.Contains(spokenInTurn, "vou transferir agora") {
							_ = getProvSess().SendText("SISTEMA: Você já disse a frase de transferência ao cliente. Chame IMEDIATAMENTE a ferramenta transfer_to_agent passando agent_number=0, sem falar mais nada.")
						}
					}

					// --- GUARDIÃO JEV (OpenRouter `typesafe/jev-router`) ---
					gCfg := resolveGuardianConfig()
					guardianMu.Lock()
					callerTurnSnapshot := strings.TrimSpace(currentCallerTurnText.String())
					currentCallerTurnText.Reset()
					if len([]rune(callerTurnSnapshot)) >= 2 {
						totalCallerTurns++
						consecutiveAgentTurnsWithoutCaller = 0
						muteWarnings = 0
					} else if rawAgentTurn != "" {
						consecutiveAgentTurnsWithoutCaller++
					}
					consecSnap := consecutiveAgentTurnsWithoutCaller
					totalCallerSnap := totalCallerTurns
					lastCallerAtSnap := lastCallerTranscriptAt
					lastCallerTxtSnap := lastCallerTurnText
					if silenceTimer != nil {
						silenceTimer.Stop()
					}
					// Agenda timer de vigilância caso o cliente fique totalmente mudo após o término do áudio do agente
					if gCfg.Enabled && gCfg.CheckTalkingToSelf && !callEnding.Load() {
						playbackMu.Lock()
						remPlay := time.Until(agentPlaybackEndsAt)
						playbackMu.Unlock()
						if remPlay < 0 {
							remPlay = 0
						}
						silenceWait := remPlay + 13*time.Second
						silenceTimer = time.AfterFunc(silenceWait, func() {
							if callEnding.Load() || sessionCtx.Err() != nil {
								return
							}
							guardianMu.Lock()
							curMuteWarn := muteWarnings
							muteWarnings++
							guardianMu.Unlock()

							if curMuteWarn == 0 {
								alertPayload := map[string]any{
									"kind":            "talking_to_self",
									"action":          "ask_confirmation",
									"talking_to_self": 0.92,
									"reason":          "Cliente em silêncio prolongado após fala do agente — perguntando se ainda está na linha",
									"model":           gCfg.Model,
								}
								_ = rt.Store.RecordEvent(sessionCtx, orgID, conv.ID, "guardian_alert", alertPayload)
								_ = getProvSess().SendText("SISTEMA [GUARDIÃO JEV]: O cliente está mudo na linha há mais de 12 segundos. Pergunte de forma curta e natural: 'Alô, você ainda está me ouvindo?' e aguarde a resposta.")
							} else {
								alertPayload := map[string]any{
									"kind":            "talking_to_self",
									"action":          "hangup",
									"talking_to_self": 0.98,
									"reason":          "Cliente permaneceu mudo após tentativa de confirmação — encerrando chamada",
									"model":           gCfg.Model,
								}
								_ = rt.Store.RecordEvent(sessionCtx, orgID, conv.ID, "guardian_alert", alertPayload)
								pendingHangupAfterTurn = true
								_ = getProvSess().SendText("SISTEMA [GUARDIÃO JEV]: O cliente continua mudo na linha. Diga apenas uma despedida curta ('Como não estou conseguindo te ouvir, vou encerrar nossa ligação. Até logo!') e encerre em silêncio.")
								time.AfterFunc(6*time.Second, func() {
									terminate("completed", "caller_silent")
								})
							}
						})
					}
					guardianMu.Unlock()

					if gCfg.Enabled && rt.Guardian != nil && rt.Guardian.IsConfigured(gCfg.APIKey) && rawAgentTurn != "" {
						recentSlice := transcriptHistory
						if len(recentSlice) > 10 {
							recentSlice = recentSlice[len(recentSlice)-10:]
						}
						recentCopy := append([]string(nil), recentSlice...)
						roleSummary := cfg.SystemPrompt
						if len(roleSummary) > 1200 {
							roleSummary = roleSummary[:1200]
						}
						varsCopy := make(map[string]string, len(allVars))
						for k, v := range allVars {
							varsCopy[k] = v
						}
						secsSinceCaller := 0
						if !lastCallerAtSnap.IsZero() {
							secsSinceCaller = int(time.Since(lastCallerAtSnap).Seconds())
						}
						evalIn := guardian.EvalInput{
							AgentName:                          cfg.Name,
							AgentRoleSummary:                   roleSummary,
							ClientVariables:                    varsCopy,
							RecentTranscript:                   recentCopy,
							LastAgentTurn:                      rawAgentTurn,
							LastCallerTurn:                     lastCallerTxtSnap,
							ConsecutiveAgentTurnsWithoutCaller: consecSnap,
							SecondsSinceLastCallerSpeech:       secsSinceCaller,
							TotalCallerTurns:                   totalCallerSnap,
						}
						go func(in guardian.EvalInput, gc store.GuardianJevConfig) {
							res, err := rt.Guardian.Evaluate(sessionCtx, in, gc.Provider, gc.Model, gc.APIKey)
							if err != nil || callEnding.Load() || sessionCtx.Err() != nil {
								if err != nil {
									rt.Log.Debug("guardian_jev_eval_skipped", "error", err)
								}
								return
							}
							// 1. Caixa Postal detectada pelo Jev -> Encerrar na hora
							if gc.CheckVoicemail && res.IsVoicemail >= gc.VoicemailThreshold {
								callEnding.Store(true)
								_ = tr.SendMessage(sessionCtx, serverMessage{Type: "interrupted"})
								_ = rt.Store.RecordEvent(sessionCtx, orgID, conv.ID, "guardian_alert", map[string]any{
									"kind":         "voicemail",
									"action":       "hangup",
									"is_voicemail": res.IsVoicemail,
									"reason":       res.Reason,
									"model":        res.Model,
								})
								terminate("completed", "voicemail_detected")
								return
							}
							// 2. Agente Alucinando -> Corrigir em tempo real no 1º alerta; encerrar se repetir ou for severo
							if gc.CheckHallucination && res.AgentHallucinating >= gc.HallucinationThreshold {
								guardianMu.Lock()
								hWarn := hallucinationWarnings
								hallucinationWarnings++
								guardianMu.Unlock()

								if hWarn == 0 && res.HallucinationSeverity < 1.85 {
									_ = rt.Store.RecordEvent(sessionCtx, orgID, conv.ID, "guardian_alert", map[string]any{
										"kind":                   "hallucination",
										"action":                 "correct_realtime",
										"agent_hallucinating":    res.AgentHallucinating,
										"hallucination_severity": res.HallucinationSeverity,
										"reason":                 res.Reason,
										"model":                  res.Model,
									})
									hint := res.CorrectionHint
									if hint == "" {
										hint = res.Reason
									}
									_ = getProvSess().SendText(fmt.Sprintf("SISTEMA [GUARDIÃO JEV]: Alerta de alucinação/desvio detectado na sua última fala (%s). Retome imediatamente o roteiro oficial em português do Brasil e respeite estritamente as variáveis do cliente, sem inventar dados.", hint))
								} else {
									callEnding.Store(true)
									_ = rt.Store.RecordEvent(sessionCtx, orgID, conv.ID, "guardian_alert", map[string]any{
										"kind":                   "hallucination",
										"action":                 "hangup",
										"agent_hallucinating":    res.AgentHallucinating,
										"hallucination_severity": res.HallucinationSeverity,
										"reason":                 res.Reason,
										"model":                  res.Model,
									})
									terminate("completed", "guardian_hallucination_stop")
									return
								}
							}
							// 3. Cliente Mudo / Agente Falando Sozinho -> Perguntar 1x e depois encerrar
							if gc.CheckTalkingToSelf && in.ConsecutiveAgentTurnsWithoutCaller >= 2 && res.TalkingToSelf >= gc.TalkingToSelfThreshold {
								guardianMu.Lock()
								mWarn := muteWarnings
								muteWarnings++
								guardianMu.Unlock()

								if mWarn == 0 {
									_ = rt.Store.RecordEvent(sessionCtx, orgID, conv.ID, "guardian_alert", map[string]any{
										"kind":            "talking_to_self",
										"action":          "ask_confirmation",
										"talking_to_self": res.TalkingToSelf,
										"reason":          res.Reason,
										"model":           res.Model,
									})
									_ = getProvSess().SendText("SISTEMA [GUARDIÃO JEV]: O cliente parece estar mudo enquanto você continua falando sozinho. Pare de avançar no roteiro e pergunte apenas: 'Alô, você ainda está me ouvindo?' e aguarde a resposta.")
								} else {
									_ = rt.Store.RecordEvent(sessionCtx, orgID, conv.ID, "guardian_alert", map[string]any{
										"kind":            "talking_to_self",
										"action":          "hangup",
										"talking_to_self": res.TalkingToSelf,
										"reason":          res.Reason,
										"model":           res.Model,
									})
									pendingHangupAfterTurn = true
									_ = getProvSess().SendText("SISTEMA [GUARDIÃO JEV]: O cliente permanece mudo na linha. Despeça-se brevemente ('Como não estou conseguindo te ouvir, vou encerrar nossa ligação. Até logo!') e encerre em silêncio.")
									time.AfterFunc(6*time.Second, func() {
										terminate("completed", "caller_silent")
									})
								}
							}
						}(evalIn, gCfg)
					}

					turnAgentAudioBytes = 0

				case voice.EventToolCall:
					if ev.ToolCall == nil {
						continue
					}
					fc := *ev.ToolCall
					_ = sendJSON(serverMessage{Type: "tool_call", Text: fc.Name})
					_ = rt.Store.RecordEvent(sessionCtx, orgID, conv.ID, "tool_call", map[string]any{
						"id": fc.ID, "name": fc.Name, "args": fc.Args,
					})

					turnHadToolCall = true
					if fc.Name == "ativar_procedure" {
						procName, _ := fc.Args["nome"].(string)
						var targetProc *store.ProcedureSnapshot
						for _, p := range activeProcedures {
							if strings.EqualFold(strings.TrimSpace(p.Name), strings.TrimSpace(procName)) {
								snap := p
								targetProc = &snap
								break
							}
						}

						if targetProc != nil {
							expanded, err := procExpander.ExpandProcedure(sessionCtx, orgID, agentID, *targetProc, procsMap)
							if err == nil {
								activeProcedureName = targetProc.Name
								activeProcedureID = &targetProc.ID

								// Regra da ElevenLabs: mantém os até 5 procedures mais recentemente iniciados na janela ativa
								recentActivatedProcs = append(recentActivatedProcs, expanded)
								if len(recentActivatedProcs) > 5 {
									recentActivatedProcs = recentActivatedProcs[len(recentActivatedProcs)-5:]
								}

								activeAllowedTools = make(map[string]bool)
								activeToolPolicies = make(map[string]procedures.ToolExecutionPolicy)
								for _, globalToolID := range cfg.Tools {
									if t, err := rt.Store.GetTool(sessionCtx, orgID, globalToolID); err == nil {
										activeAllowedTools[t.Name] = true
									}
								}
								for _, rp := range recentActivatedProcs {
									for k, v := range rp.AllowedTools {
										if v {
											activeAllowedTools[k] = true
										}
									}
									for k, pol := range rp.ToolPolicies {
										activeToolPolicies[k] = pol
									}
								}

								res := map[string]any{
									"success":      true,
									"procedimento": targetProc.Name,
									"tipo":         expanded.Type,
									"instrucoes":   expanded.ExpandedContent,
								}
								_ = getProvSess().SendToolResult(fc, res)
								_ = sendJSON(serverMessage{Type: "procedure_activated", Text: targetProc.Name})
								_ = rt.Store.RecordEvent(sessionCtx, orgID, conv.ID, "procedure_activated", map[string]any{
									"procedure_id": targetProc.ID, "name": targetProc.Name,
								})
								_ = rt.Store.RecordToolExecution(sessionCtx, store.ToolExecution{
									OrganizationID:  orgID,
									AgentID:         agentID,
									ConversationID:  conv.ID,
									ToolName:        "ativar_procedure",
									ProcedureID:     &targetProc.ID,
									Request:         mustJSON(fc.Args),
									Response:        mustJSON(res),
									Status:          "success",
									ExecutionTimeMS: 0,
								})
								continue
							}
						}

						// Procedimento não encontrado
						resErr := map[string]any{"success": false, "error": fmt.Sprintf("procedimento %q não encontrado", procName)}
						_ = getProvSess().SendToolResult(fc, resErr)
						continue
					}

					if fc.Name == "encerrar_conversa" || fc.Name == "end_call" {
						res := map[string]any{"success": true}
						_ = rt.Store.RecordToolExecution(sessionCtx, store.ToolExecution{
							OrganizationID:  orgID,
							AgentID:         agentID,
							ConversationID:  conv.ID,
							ToolID:          nil,
							ToolName:        fc.Name,
							ProcedureID:     activeProcedureID,
							Request:         mustJSON(fc.Args),
							Response:        mustJSON(res),
							Status:          "success",
							ExecutionTimeMS: 0,
						})
						_ = sendJSON(serverMessage{Type: "tool_result", Text: fc.Name})
						_ = rt.Store.RecordEvent(sessionCtx, orgID, conv.ID, "tool_result", map[string]any{
							"name": fc.Name, "status": "success",
						})

						ignoringCallerAudio.Store(true)

						if turnAgentAudioBytes > 0 {
							// O agente JÁ falou a despedida neste turno antes de chamar end_call.
							// Bloqueia imediatamente qualquer fala extra do modelo (como "The call has been terminated"),
							// aguarda o áudio da despedida terminar de tocar no buffer da Twilio/navegador e desliga!
							callEnding.Store(true)
							bytesPerSec := cat.OutputSampleRate * 2
							if bytesPerSec <= 0 {
								bytesPerSec = 48000
							}
							totalAudioDur := time.Duration(float64(turnAgentAudioBytes) / float64(bytesPerSec) * float64(time.Second))
							elapsed := time.Since(turnFirstAudioAt)
							waitDur := totalAudioDur - elapsed + 700*time.Millisecond
							if waitDur < 800*time.Millisecond {
								waitDur = 800 * time.Millisecond
							}
							if waitDur > 8*time.Second {
								waitDur = 8 * time.Second
							}
							go func(d time.Duration) {
								time.Sleep(d)
								terminate("completed", "agent_ended")
							}(waitDur)
						} else {
							// Caso o agente tenha chamado end_call antes de falar a despedida:
							pendingHangupAfterTurn = true
							_ = getProvSess().SendToolResult(fc, map[string]any{
								"success":    true,
								"instrucoes": "Diga apenas uma despedida curta em português (ex: Obrigada pela atenção e tenha um ótimo dia!) e encerre em silêncio. NUNCA fale em inglês nem anuncie que a ligação foi encerrada.",
							})
							// Timer de segurança garantido caso o modelo não emita mais áudio
							go func() {
								time.Sleep(5 * time.Second)
								terminate("completed", "agent_ended")
							}()
						}
					} else if fc.Name == "transferir_numero" {
						targetNum, _ := fc.Args["numero"].(string)
						motivo, _ := fc.Args["motivo"].(string)
						if targetNum == "" {
							targetNum = "atendente"
						}
						pendingTransferTarget = targetNum
						res := map[string]any{
							"success": true,
							"message": fmt.Sprintf("transferência para %s iniciada com sucesso; despeça-se do Caller informando a transferência", targetNum),
						}
						_ = getProvSess().SendToolResult(fc, res)
						_ = rt.Store.RecordToolExecution(sessionCtx, store.ToolExecution{
							OrganizationID:  orgID,
							AgentID:         agentID,
							ConversationID:  conv.ID,
							ToolID:          nil,
							ToolName:        fc.Name,
							ProcedureID:     activeProcedureID,
							Request:         mustJSON(fc.Args),
							Response:        mustJSON(res),
							Status:          "success",
							ExecutionTimeMS: 0,
						})
						_ = sendJSON(serverMessage{Type: "tool_result", Text: fc.Name})
						_ = rt.Store.RecordEvent(sessionCtx, orgID, conv.ID, "tool_result", map[string]any{
							"name": fc.Name, "target": targetNum, "reason": motivo, "status": "success",
						})
					} else if fc.Name == "transfer_to_agent" || fc.Name == "transferir_agente" {
						reason, _ := fc.Args["reason"].(string)
						agentNum := 0
						if rawNum, exists := fc.Args["agent_number"]; exists {
							switch v := rawNum.(type) {
							case float64:
								agentNum = int(v)
							case int:
								agentNum = v
							case string:
								if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
									agentNum = parsed
								}
							}
						}

						var rules []store.AgentTransferRule
						if cfg.SystemToolsConfig.TransferToAgent != nil {
							rules = cfg.SystemToolsConfig.TransferToAgent.Transfers
						}
						if len(rules) == 0 {
							errRes := map[string]any{"success": false, "error": "nenhuma regra de transferência configurada neste agente"}
							_ = getProvSess().SendToolResult(fc, errRes)
							continue
						}
						if agentNum < 0 || agentNum >= len(rules) {
							agentNum = 0
						}
						selectedRule := rules[agentNum]

						targetAgent, err := rt.Store.GetAgent(sessionCtx, orgID, selectedRule.AgentID)
						if err != nil {
							errRes := map[string]any{"success": false, "error": fmt.Sprintf("agente de destino %q não encontrado", selectedRule.AgentID)}
							_ = getProvSess().SendToolResult(fc, errRes)
							continue
						}
						targetCfg, err := rt.Store.DraftConfig(sessionCtx, orgID, targetAgent)
						if err != nil {
							errRes := map[string]any{"success": false, "error": err.Error()}
							_ = getProvSess().SendToolResult(fc, errRes)
							continue
						}

						// Registra marcador de limite transfer_to_agent na transcrição/histórico de auditoria (Pós-chamada ElevenLabs)
						res := map[string]any{
							"success":         true,
							"transferred_to":  targetAgent.Name,
							"target_agent_id": targetAgent.ID,
							"agent_number":    agentNum,
							"reason":          reason,
						}
						_ = rt.Store.RecordToolExecution(sessionCtx, store.ToolExecution{
							OrganizationID:  orgID,
							AgentID:         agentID,
							ConversationID:  conv.ID,
							ToolID:          nil,
							ToolName:        "transfer_to_agent",
							ProcedureID:     activeProcedureID,
							Request:         mustJSON(fc.Args),
							Response:        mustJSON(res),
							Status:          "success",
							ExecutionTimeMS: selectedRule.DelayMS,
						})
						_ = sendJSON(serverMessage{Type: "tool_result", Text: fmt.Sprintf("transfer_to_agent -> %s", targetAgent.Name)})
						_ = rt.Store.RecordEvent(sessionCtx, orgID, conv.ID, "transfer_to_agent", map[string]any{
							"from_agent_id":   agentID,
							"from_agent_name": cfg.Name,
							"to_agent_id":     targetAgent.ID,
							"to_agent_name":   targetAgent.Name,
							"agent_number":    agentNum,
							"reason":          reason,
						})

						// Silencia o microfone do Caller durante o handoff para que ruídos da linha não interrompam
						ignoringCallerAudio.Store(true)

						// Calcula quanto tempo falta para a fala de encaminhamento do Agente Pai terminar de tocar no telefone
						bytesPerSec := cat.OutputSampleRate * 2
						if bytesPerSec <= 0 {
							bytesPerSec = 48000
						}
						var remainingParentSpeech time.Duration
						if turnAgentAudioBytes > 0 && !turnFirstAudioAt.IsZero() {
							totalDur := time.Duration(float64(turnAgentAudioBytes) / float64(bytesPerSec) * float64(time.Second))
							elapsed := time.Since(turnFirstAudioAt)
							if totalDur > elapsed {
								remainingParentSpeech = totalDur - elapsed
							}
						}
						connectStartedAt := time.Now()

						// Herança de idioma do agente pai (se suportado pelo provedor do agente filho)
						childProv, err := rt.Voices.Get(targetCfg.Provider)
						if err != nil {
							childProv = prov
						}
						childCat := childProv.Catalog()
						inheritedLang := cfg.Language
						if inheritedLang == "" || !childCat.HasLanguage(inheritedLang) {
							inheritedLang = targetCfg.Language
						}

						// Reconstrói prompt, procedures e tools do Agente Filho (sem expor a chamada transfer_to_agent no histórico do LLM)
						childPrompt := variables.Render(targetCfg.SystemPrompt, allVars)
						if inheritedLang != "" {
							childPrompt += fmt.Sprintf("\n\nDIRETRIZ DE IDIOMA: Responda obrigatoriamente no idioma configurado (%s).", inheritedLang)
						}
						if len(transcriptHistory) > 0 {
							childPrompt += "\n\n## HISTÓRICO DA CONVERSA ATÉ AGORA (CONTINUE NATURALMENTE SEM MENCIONAR TRANSFERÊNCIA INTERNA):\n" + strings.Join(transcriptHistory, "\n")
						}

						newProcsMap := make(map[string]store.ProcedureSnapshot)
						var newActiveProcedures []store.ProcedureSnapshot
						var newEntryProcedures []store.ProcedureSnapshot
						for _, p := range targetCfg.Procedures {
							if p.Active {
								newProcsMap[p.ID] = p
								newActiveProcedures = append(newActiveProcedures, p)
								if strings.TrimSpace(p.Trigger) != "" {
									newEntryProcedures = append(newEntryProcedures, p)
								}
							}
						}

						var childToolSpecs []voice.ToolSpec
						newRegisteredTools := make(map[string]store.Tool)
						newAddedToolIDs := make(map[string]bool)

						for _, st := range targetCfg.SystemTools {
							if st == "encerrar_conversa" {
								childToolSpecs = append(childToolSpecs,
									voice.ToolSpec{Name: "encerrar_conversa", Description: "Encerra a conversa com o Caller após se despedir cordialmente.", Parameters: []voice.Param{}},
									voice.ToolSpec{Name: "end_call", Description: "Encerra a chamada telefônica imediatamente.", Parameters: []voice.Param{}},
								)
							} else if st == "transferir_numero" {
								childToolSpecs = append(childToolSpecs, voice.ToolSpec{
									Name:        "transferir_numero",
									Description: "Transfere a chamada telefônica para outro número ou ramal.",
									Parameters: []voice.Param{
										{Name: "numero", Type: "string", Description: "Número de telefone de destino", Required: true},
										{Name: "motivo", Type: "string", Description: "Motivo da transferência", Required: false},
									},
								})
							} else if st == "transferir_agente" || st == "transfer_to_agent" {
								tDesc := "Transfere a conversa em andamento para outro agente de IA especializado."
								if targetCfg.SystemToolsConfig.TransferToAgent != nil && len(targetCfg.SystemToolsConfig.TransferToAgent.Transfers) > 0 {
									var sb strings.Builder
									sb.WriteString("\n\n## REGRAS DE TRANSFERÊNCIA ENTRE AGENTES:\n")
									for rIdx, rItem := range targetCfg.SystemToolsConfig.TransferToAgent.Transfers {
										sb.WriteString(fmt.Sprintf("- agent_number = %d: %s\n", rIdx, rItem.Condition))
									}
									tDesc += " " + sb.String()
									childPrompt += sb.String()
								}
								tParams := []voice.Param{
									{Name: "agent_number", Type: "integer", Description: "Índice (0, 1...) do agente de destino", Required: true},
									{Name: "reason", Type: "string", Description: "Motivo da transferência", Required: false},
								}
								childToolSpecs = append(childToolSpecs,
									voice.ToolSpec{Name: "transfer_to_agent", Description: tDesc, Parameters: tParams},
									voice.ToolSpec{Name: "transferir_agente", Description: tDesc, Parameters: tParams},
								)
							}
						}

						if len(newEntryProcedures) > 0 {
							childToolSpecs = append(childToolSpecs, voice.ToolSpec{
								Name:        "ativar_procedure",
								Description: "Ativa um procedimento de negócio quando o assunto corresponder ao seu gatilho.",
								Parameters:  []voice.Param{{Name: "nome", Type: "string", Description: "Nome do procedimento", Required: true}},
							})
							var trigSb strings.Builder
							trigSb.WriteString("\n\n## PROCEDIMENTOS DISPONÍVEIS:\n")
							for _, p := range newEntryProcedures {
								trigSb.WriteString(fmt.Sprintf("- \"%s\": %s\n", p.Name, p.Trigger))
							}
							childPrompt += trigSb.String()
						}

						for _, tid := range targetCfg.Tools {
							if tid != "" && !newAddedToolIDs[tid] {
								if t, err := rt.Store.GetTool(sessionCtx, orgID, tid); err == nil && t.Active {
									newAddedToolIDs[tid] = true
									childToolSpecs = append(childToolSpecs, voice.ToolSpec{Name: t.Name, Description: t.Description, Parameters: t.InputSchema})
									newRegisteredTools[t.Name] = t
								}
							}
						}

						childSess, err := childProv.Connect(sessionCtx, voice.SessionConfig{
							Model:        targetCfg.Model,
							Voice:        targetCfg.Voice,
							Language:     inheritedLang,
							SystemPrompt: childPrompt,
							Tools:        childToolSpecs,
						})
						if err != nil {
							errRes := map[string]any{"success": false, "error": fmt.Sprintf("falha ao conectar ao agente %s: %v", targetAgent.Name, err)}
							_ = getProvSess().SendToolResult(fc, errRes)
							continue
						}

						// Aguarda o término da fala do Agente Pai no buffer da Twilio + o delay_ms configurado
						totalWait := remainingParentSpeech + time.Duration(selectedRule.DelayMS)*time.Millisecond
						alreadyWaited := time.Since(connectStartedAt)
						if totalWait > alreadyWaited {
							time.Sleep(totalWait - alreadyWaited)
						}

						// Troca a sessão ativa do provedor pelo Agente Filho
						provSessMu.Lock()
						oldSess := provSess
						provSess = childSess
						provSessMu.Unlock()
						_ = oldSess.Close()

						agentID = targetAgent.ID
						cfg = targetCfg
						procsMap = newProcsMap
						activeProcedures = newActiveProcedures
						entryProcedures = newEntryProcedures
						registeredTools = newRegisteredTools
						recentActivatedProcs = nil
						activeAllowedTools = make(map[string]bool)
						for tName := range newRegisteredTools {
							activeAllowedTools[tName] = true
						}
						turnAgentAudioBytes = 0
						turnHadToolCall = false
						currentTurnAgentText.Reset()

						// Protege os primeiros 4 segundos da abertura do Agente Filho contra ruídos da linha
						time.AfterFunc(4*time.Second, func() {
							ignoringCallerAudio.Store(false)
						})

						// Mensagem de transferência opcional (transfer_message) + Primeira mensagem do agente transferido (enable_transferred_agent_first_message)
						var greetingParts []string
						if selectedRule.TransferMessage != nil && strings.TrimSpace(*selectedRule.TransferMessage) != "" {
							greetingParts = append(greetingParts, variables.Render(strings.TrimSpace(*selectedRule.TransferMessage), allVars))
						}
						if selectedRule.EnableTransferredAgentFirstMessage && strings.TrimSpace(targetCfg.FirstMessage) != "" {
							greetingParts = append(greetingParts, variables.Render(strings.TrimSpace(targetCfg.FirstMessage), allVars))
						}

						if len(greetingParts) > 0 {
							joined := strings.Join(greetingParts, " ")
							_ = childSess.SendText(fmt.Sprintf("Diga agora exatamente a seguinte frase e aguarde o usuário responder:\n%q", joined))
						} else {
							_ = childSess.SendText(fmt.Sprintf("A conversa foi encaminhada para você (motivo: %q). Continue o atendimento imediatamente a partir do histórico da conversa, sem mencionar a transferência interna.", reason))
						}
						continue
					} else if targetTool, ok := registeredTools[fc.Name]; ok {
						// Checa allowlist do Procedure ativo
						hasSpecificTools := false
						for tName := range activeAllowedTools {
							if tName != "encerrar_conversa" && tName != "end_call" {
								hasSpecificTools = true
								break
							}
						}
						if hasSpecificTools && !activeAllowedTools[targetTool.Name] {
							rejected := map[string]any{
								"success": false,
								"error":   fmt.Sprintf("A ferramenta '%s' não é permitida no procedimento ativo ('%s').", targetTool.Name, activeProcedureName),
							}
							_ = getProvSess().SendToolResult(fc, rejected)
							_ = rt.Store.RecordToolExecution(sessionCtx, store.ToolExecution{
								OrganizationID:  orgID,
								AgentID:         agentID,
								ConversationID:  conv.ID,
								ToolID:          &targetTool.ID,
								ToolName:        targetTool.Name,
								ProcedureID:     activeProcedureID,
								Request:         mustJSON(fc.Args),
								Response:        mustJSON(rejected),
								Status:          "rejected",
								ExecutionTimeMS: 0,
							})
							continue
						}

						// Aplica schema_overrides e retries de Structured Procedures (se definidos)
						callArgs := make(map[string]any, len(fc.Args))
						for k, v := range fc.Args {
							callArgs[k] = v
						}
						maxRetries := 0
						if pol, hasPol := activeToolPolicies[targetTool.Name]; hasPol {
							maxRetries = pol.MaxRetries
							for paramKey, ov := range pol.SchemaOverrides {
								cleanKey := strings.TrimPrefix(paramKey, "request_body.")
								switch ov.Source {
								case "constant":
									callArgs[cleanKey] = ov.ConstantValue
								case "dynamic_variable":
									if val, ok := allVars[ov.DynamicVariable]; ok {
										callArgs[cleanKey] = val
									}
								case "omit":
									delete(callArgs, cleanKey)
								}
							}
						}

						execRes := executor.ExecuteWithVars(sessionCtx, targetTool, callArgs, allVars)
						for attempt := 0; attempt < maxRetries && execRes.Status != "success"; attempt++ {
							time.Sleep(250 * time.Millisecond)
							execRes = executor.ExecuteWithVars(sessionCtx, targetTool, callArgs, allVars)
						}
						for varKey, varVal := range execRes.AssignedVars {
							allVars[varKey] = varVal
						}
						_ = getProvSess().SendToolResult(fc, execRes.Result)
						_ = rt.Store.RecordToolExecution(sessionCtx, store.ToolExecution{
							OrganizationID:  orgID,
							AgentID:         agentID,
							ConversationID:  conv.ID,
							ToolID:          &targetTool.ID,
							ToolName:        targetTool.Name,
							ProcedureID:     activeProcedureID,
							Request:         mustJSON(fc.Args),
							Response:        mustJSON(execRes.Result),
							Status:          execRes.Status,
							ExecutionTimeMS: execRes.DurationMS,
						})
						_ = sendJSON(serverMessage{Type: "tool_result", Text: targetTool.Name})
						_ = rt.Store.RecordEvent(sessionCtx, orgID, conv.ID, "tool_result", map[string]any{
							"name": targetTool.Name, "status": execRes.Status, "duration_ms": execRes.DurationMS,
						})
					} else {
						// Ferramenta não permitida
						rejected := map[string]any{"success": false, "error": "ferramenta não autorizada neste momento"}
						_ = getProvSess().SendToolResult(fc, rejected)
						_ = rt.Store.RecordToolExecution(sessionCtx, store.ToolExecution{
							OrganizationID:  orgID,
							AgentID:         agentID,
							ConversationID:  conv.ID,
							ToolID:          nil,
							ToolName:        fc.Name,
							ProcedureID:     activeProcedureID,
							Request:         mustJSON(fc.Args),
							Response:        mustJSON(rejected),
							Status:          "rejected",
							ExecutionTimeMS: 0,
						})
					}

				case voice.EventClosed:
					if ev.Err != nil && !errors.Is(ev.Err, context.Canceled) {
						rt.Log.Warn("provider_session_closed_with_error", "conversation_id", conv.ID, "error", ev.Err)
						terminate("failed", "provider_error")
					} else {
						terminate("completed", "agent_ended")
					}
					return
				}
			}
		}
	}()

	// Goroutine B: Timeout de duração máxima
	go func() {
		select {
		case <-sessionCtx.Done():
			return
		case <-maxTimer.C:
			rt.Log.Info("max_duration_reached", "conversation_id", conv.ID)
			terminate("completed", "max_duration")
		}
	}()

	// Loop Principal: Caller -> Provider (bloqueia na leitura do transporte)
	for {
		ev, err := tr.ReadEvent(sessionCtx)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				rt.Log.Debug("transport_read_ended", "conversation_id", conv.ID, "error", err)
			}
			break
		}

		if ev.Hangup {
			terminate("completed", "caller_hangup")
			break
		}

		if ev.IsAudio {
			// Áudio PCM16 vindo do microfone do Caller
			rec.AddCallerAudio(ev.Audio)
			lastCallerSpeechEnd = time.Now()

			if !ignoringCallerAudio.Load() {
				audioToSend := ev.Audio
				isLiveDuplex := strings.HasPrefix(strings.ToLower(strings.TrimSpace(cfg.Model)), "gpt-live")
				if !isLiveDuplex {
					playbackMu.Lock()
					isAgentPlaying := time.Now().Before(agentPlaybackEndsAt)
					playbackMu.Unlock()

					// Enquanto o áudio do agente está tocando no alto-falante do telefone (isAgentPlaying),
					// filtra apenas o eco baixo de linha/alto-falante (< 650) sem cortar o início das palavras do cliente.
					if isAgentPlaying {
						peak := maxAbsPCM16(ev.Audio)
						if peak < 650 {
							audioToSend = make([]byte, len(ev.Audio))
						}
					}
				}

				if err := getProvSess().SendAudio(audioToSend); err != nil {
					rt.Log.Warn("send_audio_to_provider_failed", "error", err)
					terminate("failed", "provider_error")
					break
				}
			}
		}
	}

	cancelSession()
	finalCost := endCall(finalStatus, finalReason)
	_ = sendJSON(serverMessage{
		Type:    "ended",
		Reason:  finalReason,
		CostUSD: finalCost.TotalCostUSD,
		CostBRL: finalCost.TotalCostBRL,
	})

	// Gravação WAV Estéreo para o MinIO (Pergunta 16)
	if cfg.RecordingEnabled && rt.Storage != nil {
		wavBytes := rec.BuildWAV()
		if len(wavBytes) > 0 {
			recKey := fmt.Sprintf("recordings/%s/%s/%s/audio.wav", orgID, agentID, conv.ID)
			uploadCtx, cancelUpload := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancelUpload()
			if err := rt.Storage.UploadRecording(uploadCtx, recKey, wavBytes); err == nil {
				_, _ = rt.Store.Pool.Exec(uploadCtx,
					`UPDATE conversations SET recording_key = $3 WHERE organization_id = $1 AND id = $2`,
					orgID, conv.ID, recKey)
			} else {
				rt.Log.Warn("upload_recording_failed", "conversation_id", conv.ID, "error", err)
			}
		}
	}

	_ = tr.Close("sessão finalizada")
}

func maxAbsPCM16(pcm []byte) int {
	maxVal := 0
	num := len(pcm) / 2
	for i := 0; i < num; i++ {
		s := int(int16(binary.LittleEndian.Uint16(pcm[i*2:])))
		if s < 0 {
			s = -s
		}
		if s > maxVal {
			maxVal = s
		}
	}
	return maxVal
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}
