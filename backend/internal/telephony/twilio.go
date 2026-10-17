package telephony

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"voiceagent/internal/runtime"
	"voiceagent/internal/telephony/audio"
)

// Estruturas para geração segura de TwiML (XML) da Twilio.
type twimlResponse struct {
	XMLName xml.Name      `xml:"Response"`
	Say     *twimlSay     `xml:"Say,omitempty"`
	Connect *twimlConnect `xml:"Connect,omitempty"`
	Hangup  *struct{}     `xml:"Hangup,omitempty"`
}

type twimlSay struct {
	Language string `xml:"language,attr,omitempty"`
	Text     string `xml:",chardata"`
}

type twimlConnect struct {
	Stream twimlStream `xml:"Stream"`
}

type twimlStream struct {
	URL        string           `xml:"url,attr"`
	Parameters []twimlParameter `xml:"Parameter,omitempty"`
}

type twimlParameter struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

// BuildTwilioStreamTwiML gera o XML TwiML instruindo a Twilio a abrir um Media Stream WebSocket bidirecional.
func BuildTwilioStreamTwiML(wsURL string, params map[string]string) string {
	var list []twimlParameter
	for k, v := range params {
		list = append(list, twimlParameter{Name: k, Value: v})
	}
	resp := twimlResponse{
		Connect: &twimlConnect{
			Stream: twimlStream{
				URL:        wsURL,
				Parameters: list,
			},
		},
	}
	out, _ := xml.MarshalIndent(resp, "", "  ")
	return xml.Header + string(out)
}

// BuildTwilioRejectTwiML gera o XML TwiML que fala uma mensagem em português e encerra a ligação.
func BuildTwilioRejectTwiML(message string) string {
	resp := twimlResponse{
		Say: &twimlSay{
			Language: "pt-BR",
			Text:     message,
		},
		Hangup: &struct{}{},
	}
	out, _ := xml.MarshalIndent(resp, "", "  ")
	return xml.Header + string(out)
}

// Mensagens JSON trocadas no WebSocket do Twilio Media Streams.
type twilioWSMessage struct {
	Event     string            `json:"event"`
	StreamSid string            `json:"streamSid,omitempty"`
	Start     *twilioWSStart    `json:"start,omitempty"`
	Media     *twilioWSMedia    `json:"media,omitempty"`
}

type twilioWSStart struct {
	StreamSid        string            `json:"streamSid"`
	AccountSid       string            `json:"accountSid"`
	CallSid          string            `json:"callSid"`
	CustomParameters map[string]string `json:"customParameters"`
}

