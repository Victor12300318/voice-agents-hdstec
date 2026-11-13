package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"voiceagent/internal/store"
)

func TestMCPStreamableHTTPDiscoverAndCall(t *testing.T) {
	var initializedNotif atomic.Bool
	var sessionSeq atomic.Int32
	var currentSID string
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-123" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("unauthorized"))
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params map[string]any  `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		switch req.Method {
		case "initialize":
			n := sessionSeq.Add(1)
			mu.Lock()
			currentSID = fmt.Sprintf("sid-%d", n)
			sid := currentSID
			mu.Unlock()
			w.Header().Set("Mcp-Session-Id", sid)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2025-03-26","capabilities":{"tools":{}}}}`, string(req.ID))

		case "notifications/initialized":
			initializedNotif.Store(true)
			w.WriteHeader(http.StatusAccepted)

		case "tools/list":
			if !initializedNotif.Load() {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"error":{"code":-32000,"message":"Server not initialized"}}`, string(req.ID))
				return
			}
			mu.Lock()
			sid := currentSID
			mu.Unlock()
			if r.Header.Get("Mcp-Session-Id") != sid {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprintf(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{\"tools\":[{\"name\":\"panorama_disparos\",\"description\":\"Retorna disparos de hoje\",\"inputSchema\":{\"type\":\"object\",\"properties\":{\"projeto\":{\"type\":\"string\"}},\"required\":[\"projeto\"]}}]}}\n\n", string(req.ID))

		case "tools/call":
			mu.Lock()
			sid := currentSID
			mu.Unlock()
			if r.Header.Get("Mcp-Session-Id") != sid {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			name, _ := req.Params["name"].(string)
			args, _ := req.Params["arguments"].(map[string]any)
			if name != "panorama_disparos" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"error":{"code":-32601,"message":"tool not found"}}`, string(req.ID))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"content":[{"type":"text","text":"{\"ativos\":42,\"projeto\":\"%v\"}"}]}}`, string(req.ID), args["projeto"])
		}
	}))
	defer srv.Close()

	// Simula o bug clássico: o usuário cadastrou com Transport="SSE" (default do UI) em uma URL Streamable HTTP!
	tool := store.Tool{
		Name: "MCPNac_Ativo",
		Kind: "mcp",
		Config: store.WebhookConfig{
			URL:       srv.URL + "/mcp/nac-ativo",
			Transport: "SSE",
		},
		SecretHeaders: map[string]string{
			"Authorization": "Bearer tok-123",
		},
		TimeoutMS: 5000,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sess, list, err := ConnectMCPSession(ctx, http.DefaultClient, tool, nil)
	if err != nil {
		t.Fatalf("ConnectMCPSession falhou: %v", err)
	}
	defer sess.Close()

	if !initializedNotif.Load() {
		t.Fatal("notifications/initialized não foi enviado após initialize")
	}
	if sess.TransportUsed() != MCPTransportStreamableHTTP {
		t.Fatalf("esperava STREAMABLE_HTTP, usou %s", sess.TransportUsed())
	}
	if len(list) != 1 || list[0].Name != "panorama_disparos" {
		t.Fatalf("lista inesperada: %+v", list)
	}

	res := sess.CallTool(ctx, "panorama_disparos", map[string]any{"projeto": "NAC Ativo"})
	if res.Status != "success" {
		t.Fatalf("CallTool falhou: %s (%s)", res.Status, res.ErrorMessage)
	}
	data, ok := res.RawResponse.(map[string]any)
	if !ok || data["ativos"] != float64(42) {
		t.Fatalf("resposta inesperada: %#v", res.RawResponse)
	}
}

func TestMCPLegacySSETransport(t *testing.T) {
	var msgChMu sync.Mutex
	msgChans := map[string]chan string{}

	mux := http.NewServeMux()
	mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		sid := "sse-1"
		ch := make(chan string, 16)
		msgChMu.Lock()
		msgChans[sid] = ch
		msgChMu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, _ := w.(http.Flusher)
		_, _ = fmt.Fprintf(w, "event: endpoint\ndata: /messages?sessionId=%s\n\n", sid)
		if flusher != nil {
			flusher.Flush()
		}

		for {
			select {
			case <-r.Context().Done():
				return
			case payload := <-ch:
				_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", payload)
				if flusher != nil {
					flusher.Flush()
				}
			}
		}
	})

	mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		sid := r.URL.Query().Get("sessionId")
		msgChMu.Lock()
		ch := msgChans[sid]
		msgChMu.Unlock()
		if ch == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params map[string]any  `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		switch req.Method {
		case "initialize":
			ch <- fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2024-11-05","capabilities":{"tools":{}}}}`, string(req.ID))
		case "notifications/initialized":
		case "tools/list":
			ch <- fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{"tools":[{"name":"consultar_corretores","description":"Lista corretores","inputSchema":{"type":"object","properties":{}}}]}}`, string(req.ID))
		case "tools/call":
			ch <- fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{"content":[{"type":"text","text":"{\"total\":7}"}]}}`, string(req.ID))
		}
		w.WriteHeader(http.StatusAccepted)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Simula cliente HTTP com Timeout curto (como main.go:118) para garantir que não mata a stream SSE
	shortClient := &http.Client{Timeout: 500 * time.Millisecond}

	tool := store.Tool{
		Name: "MCPNac_SSE",
		Kind: "mcp",
		Config: store.WebhookConfig{
			URL:       srv.URL + "/sse",
			Transport: "STREAMABLE_HTTP", // mesmo se selecionado errado, auto-fallback para SSE
		},
		TimeoutMS: 5000,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sess, list, err := ConnectMCPSession(ctx, shortClient, tool, nil)
	if err != nil {
		t.Fatalf("ConnectMCPSession SSE falhou: %v", err)
	}
	defer sess.Close()

	if sess.TransportUsed() != MCPTransportSSE {
		t.Fatalf("esperava SSE, usou %s", sess.TransportUsed())
	}
	if len(list) != 1 || list[0].Name != "consultar_corretores" {
		t.Fatalf("tools inesperadas: %+v", list)
	}

	res := sess.CallTool(ctx, "consultar_corretores", nil)
	if res.Status != "success" {
		t.Fatalf("CallTool SSE falhou: %s (%s)", res.Status, res.ErrorMessage)
	}
	data, ok := res.RawResponse.(map[string]any)
	if !ok || data["total"] != float64(7) {
		t.Fatalf("resposta SSE inesperada: %#v", res.RawResponse)
	}
}
