// Package tools executa ferramentas externas (Webhooks, MCP Servers, Client Tools e Code Tools) e normaliza seus envelopes.
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"voiceagent/internal/store"
)

type ExecutionResult struct {
	Status       string            // success | error
	Result       map[string]any    // envelope normalizado enviado ao modelo
	RawResponse  any               // resposta decodificada original
	DurationMS   int               // tempo em ms
	ErrorMessage string            // mensagem de erro (se houver)
	AssignedVars map[string]string // variáveis dinâmicas extraídas via assignments
	Logs         []string          // logs de console (Code Tools)
}

type Executor struct {
	HTTPClient *http.Client
}

func NewExecutor(client *http.Client) *Executor {
	if client == nil {
		client = http.DefaultClient
	}
	return &Executor{HTTPClient: client}
}

// Execute executa uma Tool mantendo compatibilidade com chamadas sem variáveis dinâmicas.
func (e *Executor) Execute(ctx context.Context, tool store.Tool, args map[string]any) ExecutionResult {
	return e.ExecuteWithVars(ctx, tool, args, nil)
}

// ExecuteWithVars executa uma Tool (Webhook, MCP, Client ou Code) aplicando variáveis dinâmicas, mocks e assignments.
func (e *Executor) ExecuteWithVars(ctx context.Context, tool store.Tool, args map[string]any, vars map[string]string) ExecutionResult {
	start := time.Now()
	timeout := time.Duration(tool.TimeoutMS) * time.Millisecond
	if timeout <= 0 && tool.Config.ResponseTimeoutSecs > 0 {
		timeout = time.Duration(tool.Config.ResponseTimeoutSecs) * time.Second
	}
	if timeout <= 0 {
		timeout = 20 * time.Second
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if args == nil {
		args = map[string]any{}
	}

	// 1. Verifica Mocks de Resposta ativos (avaliados de cima para baixo)
	for _, mock := range tool.Config.ResponseMocks {
		if !mock.Enabled {
			continue
		}
		matched := false
		if mock.ConditionType == "" || mock.ConditionType == "always" {
			matched = true
		} else if mock.ConditionType == "parameter_equals" && mock.ParameterName != "" {
			if val, exists := args[mock.ParameterName]; exists && fmt.Sprint(val) == mock.ParameterValue {
				matched = true
			}
		}
		if matched {
			var parsed any
			if err := json.Unmarshal([]byte(mock.Response), &parsed); err != nil {
				parsed = mock.Response
			}
			assigned := evaluateAssignments(tool.Config.Assignments, parsed)
			return ExecutionResult{
				Status:       "success",
				Result:       map[string]any{"success": true, "data": parsed, "mocked": true},
				RawResponse:  parsed,
				DurationMS:   int(time.Since(start).Milliseconds()),
				AssignedVars: assigned,
			}
		}
	}

	// 2. Mescla placeholders de variáveis dinâmicas + variáveis reais da conversa
	effectiveVars := map[string]string{}
	if tool.Config.DynamicVariables != nil {
		for k, v := range tool.Config.DynamicVariables.DynamicVariablePlaceholders {
			effectiveVars[k] = fmt.Sprint(v)
		}
	}
	for k, v := range vars {
		effectiveVars[k] = v
	}

	var res ExecutionResult

	switch tool.Kind {
	case "code":
		res = e.executeCode(execCtx, tool, args, effectiveVars, start)
	case "client":
		res = ExecutionResult{
			Status:      "success",
			Result:      map[string]any{"success": true, "executed_on": "client", "tool": tool.Name, "args": args},
			RawResponse: args,
			DurationMS:  int(time.Since(start).Milliseconds()),
		}
	case "mcp":
		res = e.executeMCP(execCtx, tool, args, effectiveVars, start)
	default:
		if strings.Contains(strings.ToLower(tool.Config.URL), "/mcp") {
			res = e.executeMCP(execCtx, tool, args, effectiveVars, start)
		} else {
			res = e.executeWebhook(execCtx, tool, args, effectiveVars, start)
			if res.Status == "error" && strings.Contains(res.ErrorMessage, "406") && strings.Contains(res.ErrorMessage, "event-stream") {
				res = e.executeMCP(execCtx, tool, args, effectiveVars, start)
			}
		}
	}

	if res.Status == "success" && len(tool.Config.Assignments) > 0 {
		res.AssignedVars = evaluateAssignments(tool.Config.Assignments, res.RawResponse)
	}
	return res
}

func interpolateVars(input string, vars map[string]string) string {
	out := input
	for k, v := range vars {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	return out
}

func resolveSchemaParamValue(ps store.ToolParamSchema, args map[string]any, vars map[string]string) (any, bool) {
	switch ps.ValueType {
	case "constant":
		if ps.ConstantValue != nil {
			return ps.ConstantValue, true
		}
	case "dynamic_variable":
		if ps.DynamicVariable != "" {
			if v, ok := vars[ps.DynamicVariable]; ok {
				return v, true
			}
		}
	}
	if v, ok := args[ps.Name]; ok {
		if s, isStr := v.(string); isStr {
			return interpolateVars(s, vars), true
		}
		return v, true
	}
	return nil, false
}

func (e *Executor) executeWebhook(ctx context.Context, tool store.Tool, args map[string]any, vars map[string]string, start time.Time) ExecutionResult {
	targetURL := tool.Config.URL
	method := strings.ToUpper(strings.TrimSpace(tool.Config.Method))
	contentType := "application/json"

	if tool.Config.APISchema != nil {
		if tool.Config.APISchema.URL != "" {
			targetURL = tool.Config.APISchema.URL
		}
		if tool.Config.APISchema.Method != "" {
			method = strings.ToUpper(strings.TrimSpace(tool.Config.APISchema.Method))
		}
		if tool.Config.APISchema.ContentType != "" {
			contentType = tool.Config.APISchema.ContentType
		}
	}
	if method == "" {
		method = http.MethodPost
	}
	targetURL = interpolateVars(targetURL, vars)

	usedInPathOrQuery := map[string]bool{}

	// 1. Substitui parâmetros de caminho ({param}) definidos em path_params_schema ou presentes na URL
	if tool.Config.APISchema != nil {
		for _, pp := range tool.Config.APISchema.PathParamsSchema {
			if val, ok := resolveSchemaParamValue(pp, args, vars); ok {
				placeholder := "{" + pp.Name + "}"
				if strings.Contains(targetURL, placeholder) {
					targetURL = strings.ReplaceAll(targetURL, placeholder, url.PathEscape(fmt.Sprint(val)))
					usedInPathOrQuery[pp.Name] = true
				}
			}
		}
	}
	for k, v := range args {
		placeholder := "{" + k + "}"
		if strings.Contains(targetURL, placeholder) {
			targetURL = strings.ReplaceAll(targetURL, placeholder, url.PathEscape(fmt.Sprint(v)))
			usedInPathOrQuery[k] = true
		}
	}

	// 2. Parâmetros de consulta (Query Params)
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		ms := int(time.Since(start).Milliseconds())
		return ExecutionResult{
			Status:       "error",
			Result:       map[string]any{"success": false, "error": fmt.Sprintf("URL inválida: %v", err)},
			DurationMS:   ms,
			ErrorMessage: err.Error(),
		}
	}
	queryVals := parsedURL.Query()
	if tool.Config.APISchema != nil && len(tool.Config.APISchema.QueryParamsSchema) > 0 {
		for _, qp := range tool.Config.APISchema.QueryParamsSchema {
			if val, ok := resolveSchemaParamValue(qp, args, vars); ok && fmt.Sprint(val) != "" {
				queryVals.Set(qp.Name, fmt.Sprint(val))
				usedInPathOrQuery[qp.Name] = true
			}
		}
	} else if method == http.MethodGet || method == http.MethodDelete {
		for k, v := range args {
			if !usedInPathOrQuery[k] && v != nil {
				queryVals.Set(k, interpolateVars(fmt.Sprint(v), vars))
				usedInPathOrQuery[k] = true
			}
		}
	}
	parsedURL.RawQuery = queryVals.Encode()
	finalURL := parsedURL.String()

	// 3. Corpo da requisição (POST, PUT, PATCH)
	var bodyReader io.Reader
	if method != http.MethodGet && method != http.MethodHead && method != http.MethodDelete {
		bodyMap := map[string]any{}
		for k, v := range args {
			if !usedInPathOrQuery[k] {
				if s, isStr := v.(string); isStr {
					bodyMap[k] = interpolateVars(s, vars)
				} else {
					bodyMap[k] = v
				}
			}
		}
		if tool.Config.APISchema != nil && tool.Config.APISchema.RequestBodySchema != nil {
			for _, bp := range tool.Config.APISchema.RequestBodySchema.Properties {
				if val, ok := resolveSchemaParamValue(bp, args, vars); ok {
					bodyMap[bp.Name] = val
				}
			}
		}

		if contentType == "application/x-www-form-urlencoded" {
			form := url.Values{}
			for k, v := range bodyMap {
				form.Set(k, fmt.Sprint(v))
			}
			bodyReader = strings.NewReader(form.Encode())
		} else {
			reqBytes, err := json.Marshal(bodyMap)
			if err != nil {
				ms := int(time.Since(start).Milliseconds())
				return ExecutionResult{
					Status:       "error",
					Result:       map[string]any{"success": false, "error": fmt.Sprintf("serialização de argumentos falhou: %v", err)},
					DurationMS:   ms,
					ErrorMessage: err.Error(),
				}
			}
			bodyReader = bytes.NewReader(reqBytes)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, finalURL, bodyReader)
	if err != nil {
		ms := int(time.Since(start).Milliseconds())
		return ExecutionResult{
			Status:       "error",
			Result:       map[string]any{"success": false, "error": fmt.Sprintf("URL inválida: %v", err)},
			DurationMS:   ms,
			ErrorMessage: err.Error(),
		}
	}

	if bodyReader != nil {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json, text/event-stream, */*")

	// 4. Cabeçalhos configurados em api_schema.request_headers e secret_headers
	if tool.Config.APISchema != nil {
		for _, h := range tool.Config.APISchema.RequestHeaders {
			name := strings.TrimSpace(h.Name)
			if name == "" {
				continue
			}
			val := h.Value
			if h.Type == "dynamic_variable" {
				if dv, ok := vars[h.Value]; ok {
					val = dv
				}
			} else {
				val = interpolateVars(val, vars)
			}
			req.Header.Set(name, val)
		}
	}
	for k, v := range tool.SecretHeaders {
		req.Header.Set(k, interpolateVars(v, vars))
	}

	resp, err := e.HTTPClient.Do(req)
	ms := int(time.Since(start).Milliseconds())
	if err != nil {
		reason := err.Error()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			reason = "timeout na execução do webhook"
		}
		return ExecutionResult{
			Status:       "error",
			Result:       map[string]any{"success": false, "error": reason},
			DurationMS:   ms,
			ErrorMessage: reason,
		}
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ExecutionResult{
			Status:       "error",
			Result:       map[string]any{"success": false, "error": fmt.Sprintf("leitura de resposta falhou: %v", err)},
			DurationMS:   ms,
			ErrorMessage: err.Error(),
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		errMsg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
		return ExecutionResult{
			Status:       "error",
			Result:       map[string]any{"success": false, "error": errMsg},
			DurationMS:   ms,
			ErrorMessage: errMsg,
			RawResponse:  string(bodyBytes),
		}
	}

	var parsed any
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		errMsg := fmt.Sprintf("resposta não é JSON válido: %s", string(bodyBytes))
		return ExecutionResult{
			Status:       "error",
			Result:       map[string]any{"success": false, "error": errMsg},
			DurationMS:   ms,
			ErrorMessage: errMsg,
			RawResponse:  string(bodyBytes),
		}
	}

	return ExecutionResult{
		Status:      "success",
		Result:      map[string]any{"success": true, "data": parsed},
		RawResponse: parsed,
		DurationMS:  ms,
	}
}

// executeCode executa uma Code Tool JavaScript (export default async (ctx) => { ... }) em sandbox Node.js.
func (e *Executor) executeCode(ctx context.Context, tool store.Tool, args map[string]any, vars map[string]string, start time.Time) ExecutionResult {
	codeStr := strings.TrimSpace(tool.Config.Code)
	if codeStr == "" {
		codeStr = "export default async (ctx) => { return { result: ctx.args }; };"
	}

	configMap := map[string]string{}
	if tool.Config.ContextObject != nil && tool.Config.ContextObject.Config != nil {
		for k, v := range tool.Config.ContextObject.Config {
			configMap[k] = interpolateVars(v, vars)
		}
	}
	secretsMap := map[string]string{}
	for k, v := range tool.SecretHeaders {
		secretsMap[k] = v
	}

	tmpDir, err := os.MkdirTemp("", "code-tool-*")
	if err != nil {
		ms := int(time.Since(start).Milliseconds())
		return ExecutionResult{Status: "error", Result: map[string]any{"success": false, "error": err.Error()}, DurationMS: ms, ErrorMessage: err.Error()}
	}
	defer os.RemoveAll(tmpDir)

	userModPath := filepath.Join(tmpDir, "tool.mjs")
	if err := os.WriteFile(userModPath, []byte(codeStr), 0600); err != nil {
		ms := int(time.Since(start).Milliseconds())
		return ExecutionResult{Status: "error", Result: map[string]any{"success": false, "error": err.Error()}, DurationMS: ms, ErrorMessage: err.Error()}
	}

	ctxPayload, _ := json.Marshal(map[string]any{
		"args":             args,
		"config":           configMap,
		"secrets":          secretsMap,
		"auth_connections": map[string]string{},
	})

	runnerCode := fmt.Sprintf(`import fn from './tool.mjs';
const logs = [];
const origLog = console.log;
const fmt = (...a) => a.map(x => typeof x === 'object' ? JSON.stringify(x) : String(x)).join(' ');
console.log = (...a) => logs.push(fmt(...a));
console.warn = (...a) => logs.push('[WARN] ' + fmt(...a));
console.error = (...a) => logs.push('[ERROR] ' + fmt(...a));
const ctx = %s;
try {
  const res = await fn(ctx);
  origLog(JSON.stringify({ ok: true, result: res ?? null, logs }));
} catch (err) {
  origLog(JSON.stringify({ ok: false, error: err?.message || String(err), logs }));
}
`, string(ctxPayload))

	runnerPath := filepath.Join(tmpDir, "runner.mjs")
	_ = os.WriteFile(runnerPath, []byte(runnerCode), 0600)

	cmd := exec.CommandContext(ctx, "node", runnerPath)
	cmd.Dir = tmpDir
	outBytes, err := cmd.CombinedOutput()
	ms := int(time.Since(start).Milliseconds())
	if err != nil {
		errMsg := fmt.Sprintf("erro ao executar Code Tool: %v (%s)", err, strings.TrimSpace(string(outBytes)))
		return ExecutionResult{
			Status:       "error",
			Result:       map[string]any{"success": false, "error": errMsg},
			DurationMS:   ms,
			ErrorMessage: errMsg,
		}
	}

	lines := strings.Split(strings.TrimSpace(string(outBytes)), "\n")
	lastLine := lines[len(lines)-1]
	var runOut struct {
		OK     bool     `json:"ok"`
		Result any      `json:"result"`
		Error  string   `json:"error"`
		Logs   []string `json:"logs"`
	}
	if err := json.Unmarshal([]byte(lastLine), &runOut); err != nil {
		return ExecutionResult{
			Status:      "success",
			Result:      map[string]any{"success": true, "data": strings.TrimSpace(string(outBytes))},
			RawResponse: strings.TrimSpace(string(outBytes)),
			DurationMS:  ms,
		}
	}
	if !runOut.OK {
		return ExecutionResult{
			Status:       "error",
			Result:       map[string]any{"success": false, "error": runOut.Error},
			DurationMS:   ms,
			ErrorMessage: runOut.Error,
			Logs:         runOut.Logs,
		}
	}
	return ExecutionResult{
		Status:      "success",
		Result:      map[string]any{"success": true, "data": runOut.Result},
		RawResponse: runOut.Result,
		DurationMS:  ms,
		Logs:        runOut.Logs,
	}
}

// executeMCP estabelece a sessão MCP via handshake JSON-RPC 2.0 e executa a chamada tools/call.
func (e *Executor) executeMCP(ctx context.Context, tool store.Tool, args map[string]any, vars map[string]string, start time.Time) ExecutionResult {
	normalizedArgs := make(map[string]any)
	for k, v := range args {
		normalizedArgs[k] = v
	}
	if _, ok := normalizedArgs["parameters0_Value"]; !ok {
		if cpfVal, exists := normalizedArgs["cpf"]; exists {
			normalizedArgs["parameters0_Value"] = cpfVal
		}
		if telVal, exists := normalizedArgs["telefone"]; exists {
			normalizedArgs["parameters1_Value"] = telVal
		}
		if nomeVal, exists := normalizedArgs["nome"]; exists {
			normalizedArgs["parameters2_Value"] = nomeVal
		}
	}

	targetURL := interpolateVars(tool.Config.URL, vars)
	if tool.Config.APISchema != nil && tool.Config.APISchema.URL != "" {
		targetURL = interpolateVars(tool.Config.APISchema.URL, vars)
	}

	applyHeaders := func(req *http.Request) {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if tool.Config.APISchema != nil {
			for _, h := range tool.Config.APISchema.RequestHeaders {
				if strings.TrimSpace(h.Name) != "" {
					req.Header.Set(strings.TrimSpace(h.Name), interpolateVars(h.Value, vars))
				}
			}
		}
		for k, v := range tool.SecretHeaders {
			req.Header.Set(k, interpolateVars(v, vars))
		}
	}

	// 1. Handshake initialize
	initPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
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
	initBytes, _ := json.Marshal(initPayload)
	initReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(initBytes))
	if err != nil {
		ms := int(time.Since(start).Milliseconds())
		return ExecutionResult{Status: "error", Result: map[string]any{"success": false, "error": err.Error()}, DurationMS: ms}
	}
	applyHeaders(initReq)

	initResp, err := e.HTTPClient.Do(initReq)
	if err != nil {
		ms := int(time.Since(start).Milliseconds())
		return ExecutionResult{Status: "error", Result: map[string]any{"success": false, "error": fmt.Sprintf("MCP handshake falhou: %v", err)}, DurationMS: ms}
	}
	defer initResp.Body.Close()

	sessionID := initResp.Header.Get("Mcp-Session-Id")

	// 2. Chamada tools/call
	callPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      tool.Name,
			"arguments": normalizedArgs,
		},
	}
	callBytes, _ := json.Marshal(callPayload)
	callReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(callBytes))
	if err != nil {
		ms := int(time.Since(start).Milliseconds())
		return ExecutionResult{Status: "error", Result: map[string]any{"success": false, "error": err.Error()}, DurationMS: ms}
	}
	applyHeaders(callReq)
	if sessionID != "" {
		callReq.Header.Set("Mcp-Session-Id", sessionID)
	}

	callResp, err := e.HTTPClient.Do(callReq)
	if err != nil {
		ms := int(time.Since(start).Milliseconds())
		return ExecutionResult{Status: "error", Result: map[string]any{"success": false, "error": fmt.Sprintf("MCP tools/call falhou: %v", err)}, DurationMS: ms}
	}
	defer callResp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(callResp.Body, 1<<20))
	ms := int(time.Since(start).Milliseconds())
	if err != nil {
		return ExecutionResult{Status: "error", Result: map[string]any{"success": false, "error": err.Error()}, DurationMS: ms}
	}

	rawStr := string(bodyBytes)

	var jsonText []byte
	lines := strings.Split(rawStr, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "data:") {
			jsonText = []byte(strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
			break
		}
	}
	if len(jsonText) == 0 {
		jsonText = bodyBytes
	}

	var rpcResp struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(jsonText, &rpcResp); err != nil {
		return ExecutionResult{
			Status:       "error",
			Result:       map[string]any{"success": false, "error": fmt.Sprintf("MCP resposta inválida: %s", rawStr)},
			DurationMS:   ms,
			ErrorMessage: err.Error(),
			RawResponse:  rawStr,
		}
	}

	if rpcResp.Error != nil {
		return ExecutionResult{
			Status:       "error",
			Result:       map[string]any{"success": false, "error": rpcResp.Error.Message},
			DurationMS:   ms,
			ErrorMessage: rpcResp.Error.Message,
			RawResponse:  rawStr,
		}
	}

	var dataOutput any
	if len(rpcResp.Result.Content) > 0 {
		text := rpcResp.Result.Content[0].Text
		var inner any
		if err := json.Unmarshal([]byte(text), &inner); err == nil {
			dataOutput = inner
		} else {
			dataOutput = text
		}
	} else {
		dataOutput = rpcResp.Result
	}

	return ExecutionResult{
		Status:      "success",
		Result:      map[string]any{"success": true, "data": dataOutput},
		RawResponse: dataOutput,
		DurationMS:  ms,
	}
}

// evaluateAssignments extrai valores do JSON de resposta (por caminho com ponto, ex: "data.plano") para atualizar variáveis dinâmicas.
func evaluateAssignments(assignments []store.ToolAssignment, raw any) map[string]string {
	if len(assignments) == 0 || raw == nil {
		return nil
	}
	out := map[string]string{}
	for _, a := range assignments {
		varName := strings.TrimSpace(a.DynamicVariable)
		path := strings.TrimSpace(a.ValuePath)
		if varName == "" || path == "" {
			continue
		}
		if val, ok := extractJSONPath(raw, path); ok && val != nil {
			out[varName] = fmt.Sprint(val)
		}
	}
	return out
}

func extractJSONPath(curr any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	node := curr
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if m, ok := node.(map[string]any); ok {
			v, exists := m[p]
			if !exists {
				return nil, false
			}
			node = v
		} else if arr, ok := node.([]any); ok && len(arr) > 0 {
			// Se a raiz for um array (comum no n8n), acessa o 1º elemento
			if m0, ok0 := arr[0].(map[string]any); ok0 {
				v, exists := m0[p]
				if !exists {
					return nil, false
				}
				node = v
			} else {
				return nil, false
			}
		} else {
			return nil, false
		}
	}
	return node, true
}