type twilioWSMedia struct {
	Track     string `json:"track,omitempty"`
	Chunk     string `json:"chunk,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
	Payload   string `json:"payload"`
}

// TwilioTransport implementa runtime.CallTransport para o protocolo Twilio Media Streams.
type TwilioTransport struct {
	conn             *websocket.Conn
	inputSampleRate  int // taxa de entrada esperada pelo Voice Provider (ex: 16000 Hz no Gemini)
	outputSampleRate int // taxa de saída gerada pelo Voice Provider (ex: 24000 Hz no Gemini/OpenAI)
	streamSid        string
	callSid          string
	accountSid       string
	authToken        string
	outRemainder     []byte
	outFilterHistory []int16
	mu               sync.RWMutex
	writeMu          sync.Mutex
}

func NewTwilioTransport(
	conn *websocket.Conn,
	inputSampleRate, outputSampleRate int,
	streamSid, callSid, accountSid, authToken string,
) *TwilioTransport {
	if inputSampleRate <= 0 {
		inputSampleRate = 16000
	}
	if outputSampleRate <= 0 {
		outputSampleRate = 24000
	}
	return &TwilioTransport{
		conn:             conn,
		inputSampleRate:  inputSampleRate,
		outputSampleRate: outputSampleRate,
		streamSid:        streamSid,
		callSid:          callSid,
		accountSid:       accountSid,
		authToken:        authToken,
	}
}

// WaitForStart lê os eventos iniciais ("connected" e "start") da Twilio caso os parâmetros venham via <Parameter>.
func ReadTwilioHandshake(ctx context.Context, conn *websocket.Conn) (streamSid, callSid string, customParams map[string]string, err error) {
	handshakeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	for i := 0; i < 5; i++ {
		msgType, data, err := conn.Read(handshakeCtx)
		if err != nil {
			return "", "", nil, err
		}
		if msgType != websocket.MessageText {
			continue
		}
		var msg twilioWSMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		if msg.Event == "start" && msg.Start != nil {
			return msg.Start.StreamSid, msg.Start.CallSid, msg.Start.CustomParameters, nil
		}
	}
	return "", "", nil, fmt.Errorf("evento start da Twilio não recebido no handshake")
}

func (t *TwilioTransport) ReadEvent(ctx context.Context) (runtime.TransportEvent, error) {
	for {
		msgType, data, err := t.conn.Read(ctx)
		if err != nil {
			return runtime.TransportEvent{}, err
		}
		if msgType != websocket.MessageText {
			continue
		}

		var msg twilioWSMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		switch msg.Event {
		case "connected":
			continue

		case "start":
			if msg.Start != nil {
				t.mu.Lock()
				if msg.Start.StreamSid != "" {
					t.streamSid = msg.Start.StreamSid
				}
				if msg.Start.CallSid != "" {
					t.callSid = msg.Start.CallSid
				}
				t.mu.Unlock()
			}
			continue

		case "media":
			if msg.Media == nil || msg.Media.Payload == "" {
				continue
			}
			if msg.StreamSid != "" {
				t.mu.Lock()
				if t.streamSid == "" {
					t.streamSid = msg.StreamSid
				}
				t.mu.Unlock()
			}
			mulawBytes, err := base64.StdEncoding.DecodeString(msg.Media.Payload)
			if err != nil || len(mulawBytes) == 0 {
				continue
			}
			pcm16 := audio.TelephonyToProvider(mulawBytes, t.inputSampleRate)
			return runtime.TransportEvent{IsAudio: true, Audio: pcm16}, nil

		case "stop", "closed":
			return runtime.TransportEvent{Hangup: true}, nil
		}
	}
}

func (t *TwilioTransport) SendAudio(ctx context.Context, pcmData []byte) error {
	if len(pcmData) == 0 {
		return nil
	}

	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	// Alinha o buffer de entrada ao fator de decimação (ex: 6 bytes para 24kHz->8kHz, 4 bytes para 16kHz->8kHz)
	// para nunca perder amostras nas fronteiras entre chunks do WebSocket.
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

	t.mu.RLock()
	streamSid := t.streamSid
	t.mu.RUnlock()

	payload := base64.StdEncoding.EncodeToString(telephonyAudio)
	frame, err := json.Marshal(twilioWSMessage{
		Event:     "media",
		StreamSid: streamSid,
		Media: &twilioWSMedia{
			Payload: payload,
		},
	})
	if err != nil {
		return err
	}

	writeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return t.conn.Write(writeCtx, websocket.MessageText, frame)
}

func (t *TwilioTransport) SendMessage(ctx context.Context, msg runtime.ServerMessage) error {
	// Em caso de Interruption (barge-in do Caller), envia o comando "clear" para a Twilio
	// limpar imediatamente o buffer de saída de áudio.
	if msg.Type == "interrupted" {
		t.mu.RLock()
		streamSid := t.streamSid
		t.mu.RUnlock()
		if streamSid == "" {
			return nil
		}

		frame, _ := json.Marshal(map[string]string{
			"event":     "clear",
			"streamSid": streamSid,
		})

		t.writeMu.Lock()
		t.outRemainder = nil
		t.outFilterHistory = nil
		defer t.writeMu.Unlock()

		writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		return t.conn.Write(writeCtx, websocket.MessageText, frame)
	}
	return nil
}

func (t *TwilioTransport) Close(reason string) error {
	t.mu.RLock()
	callSid := t.callSid
	accountSid := t.accountSid
	authToken := t.authToken
	t.mu.RUnlock()

	if callSid != "" && accountSid != "" && authToken != "" {
		go func(acc, tok, sid string) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			apiURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Calls/%s.json",
				url.PathEscape(acc),
				url.PathEscape(sid),
			)
			form := url.Values{}
			form.Set("Status", "completed")
			req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(form.Encode()))
			if err != nil {
				return
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.SetBasicAuth(acc, tok)
			resp, err := http.DefaultClient.Do(req)
			if err == nil && resp != nil {
				resp.Body.Close()
			}
		}(accountSid, authToken, callSid)
	}

	return t.conn.Close(websocket.StatusNormalClosure, reason)
}

// OnTransfer executa transferência de chamada via API REST da Twilio atualizando a Call com TwiML <Dial>.
func (t *TwilioTransport) OnTransfer(ctx context.Context, targetNumber string) error {
	t.mu.RLock()
	callSid := t.callSid
	accountSid := t.accountSid
	authToken := t.authToken
	t.mu.RUnlock()

	if callSid == "" || accountSid == "" || authToken == "" {
		return nil
	}

	apiURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Calls/%s.json",
		url.PathEscape(accountSid),
		url.PathEscape(callSid),
	)

	twiml := fmt.Sprintf("<Response><Dial>%s</Dial></Response>", targetNumber)
	form := url.Values{}
	form.Set("Twiml", twiml)

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(accountSid, authToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("twilio transfer error HTTP %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// TwilioIncomingNumber representa um número telefônico retornado pela API REST da conta Twilio.
type TwilioIncomingNumber struct {
	Sid          string `json:"sid"`
	PhoneNumber  string `json:"phone_number"`
	FriendlyName string `json:"friendly_name"`
	VoiceURL     string `json:"voice_url"`
}

// ListTwilioIncomingNumbers consulta os números ativos na conta da Twilio usando Account SID e Auth Token.
func ListTwilioIncomingNumbers(ctx context.Context, client *http.Client, accountSid, authToken string) ([]TwilioIncomingNumber, error) {
	if client == nil {
		client = http.DefaultClient
	}
	apiURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/IncomingPhoneNumbers.json?PageSize=50",
		url.PathEscape(strings.TrimSpace(accountSid)),
	)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(strings.TrimSpace(accountSid), strings.TrimSpace(authToken))

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro ao conectar na API da Twilio: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("credenciais da Twilio inválidas (Account SID ou Auth Token incorretos)")
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("API da Twilio retornou HTTP %d: %s", resp.StatusCode, string(b))
	}

	var payload struct {
		IncomingPhoneNumbers []TwilioIncomingNumber `json:"incoming_phone_numbers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("resposta inválida da Twilio: %w", err)
	}
	return payload.IncomingPhoneNumbers, nil
}

// ConfigureTwilioNumberWebhook atualiza automaticamente o VoiceUrl de um número na Twilio para apontar para a plataforma.
func ConfigureTwilioNumberWebhook(ctx context.Context, client *http.Client, accountSid, authToken, phoneSid, voiceWebhookURL string) error {
	if client == nil {
		client = http.DefaultClient
	}
	apiURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/IncomingPhoneNumbers/%s.json",
		url.PathEscape(strings.TrimSpace(accountSid)),
		url.PathEscape(strings.TrimSpace(phoneSid)),
	)

	form := url.Values{}
	form.Set("VoiceUrl", strings.TrimSpace(voiceWebhookURL))
	form.Set("VoiceMethod", "POST")

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(strings.TrimSpace(accountSid), strings.TrimSpace(authToken))

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("erro ao configurar webhook na Twilio: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("falha ao atualizar VoiceUrl na Twilio (HTTP %d): %s", resp.StatusCode, string(b))
	}
	return nil
}

var _ runtime.CallTransport = (*TwilioTransport)(nil)
