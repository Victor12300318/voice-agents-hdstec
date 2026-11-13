package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"voiceagent/internal/store"
)

const (
	MCPTransportSSE            = "SSE"
	MCPTransportStreamableHTTP = "STREAMABLE_HTTP"
)

type MCPToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// MCPSession mantém uma conexão MCP ativa (Streamable HTTP ou Legacy HTTP+SSE)
// durante uma sessão de conversa ou chamada pontual.
type MCPSession struct {
	tool       store.Tool
	vars       map[string]string
	httpClient *http.Client
	targetURL  string

	transport       string
	sessionID       string
	protocolVersion string
	postURL         string

	sseCancel  context.CancelFunc
	sseDone    chan struct{}
	endpointCh chan string
	sseErrMu   sync.Mutex
	sseErr     error

	reqID       atomic.Int64
	connEpoch   atomic.Int64
	pendingMu   sync.Mutex
	pending     map[string]chan rpcMessage
	reconnectMu sync.Mutex
	closed      atomic.Bool
}

func streamSafeHTTPClient(base *http.Client) *http.Client {
	if base == nil {
		return &http.Client{}
	}
	return &http.Client{
		Transport:     base.Transport,
		CheckRedirect: base.CheckRedirect,
		Jar:           base.Jar,
		Timeout:       0,
	}
}

func resolveMCPTargetURL(tool store.Tool, vars map[string]string) string {
	targetURL := interpolateVars(tool.Config.URL, vars)
	if tool.Config.APISchema != nil && strings.TrimSpace(tool.Config.APISchema.URL) != "" {
		targetURL = interpolateVars(tool.Config.APISchema.URL, vars)
	}
	return strings.TrimSpace(targetURL)
}

func (m *MCPSession) applyHeaders(req *http.Request, accept string) {
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if m.tool.Config.APISchema != nil {
		for _, h := range m.tool.Config.APISchema.RequestHeaders {
			name := strings.TrimSpace(h.Name)
			if name == "" {
				continue
			}
			val := h.Value
			if h.Type == "dynamic_variable" {
				if dv, ok := m.vars[h.Value]; ok {
					val = dv
				}
			} else {
				val = interpolateVars(val, m.vars)
			}
			req.Header.Set(name, val)
		}
	}
	for k, v := range m.tool.SecretHeaders {
		if strings.TrimSpace(k) != "" {
			req.Header.Set(k, interpolateVars(v, m.vars))
		}
	}
	if m.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", m.sessionID)
	}
	if m.protocolVersion != "" {
		req.Header.Set("MCP-Protocol-Version", m.protocolVersion)
	}
}

// ConnectMCPSession estabelece a sessão MCP (auto-detectando Streamable HTTP vs Legacy SSE)
// e lista todas as ferramentas expostas pelo servidor via tools/list.
func ConnectMCPSession(ctx context.Context, httpClient *http.Client, tool store.Tool, vars map[string]string) (*MCPSession, []MCPToolDef, error) {
	targetURL := resolveMCPTargetURL(tool, vars)
	if targetURL == "" {
		return nil, nil, errors.New("URL do servidor MCP não configurada")
	}

	sess := &MCPSession{
		tool:       tool,
		vars:       vars,
		httpClient: streamSafeHTTPClient(httpClient),
		targetURL:  targetURL,
		pending:    make(map[string]chan rpcMessage),
	}

	if err := sess.connect(ctx); err != nil {
		sess.Close()
		return nil, nil, err
	}

	toolsList, err := sess.ListTools(ctx)
	if err != nil {
		sess.Close()
		return nil, nil, err
	}

	return sess, toolsList, nil
}

func (m *MCPSession) TransportUsed() string {
	return m.transport
}

func (m *MCPSession) Close() {
	m.closed.Store(true)
	m.stopSSE()
}

func (m *MCPSession) stopSSE() {
	if m.sseCancel != nil {
		m.sseCancel()
		m.sseCancel = nil
	}
}

