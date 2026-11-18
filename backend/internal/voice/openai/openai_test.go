package openai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"voiceagent/internal/voice"
)

func TestOpenAICatalogCompliance(t *testing.T) {
	p := New("")
	voice.VerifyCatalogCompliance(t, p.Catalog())
}

func TestOpenAIConnectRequiresKey(t *testing.T) {
	p := New("")
	_, err := p.Connect(context.Background(), voice.SessionConfig{Model: "gpt-4o-realtime-preview"})
	if err == nil {
		t.Fatal("esperava erro sem chave de API")
	}
}

func TestOpenAIDispatchEvents(t *testing.T) {
	sess := &openAISession{events: make(chan voice.Event, 16)}

	// 1. Áudio delta (base64)
	fakePCM := []byte{0x01, 0x02, 0x03, 0x04}
	b64 := base64.StdEncoding.EncodeToString(fakePCM)
	audioMsg, _ := json.Marshal(map[string]string{
		"type":  "response.audio.delta",
		"delta": b64,
	})
	sess.dispatch(audioMsg)

	select {
	case ev := <-sess.events:
		if ev.Type != voice.EventAudio || len(ev.Audio) != 4 {
			t.Fatalf("evento de áudio incorreto: %+v", ev)
		}
	default:
		t.Fatal("evento de áudio não recebido")
	}

	// 2. Interrupção por detecção de fala
	speechStarted, _ := json.Marshal(map[string]string{"type": "input_audio_buffer.speech_started"})
	sess.dispatch(speechStarted)

	select {
	case ev := <-sess.events:
		if ev.Type != voice.EventInterrupted {
			t.Fatalf("esperava interrupção, veio %+v", ev)
		}
	default:
		t.Fatal("evento de interrupção não recebido")
	}

	// 3. Chamada de função
	funcCall, _ := json.Marshal(map[string]string{
		"type":      "response.function_call_arguments.done",
		"call_id":   "call_openai_123",
		"name":      "consultar_saldo",
		"arguments": `{"cpf":"12345678900"}`,
	})
	sess.dispatch(funcCall)

	select {
	case ev := <-sess.events:
		if ev.Type != voice.EventToolCall || ev.ToolCall.Name != "consultar_saldo" {
			t.Fatalf("evento de tool call incorreto: %+v", ev)
		}
		if ev.ToolCall.Args["cpf"] != "12345678900" {
			t.Fatalf("argumentos incorretos: %+v", ev.ToolCall.Args)
		}
	default:
		t.Fatal("evento de tool call não recebido")
	}
}

func TestOpenAIDelegationModelsCatalog(t *testing.T) {
	p := New("")
	cat := p.Catalog()
	if len(cat.DelegationModels) == 0 {
		t.Fatal("esperava DelegationModels preenchido no catálogo OpenAI")
	}
	foundDefault := false
	for _, dm := range cat.DelegationModels {
		if dm.Default {
			foundDefault = true
		}
	}
	if !foundDefault {
		t.Fatal("esperava um modelo de delegação padrão no catálogo OpenAI")
	}
}

func TestOpenAILiveDispatchMarksFirstOutputAudio(t *testing.T) {
	sess := &openAILiveSession{
		events:             make(chan voice.Event, 16),
		firstMessagePrimed: true,
	}
	// Silêncio puro antes da fala não deve marcar firstOutputAudioReceived
	silentMsg, _ := json.Marshal(map[string]string{
		"type":  "session.output_audio.delta",
		"delta": base64.StdEncoding.EncodeToString(make([]byte, 320)),
	})
	sess.dispatch(silentMsg)
	if sess.firstOutputAudioReceived.Load() {
		t.Fatal("silêncio puro não deveria marcar firstOutputAudioReceived")
	}

	// Áudio audível deve marcar firstOutputAudioReceived e emitir EventAudio
	audiblePCM := []byte{0x00, 0x02, 0x00, 0x02}
	audibleMsg, _ := json.Marshal(map[string]string{
		"type":  "session.output_audio.delta",
		"delta": base64.StdEncoding.EncodeToString(audiblePCM),
	})
	sess.dispatch(audibleMsg)
	if !sess.firstOutputAudioReceived.Load() {
		t.Fatal("esperava firstOutputAudioReceived=true após áudio audível")
	}
	if sess.turnTimer != nil {
		sess.turnTimer.Stop()
	}
}
