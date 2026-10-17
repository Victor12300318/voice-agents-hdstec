package telephony

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"voiceagent/internal/runtime"
	"voiceagent/internal/telephony/audio"
)

// JambonzWebhookRequest representa a carga enviada pelo Jambonz ao receber uma ligação de entrada (INVITE SIP).
type JambonzWebhookRequest struct {
	CallSid          string `json:"call_sid"`
	CallID           string `json:"call_id"`
	Direction        string `json:"direction"`
	From             string `json:"from"`
	To               string `json:"to"`
	OriginatingSIPIP string `json:"originating_sip_ip"`
	SIPStatus        int    `json:"sip_status"`
}

// JambonzVerb representa uma ação no fluxo do Jambonz (Listen, Say, Hangup, Dial, etc).
type JambonzVerb map[string]any

// BuildListenApp constrói a resposta Jambonz App com o verb 'listen' bidirecional apontando para o nosso WebSocket de áudio.
func BuildListenApp(wsURL string) []JambonzVerb {
	return []JambonzVerb{
		{
			"verb": "listen",
			"url":  wsURL,
			"bidirectionalAudio": map[string]any{
				"enabled":    true,
				"streaming":  true,
				"sampleRate": 8000,
			},
		},
	}
}

// BuildRejectApp constrói uma resposta que avisa o usuário e desliga a chamada (DID não configurado).
func BuildRejectApp(message string) []JambonzVerb {
	return []JambonzVerb{
		{
			"verb": "say",
			"text": message,
		},
		{
			"verb": "hangup",
		},
	}
}

// JambonzTransport implementa runtime.CallTransport conectando a mídia do Jambonz ao Voice Provider.
type JambonzTransport struct {
	conn             *websocket.Conn
	inputSampleRate  int // taxa de entrada esperada pelo Voice Provider (16000 Gemini, 24000 OpenAI)
	outputSampleRate int // taxa de saída gerada pelo Voice Provider (24000 Gemini/OpenAI)
	callSid          string
	accountSid       string
	apiURL           string
	apiKey           string
	outRemainder     []byte
	outFilterHistory []int16
	writeMu          sync.Mutex
}

func NewJambonzTransport(
	conn *websocket.Conn,
	inputSampleRate, outputSampleRate int,
	callSid, accountSid, apiURL, apiKey string,
) *JambonzTransport {
	if inputSampleRate <= 0 {
		inputSampleRate = 16000
	}
	if outputSampleRate <= 0 {
		outputSampleRate = 24000
	}
	return &JambonzTransport{
		conn:             conn,
		inputSampleRate:  inputSampleRate,
		outputSampleRate: outputSampleRate,
		callSid:          callSid,
		accountSid:       accountSid,
		apiURL:           apiURL,
		apiKey:           apiKey,
	}
}

func (t *JambonzTransport) ReadEvent(ctx context.Context) (runtime.TransportEvent, error) {
	msgType, data, err := t.conn.Read(ctx)
	if err != nil {
		return runtime.TransportEvent{}, err
	}

	if msgType == websocket.MessageBinary {
		// Áudio telefônico G.711 μ-law (8kHz) vindo do Jambonz -> converte para PCM16 na taxa de entrada do Provider
		pcm16 := audio.TelephonyToProvider(data, t.inputSampleRate)
		return runtime.TransportEvent{IsAudio: true, Audio: pcm16}, nil
	}

	if msgType == websocket.MessageText {
		var textMsg struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &textMsg); err == nil {
			if textMsg.Type == "hangup" || textMsg.Type == "kill" {
				return runtime.TransportEvent{Hangup: true}, nil
			}
		}
	}

	return runtime.TransportEvent{}, nil
}

func (t *JambonzTransport) SendAudio(ctx context.Context, pcmData []byte) error {
	if len(pcmData) == 0 {
		return nil
	}

	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	alignBytes := 2
	if t.outputSampleRate == 24000 {
		alignBytes = 6
	} else if t.outputSampleRate == 16000 {
		alignBytes = 4
	}

	combined := append(t.outRemainder, pcmData...)
	usableLen := (len(combined) / alignBytes) * alignBytes
	if usableLen == 0 {
		t.outRemainder = combined
		return nil
	}
	toConvert := combined[:usableLen]
	t.outRemainder = append([]byte(nil), combined[usableLen:]...)

	telephonyAudio := audio.ProviderToTelephonyWithHistory(toConvert, &t.outFilterHistory, t.outputSampleRate)
	if len(telephonyAudio) == 0 {
		return nil
	}

	writeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return t.conn.Write(writeCtx, websocket.MessageBinary, telephonyAudio)
}

func (t *JambonzTransport) SendMessage(ctx context.Context, msg runtime.ServerMessage) error {
	// Jambonz opera sobre áudio em binário; eventos de controle podem ser logados ou descartados
	return nil
}

func (t *JambonzTransport) Close(reason string) error {
	return t.conn.Close(websocket.StatusNormalClosure, reason)
}

// OnTransfer executa uma transferência cega (SIP REFER ou redirecionamento de chamada no Jambonz).
func (t *JambonzTransport) OnTransfer(ctx context.Context, targetNumber string) error {
	if t.apiURL == "" || t.callSid == "" || t.accountSid == "" {
		// Sem cliente REST configurado, sinaliza encerramento local
		return nil
	}

	// Chamada da API REST do Jambonz para redirecionar a chamada
	url := fmt.Sprintf("%s/v1/Accounts/%s/Calls/%s", t.apiURL, t.accountSid, t.callSid)
	body, _ := json.Marshal(map[string]any{
		"action": "redirect",
		"data": []map[string]any{
			{
				"verb": "dial",
				"target": []map[string]any{
					{
						"type":   "phone",
						"number": targetNumber,
					},
				},
			},
		},
	})

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if t.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("jambonz transfer error: HTTP %d", resp.StatusCode)
	}
	return nil
}

var _ runtime.CallTransport = (*JambonzTransport)(nil)