func (m *MCPSession) connect(ctx context.Context) error {
	m.stopSSE()
	m.sessionID = ""
	m.protocolVersion = ""
	m.postURL = ""

	parsed, err := url.Parse(m.targetURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("URL MCP inválida: %q", m.targetURL)
	}

	pathLower := strings.ToLower(strings.TrimSuffix(parsed.Path, "/"))
	explicitTransport := strings.ToUpper(strings.TrimSpace(m.tool.Config.Transport))

	var order []string
	if strings.HasSuffix(pathLower, "/sse") {
		order = []string{MCPTransportSSE, MCPTransportStreamableHTTP}
	} else if explicitTransport == MCPTransportStreamableHTTP {
		order = []string{MCPTransportStreamableHTTP, MCPTransportSSE}
	} else {
		// Quando Transport é "SSE" (padrão da UI) sem sufixo /sse, testamos Streamable HTTP primeiro (POST 1 RTT)
		// para evitar que um endpoint Streamable HTTP mantenha um GET SSE ocioso aberto sem enviar "event: endpoint".
		order = []string{MCPTransportStreamableHTTP, MCPTransportSSE}
	}

	var errs []string
	for _, tr := range order {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var err error
		if tr == MCPTransportStreamableHTTP {
			err = m.connectStreamableHTTP(ctx)
		} else {
			err = m.connectSSE(ctx)
		}
		if err == nil {
			m.transport = tr
			m.connEpoch.Add(1)
			return nil
		}
		errs = append(errs, fmt.Sprintf("%s: %v", tr, err))
		if isAuthHTTPError(err) {
			return err
		}
	}
	return errors.New(strings.Join(errs, " | "))
}

type httpStatusError struct {
	StatusCode int
	Body       string
}

