package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/coder/websocket"
)

// TransportEvent representa uma mensagem ou pacote de áudio vindo do Caller.
type TransportEvent struct {
	IsAudio bool
	Audio   []byte
	Hangup  bool
}

// CallTransport abstrai o canal de comunicação com o Caller (navegador WebSockets, SIP/Jambonz, etc).
type CallTransport interface {
	// ReadEvent lê o próximo pacote do Caller (áudio ou sinal de hangup).
	ReadEvent(ctx context.Context) (TransportEvent, error)
	// SendAudio envia áudio PCM16 do Agent para o Caller.
	SendAudio(ctx context.Context, data []byte) error
	// SendMessage envia uma mensagem de controle/evento estruturado para o Caller.
	SendMessage(ctx context.Context, msg ServerMessage) error
	// Close encerra a conexão de transporte.
	Close(reason string) error
	// OnTransfer avisa o transporte para transferir a chamada para um número alvo (ex: SIP REFER).
	OnTransfer(ctx context.Context, targetNumber string) error
}

// WebSocketTransport implementa CallTransport para conexões WebSocket do navegador.
type WebSocketTransport struct {
	conn *websocket.Conn
}

func NewWebSocketTransport(conn *websocket.Conn) *WebSocketTransport {
	return &WebSocketTransport{conn: conn}
}

func (t *WebSocketTransport) ReadEvent(ctx context.Context) (TransportEvent, error) {
	msgType, data, err := t.conn.Read(ctx)
	if err != nil {
		return TransportEvent{}, err
	}
	if msgType == websocket.MessageBinary {
		return TransportEvent{IsAudio: true, Audio: data}, nil
	}
	if msgType == websocket.MessageText {
		var clientMsg clientMessage
		if err := json.Unmarshal(data, &clientMsg); err == nil && clientMsg.Type == "hangup" {
			return TransportEvent{Hangup: true}, nil
		}
	}
	return TransportEvent{}, nil
}

func (t *WebSocketTransport) SendAudio(ctx context.Context, data []byte) error {
	writeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return t.conn.Write(writeCtx, websocket.MessageBinary, data)
}

func (t *WebSocketTransport) SendMessage(ctx context.Context, msg serverMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return t.conn.Write(writeCtx, websocket.MessageText, data)
}

func (t *WebSocketTransport) Close(reason string) error {
	return t.conn.Close(websocket.StatusNormalClosure, reason)
}

func (t *WebSocketTransport) OnTransfer(ctx context.Context, targetNumber string) error {
	// No navegador web não há SIP REFER, mas enviamos mensagem informativa
	return t.SendMessage(ctx, serverMessage{
		Type:    "transfer_requested",
		Message: "Transferência para " + targetNumber,
	})
}

// SessionOptions contém os parâmetros para inicializar uma sessão de voz com qualquer transporte.
type SessionOptions struct {
	OrgID      string
	AgentID    string
	VersionID  *string
	IsTest     bool
	Caller     string
	Direction  string // "web" | "inbound" | "outbound"
	FromNumber string
	ToNumber   string
	CallSid    string
	Transport  CallTransport
}

var ErrTransferNotSupported = errors.New("transfer not supported by transport")
