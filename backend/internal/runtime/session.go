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

type deferredConvEvent struct {
	Type    string
	Payload any
}

type preparedSession struct {
	cfg                   store.AgentConfig
	prov                  voice.Provider
	cat                   voice.Catalog
	provSess              voice.Session
	cancelLifetime        context.CancelFunc
	allVars               map[string]string
	firstMessage          string
	systemPrompt          string
	procsMap              map[string]store.ProcedureSnapshot
	activeProcedures      []store.ProcedureSnapshot
	entryProcedures       []store.ProcedureSnapshot
	promptReferencedTools map[string]bool
	toolSpecs             []voice.ToolSpec
	registeredTools       map[string]store.Tool
	mcpSessions           map[string]*tools.MCPSession
	mcpBindings           map[string]mcpToolBinding
	deferredEvents        []deferredConvEvent
}

func (ps *preparedSession) cleanup() {
	if ps == nil {
		return
	}
	if ps.provSess != nil {
		_ = ps.provSess.Close()
	}
	for _, ms := range ps.mcpSessions {
		if ms != nil {
			ms.Close()
		}
	}
	if ps.cancelLifetime != nil {
		ps.cancelLifetime()
	}
}

func (rt *Runtime) prepareSession(
	ctx context.Context,
	startedAt time.Time,
	orgID, agentID, convIDForVars, callerID, toNumber, callSid string,
	cfg store.AgentConfig,
	prov voice.Provider,
) (*preparedSession, error) {
	ps := &preparedSession{
		allVars:               map[string]string{},
		procsMap:              make(map[string]store.ProcedureSnapshot),
		promptReferencedTools: make(map[string]bool),
		registeredTools:       make(map[string]store.Tool),
		mcpSessions:           make(map[string]*tools.MCPSession),
		mcpBindings:           make(map[string]mcpToolBinding),
	}

	addDeferred := func(evType string, payload any) {
		ps.deferredEvents = append(ps.deferredEvents, deferredConvEvent{Type: evType, Payload: payload})
	}

	for _, decl := range cfg.Variables {
		ps.allVars[decl.Name] = ""
	}

	// Dynamic variables + Overrides via Initiation Webhook (padrão ElevenLabs Twilio Personalization)
	if cfg.InitiationWebhookURL != "" {
		var initRes variables.InitiationResult
		var initErr error

		if prefRes, ok, prefErr := rt.consumePrefetchedInitiation(ctx, callSid); ok {
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
					CalledNumber:   toNumber,
					CallSid:        callSid,
					ConversationID: convIDForVars,
					Caller:         callerID,
					Metadata:       map[string]any{},
				},
				cfg.Variables,
			)
		}

		if initErr != nil {
			if rt.Log != nil {
				rt.Log.Warn("initiation_webhook_failed", "conversation_id", convIDForVars, "caller_id", callerID, "error", initErr)
			}
			addDeferred("initiation_webhook_warning", map[string]any{
				"phase": "initiation",
				"error": initErr.Error(),
			})
		} else {
			for k, v := range initRes.DynamicVariables {
				ps.allVars[k] = v
			}
			if len(initRes.MissingVariables) > 0 && rt.Log != nil {
				rt.Log.Warn("initiation_webhook_missing_vars", "conversation_id", convIDForVars, "missing", initRes.MissingVariables)
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
			addDeferred("initiation_webhook_loaded", map[string]any{
				"dynamic_variables": initRes.DynamicVariables,
				"has_overrides":     initRes.ConversationConfigOverride != nil,
			})
		}
	}

	if prov == nil && rt.Voices != nil {
		if resolvedProv, pErr := rt.Voices.Get(cfg.Provider); pErr == nil {
			prov = resolvedProv
		}
	}

	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		loc = time.UTC
	}
	sysVars := variables.SystemForCall(startedAt, loc, convIDForVars, agentID, callerID, toNumber, callSid, cfg.Timezone)
	for k, v := range sysVars {
		ps.allVars[k] = v
	}

	if ps.allVars["primNome"] == "" && ps.allVars["priNome"] != "" {
		ps.allVars["primNome"] = ps.allVars["priNome"]
	}
	if ps.allVars["priNome"] == "" && ps.allVars["primNome"] != "" {
		ps.allVars["priNome"] = ps.allVars["primNome"]
	}
	if ps.allVars["primNome"] == "" && ps.allVars["nome"] != "" {
		fields := strings.Fields(ps.allVars["nome"])
		if len(fields) > 0 {
			ps.allVars["primNome"] = fields[0]
			ps.allVars["priNome"] = fields[0]
		}
	}

	firstMessage := variables.Render(cfg.FirstMessage, ps.allVars)
	rawRenderedPrompt := variables.Render(cfg.SystemPrompt, ps.allVars)
	speakerExplicitPrompt := variables.Render(cfg.SystemToolsConfig.SpeakerPrompt, ps.allVars)

	systemPrompt := rawRenderedPrompt
	if cfg.Language != "" {
		systemPrompt += fmt.Sprintf("\n\nDIRETRIZ DE IDIOMA: Responda obrigatoriamente no idioma configurado (%s). Fale em ritmo ágil, natural e fluido. Tags entre colchetes como [super fast] são marcações de ritmo de fala e jamais devem ser lidas em voz alta.", cfg.Language)
	}
	if strings.TrimSpace(firstMessage) != "" {
		systemPrompt += fmt.Sprintf("\n\nREGRA OBRIGATÓRIA DE PRIMEIRA MENSAGEM: Na sua PRIMEIRA fala ao iniciar a conversa, você DEVE dizer EXCLUSIVAMENTE a frase de abertura configurada (%q) e PARAR de falar para aguardar a resposta do cliente. Só inicie o passo 1 de # Actions DEPOIS que o cliente responder à sua primeira mensagem.", firstMessage)
	}

	if len(cfg.Procedures) == 0 && rt.Store != nil {
		if draftCfg, dErr := rt.Store.DraftConfig(ctx, orgID, store.Agent{ID: agentID, OrganizationID: orgID}); dErr == nil && len(draftCfg.Procedures) > 0 {
			cfg.Procedures = draftCfg.Procedures
		}
	}

	for _, rawProc := range cfg.Procedures {
		if !rawProc.Active {
			continue
		}
		p := rawProc
		p.Trigger = variables.Render(p.Trigger, ps.allVars)
		p.Content = variables.Render(p.Content, ps.allVars)
		if len(p.Guardrails) > 0 {
			renderedGuards := make([]string, len(p.Guardrails))
			for i, g := range p.Guardrails {
				renderedGuards[i] = variables.Render(g, ps.allVars)
			}
			p.Guardrails = renderedGuards
		}
		ps.procsMap[p.ID] = p
		ps.activeProcedures = append(ps.activeProcedures, p)
	}

	referencedAsSubProc := make(map[string]bool)
	for _, p := range ps.activeProcedures {
		meta := procedures.InspectProcedure(store.Procedure{
			ID:      p.ID,
			Name:    p.Name,
			Type:    p.Type,
			Trigger: p.Trigger,
			Content: p.Content,
		}, ps.procsMap, nil, nil)
		for _, refID := range meta.ReferencedProcedureIDs {
			referencedAsSubProc[refID] = true
			for _, cand := range ps.activeProcedures {
				if strings.EqualFold(strings.TrimSpace(cand.Name), strings.TrimSpace(refID)) {
					referencedAsSubProc[cand.ID] = true
				}
			}
		}
	}

	for _, p := range ps.activeProcedures {
		if strings.TrimSpace(p.Trigger) != "" {
			ps.entryProcedures = append(ps.entryProcedures, p)
		} else if !referencedAsSubProc[p.ID] {
			p.Trigger = "Quando o atendimento envolver " + p.Name
			ps.entryProcedures = append(ps.entryProcedures, p)
		}
	}

	procExpander := procedures.NewExpander(rt.Store)
	systemPrompt = procExpander.ExpandInlineReferences(ctx, orgID, systemPrompt, ps.procsMap, ps.promptReferencedTools)
	reasonerPrompt := procExpander.ExpandInlineReferences(ctx, orgID, rawRenderedPrompt, ps.procsMap, ps.promptReferencedTools)

	for _, st := range cfg.SystemTools {
		if st == "encerrar_conversa" {
			ps.toolSpecs = append(ps.toolSpecs,
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
			ps.toolSpecs = append(ps.toolSpecs, voice.ToolSpec{
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
						if targetLabel == "" && rt.Store != nil {
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
					reasonerPrompt += rulesPrompt.String()
				}
			}
			transferParams := []voice.Param{
				{Name: "agent_number", Type: "integer", Description: "Número indexado a partir de zero (0, 1, 2...) do agente para o qual transferir com base nas regras configuradas", Required: true},
				{Name: "reason", Type: "string", Description: "O motivo da transferência do agente", Required: false},
			}
			ps.toolSpecs = append(ps.toolSpecs,
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

	var skillSummaries []voice.SkillSummary

	if len(ps.entryProcedures) > 0 {
		var triggerIndex strings.Builder
		var procNamesList []string
		triggerIndex.WriteString("## SKILLS / PROCEDIMENTOS DE ATENDIMENTO DO REASONER (PRIORIDADE MÁXIMA):\n")
		triggerIndex.WriteString("ATENÇÃO CRÍTICA: Os procedimentos abaixo contêm o fluxo obrigatório e as ferramentas de cada assunto. Sempre que a solicitação do cliente corresponder ao gatilho (Trigger) de um procedimento abaixo (ex.: atendimento para terceiro/familiar como marido, esposa, mãe, filho ou dependente; segunda via de boleto/fatura; ou consulta por CPF), siga estritamente as etapas e regras desse procedimento na ordem exata:\n")
		for _, p := range ps.entryProcedures {
			skillSummaries = append(skillSummaries, voice.SkillSummary{
				Name:        p.Name,
				Description: p.Trigger,
			})
			procNamesList = append(procNamesList, fmt.Sprintf("%q (%s)", p.Name, p.Trigger))
			triggerIndex.WriteString(fmt.Sprintf("\n### SKILL / PROCEDIMENTO: %q\n- **Quando aplicar (Trigger):** %s\n", p.Name, p.Trigger))
			if expanded, expErr := procExpander.ExpandProcedure(ctx, orgID, agentID, p, ps.procsMap); expErr == nil && strings.TrimSpace(expanded.ExpandedContent) != "" {
				triggerIndex.WriteString(expanded.ExpandedContent)
				triggerIndex.WriteString("\n")
				for tName, ok := range expanded.AllowedTools {
					if ok {
						ps.promptReferencedTools[tName] = true
					}
				}
			} else if strings.TrimSpace(p.Content) != "" {
				triggerIndex.WriteString(p.Content)
				triggerIndex.WriteString("\n")
			}
		}
		for _, p := range ps.activeProcedures {
			if referencedAsSubProc[p.ID] {
				if strings.TrimSpace(p.Trigger) != "" {
					skillSummaries = append(skillSummaries, voice.SkillSummary{
						Name:        p.Name,
						Description: p.Trigger,
					})
				}
				triggerIndex.WriteString(fmt.Sprintf("\n### SUB-SKILL / PROCEDIMENTO COMPLEMENTAR OBRIGATÓRIO: %q\n- **Quando aplicar (Trigger):** %s\n", p.Name, p.Trigger))
				if expanded, expErr := procExpander.ExpandProcedure(ctx, orgID, agentID, p, ps.procsMap); expErr == nil && strings.TrimSpace(expanded.ExpandedContent) != "" {
					triggerIndex.WriteString(expanded.ExpandedContent)
					triggerIndex.WriteString("\n")
					for tName, ok := range expanded.AllowedTools {
						if ok {
							ps.promptReferencedTools[tName] = true
						}
					}
				}
			}
		}
		// Procedimentos completos ficam no systemPrompt (Gemini/Realtime) e no reasonerPrompt (Reasoner do GPT-Live),
		// NUNCA no Speaker do GPT-Live (que recebe apenas skillSummaries de 1 linha estilo Vapi).
		systemPrompt = triggerIndex.String() + "\n\n---\n\n" + systemPrompt + "\n\n[Lembrete: siga estritamente os procedimentos obrigatórios no topo deste prompt sempre que seus gatilhos forem acionados.]"
		reasonerPrompt = triggerIndex.String() + "\n\n---\n\n" + reasonerPrompt + "\n\n[Lembrete: siga estritamente os procedimentos obrigatórios no topo deste prompt sempre que seus gatilhos forem acionados.]"

		ps.toolSpecs = append(ps.toolSpecs, voice.ToolSpec{
			Name:        "ativar_procedure",
			Description: "Ativa um procedimento de negócio quando o assunto do Caller corresponder ao seu gatilho: " + strings.Join(procNamesList, "; "),
			Parameters: []voice.Param{
				{Name: "nome", Type: "string", Description: "Nome exato do procedimento a ativar", Required: true},
			},
		})
	}

	mcpSummaryLines := rt.bindAgentToolsAndMCP(
		ctx, orgID, convIDForVars, addDeferred, cfg.Tools, ps.promptReferencedTools, cfg.SystemPrompt, ps.activeProcedures, ps.procsMap, ps.allVars,
		&ps.toolSpecs, ps.registeredTools, ps.mcpSessions, ps.mcpBindings,
	)
	if len(mcpSummaryLines) > 0 {
		mcpBlock := "\n\n## FERRAMENTAS DE INTEGRAÇÃO MCP CONECTADAS:\n" +
			"Quando um procedimento mencionar uma integração MCP pelo nome do servidor, chame diretamente a ferramenta específica listada abaixo correspondente à solicitação do usuário:\n" +
			strings.Join(mcpSummaryLines, "\n")
		systemPrompt += mcpBlock
		reasonerPrompt += mcpBlock
	}

	ps.cfg = cfg
	ps.prov = prov
	if prov != nil {
		ps.cat = prov.Catalog()
	}
	ps.firstMessage = firstMessage
	ps.systemPrompt = systemPrompt

	provSess, err := prov.Connect(ctx, voice.SessionConfig{
		AgentName:        cfg.Name,
		Model:            cfg.Model,
		DelegationModel:  cfg.SystemToolsConfig.DelegationModel,
		ReasoningEffort:  cfg.SystemToolsConfig.ReasoningEffort,
		Voice:            cfg.Voice,
		Language:         cfg.Language,
		SystemPrompt:     systemPrompt,
		VoicePrompt:      rawRenderedPrompt,
		SpeakerPrompt:    speakerExplicitPrompt,
		ReasonerPrompt:   reasonerPrompt,
		PersonalityPacks: cfg.SystemToolsConfig.PersonalityPacks,
		SkillSummaries:   skillSummaries,
		FirstMessage:     firstMessage,
		ThinkingLevel:    "",
		Tools:            ps.toolSpecs,
	})
	if err != nil {
		for _, ms := range ps.mcpSessions {
			if ms != nil {
				ms.Close()
			}
		}
		ps.mcpSessions = nil
		return ps, err
	}

	ps.provSess = provSess
	return ps, nil
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

	// Fila assíncrona ordenada para gravar eventos no banco sem nunca bloquear os frames de áudio em tempo real
	eventQueue := make(chan deferredConvEvent, 512)
	eventWorkerDone := make(chan struct{})
	var eventQueueMu sync.Mutex
	eventQueueClosed := false
	go func() {
		defer close(eventWorkerDone)
		bgCtx := context.Background()
		for item := range eventQueue {
			_ = rt.Store.RecordEvent(bgCtx, orgID, conv.ID, item.Type, item.Payload)
		}
	}()
	recordEventAsync := func(evType string, payload any) {
		eventQueueMu.Lock()
		if eventQueueClosed {
			eventQueueMu.Unlock()
			return
		}
		select {
		case eventQueue <- deferredConvEvent{Type: evType, Payload: payload}:
			eventQueueMu.Unlock()
		default:
			eventQueueMu.Unlock()
			go func() {
				_ = rt.Store.RecordEvent(context.Background(), orgID, conv.ID, evType, payload)
			}()
		}
	}
	flushEvents := func() {
		eventQueueMu.Lock()
		if !eventQueueClosed {
			eventQueueClosed = true
			close(eventQueue)
		}
		eventQueueMu.Unlock()
		<-eventWorkerDone
	}

	endCall := func(status, reason string) billing.CostResult {
		flushEvents()
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

	callerID := opts.FromNumber
	if callerID == "" {
		callerID = caller
	}

	executor := tools.NewExecutor(rt.HTTPClient)
	procExpander := procedures.NewExpander(rt.Store)

	// 2. Consome sessão pré-aquecida durante o webhook da telefonia (se disponível) ou prepara/conecta agora
	var ps *preparedSession
	if prewarmPS, wasPrewarmed, prewarmErr := rt.consumePrewarmedCall(ctx, opts.CallSid); wasPrewarmed && prewarmErr == nil && prewarmPS != nil && prewarmPS.provSess != nil {
		ps = prewarmPS
		ps.allVars["system__conversation_id"] = conv.ID
	} else {
		if prewarmPS != nil {
			for _, dev := range prewarmPS.deferredEvents {
				recordEventAsync(dev.Type, dev.Payload)
			}
			prewarmPS.cleanup()
		}
		ps, err = rt.prepareSession(ctx, startedAt, orgID, agentID, conv.ID, callerID, opts.ToNumber, opts.CallSid, cfg, prov)
		if err != nil {
			if ps != nil {
				for _, dev := range ps.deferredEvents {
					recordEventAsync(dev.Type, dev.Payload)
				}
				ps.cleanup()
			}
			rt.Log.Error("provider_connect_failed", "conversation_id", conv.ID, "provider", cfg.Provider, "error", err)
			recordEventAsync("error", map[string]any{"phase": "connect", "error": err.Error()})
			endCall("failed", "provider_error")
			sendJSON(serverMessage{Type: "error", Message: fmt.Sprintf("falha ao conectar com o Voice Provider: %v", err)})
			return
		}
	}

	if ps.cancelLifetime != nil {
		defer ps.cancelLifetime()
	}

	cfg = ps.cfg
	if ps.prov != nil {
		prov = ps.prov
		cat = ps.cat
	}
	allVars := ps.allVars
	firstMessage := ps.firstMessage
	procsMap := ps.procsMap
	activeProcedures := ps.activeProcedures
	entryProcedures := ps.entryProcedures
	promptReferencedTools := ps.promptReferencedTools
	registeredTools := ps.registeredTools
	mcpSessions := ps.mcpSessions
	mcpBindings := ps.mcpBindings
	provSess := ps.provSess

	for _, dev := range ps.deferredEvents {
		recordEventAsync(dev.Type, dev.Payload)
	}

	defer func() {
		for _, ms := range mcpSessions {
			if ms != nil {
				ms.Close()
			}
		}
	}()

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

	// Persiste variáveis e status ativo em background sem atrasar o primeiro pacote de áudio
	varsSnapshot := make(map[string]string, len(allVars))
	for k, v := range allVars {
		varsSnapshot[k] = v
	}
	go func() {
		bgCtx := context.Background()
		rawVars, _ := json.Marshal(varsSnapshot)
		_, _ = rt.Store.Pool.Exec(bgCtx, `UPDATE conversations SET variables = $3 WHERE organization_id = $1 AND id = $2`, orgID, conv.ID, rawVars)
		_ = rt.Store.MarkConversationActive(bgCtx, orgID, conv.ID)
	}()

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

	// 5. Inicia First Message se configurada (se o provider já não disparou no Connect)
	if firstMessage != "" {
		recordEventAsync("first_message_sent", map[string]any{"text": firstMessage})
		if primer, ok := provSess.(voice.FirstMessagePrimer); !ok || !primer.FirstMessagePrimed() {
			instruction := fmt.Sprintf("ATENÇÃO (PRIMEIRO TURNO DA LIGAÇÃO): Diga agora APENAS e EXATAMENTE a seguinte frase de saudação e aguarde em silêncio o cliente responder (NÃO fale a apresentação da empresa ainda neste turno):\n\"%s\"", firstMessage)
			if err := provSess.SendText(instruction); err != nil {
				rt.Log.Warn("first_message_send_failed", "conversation_id", conv.ID, "error", err)
			}
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
	var procMu sync.Mutex
	var activeProcedureName string
	var activeProcedureID *string
	var recentActivatedProcs []procedures.ActivatedProcedure
	activatedProcIDs := make(map[string]bool)
	activeAllowedTools := make(map[string]bool)
	activeToolPolicies := make(map[string]procedures.ToolExecutionPolicy)
	// Se o agente não possui procedures de entrada, todas as tools registradas ficam permitidas
	if len(entryProcedures) == 0 {
		for name := range registeredTools {
			activeAllowedTools[name] = true
		}
		for name := range mcpBindings {
			activeAllowedTools[name] = true
		}
	} else {
		// Ferramentas anexadas globalmente ao agente ou referenciadas em procedures ativos ficam permitidas
		for _, globalToolID := range cfg.Tools {
			if t, err := rt.Store.GetTool(ctx, orgID, globalToolID); err == nil {
				activeAllowedTools[t.Name] = true
			}
		}
		for tName, ok := range promptReferencedTools {
			if ok {
				activeAllowedTools[tName] = true
			}
		}
		for name := range registeredTools {
			activeAllowedTools[name] = true
		}
		for name := range mcpBindings {
			activeAllowedTools[name] = true
		}
	}

	var activateProcedure func(targetProc store.ProcedureSnapshot) (procedures.ActivatedProcedure, bool, error)
	activateProcedure = func(targetProc store.ProcedureSnapshot) (procedures.ActivatedProcedure, bool, error) {
		expanded, err := procExpander.ExpandProcedure(sessionCtx, orgID, agentID, targetProc, procsMap)
		if err != nil {
			return procedures.ActivatedProcedure{}, false, err
		}

		procMu.Lock()
		newlyActivated := !activatedProcIDs[targetProc.ID]
		activatedProcIDs[targetProc.ID] = true
		activeProcedureName = targetProc.Name
		procIDCopy := targetProc.ID
		activeProcedureID = &procIDCopy

		recentActivatedProcs = append(recentActivatedProcs, expanded)
		if len(recentActivatedProcs) > 5 {
			recentActivatedProcs = recentActivatedProcs[len(recentActivatedProcs)-5:]
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
		procMu.Unlock()

		if newlyActivated {
			_ = sendJSON(serverMessage{Type: "procedure_activated", Text: targetProc.Name})
			recordEventAsync("procedure_activated", map[string]any{
				"procedure_id": targetProc.ID,
				"name":         targetProc.Name,
				"trigger":      targetProc.Trigger,
			})
		}

		// Se este procedimento referencia outros procedures (ex: Segunda Via -> Identificação/Autorização),
		// ativa também os sub-procedures referenciados que ainda não foram ativados.
		meta := procedures.InspectProcedure(store.Procedure{
			ID:      targetProc.ID,
			Name:    targetProc.Name,
			Type:    targetProc.Type,
			Trigger: targetProc.Trigger,
			Content: targetProc.Content,
		}, procsMap, nil, nil)
		for _, subRef := range meta.ReferencedProcedureIDs {
			for _, cand := range activeProcedures {
				if cand.ID == subRef || strings.EqualFold(strings.TrimSpace(cand.Name), strings.TrimSpace(subRef)) {
					procMu.Lock()
					already := activatedProcIDs[cand.ID]
					procMu.Unlock()
					if !already {
						_, _, _ = activateProcedure(cand)
					}
				}
			}
		}

		return expanded, newlyActivated, nil
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
					if err := sendBinary(ev.Audio); err != nil {
						return
					}
					if !lastCallerSpeechEnd.IsZero() {
						latencyMS := time.Since(lastCallerSpeechEnd).Milliseconds()
						lastCallerSpeechEnd = time.Time{}
						recordEventAsync("response_latency", map[string]any{
							"latency_ms": latencyMS, "had_tool_call": turnHadToolCall,
						})
					}

				case voice.EventCallerSpeechEnd:
					lastCallerSpeechEnd = time.Now()
					turnAgentAudioBytes = 0
					currentTurnAgentText.Reset()

				case voice.EventCallerTranscript:
					if callEnding.Load() {
						continue
					}
					normCaller := normalizeCallerTranscriptPT(ev.Text, cfg.Language)
					trimmedCaller := strings.TrimSpace(normCaller)
					if trimmedCaller != "" {
						guardianMu.Lock()
						if silenceTimer != nil {
							silenceTimer.Stop()
						}
						appendStreamDelta(&currentCallerTurnText, normCaller)
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
					_ = sendJSON(serverMessage{Type: "caller_transcript", Text: normCaller})
					recordEventAsync("caller_transcript", map[string]any{"text": normCaller})

					// Avaliação automática em tempo real dos gatilhos (Triggers) dos procedimentos a partir da fala do Caller
					if trimmedCaller != "" && len(activeProcedures) > 0 {
						guardianMu.Lock()
						turnCallerText := lastCallerTurnText
						guardianMu.Unlock()
						matchedProcs := matchProceduresByCallerTranscript(turnCallerText, activeProcedures)
						for _, mp := range matchedProcs {
							procMu.Lock()
							already := activatedProcIDs[mp.ID]
							procMu.Unlock()
							if !already {
								_, _, _ = activateProcedure(mp)
							}
						}
					}

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
						appendStreamDelta(&currentTurnAgentText, ev.Text)
					}
					_ = sendJSON(serverMessage{Type: "agent_transcript", Text: ev.Text})
					recordEventAsync("agent_transcript", map[string]any{"text": ev.Text})

				case voice.EventInterrupted:
					if callEnding.Load() || pendingHangupAfterTurn {
						continue
					}
					playbackMu.Lock()
					agentPlaybackEndsAt = time.Now()
					playbackMu.Unlock()
					rec.TruncateAgentOnInterruption()
					_ = sendJSON(serverMessage{Type: "interrupted"})
					recordEventAsync("interruption", map[string]any{})

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
						recordEventAsync("usage", ev.Usage)
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
					if callerTurnSnapshot != "" {
						transcriptHistory = append(transcriptHistory, fmt.Sprintf("Usuário: %s", callerTurnSnapshot))
					}
					if rawAgentTurn != "" {
						transcriptHistory = append(transcriptHistory, fmt.Sprintf("Agente (%s): %s", cfg.Name, rawAgentTurn))
					}
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
						if strings.Contains(spokenInTurn, "verificar") ||
							strings.Contains(spokenInTurn, "consultar") ||
							strings.Contains(spokenInTurn, "um instante") ||
							strings.Contains(spokenInTurn, "um momento") ||
							strings.Contains(spokenInTurn, "já te retorno") ||
							strings.Contains(spokenInTurn, "aguarde") {
							silenceWait = remPlay + 28*time.Second
						}
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
							// 2. Agente Alucinando -> Corrigir silenciosamente apenas em alucinações claras do agente (>= 0.88 e severidade >= 1.5),
							// nunca interrompendo a fala do usuário por ruído de transcrição do Caller.
							if gc.CheckHallucination && res.AgentHallucinating >= gc.HallucinationThreshold && res.AgentHallucinating >= 0.88 && res.HallucinationSeverity >= 1.5 {
								guardianMu.Lock()
								hWarn := hallucinationWarnings
								hallucinationWarnings++
								guardianMu.Unlock()

								if hWarn == 0 && res.HallucinationSeverity < 1.92 {
									_ = rt.Store.RecordEvent(sessionCtx, orgID, conv.ID, "guardian_alert", map[string]any{
										"kind":                   "hallucination",
										"action":                 "correct_realtime",
										"agent_hallucinating":    res.AgentHallucinating,
										"hallucination_severity": res.HallucinationSeverity,
										"reason":                 res.Reason,
										"model":                  res.Model,
									})
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
					guardianMu.Lock()
					if silenceTimer != nil {
						silenceTimer.Stop()
					}
					guardianMu.Unlock()
					fc := *ev.ToolCall
					_ = sendJSON(serverMessage{Type: "tool_call", Text: fc.Name})
					recordEventAsync("tool_call", map[string]any{
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
							expanded, _, err := activateProcedure(*targetProc)
							if err == nil {
								res := map[string]any{
									"success":      true,
									"procedimento": targetProc.Name,
									"tipo":         expanded.Type,
									"instrucoes":   expanded.ExpandedContent,
								}
								_ = getProvSess().SendToolResult(fc, res)
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
						for _, rawP := range targetCfg.Procedures {
							if rawP.Active {
								p := rawP
								p.Trigger = variables.Render(p.Trigger, allVars)
								p.Content = variables.Render(p.Content, allVars)
								if len(p.Guardrails) > 0 {
									rg := make([]string, len(p.Guardrails))
									for i, g := range p.Guardrails {
										rg[i] = variables.Render(g, allVars)
									}
									p.Guardrails = rg
								}
								newProcsMap[p.ID] = p
								newActiveProcedures = append(newActiveProcedures, p)
								if strings.TrimSpace(p.Trigger) != "" {
									newEntryProcedures = append(newEntryProcedures, p)
								} else {
									p.Trigger = "Quando o atendimento envolver " + p.Name
									newEntryProcedures = append(newEntryProcedures, p)
								}
							}
						}
						childPrompt = procExpander.ExpandInlineReferences(sessionCtx, orgID, childPrompt, newProcsMap, nil)

						var childToolSpecs []voice.ToolSpec
						newRegisteredTools := make(map[string]store.Tool)
						newMCPSessions := make(map[string]*tools.MCPSession)
						newMCPBindings := make(map[string]mcpToolBinding)

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
							trigSb.WriteString("\n\n## PROCEDIMENTOS DO AGENTE (SIGA RIGOROSAMENTE QUANDO APLICÁVEL):\n")
							for _, p := range newEntryProcedures {
								trigSb.WriteString(fmt.Sprintf("\n### PROCEDIMENTO: %q\n- **Quando acionar (Trigger):** %s\n", p.Name, p.Trigger))
								if expanded, expErr := procExpander.ExpandProcedure(sessionCtx, orgID, targetAgent.ID, p, newProcsMap); expErr == nil && strings.TrimSpace(expanded.ExpandedContent) != "" {
									trigSb.WriteString(expanded.ExpandedContent)
									trigSb.WriteString("\n")
								} else if strings.TrimSpace(p.Content) != "" {
									trigSb.WriteString(p.Content)
									trigSb.WriteString("\n")
								}
							}
							childPrompt += trigSb.String()
						}

						childMCPLines := rt.bindAgentToolsAndMCP(
							sessionCtx, orgID, conv.ID, recordEventAsync, targetCfg.Tools, nil, targetCfg.SystemPrompt, newActiveProcedures, newProcsMap, allVars,
							&childToolSpecs, newRegisteredTools, newMCPSessions, newMCPBindings,
						)
						if len(childMCPLines) > 0 {
							childPrompt += "\n\n## FERRAMENTAS DE INTEGRAÇÃO MCP CONECTADAS:\n" + strings.Join(childMCPLines, "\n")
						}

						var childSkillSummaries []voice.SkillSummary
						for _, p := range newEntryProcedures {
							childSkillSummaries = append(childSkillSummaries, voice.SkillSummary{
								Name:        p.Name,
								Description: p.Trigger,
							})
						}
						childSess, err := childProv.Connect(sessionCtx, voice.SessionConfig{
							AgentName:        targetCfg.Name,
							Model:            targetCfg.Model,
							DelegationModel:  targetCfg.SystemToolsConfig.DelegationModel,
							ReasoningEffort:  targetCfg.SystemToolsConfig.ReasoningEffort,
							Voice:            targetCfg.Voice,
							Language:         inheritedLang,
							SystemPrompt:     childPrompt,
							VoicePrompt:      variables.Render(targetCfg.SystemPrompt, allVars),
							SpeakerPrompt:    variables.Render(targetCfg.SystemToolsConfig.SpeakerPrompt, allVars),
							ReasonerPrompt:   childPrompt,
							PersonalityPacks: targetCfg.SystemToolsConfig.PersonalityPacks,
							SkillSummaries:   childSkillSummaries,
							Tools:            childToolSpecs,
						})
						if err != nil {
							for _, ms := range newMCPSessions {
								if ms != nil {
									ms.Close()
								}
							}
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

						for _, oldMS := range mcpSessions {
							if oldMS != nil {
								oldMS.Close()
							}
						}

						agentID = targetAgent.ID
						cfg = targetCfg
						procsMap = newProcsMap
						activeProcedures = newActiveProcedures
						entryProcedures = newEntryProcedures
						registeredTools = newRegisteredTools
						mcpSessions = newMCPSessions
						mcpBindings = newMCPBindings
						procMu.Lock()
						recentActivatedProcs = nil
						activatedProcIDs = make(map[string]bool)
						activeAllowedTools = make(map[string]bool)
						for tName := range newRegisteredTools {
							activeAllowedTools[tName] = true
						}
						for tName := range newMCPBindings {
							activeAllowedTools[tName] = true
						}
						procMu.Unlock()
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
					} else if binding, isMCP := mcpBindings[fc.Name]; isMCP {
						targetTool := binding.ParentTool
						for _, p := range activeProcedures {
							meta := procedures.InspectProcedure(store.Procedure{
								ID:      p.ID,
								Name:    p.Name,
								Type:    p.Type,
								Trigger: p.Trigger,
								Content: p.Content,
							}, procsMap, nil, nil)
							for _, refToolID := range meta.ReferencedToolIDs {
								if refToolID == targetTool.ID || strings.EqualFold(refToolID, targetTool.Name) {
									procMu.Lock()
									already := activatedProcIDs[p.ID]
									procMu.Unlock()
									if !already {
										_, _, _ = activateProcedure(p)
									}
								}
							}
						}

						hasSpecificTools := false
						for tName := range activeAllowedTools {
							if tName != "encerrar_conversa" && tName != "end_call" {
								hasSpecificTools = true
								break
							}
						}
						if hasSpecificTools && !activeAllowedTools[fc.Name] && !activeAllowedTools[targetTool.Name] {
							rejected := map[string]any{
								"success": false,
								"error":   fmt.Sprintf("A ferramenta MCP '%s' (%s) não é permitida no procedimento ativo ('%s').", fc.Name, targetTool.Name, activeProcedureName),
							}
							_ = getProvSess().SendToolResult(fc, rejected)
							_ = rt.Store.RecordToolExecution(sessionCtx, store.ToolExecution{
								OrganizationID:  orgID,
								AgentID:         agentID,
								ConversationID:  conv.ID,
								ToolID:          &targetTool.ID,
								ToolName:        binding.RemoteToolName,
								ProcedureID:     activeProcedureID,
								Request:         mustJSON(fc.Args),
								Response:        mustJSON(rejected),
								Status:          "rejected",
								ExecutionTimeMS: 0,
							})
							continue
						}

						callArgs := make(map[string]any, len(fc.Args))
						for k, v := range fc.Args {
							callArgs[k] = v
						}

						var execRes tools.ExecutionResult
						if mockRes, matched := tools.CheckResponseMocks(targetTool, callArgs); matched {
							execRes = mockRes
						} else {
							callTimeout := time.Duration(targetTool.TimeoutMS) * time.Millisecond
							if callTimeout <= 0 {
								callTimeout = 20 * time.Second
							}
							callCtx, cancelCall := context.WithTimeout(sessionCtx, callTimeout)
							execRes = binding.Session.CallTool(callCtx, binding.RemoteToolName, callArgs)
							cancelCall()
							if execRes.Status == "success" {
								if assigned := tools.EvaluateAssignments(targetTool, execRes.RawResponse); len(assigned) > 0 {
									execRes.AssignedVars = assigned
								}
							}
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
							ToolName:        binding.RemoteToolName,
							ProcedureID:     activeProcedureID,
							Request:         mustJSON(fc.Args),
							Response:        mustJSON(execRes.Result),
							Status:          execRes.Status,
							ExecutionTimeMS: execRes.DurationMS,
						})
						_ = sendJSON(serverMessage{Type: "tool_result", Text: fc.Name})
						evPayload := map[string]any{
							"name":        fc.Name,
							"remote_tool": binding.RemoteToolName,
							"mcp_server":  targetTool.Name,
							"status":      execRes.Status,
							"duration_ms": execRes.DurationMS,
						}
						if execRes.ErrorMessage != "" {
							evPayload["error"] = execRes.ErrorMessage
						}
						_ = rt.Store.RecordEvent(sessionCtx, orgID, conv.ID, "tool_result", evPayload)
					} else if targetTool, ok := registeredTools[fc.Name]; ok {
						// Se a ferramenta chamada pertence a um procedure que ainda não foi marcado como ativado, ativa-o automaticamente
						for _, p := range activeProcedures {
							meta := procedures.InspectProcedure(store.Procedure{
								ID:      p.ID,
								Name:    p.Name,
								Type:    p.Type,
								Trigger: p.Trigger,
								Content: p.Content,
							}, procsMap, nil, nil)
							for _, refToolID := range meta.ReferencedToolIDs {
								if refToolID == targetTool.ID || strings.EqualFold(refToolID, targetTool.Name) {
									procMu.Lock()
									already := activatedProcIDs[p.ID]
									procMu.Unlock()
									if !already {
										_, _, _ = activateProcedure(p)
									}
								}
							}
						}

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
						evPayload := map[string]any{
							"name": targetTool.Name, "status": execRes.Status, "duration_ms": execRes.DurationMS,
						}
						if execRes.ErrorMessage != "" {
							evPayload["error"] = execRes.ErrorMessage
						}
						_ = rt.Store.RecordEvent(sessionCtx, orgID, conv.ID, "tool_result", evPayload)
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

			if !ignoringCallerAudio.Load() {
				// Envia o áudio do microfone/linha continuamente ao provedor sem cortar pacotes de 20ms,
				// preservando consoantes fricativas/nasais (s, f, p, t, c, n, m) e o VAD neural nativo.
				if err := getProvSess().SendAudio(ev.Audio); err != nil {
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

// appendStreamDelta concatena fragmentos de transcrição preservando espaços originais
// entre tokens (seja espaço no fim do token anterior ou no início do novo token).
func appendStreamDelta(sb *strings.Builder, delta string) {
	if delta == "" {
		return
	}
	cur := sb.String()
	if cur == "" {
		sb.WriteString(delta)
		return
	}
	prevEndsSpace := strings.HasSuffix(cur, " ") || strings.HasSuffix(cur, "\t") || strings.HasSuffix(cur, "\n")
	nextStartsSpace := strings.HasPrefix(delta, " ") || strings.HasPrefix(delta, "\t") || strings.HasPrefix(delta, "\n")
	trimmedNext := strings.TrimLeft(delta, " \t\n")
	if trimmedNext == "" {
		if !prevEndsSpace {
			sb.WriteString(" ")
		}
		return
	}
	if prevEndsSpace || nextStartsSpace {
		if !prevEndsSpace {
			sb.WriteString(" ")
		}
		sb.WriteString(trimmedNext)
		return
	}
	sb.WriteString(delta)
}

// normalizeCallerTranscriptPT corrige homófonos curtos que modelos multilíngues em 8kHz
// transcrevem em espanhol/inglês/alemão quando o idioma da chamada é Português (pt-BR).
func normalizeCallerTranscriptPT(text, lang string) string {
	l := strings.ToLower(strings.TrimSpace(lang))
	if l != "" && !strings.HasPrefix(l, "pt") {
		return text
	}
	trimmed := strings.TrimSpace(text)
	switch strings.ToLower(trimmed) {
	case "¿sí?", "sí?", "sí.", "sí", "si.", "si?":
		return "Sim."
	case "ja.", "ja!", "ja":
		return "Já."
	case "now.", "now!", "now":
		return "Não."
	case "comarido", "comarido.", "comarido!":
		return "Com o marido."
	}
	if strings.Contains(strings.ToLower(text), "comarido") {
		text = strings.ReplaceAll(text, "comarido", "com o marido")
		text = strings.ReplaceAll(text, "Comarido", "Com o marido")
	}
	return text
}

var triggerStopwords = map[string]bool{
	"quando": true, "cliente": true, "solicitar": true, "pedir": true, "sobre": true,
	"para": true, "como": true, "pelo": true, "pela": true, "este": true, "esta": true,
	"falar": true, "dizer": true, "informar": true, "fornecer": true, "atendimento": true,
	"servico": true, "exija": true, "assunto": true, "topico": true, "duvidas": true,
	"tiver": true, "quiser": true, "necessitar": true, "desejar": true, "precisar": true,
	"algum": true, "qualquer": true, "uma": true, "uns": true, "umas": true, "dos": true,
	"das": true, "nos": true, "nas": true, "com": true, "sem": true, "por": true, "que": true,
	"seu": true, "sua": true, "meu": true, "minha": true, "usuario": true, "beneficiario": true,
}

func foldDiacriticsPT(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	repl := strings.NewReplacer(
		"á", "a", "à", "a", "â", "a", "ã", "a",
		"é", "e", "ê", "e",
		"í", "i",
		"ó", "o", "ô", "o", "õ", "o",
		"ú", "u", "ü", "u",
		"ç", "c",
	)
	return repl.Replace(s)
}

func tokenizeTriggerWords(s string) []string {
	folded := foldDiacriticsPT(s)
	var token strings.Builder
	var out []string
	flush := func() {
		if token.Len() >= 3 {
			w := token.String()
			if !triggerStopwords[w] {
				out = append(out, w)
			}
		}
		token.Reset()
	}
	for _, r := range folded {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			token.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}

// matchProceduresByCallerTranscript avalia a fala atual do Caller contra o Nome, Trigger e domínio dos procedures ativos.
func matchProceduresByCallerTranscript(callerText string, procs []store.ProcedureSnapshot) []store.ProcedureSnapshot {
	foldedCaller := foldDiacriticsPT(callerText)
	if len(foldedCaller) < 3 {
		return nil
	}
	callerTokens := tokenizeTriggerWords(callerText)
	callerSet := make(map[string]bool, len(callerTokens))
	digitCount := 0
	for _, r := range foldedCaller {
		if r >= '0' && r <= '9' {
			digitCount++
		}
	}
	for _, t := range callerTokens {
		callerSet[t] = true
	}

	var matched []store.ProcedureSnapshot
	for _, p := range procs {
		if !p.Active {
			continue
		}
		procSpec := foldDiacriticsPT(p.Name + " " + p.Trigger)
		procContent := foldDiacriticsPT(p.Content)
		procTokens := tokenizeTriggerWords(p.Name + " " + p.Trigger)

		isMatch := false
		for _, pt := range procTokens {
			if callerSet[pt] {
				isMatch = true
				break
			}
			// Correspondência por radical (>= 5 letras, ex: "boletos" <-> "boleto", "cadastral" <-> "cadastro")
			if len(pt) >= 5 {
				stem := pt[:5]
				for ct := range callerSet {
					if len(ct) >= 5 && strings.HasPrefix(ct, stem) {
						isMatch = true
						break
					}
				}
			}
			if isMatch {
				break
			}
		}

		// Regras semânticas de domínio para Boletos/Financeiro e Identificação/Autorização/Terceiros/CPF
		if !isMatch {
			if strings.Contains(procSpec, "boleto") || strings.Contains(procSpec, "segunda via") || strings.Contains(procSpec, "financeiro") {
				for _, kw := range []string{"boleto", "segunda via", "2 via", "2a via", "fatura", "mensalidade", "pagamento", "pagar", "vencimento", "cobranca", "codigo de barras"} {
					if strings.Contains(foldedCaller, kw) {
						isMatch = true
						break
					}
				}
			}
		}
		if !isMatch {
			if strings.Contains(procSpec, "identificacao") || strings.Contains(procSpec, "autorizacao") || strings.Contains(procSpec, "cpf") || strings.Contains(procSpec, "cadastr") || strings.Contains(procContent, "terceiro") || strings.Contains(procContent, "dependente") {
				if digitCount >= 3 {
					isMatch = true
				} else {
					for _, kw := range []string{"cpf", "cadastro", "cadastral", "marido", "esposa", "esposo", "mae", "pai", "filho", "filha", "dependente", "parente", "terceiro", "titular"} {
						if strings.Contains(foldedCaller, kw) {
							isMatch = true
							break
						}
					}
				}
			}
		}

		if isMatch {
			matched = append(matched, p)
		}
	}
	return matched
}

type mcpToolBinding struct {
	ParentTool     store.Tool
	RemoteToolName string
	Session        *tools.MCPSession
}

func deriveWebhookToolParams(t store.Tool) []voice.Param {
	if len(t.InputSchema) > 0 {
		return t.InputSchema
	}
	if t.Config.APISchema == nil {
		return nil
	}
	var out []voice.Param
	seen := make(map[string]bool)
	addParam := func(ps store.ToolParamSchema, forceReq bool) {
		name := strings.TrimSpace(ps.Name)
		if name == "" || seen[name] {
			return
		}
		if ps.ValueType != "" && ps.ValueType != "llm_prompt" {
			return
		}
		seen[name] = true
		typ := strings.TrimSpace(ps.Type)
		if typ == "" {
			typ = "string"
		}
		out = append(out, voice.Param{
			Name:        name,
			Type:        typ,
			Description: ps.Description,
			Required:    forceReq || ps.Required,
		})
	}
	for _, pp := range t.Config.APISchema.PathParamsSchema {
		addParam(pp, true)
	}
	for _, qp := range t.Config.APISchema.QueryParamsSchema {
		addParam(qp, false)
	}
	if t.Config.APISchema.RequestBodySchema != nil {
		for _, bp := range t.Config.APISchema.RequestBodySchema.Properties {
			addParam(bp, false)
		}
	}
	return out
}

func (rt *Runtime) bindAgentToolsAndMCP(
	ctx context.Context,
	orgID, convID string,
	onEvent func(evType string, payload any),
	cfgToolIDs []string,
	promptReferencedTools map[string]bool,
	rawPrompt string,
	activeProcedures []store.ProcedureSnapshot,
	procsMap map[string]store.ProcedureSnapshot,
	allVars map[string]string,
	toolSpecs *[]voice.ToolSpec,
	registeredTools map[string]store.Tool,
	mcpSessions map[string]*tools.MCPSession,
	mcpBindings map[string]mcpToolBinding,
) []string {
	emitEvent := func(evType string, payload any) {
		if onEvent != nil {
			onEvent(evType, payload)
			return
		}
		if rt.Store != nil && convID != "" {
			_ = rt.Store.RecordEvent(ctx, orgID, convID, evType, payload)
		}
	}
	addedToolIDs := make(map[string]bool)
	usedToolNames := make(map[string]bool)
	for _, ts := range *toolSpecs {
		usedToolNames[ts.Name] = true
	}

	var mcpSummaryLines []string
	var orderedTools []store.Tool

	collectTool := func(ref string) {
		ref = strings.TrimSpace(ref)
		if ref == "" || addedToolIDs[ref] || rt.Store == nil {
			return
		}
		var t store.Tool
		var err error
		if len(ref) == 36 && strings.Count(ref, "-") == 4 {
			t, err = rt.Store.GetTool(ctx, orgID, ref)
		} else {
			t, err = rt.Store.GetToolByName(ctx, orgID, ref)
		}
		if err != nil || !t.Active || addedToolIDs[t.ID] {
			return
		}
		addedToolIDs[ref] = true
		addedToolIDs[t.ID] = true
		addedToolIDs[strings.ToLower(t.Name)] = true
		orderedTools = append(orderedTools, t)
	}

	for _, toolID := range cfgToolIDs {
		collectTool(toolID)
	}
	for tName, ok := range promptReferencedTools {
		if ok {
			collectTool(tName)
		}
	}
	for _, ref := range procedures.ParseReferences(rawPrompt) {
		if ref.Kind == "tool" {
			collectTool(ref.ID)
		}
	}
	var combinedCorpus strings.Builder
	combinedCorpus.WriteString(strings.ToLower(rawPrompt))
	for _, p := range activeProcedures {
		combinedCorpus.WriteString("\n")
		combinedCorpus.WriteString(strings.ToLower(p.Trigger))
		combinedCorpus.WriteString("\n")
		combinedCorpus.WriteString(strings.ToLower(p.Content))
		meta := procedures.InspectProcedure(store.Procedure{
			ID:      p.ID,
			Name:    p.Name,
			Type:    p.Type,
			Trigger: p.Trigger,
			Content: p.Content,
		}, procsMap, nil, nil)
		for _, refToolID := range meta.ReferencedToolIDs {
			collectTool(refToolID)
		}
	}

	// Também vincula automaticamente qualquer Tool/MCP ativa da organização cujo nome ou ID apareça
	// explicitamente no prompt ou nos procedimentos ativos (ex.: @MCPNac_Ativo, MCPNac_Ativo, consultar_cpf).
	if rt.Store != nil {
		corpusStr := combinedCorpus.String()
		if orgTools, listErr := rt.Store.ListTools(ctx, orgID); listErr == nil {
			for _, ot := range orgTools {
				if !ot.Active || addedToolIDs[ot.ID] {
					continue
				}
				nameLower := strings.ToLower(strings.TrimSpace(ot.Name))
				idLower := strings.ToLower(strings.TrimSpace(ot.ID))
				if (len(nameLower) >= 3 && strings.Contains(corpusStr, nameLower)) ||
					(idLower != "" && strings.Contains(corpusStr, idLower)) {
					addedToolIDs[ot.ID] = true
					addedToolIDs[nameLower] = true
					orderedTools = append(orderedTools, ot.Tool)
				}
			}
		}
	}

	// Descobre ferramentas dos servidores MCP em paralelo para não somar latência no início da ligação
	type mcpResult struct {
		tool        store.Tool
		sess        *tools.MCPSession
		remoteTools []tools.MCPToolDef
		err         error
	}
	mcpResults := make(map[string]mcpResult)
	var mcpMu sync.Mutex
	var wg sync.WaitGroup

	for _, t := range orderedTools {
		isMCP := t.Kind == "mcp" || (t.Kind == "webhook" && strings.Contains(strings.ToLower(t.Config.URL), "/mcp") && len(t.InputSchema) == 0)
		if !isMCP {
			continue
		}
		wg.Add(1)
		go func(toolItem store.Tool) {
			defer wg.Done()
			discCtx, discCancel := context.WithTimeout(ctx, 6*time.Second)
			defer discCancel()
			sess, remoteTools, discErr := tools.ConnectOrWarmMCPSession(discCtx, rt.HTTPClient, toolItem, allVars)
			mcpMu.Lock()
			mcpResults[toolItem.ID] = mcpResult{
				tool:        toolItem,
				sess:        sess,
				remoteTools: remoteTools,
				err:         discErr,
			}
			mcpMu.Unlock()
		}(t)
	}
	wg.Wait()

	for _, t := range orderedTools {
		if res, wasMCP := mcpResults[t.ID]; wasMCP {
			if res.err == nil && len(res.remoteTools) > 0 {
				mcpSessions[t.ID] = res.sess
				var exposedNames []string
				for _, rtTool := range res.remoteTools {
					fnName := sanitizeLLMToolName(rtTool.Name, usedToolNames, t.Name)
					usedToolNames[fnName] = true
					desc := strings.TrimSpace(rtTool.Description)
					if desc == "" {
						desc = t.Description
					}
					var paramSchema map[string]any
					if len(rtTool.InputSchema) > 0 {
						paramSchema = voice.SanitizeToolJSONSchema(rtTool.InputSchema)
					} else {
						paramSchema = map[string]any{
							"type":       "object",
							"properties": map[string]any{},
							"required":   []string{},
						}
					}
					*toolSpecs = append(*toolSpecs, voice.ToolSpec{
						Name:             fnName,
						Description:      desc,
						Parameters:       voice.ExtractFlatParams(paramSchema),
						ParametersSchema: paramSchema,
					})
					mcpBindings[fnName] = mcpToolBinding{
						ParentTool:     t,
						RemoteToolName: rtTool.Name,
						Session:        res.sess,
					}
					exposedNames = append(exposedNames, fnName)
				}
				mcpSummaryLines = append(mcpSummaryLines, fmt.Sprintf("- Servidor MCP %q (transporte %s): ferramentas disponíveis -> %s", t.Name, res.sess.TransportUsed(), strings.Join(exposedNames, ", ")))
				emitEvent("mcp_connected", map[string]any{
					"tool_id":    t.ID,
					"tool_name":  t.Name,
					"transport":  res.sess.TransportUsed(),
					"tools":      exposedNames,
					"tool_count": len(exposedNames),
				})
				continue
			}
			if res.sess != nil {
				res.sess.Close()
			}
			errText := "nenhuma ferramenta retornada por tools/list"
			if res.err != nil {
				errText = res.err.Error()
			}
			if rt.Log != nil {
				rt.Log.Warn("mcp_discover_failed", "conversation_id", convID, "tool", t.Name, "url", t.Config.URL, "error", errText)
			}
			emitEvent("mcp_connection_error", map[string]any{
				"tool_id":   t.ID,
				"tool_name": t.Name,
				"url":       t.Config.URL,
				"error":     errText,
			})
		}

		fnName := sanitizeLLMToolName(t.Name, usedToolNames, "tool")
		usedToolNames[fnName] = true
		*toolSpecs = append(*toolSpecs, voice.ToolSpec{
			Name:        fnName,
			Description: t.Description,
			Parameters:  deriveWebhookToolParams(t),
		})
		registeredTools[fnName] = t
	}

	return mcpSummaryLines
}

func sanitizeLLMToolName(raw string, used map[string]bool, fallbackPrefix string) string {
	clean := func(s string) string {
		var sb strings.Builder
		for _, r := range s {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
				sb.WriteRune(r)
			} else {
				sb.WriteByte('_')
			}
		}
		res := strings.Trim(sb.String(), "_")
		if res == "" {
			res = "mcp_tool"
		}
		if res[0] >= '0' && res[0] <= '9' || res[0] == '-' {
			res = "t_" + res
		}
		if len(res) > 64 {
			res = res[:64]
		}
		return res
	}

	base := clean(raw)
	if !used[base] {
		return base
	}
	pref := clean(fallbackPrefix)
	candidate := pref + "_" + base
	if len(candidate) > 64 {
		candidate = candidate[:64]
	}
	if !used[candidate] {
		return candidate
	}
	for i := 2; i <= 99; i++ {
		suf := fmt.Sprintf("_%d", i)
		c := base
		if len(c)+len(suf) > 64 {
			c = c[:64-len(suf)]
		}
		c += suf
		if !used[c] {
			return c
		}
	}
	return base
}