func (e *httpStatusError) Error() string {
	b := strings.TrimSpace(e.Body)
	if len(b) > 300 {
		b = b[:300] + "..."
	}
	if b == "" {
		return fmt.Sprintf("HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, b)
}

func isAuthHTTPError(err error) bool {
	var he *httpStatusError
	if errors.As(err, &he) {
		return he.StatusCode == http.StatusUnauthorized || he.StatusCode == http.StatusForbidden
	}
	return false
}

func (m *MCPSession) connectStreamableHTTP(ctx context.Context) error {
	versions := []string{"2025-03-26", "2024-11-05"}
	var lastErr error
	for _, protoVer := range versions {
		id := m.reqID.Add(1)
		initPayload := map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"method":  "initialize",
			"params": map[string]any{
				"protocolVersion": protoVer,
				"capabilities":    map[string]any{},
				"clientInfo": map[string]any{
					"name":    "voiceagent",
					"version": "1.0.0",
				},
			},
		}

		msg, respHeaders, err := m.postStreamable(ctx, m.targetURL, initPayload, id)
		if err != nil {
			lastErr = err
			var he *httpStatusError
			if errors.As(err, &he) && (he.StatusCode == http.StatusMethodNotAllowed || he.StatusCode == http.StatusNotFound || he.StatusCode == http.StatusUnauthorized || he.StatusCode == http.StatusForbidden) {
				return err
			}
			continue
		}
		if msg.Error != nil {
			lastErr = fmt.Errorf("MCP initialize erro (%d): %s", msg.Error.Code, msg.Error.Message)
			continue
		}

		if sid := respHeaders.Get("Mcp-Session-Id"); sid != "" {
			m.sessionID = sid
		}
		var initRes struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if err := json.Unmarshal(msg.Result, &initRes); err == nil && initRes.ProtocolVersion != "" {
			m.protocolVersion = initRes.ProtocolVersion
		} else {
			m.protocolVersion = protoVer
		}

		// Envia notifications/initialized exigida pela especificação MCP antes de tools/list ou tools/call
		notifPayload := map[string]any{
			"jsonrpc": "2.0",
			"method":  "notifications/initialized",
		}
		_ = m.postNotification(ctx, m.targetURL, notifPayload)
		return nil
	}
	return lastErr
}

func (m *MCPSession) connectSSE(ctx context.Context) error {
	m.stopSSE()
	sseCtx, cancel := context.WithCancel(context.Background())
	sseDone := make(chan struct{})
	endpointCh := make(chan string, 1)
	m.sseCancel = cancel
	m.sseDone = sseDone
	m.endpointCh = endpointCh
	m.setSSEErr(nil)

	req, err := http.NewRequestWithContext(sseCtx, http.MethodGet, m.targetURL, nil)
	if err != nil {
		cancel()
		return err
	}
	m.applyHeaders(req, "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		cancel()
		return fmt.Errorf("SSE GET falhou: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		cancel()
		return &httpStatusError{StatusCode: resp.StatusCode, Body: string(bodyBytes)}
	}

	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if ct != "" && !strings.Contains(ct, "text/event-stream") {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		cancel()
		return fmt.Errorf("Content-Type inesperado para SSE (%s): %s", ct, string(bodyBytes))
	}

	go m.readSSEStream(resp, sseDone, endpointCh)

	// Aguarda evento "endpoint" do servidor SSE
	epWait := 5 * time.Second
	if dl, ok := ctx.Deadline(); ok {
		if rem := time.Until(dl); rem < epWait {
			epWait = rem
		}
	}
	if epWait <= 0 {
		epWait = 3 * time.Second
	}

	var rawEndpoint string
	select {
	case rawEndpoint = <-endpointCh:
	case <-sseDone:
		cancel()
		if err := m.getSSEErr(); err != nil {
			return fmt.Errorf("stream SSE encerrada antes do evento endpoint: %w", err)
		}
		return errors.New("stream SSE encerrada antes de enviar evento endpoint")
	case <-ctx.Done():
		cancel()
		return ctx.Err()
	case <-time.After(epWait):
		cancel()
		return errors.New("timeout aguardando evento 'endpoint' na stream SSE")
	}

	baseURL, err := url.Parse(m.targetURL)
	if err != nil {
		cancel()
		return err
	}
	epURL, err := url.Parse(strings.TrimSpace(rawEndpoint))
	if err != nil {
		cancel()
		return fmt.Errorf("endpoint SSE inválido %q: %w", rawEndpoint, err)
	}
	m.postURL = baseURL.ResolveReference(epURL).String()

	// Initialize via SSE
	id := m.reqID.Add(1)
	initPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "voiceagent",
				"version": "1.0.0",
			},
		},
	}

	msg, err := m.callOverSSE(ctx, initPayload, id)
	if err != nil {
		cancel()
		return err
	}
	if msg.Error != nil {
		cancel()
		return fmt.Errorf("MCP initialize erro (%d): %s", msg.Error.Code, msg.Error.Message)
	}

	var initRes struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(msg.Result, &initRes); err == nil && initRes.ProtocolVersion != "" {
		m.protocolVersion = initRes.ProtocolVersion
	} else {
		m.protocolVersion = "2024-11-05"
	}

	_ = m.postNotification(ctx, m.postURL, map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	})
	return nil
}

func (m *MCPSession) setSSEErr(err error) {
	m.sseErrMu.Lock()
	m.sseErr = err
	m.sseErrMu.Unlock()
}

func (m *MCPSession) getSSEErr() error {
	m.sseErrMu.Lock()
	defer m.sseErrMu.Unlock()
	return m.sseErr
}

func (m *MCPSession) readSSEStream(resp *http.Response, sseDone chan struct{}, endpointCh chan string) {
	defer resp.Body.Close()
	defer close(sseDone)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	var eventName string
	var dataLines []string
	endpointSent := false

	dispatchEvent := func() {
		if len(dataLines) == 0 {
			eventName = ""
			return
		}
		data := strings.Join(dataLines, "\n")
		ev := strings.TrimSpace(eventName)
		eventName = ""
		dataLines = nil

		trimmed := strings.TrimSpace(data)
		if trimmed == "" {
			return
		}

		if ev == "endpoint" || (!endpointSent && !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[")) {
			endpointSent = true
			select {
			case endpointCh <- trimmed:
			default:
			}
			return
		}

		var msg rpcMessage
		if err := json.Unmarshal([]byte(trimmed), &msg); err != nil {
			return
		}
		if idKey := normalizeRPCID(msg.ID); idKey != "" {
			m.deliverPending(idKey, msg)
		}
	}

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" {
			dispatchEvent()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			val := strings.TrimPrefix(line, "data:")
			if strings.HasPrefix(val, " ") {
				val = val[1:]
			}
			dataLines = append(dataLines, val)
		}
	}
	dispatchEvent()
	if err := scanner.Err(); err != nil && !m.closed.Load() {
		m.setSSEErr(err)
	}
}

