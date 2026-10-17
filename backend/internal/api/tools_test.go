package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"

	"voiceagent/internal/auth"
	"voiceagent/internal/runtime"
	"voiceagent/internal/store"
	"voiceagent/internal/voice"
)

func TestToolsCRUDAndExecution(t *testing.T) {
	ts, st := newTestServer(t)
	suffix := randSuffix()
	createOrg(t, st, "Org Tool", "t-"+suffix+"@test.local", "senha-tool-123")
	createOrg(t, st, "Org Other", "other-"+suffix+"@test.local", "senha-other-123")

	c := newClient(t, ts.URL)
	c.do("POST", "/api/auth/login", map[string]string{"email": "t-" + suffix + "@test.local", "password": "senha-tool-123"}, nil)

	// Mock endpoint externo do webhook
	externalWebhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		if req["id_cliente"] == "cli-123" {
			w.Write([]byte(`{"saldo": 450.75, "status": "ativo"}`))
		} else {
			w.WriteHeader(404)
			w.Write([]byte(`{"erro": "cliente não encontrado"}`))
		}
	}))
	defer externalWebhook.Close()

	// 1. Cria Tool
	toolData := map[string]any{
		"name":        "consultar_saldo",
		"description": "Consulta o saldo do cliente",
		"kind":        "webhook",
		"config":      map[string]any{"url": externalWebhook.URL, "method": "POST"},
		"input_schema": []map[string]any{
			{"name": "id_cliente", "type": "string", "description": "ID do cliente", "required": true},
		},
		"timeout_ms": 3000,
		"active":     true,
	}

	var created store.Tool
	if code := c.do("POST", "/api/tools", toolData, &created); code != http.StatusCreated {
		t.Fatalf("criar tool: %d", code)
	}
	if created.ID == "" || created.Name != "consultar_saldo" {
		t.Fatalf("tool inválida: %+v", created)
	}

	// 2. Testa Tool diretamente via POST /api/tools/{id}/test
	var testRes struct {
		Status     string         `json:"status"`
		Normalized map[string]any `json:"normalized"`
	}
	if code := c.do("POST", "/api/tools/"+created.ID+"/test", map[string]any{"args": map[string]any{"id_cliente": "cli-123"}}, &testRes); code != http.StatusOK {
		t.Fatalf("testar tool: %d", code)
	}
	if testRes.Status != "success" || testRes.Normalized["success"] != true {
		t.Fatalf("resultado inesperado: %+v", testRes)
	}

	// 3. Isolamento: outro usuário não acessa
	other := newClient(t, ts.URL)
	other.do("POST", "/api/auth/login", map[string]string{"email": "other-" + suffix + "@test.local", "password": "senha-other-123"}, nil)
	if code := other.do("GET", "/api/tools/"+created.ID, nil, nil); code != http.StatusNotFound {
		t.Fatalf("isolamento: esperado 404, veio %d", code)
	}

	// 4. Cria Agente associado à Tool e testa execução durante chamada de voz
	var ag store.Agent
	c.do("POST", "/api/agents", map[string]string{"name": "Agente Financeiro"}, &ag)

	// Atualiza com a tool criada
	update := store.AgentUpdate{
		Name: ag.Name, Provider: "fake", Model: "m1", Voice: "v1", Language: "pt-BR", Timezone: "America/Sao_Paulo",
		SystemPrompt: "Você é um atendente.", SystemTools: []string{"encerrar_conversa"},
		Tools: []string{created.ID}, MaxDurationSeconds: 120, Active: true,
	}
	if code := c.do("PUT", "/api/agents/"+ag.ID, update, nil); code != http.StatusOK {
		t.Fatalf("update agent falhou: %d", code)
	}

	// Configura runtime mock para disparar a tool call
	mockP := &toolMockProvider{
		callToolName: "consultar_saldo",
		callArgs:     map[string]any{"id_cliente": "cli-123"},
	}

	rt := &runtime.Runtime{
		Store:      st,
		Voices:     voice.NewRegistry(mockP),
		Sessions:   &auth.Sessions{Redis: getTestRedis(t)},
		HTTPClient: http.DefaultClient,
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ws/conversations/{agent_id}", rt.ServeWS)
	wsSrv := httptest.NewServer(mux)
	defer wsSrv.Close()

	u, _ := st.UserByEmail(context.Background(), "t-"+suffix+"@test.local")
	tok, _ := rt.Sessions.Create(context.Background(), auth.Session{UserID: u.ID, OrganizationID: u.OrganizationID, Email: u.Email})

	wsURL := "ws" + wsSrv.URL[4:] + "/ws/conversations/" + ag.ID + "?version=draft"
	header := http.Header{"Cookie": []string{auth.CookieName + "=" + tok}}
	ws, _, err := websocket.Dial(context.Background(), wsURL, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close(websocket.StatusNormalClosure, "")

	// Lê mensagens até o encerramento
	convID := ""
	toolCalled := false
	for i := 0; i < 5; i++ {
		_, data, err := ws.Read(context.Background())
		if err != nil {
			break
		}
		var msg struct {
			Type           string `json:"type"`
			ConversationID string `json:"conversation_id"`
			Text           string `json:"text"`
		}
		json.Unmarshal(data, &msg)
		if msg.Type == "ready" {
			convID = msg.ConversationID
			// Dispara tool call a partir do provider mock
			mockP.sess.events <- voice.Event{
				Type:     voice.EventToolCall,
				ToolCall: &voice.ToolCall{ID: "call-1", Name: "consultar_saldo", Args: map[string]any{"id_cliente": "cli-123"}},
			}
		}
		if msg.Type == "tool_call" && msg.Text == "consultar_saldo" {
			toolCalled = true
			break
		}
	}

	if !toolCalled {
		t.Fatal("tool_call não recebido no WebSocket")
	}

	// Aguarda processamento e verifica em tool_executions no banco
	time.Sleep(300 * time.Millisecond)
	execs, err := st.ListToolExecutions(context.Background(), u.OrganizationID, convID)
	if err != nil {
		t.Fatal(err)
	}
	if len(execs) == 0 || execs[0].ToolName != "consultar_saldo" || execs[0].Status != "success" {
		t.Fatalf("execuções de tool inválidas no banco: %+v", execs)
	}
	t.Logf("execução gravada com sucesso no banco: status=%s dur=%d ms", execs[0].Status, execs[0].ExecutionTimeMS)
}

type toolMockProvider struct {
	callToolName string
	callArgs     map[string]any
	sess         *toolMockSession
}

func (m *toolMockProvider) Catalog() voice.Catalog {
	return voice.Catalog{
		Provider: "fake", Label: "Fake", MaxSessionSeconds: 540,
		Models: []voice.Option{{ID: "m1", Default: true}}, Voices: []voice.Option{{ID: "v1", Default: true}},
		Languages: []voice.Option{{ID: "pt-BR", Default: true}}, InputSampleRate: 16000, OutputSampleRate: 24000,
	}
}

func (m *toolMockProvider) Connect(ctx context.Context, cfg voice.SessionConfig) (voice.Session, error) {
	m.sess = &toolMockSession{events: make(chan voice.Event, 16)}
	return m.sess, nil
}

type toolMockSession struct {
	events chan voice.Event
	closed bool
}

func (s *toolMockSession) SendAudio([]byte) error                        { return nil }
func (s *toolMockSession) SendText(string) error                         { return nil }
func (s *toolMockSession) SendToolResult(voice.ToolCall, map[string]any) error { return nil }
func (s *toolMockSession) Events() <-chan voice.Event                    { return s.events }
func (s *toolMockSession) Close() error {
	if !s.closed {
		s.closed = true
		close(s.events)
	}
	return nil
}
