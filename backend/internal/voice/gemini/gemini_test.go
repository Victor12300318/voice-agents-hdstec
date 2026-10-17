package gemini

import (
	"context"
	"os"
	"testing"
	"time"

	"voiceagent/internal/voice"
)

// Testes contra a API real. Rodam só com TEST_GEMINI_API_KEY definido.
func connect(t *testing.T, cfg voice.SessionConfig) voice.Session {
	t.Helper()
	key := os.Getenv("TEST_GEMINI_API_KEY")
	if key == "" {
		t.Skip("TEST_GEMINI_API_KEY não definido")
	}
	ctx := context.Background()
	p, err := New(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model == "" {
		cfg.Model = "gemini-3.8-live"
	}
	if cfg.Voice == "" {
		cfg.Voice = "Kore"
	}
	s, err := p.Connect(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// collect lê eventos até TurnComplete (ou timeout), respondendo Tools com onTool.
func collect(t *testing.T, s voice.Session, onTool func(voice.ToolCall) map[string]any) (audioBytes int, transcript string, calls []voice.ToolCall) {
	t.Helper()
	timeout := time.After(45 * time.Second)
	for {
		select {
		case ev, ok := <-s.Events():
			if !ok {
				t.Fatal("canal fechado antes do fim do turno")
			}
			switch ev.Type {
			case voice.EventAudio:
				audioBytes += len(ev.Audio)
			case voice.EventAgentTranscript:
				transcript += ev.Text
			case voice.EventToolCall:
				calls = append(calls, *ev.ToolCall)
				if err := s.SendToolResult(*ev.ToolCall, onTool(*ev.ToolCall)); err != nil {
					t.Fatal(err)
				}
			case voice.EventTurnComplete:
				if audioBytes > 0 {
					return
				}
			case voice.EventClosed:
				t.Fatalf("sessão fechada: %v", ev.Err)
			}
		case <-timeout:
			t.Fatalf("timeout; áudio=%d transcrição=%q", audioBytes, transcript)
		}
	}
}

func TestLiveSpeaksText(t *testing.T) {
	s := connect(t, voice.SessionConfig{SystemPrompt: "Você é um assistente. Responda sempre em português do Brasil, em uma frase curta."})
	if err := s.SendText(`Diga exatamente: "Olá, tudo bem?"`); err != nil {
		t.Fatal(err)
	}
	audio, transcript, _ := collect(t, s, nil)
	t.Logf("áudio=%d bytes transcrição=%q", audio, transcript)
	if audio == 0 {
		t.Fatal("nenhum áudio recebido")
	}
}

func TestLiveCallsTool(t *testing.T) {
	s := connect(t, voice.SessionConfig{
		SystemPrompt: "Você é um atendente. Para consultar boletos, SEMPRE use a ferramenta consultar_boleto. Responda em português do Brasil.",
		Tools: []voice.ToolSpec{{
			Name:        "consultar_boleto",
			Description: "Consulta o boleto em aberto de um cliente pelo CPF.",
			Parameters:  []voice.Param{{Name: "cpf", Type: "string", Description: "CPF do cliente, só números", Required: true}},
		}},
	})
	if err := s.SendText("Meu CPF é 12345678900. Qual o valor do meu boleto em aberto?"); err != nil {
		t.Fatal(err)
	}
	audio, transcript, calls := collect(t, s, func(c voice.ToolCall) map[string]any {
		return map[string]any{"success": true, "data": map[string]any{"valor": "312,50", "vencimento": "10/10/2026"}}
	})
	t.Logf("calls=%+v áudio=%d transcrição=%q", calls, audio, transcript)
	if len(calls) == 0 || calls[0].Name != "consultar_boleto" {
		t.Fatalf("esperava chamada a consultar_boleto, veio %+v", calls)
	}
	if calls[0].Args["cpf"] == nil {
		t.Fatalf("argumento cpf ausente: %+v", calls[0].Args)
	}
}

func TestGeminiCatalogCompliance(t *testing.T) {
	p := &Provider{}
	voice.VerifyCatalogCompliance(t, p.Catalog())
}