func normalizeRPCID(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	return s
}

func (m *MCPSession) registerPending(id int64) (string, chan rpcMessage) {
	key := fmt.Sprintf("%d", id)
	ch := make(chan rpcMessage, 1)
	m.pendingMu.Lock()
	m.pending[key] = ch
	m.pendingMu.Unlock()
	return key, ch
}

func (m *MCPSession) unregisterPending(key string) {
	m.pendingMu.Lock()
	delete(m.pending, key)
	m.pendingMu.Unlock()
}

func (m *MCPSession) deliverPending(key string, msg rpcMessage) {
	m.pendingMu.Lock()
	ch, ok := m.pending[key]
	if ok {
		delete(m.pending, key)
	}
	m.pendingMu.Unlock()
	if ok {
		ch <- msg
	}
}

func (m *MCPSession) postNotification(ctx context.Context, postURL string, payload any) error {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, postURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	m.applyHeaders(req, "application/json, text/event-stream")
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return nil
}

func (m *MCPSession) postStreamable(ctx context.Context, postURL string, payload any, reqID int64) (rpcMessage, http.Header, error) {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, postURL, bytes.NewReader(body))
	if err != nil {
		return rpcMessage{}, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	m.applyHeaders(req, "application/json, text/event-stream")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return rpcMessage{}, nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return rpcMessage{}, resp.Header, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return rpcMessage{}, resp.Header, &httpStatusError{StatusCode: resp.StatusCode, Body: string(respBytes)}
	}

	targetKey := fmt.Sprintf("%d", reqID)
	msg, err := parseRPCBody(respBytes, resp.Header.Get("Content-Type"), targetKey)
	if err != nil {
		return rpcMessage{}, resp.Header, err
	}
	return msg, resp.Header, nil
}

func (m *MCPSession) callOverSSE(ctx context.Context, payload any, reqID int64) (rpcMessage, error) {
	key, ch := m.registerPending(reqID)
	defer m.unregisterPending(key)

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.postURL, bytes.NewReader(body))
	if err != nil {
		return rpcMessage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	m.applyHeaders(req, "application/json, text/event-stream")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return rpcMessage{}, err
	}
	respBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	resp.Body.Close()
	if readErr != nil {
		return rpcMessage{}, readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return rpcMessage{}, &httpStatusError{StatusCode: resp.StatusCode, Body: string(respBytes)}
	}

	// Alguns servidores respondem JSON-RPC diretamente no corpo do POST 200 OK
	trimmed := bytes.TrimSpace(respBytes)
	if len(trimmed) > 0 && (trimmed[0] == '{' || strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream")) {
		if msg, err := parseRPCBody(trimmed, resp.Header.Get("Content-Type"), key); err == nil {
			return msg, nil
		}
	}

	select {
	case msg := <-ch:
		return msg, nil
	case <-m.sseDone:
		if err := m.getSSEErr(); err != nil {
			return rpcMessage{}, fmt.Errorf("stream SSE encerrada: %w", err)
		}
		return rpcMessage{}, errors.New("stream SSE encerrada antes da resposta MCP")
	case <-ctx.Done():
		return rpcMessage{}, ctx.Err()
	}
}

