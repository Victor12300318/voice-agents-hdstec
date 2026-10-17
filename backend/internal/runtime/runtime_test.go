package runtime

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/redis/go-redis/v9"

	"voiceagent/internal/auth"
	"voiceagent/internal/store"
	"voiceagent/internal/voice"
)

type mockProvider struct {
	lastCfg voice.SessionConfig
	sess    *mockSession
}

func (m *mockProvider) Catalog() voice.Catalog {
	return voice.Catalog{
		Provider: "mock", Label: "Mock", MaxSessionSeconds: 540,
		Models:           []voice.Option{{ID: "m1", Default: true}},
		Voices:           []voice.Option{{ID: "v1", Default: true}},
		Languages:        []voice.Option{{ID: "pt-BR", Default: true}},
		InputSampleRate:  16000,
		OutputSampleRate: 24000,
	}
}

func (m *mockProvider) Connect(ctx context.Context, cfg voice.SessionConfig) (voice.Session, error) {
	m.lastCfg = cfg
	m.sess = &mockSession{
		events: make(chan voice.Event, 32),
		audioIn: make(chan []byte, 32),
		textIn:  make(chan string, 32),
	}
	return m.sess, nil
}

type mockSession struct {
	events  chan voice.Event
	audioIn chan []byte
	textIn  chan string
	closed  bool
}

func (s *mockSession) SendAudio(pcm []byte) error {
	s.audioIn <- pcm
	return nil
}

func (s *mockSession) SendText(text string) error {
	s.textIn <- text
	return nil
}

func (s *mockSession) SendToolResult(call voice.ToolCall, res map[string]any) error {
	return nil
}

func (s *mockSession) Events() <-chan voice.Event { return s.events }

func (s *mockSession) Close() error {
	if !s.closed {
		s.closed = true
		close(s.events)
	}
	return nil
}

func TestVoiceRuntimeSession(t *testing.T) {
	dbURL, redisURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_REDIS_URL")
	if dbURL == "" || redisURL == "" {
		t.Skip("TEST_DATABASE_URL/TEST_REDIS_URL não definidos")
	}

	ctx := context.Background()
	st, err := store.Open(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	opt, _ := redis.ParseURL(redisURL)
	rdb := redis.NewClient(opt)
	defer rdb.Close()

	sessions := &auth.Sessions{Redis: rdb}
	mockP := &mockProvider{}
	voices := voice.NewRegistry(mockP)

	rt := &Runtime{
		Store:      st,
		Voices:     voices,
		Sessions:   sessions,
		HTTPClient: http.DefaultClient,
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ws/conversations/{agent_id}", rt.ServeWS)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Cria Org, User e Agent
	hash, _ := auth.HashPassword("senha12345")
	tok, _ := auth.NewToken()
	org, u, err := st.CreateOrgWithAdmin(ctx, "Org Test", "rt-"+tok[:8]+"@test.local", hash)
	if err != nil {
		t.Fatal(err)
	}

	sessToken, err := sessions.Create(ctx, auth.Session{UserID: u.ID, OrganizationID: org.ID, Email: u.Email})
	if err != nil {
		t.Fatal(err)
	}

	agent, err := st.CreateAgent(ctx, org.ID, "Agente Voz", "Desc", "mock", "m1", "v1")
	if err != nil {
		t.Fatal(err)
	}

	// Configura SystemPrompt e FirstMessage no Draft
	_, err = st.UpdateAgent(ctx, org.ID, agent.ID, store.AgentUpdate{
		Name: agent.Name, Provider: "mock", Model: "m1", Voice: "v1", Language: "pt-BR", Timezone: "America/Sao_Paulo",
		SystemPrompt: "Você é um assistente.",
		FirstMessage: "Olá!", FirstMessageInterruptible: true,
		MaxDurationSeconds: 120, RecordingEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/conversations/" + agent.ID + "?version=draft"

	header := http.Header{}
	header.Set("Cookie", auth.CookieName+"="+sessToken)

	ws, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer ws.Close(websocket.StatusNormalClosure, "")

	// 1. Deve receber mensagem "ready"
	_, data, err := ws.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var ready serverMessage
	if err := json.Unmarshal(data, &ready); err != nil || ready.Type != "ready" {
		t.Fatalf("esperava ready, veio %s (%v)", string(data), err)
	}
	if ready.ConversationID == "" {
		t.Fatal("conversation_id ausente no ready")
	}

	// 2. MockProvider deve ter recebido a primeira mensagem
	select {
	case txt := <-mockP.sess.textIn:
		if !strings.Contains(txt, "Olá!") {
			t.Fatalf("first message inesperada: %s", txt)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout aguardando first message")
	}

	// 3. Envio de áudio do cliente para o backend
	testAudio := []byte{0x01, 0x02, 0x03, 0x04}
	if err := ws.Write(ctx, websocket.MessageBinary, testAudio); err != nil {
		t.Fatal(err)
	}

	select {
	case gotAudio := <-mockP.sess.audioIn:
		if len(gotAudio) != len(testAudio) {
			t.Fatalf("tamanho de áudio diferente: %d", len(gotAudio))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout aguardando áudio no provider")
	}

	// 4. Provider emite áudio e transcrição -> chega no WebSocket
	mockP.sess.events <- voice.Event{Type: voice.EventAudio, Audio: []byte{0xAA, 0xBB}}
	mockP.sess.events <- voice.Event{Type: voice.EventAgentTranscript, Text: "Tudo bem?"}

	// Lê áudio
	msgType, bin, err := ws.Read(ctx)
	if err != nil || msgType != websocket.MessageBinary || len(bin) != 2 {
		t.Fatalf("esperava áudio binário, veio type=%v len=%d err=%v", msgType, len(bin), err)
	}

	// Lê transcrição
	msgType, txtMsg, err := ws.Read(ctx)
	if err != nil || msgType != websocket.MessageText {
		t.Fatalf("esperava transcrição texto, veio type=%v err=%v", msgType, err)
	}
	var transcript serverMessage
	_ = json.Unmarshal(txtMsg, &transcript)
	if transcript.Type != "agent_transcript" || transcript.Text != "Tudo bem?" {
		t.Fatalf("transcrição inesperada: %s", string(txtMsg))
	}

	// 5. Encerra chamada via hangup
	hangup, _ := json.Marshal(clientMessage{Type: "hangup"})
	_ = ws.Write(ctx, websocket.MessageText, hangup)

	// Aguarda fechamento e verifica no banco de dados
	time.Sleep(200 * time.Millisecond)

	conv, events, err := st.GetConversation(ctx, org.ID, ready.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	if conv.Status != "completed" || conv.EndedReason != "caller_hangup" {
		t.Fatalf("status final no banco inesperado: status=%s reason=%s", conv.Status, conv.EndedReason)
	}
	if len(events) == 0 {
		t.Fatal("nenhum evento registrado no banco")
	}
	t.Logf("conversa finalizada com sucesso! eventos=%d duração=%d ms", len(events), *conv.DurationMs)
}
