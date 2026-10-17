package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"voiceagent/internal/auth"
	"voiceagent/internal/runtime"
	"voiceagent/internal/store"
	"voiceagent/internal/voice"
)

func TestProceduresAndActivationFlow(t *testing.T) {
	ts, st := newTestServer(t)
	suffix := randSuffix()
	createOrg(t, st, "Org Proc", "proc-"+suffix+"@test.local", "senha-proc-123")

	c := newClient(t, ts.URL)
	c.do("POST", "/api/auth/login", map[string]string{"email": "proc-" + suffix + "@test.local", "password": "senha-proc-123"}, nil)

	// 1. Cria Knowledge Base com documento
	var kb store.KnowledgeBase
	c.do("POST", "/api/knowledge-bases", map[string]string{
		"name": "Regras Financeiras", "description": "Normas de boletos",
	}, &kb)

	c.do("POST", "/api/knowledge-bases/"+kb.ID+"/documents", map[string]string{
		"name": "boletos.md", "content": "Boletos vencidos há mais de 30 dias não podem ser reemitidos pelo canal de voz.",
	}, nil)

	// 2. Cria Tool permitida e Tool proibida
	fakeHook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"resultado": "ok"}`))
	}))
	defer fakeHook.Close()

	var toolPermitida, toolProibida store.Tool
	if code := c.do("POST", "/api/tools", map[string]any{
		"name": "consultar_boleto", "config": map[string]string{"url": fakeHook.URL, "method": "POST"},
		"input_schema": []map[string]any{{"name": "cpf", "type": "string", "required": true}},
		"active":       true,
	}, &toolPermitida); code != http.StatusCreated {
		t.Fatalf("criar toolPermitida: %d", code)
	}

	if code := c.do("POST", "/api/tools", map[string]any{
		"name": "cancelar_plano", "config": map[string]string{"url": fakeHook.URL, "method": "POST"},
		"input_schema": []map[string]any{},
		"active":       true,
	}, &toolProibida); code != http.StatusCreated {
		t.Fatalf("criar toolProibida: %d", code)
	}

	// 3. Cria Agente associando as duas tools
	var ag store.Agent
	if code := c.do("POST", "/api/agents", map[string]string{"name": "Livia Procedures"}, &ag); code != http.StatusCreated {
		t.Fatalf("criar agente: %d", code)
	}

	if code := c.do("PUT", "/api/agents/"+ag.ID, store.AgentUpdate{
		Name: ag.Name, Provider: "fake", Model: "m1", Voice: "v1", Language: "pt-BR", Timezone: "America/Sao_Paulo",
		SystemPrompt: "Você é um atendente.", SystemTools: []string{"encerrar_conversa"},
		Tools: []string{toolPermitida.ID, toolProibida.ID}, MaxDurationSeconds: 120, Active: true,
	}, nil); code != http.StatusOK {
		t.Fatalf("update agente: %d", code)
	}

	// 4. Cria Procedimento "Identificações"
	var procIdent store.Procedure
	c.do("POST", "/api/agents/"+ag.ID+"/procedures", map[string]any{
		"name":    "Identificações",
		"trigger": "Quando o cliente não estiver identificado",
		"content": "Peça o CPF do cliente para confirmar cadastro.",
		"active":  true,
	}, &procIdent)

	// 5. Cria Procedimento "Segunda Via" referenciando Identificações, a Tool e a Knowledge Base
	var procBoleto store.Procedure
	content := fmt.Sprintf(
		"Passo 1: Execute @[procedure:%s].\nPasso 2: Chame @[tool:%s].\nPasso 3: Consulte @[knowledge:%s].",
		procIdent.ID, toolPermitida.ID, kb.ID,
	)
	c.do("POST", "/api/agents/"+ag.ID+"/procedures", map[string]any{
		"name":       "Segunda Via",
		"trigger":    "Quando o cliente pedir segunda via de boleto",
		"guardrails": []string{"NUNCA leia o código de barras nem valores em aberto"},
		"content":    content,
		"active":     true,
	}, &procBoleto)

	// 6. Conecta via WebSocket e valida o fluxo de ativação
	mockP := &procMockProvider{}
	rt := &runtime.Runtime{
		Store:      st,
		Voices:     voice.NewRegistry(mockP),
		Sessions:   &auth.Sessions{Redis: getTestRedis(t)},
		HTTPClient: http.DefaultClient,
		Log:        slog.Default(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ws/conversations/{agent_id}", rt.ServeWS)
	wsSrv := httptest.NewServer(mux)
	defer wsSrv.Close()

	u, _ := st.UserByEmail(context.Background(), "proc-"+suffix+"@test.local")
	tok, _ := rt.Sessions.Create(context.Background(), auth.Session{UserID: u.ID, OrganizationID: u.OrganizationID, Email: u.Email})

	wsURL := "ws" + wsSrv.URL[4:] + "/ws/conversations/" + ag.ID + "?version=draft"
	header := http.Header{"Cookie": []string{auth.CookieName + "=" + tok}}
	ws, _, err := websocket.Dial(context.Background(), wsURL, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close(websocket.StatusNormalClosure, "")

	// Lê mensagem ready
	_, _, err = ws.Read(context.Background())
	if err != nil {
		t.Fatalf("erro lendo ready: %v", err)
	}

	// Verifica se o prompt gerado incluiu o índice de Triggers
	if !strings.Contains(mockP.lastCfg.SystemPrompt, "Segunda Via") || !strings.Contains(mockP.lastCfg.SystemPrompt, "Quando o cliente pedir segunda via de boleto") {
		t.Fatalf("índice de triggers não embutido no prompt: %s", mockP.lastCfg.SystemPrompt)
	}

	// Modelo aciona "ativar_procedure"
	mockP.sess.events <- voice.Event{
		Type: voice.EventToolCall,
		ToolCall: &voice.ToolCall{
			ID:   "call-proc",
			Name: "ativar_procedure",
			Args: map[string]any{"nome": "Segunda Via"},
		},
	}

	// Aguarda processamento da ativação
	time.Sleep(200 * time.Millisecond)

	// Verifica se a resposta do procedimento foi expandida inline com guardrails e KB
	lastRes := mockP.sess.lastResult
	if lastRes["success"] != true {
		t.Fatalf("ativação falhou: %+v", lastRes)
	}
	instrucoes, _ := lastRes["instrucoes"].(string)
	if !strings.Contains(instrucoes, "NUNCA leia o código de barras") {
		t.Fatalf("guardrail ausente nas instruções: %s", instrucoes)
	}
	if !strings.Contains(instrucoes, "Boletos vencidos há mais de 30 dias") {
		t.Fatalf("texto da Knowledge Base ausente nas instruções: %s", instrucoes)
	}
	if !strings.Contains(instrucoes, "PROCEDIMENTO INCLUÍDO: Identificações") {
		t.Fatalf("subprocedimento não incluído inline: %s", instrucoes)
	}

	// Agora o modelo tenta chamar uma ferramenta que NÃO está no procedure: cancelar_plano
	mockP.sess.events <- voice.Event{
		Type: voice.EventToolCall,
		ToolCall: &voice.ToolCall{
			ID:   "call-proibida",
			Name: "cancelar_plano",
			Args: map[string]any{},
		},
	}
	time.Sleep(200 * time.Millisecond)
	if mockP.sess.lastResult["success"] != false {
		t.Fatalf("ferramenta não autorizada deveria ter sido rejeitada pelo runtime: %+v", mockP.sess.lastResult)
	}

	// Modelo chama a ferramenta permitida: consultar_boleto
	mockP.sess.events <- voice.Event{
		Type: voice.EventToolCall,
		ToolCall: &voice.ToolCall{
			ID:   "call-permitida",
			Name: "consultar_boleto",
			Args: map[string]any{"cpf": "123"},
		},
	}
	time.Sleep(200 * time.Millisecond)
	if mockP.sess.lastResult["success"] != true {
		t.Fatalf("ferramenta permitida deveria ter sido executada com sucesso: %+v", mockP.sess.lastResult)
	}

	t.Log("Ativação de Procedure, expansão de Guardrails, Knowledge Base e Allowlist validadas com sucesso!")
}

type procMockProvider struct {
	lastCfg voice.SessionConfig
	sess    *procMockSession
}

func (m *procMockProvider) Catalog() voice.Catalog {
	return voice.Catalog{
		Provider: "fake", Label: "Fake", MaxSessionSeconds: 540,
		Models: []voice.Option{{ID: "m1", Default: true}}, Voices: []voice.Option{{ID: "v1", Default: true}},
		Languages: []voice.Option{{ID: "pt-BR", Default: true}}, InputSampleRate: 16000, OutputSampleRate: 24000,
	}
}

func (m *procMockProvider) Connect(ctx context.Context, cfg voice.SessionConfig) (voice.Session, error) {
	m.lastCfg = cfg
	m.sess = &procMockSession{events: make(chan voice.Event, 16)}
	return m.sess, nil
}

type procMockSession struct {
	events     chan voice.Event
	lastResult map[string]any
	closed     bool
}

func (s *procMockSession) SendAudio([]byte) error { return nil }
func (s *procMockSession) SendText(string) error  { return nil }
func (s *procMockSession) SendToolResult(call voice.ToolCall, res map[string]any) error {
	s.lastResult = res
	return nil
}
func (s *procMockSession) Events() <-chan voice.Event { return s.events }
func (s *procMockSession) Close() error {
	if !s.closed {
		s.closed = true
		close(s.events)
	}
	return nil
}