func parseRPCBody(raw []byte, contentType, expectedID string) (rpcMessage, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return rpcMessage{}, errors.New("resposta MCP vazia")
	}

	isSSE := strings.Contains(strings.ToLower(contentType), "text/event-stream") ||
		strings.HasPrefix(trimmed, "event:") ||
		strings.HasPrefix(trimmed, "data:")

	if !isSSE {
		// Pode ser objeto único ou array de respostas JSON-RPC
		if strings.HasPrefix(trimmed, "[") {
			var list []rpcMessage
			if err := json.Unmarshal([]byte(trimmed), &list); err != nil {
				return rpcMessage{}, fmt.Errorf("MCP resposta JSON inválida: %s", trimmed)
			}
			for _, item := range list {
				if normalizeRPCID(item.ID) == expectedID {
					return item, nil
				}
			}
			if len(list) > 0 {
				return list[0], nil
			}
			return rpcMessage{}, errors.New("resposta JSON-RPC vazia")
		}
		var msg rpcMessage
		if err := json.Unmarshal([]byte(trimmed), &msg); err != nil {
			return rpcMessage{}, fmt.Errorf("MCP resposta inválida: %s", trimmed)
		}
		return msg, nil
	}

	// Parseia blocos SSE e devolve o evento cujo ID corresponde (ou último evento com result/error)
	var dataLines []string
	var fallback *rpcMessage
	lines := strings.Split(trimmed, "\n")
	checkBlock := func() (rpcMessage, bool) {
		if len(dataLines) == 0 {
			return rpcMessage{}, false
		}
		joined := strings.TrimSpace(strings.Join(dataLines, "\n"))
		dataLines = nil
		if joined == "" || !strings.HasPrefix(joined, "{") {
			return rpcMessage{}, false
		}
		var msg rpcMessage
		if err := json.Unmarshal([]byte(joined), &msg); err != nil {
			return rpcMessage{}, false
		}
		if normalizeRPCID(msg.ID) == expectedID {
			return msg, true
		}
		if len(msg.Result) > 0 || msg.Error != nil {
			copyMsg := msg
			fallback = &copyMsg
		}
		return rpcMessage{}, false
	}

	for _, rawLine := range lines {
		line := strings.TrimRight(rawLine, "\r")
		if line == "" {
			if msg, ok := checkBlock(); ok {
				return msg, nil
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			val := strings.TrimPrefix(line, "data:")
			if strings.HasPrefix(val, " ") {
				val = val[1:]
			}
			dataLines = append(dataLines, val)
		}
	}
	if msg, ok := checkBlock(); ok {
		return msg, nil
	}
	if fallback != nil {
		return *fallback, nil
	}
	return rpcMessage{}, fmt.Errorf("nenhuma resposta JSON-RPC válida na stream SSE: %s", trimmed)
}

func (m *MCPSession) doRPC(ctx context.Context, method string, params any) (rpcMessage, error) {
	id := m.reqID.Add(1)
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		payload["params"] = params
	}

	if m.transport == MCPTransportSSE {
		return m.callOverSSE(ctx, payload, id)
	}
	msg, _, err := m.postStreamable(ctx, m.targetURL, payload, id)
	return msg, err
}

func shouldReconnectMCP(err error, msg rpcMessage) bool {
	if err != nil {
		var he *httpStatusError
		if errors.As(err, &he) && (he.StatusCode == http.StatusNotFound || he.StatusCode == http.StatusBadRequest) {
			return true
		}
		low := strings.ToLower(err.Error())
		return strings.Contains(low, "sse encerrada") ||
			strings.Contains(low, "eof") ||
			strings.Contains(low, "session") ||
			strings.Contains(low, "not initialized")
	}
	if msg.Error != nil {
		low := strings.ToLower(msg.Error.Message)
		return strings.Contains(low, "session") || strings.Contains(low, "not initialized")
	}
	return false
}

// ListTools lista todas as ferramentas expostas pelo servidor MCP (com paginação nextCursor).
func (m *MCPSession) ListTools(ctx context.Context) ([]MCPToolDef, error) {
	var all []MCPToolDef
	cursor := ""
	for page := 0; page < 20; page++ {
		var params any
		if cursor != "" {
			params = map[string]any{"cursor": cursor}
		} else {
			params = map[string]any{}
		}
		msg, err := m.doRPC(ctx, "tools/list", params)
		if err != nil {
			return nil, fmt.Errorf("MCP tools/list falhou: %w", err)
		}
		if msg.Error != nil {
			return nil, fmt.Errorf("MCP tools/list erro (%d): %s", msg.Error.Code, msg.Error.Message)
		}

		var res struct {
			Tools      []MCPToolDef `json:"tools"`
			NextCursor string       `json:"nextCursor"`
		}
		if err := json.Unmarshal(msg.Result, &res); err != nil {
			return nil, fmt.Errorf("MCP tools/list resposta inválida: %w", err)
		}
		all = append(all, res.Tools...)
		if res.NextCursor == "" {
			break
		}
		cursor = res.NextCursor
	}
	return all, nil
}

// CallTool executa tools/call para uma ferramenta específica do servidor MCP,
// reconectando automaticamente uma vez caso a sessão remota tenha expirado.
func (m *MCPSession) CallTool(ctx context.Context, remoteToolName string, args map[string]any) ExecutionResult {
	start := time.Now()
	if args == nil {
		args = map[string]any{}
	}

	params := map[string]any{
		"name":      remoteToolName,
		"arguments": args,
	}

	epochBefore := m.connEpoch.Load()
	msg, err := m.doRPC(ctx, "tools/call", params)
	if shouldReconnectMCP(err, msg) && ctx.Err() == nil {
		m.reconnectMu.Lock()
		recErr := error(nil)
		if m.connEpoch.Load() == epochBefore {
			recErr = m.connect(ctx)
		}
		m.reconnectMu.Unlock()
		if recErr == nil {
			msg, err = m.doRPC(ctx, "tools/call", params)
		}
	}

	ms := int(time.Since(start).Milliseconds())
	if err != nil {
		errMsg := fmt.Sprintf("MCP tools/call (%s) falhou: %v", remoteToolName, err)
		return ExecutionResult{
			Status:       "error",
			Result:       map[string]any{"success": false, "error": errMsg},
			DurationMS:   ms,
			ErrorMessage: errMsg,
		}
	}

	if msg.Error != nil {
		errMsg := fmt.Sprintf("MCP erro (%d): %s", msg.Error.Code, msg.Error.Message)
		return ExecutionResult{
			Status:       "error",
			Result:       map[string]any{"success": false, "error": errMsg},
			DurationMS:   ms,
			ErrorMessage: errMsg,
			RawResponse:  string(msg.Result),
		}
	}

	var toolRes struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
			Data string `json:"data,omitempty"`
		} `json:"content"`
		StructuredContent any  `json:"structuredContent,omitempty"`
		IsError           bool `json:"isError"`
	}
	if err := json.Unmarshal(msg.Result, &toolRes); err != nil {
		var fallbackAny any
		_ = json.Unmarshal(msg.Result, &fallbackAny)
		return ExecutionResult{
			Status:      "success",
			Result:      map[string]any{"success": true, "data": fallbackAny},
			RawResponse: fallbackAny,
			DurationMS:  ms,
		}
	}

	var dataOutput any
	var textParts []string
	for _, c := range toolRes.Content {
		if strings.TrimSpace(c.Text) != "" {
			textParts = append(textParts, c.Text)
		}
	}
	if toolRes.StructuredContent != nil && len(textParts) == 0 {
		dataOutput = toolRes.StructuredContent
	} else if len(textParts) == 1 {
		var inner any
		if err := json.Unmarshal([]byte(textParts[0]), &inner); err == nil {
			dataOutput = inner
		} else {
			dataOutput = textParts[0]
		}
	} else if len(textParts) > 1 {
		dataOutput = strings.Join(textParts, "\n")
	} else {
		var rawObj any
		_ = json.Unmarshal(msg.Result, &rawObj)
		dataOutput = rawObj
	}

	if toolRes.IsError {
		errText := fmt.Sprint(dataOutput)
		return ExecutionResult{
			Status:       "error",
			Result:       map[string]any{"success": false, "error": errText, "data": dataOutput},
			RawResponse:  dataOutput,
			DurationMS:   ms,
			ErrorMessage: errText,
		}
	}

	return ExecutionResult{
		Status:      "success",
		Result:      map[string]any{"success": true, "data": dataOutput},
		RawResponse: dataOutput,
		DurationMS:  ms,
	}
}
