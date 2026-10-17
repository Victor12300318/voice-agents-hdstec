// Package variables resolve as Variables de uma Conversation: System Variables
// calculadas pela plataforma e Dynamic Variables + Overrides vindos do Initiation Webhook (padrão ElevenLabs Twilio Personalization).
package variables

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Declared é uma Dynamic Variable declarada no Agent.
type Declared struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

var (
	nameRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)
	refRe  = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_]+)\s*\}\}`)
)

// SystemNames são reservados; não podem ser declarados como Dynamic Variables.
var SystemNames = []string{
	"horario",
	"data",
	"hora",
	"dia_semana",
	"id_conversa",
	"system__agent_id",
	"system__caller_id",
	"system__called_number",
	"system__call_duration_secs",
	"system__timezone",
	"system__call_sid",
	"system__conversation_id",
}

func IsSystem(name string) bool {
	for _, n := range SystemNames {
		if n == name {
			return true
		}
	}
	return false
}

func ValidName(name string) bool { return nameRe.MatchString(name) }

// System calcula as System Variables básicas no fuso do Agent.
func System(now time.Time, loc *time.Location, conversationID string) map[string]string {
	t := now.In(loc)
	greeting := "Boa noite"
	switch h := t.Hour(); {
	case h >= 5 && h < 12:
		greeting = "Bom dia"
	case h >= 12 && h < 18:
		greeting = "Boa tarde"
	}
	dias := []string{"domingo", "segunda-feira", "terça-feira", "quarta-feira", "quinta-feira", "sexta-feira", "sábado"}
	return map[string]string{
		"horario":                 greeting,
		"data":                    t.Format("02/01/2006"),
		"hora":                    t.Format("15:04"),
		"dia_semana":              dias[t.Weekday()],
		"id_conversa":             conversationID,
		"system__conversation_id": conversationID,
	}
}

// SystemForCall calcula todas as System Variables incluindo os metadados da chamada (Twilio / SIP / Web).
func SystemForCall(
	now time.Time,
	loc *time.Location,
	conversationID, agentID, callerID, calledNumber, callSid, timezone string,
) map[string]string {
	m := System(now, loc, conversationID)
	m["system__agent_id"] = agentID
	m["system__caller_id"] = callerID
	m["system__called_number"] = calledNumber
	m["system__call_sid"] = callSid
	m["system__timezone"] = timezone
	m["system__call_duration_secs"] = "0"
	return m
}

// Render substitui {{nome}} pelos valores; referências desconhecidas ficam intactas.
func Render(text string, vars map[string]string) string {
	return refRe.ReplaceAllStringFunc(text, func(m string) string {
		name := refRe.FindStringSubmatch(m)[1]
		if v, ok := vars[name]; ok {
			return v
		}
		return m
	})
}

// References lista os nomes usados como {{nome}} no texto, sem repetição, ordenados.
func References(texts ...string) []string {
	seen := map[string]bool{}
	for _, t := range texts {
		for _, m := range refRe.FindAllStringSubmatch(t, -1) {
			seen[m[1]] = true
		}
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// InitiationRequest é o corpo POST enviado ao Initiation Webhook no formato ElevenLabs Twilio Personalization.
type InitiationRequest struct {
	CallerID       string         `json:"caller_id"`
	AgentID        string         `json:"agent_id"`
	CalledNumber   string         `json:"called_number"`
	CallSid        string         `json:"call_sid"`
	ConversationID string         `json:"conversation_id"`
	Caller         string         `json:"caller,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// ConversationConfigOverride representa o bloco conversation_config_override do padrão ElevenLabs.
type ConversationConfigOverride struct {
	Agent *struct {
		Prompt *struct {
			Prompt string `json:"prompt"`
			LLM    string `json:"llm"`
		} `json:"prompt,omitempty"`
		FirstMessage string `json:"first_message,omitempty"`
		Language     string `json:"language,omitempty"`
	} `json:"agent,omitempty"`
	TTS *struct {
		VoiceID string `json:"voice_id,omitempty"`
	} `json:"tts,omitempty"`
	Conversation *struct {
		TextOnly bool `json:"text_only,omitempty"`
	} `json:"conversation,omitempty"`
}

// InitiationResult contém os dados retornados pelo webhook de início de conversa (conversation_initiation_client_data).
type InitiationResult struct {
	Type                       string                      `json:"type"`
	DynamicVariables           map[string]string           `json:"dynamic_variables"`
	ConversationConfigOverride *ConversationConfigOverride `json:"conversation_config_override,omitempty"`
	BranchID                   string                      `json:"branch_id,omitempty"`
	Environment                string                      `json:"environment,omitempty"`
	MissingVariables           []string                    `json:"missing_variables,omitempty"`
}

var ErrInitiation = errors.New("initiation webhook falhou")

// FetchInitiationData executa o POST ao Initiation Webhook compatível com ElevenLabs Twilio Personalization
// e extrai dynamic_variables + conversation_config_override.
func FetchInitiationData(
	ctx context.Context,
	client *http.Client,
	url string,
	timeout time.Duration,
	req InitiationRequest,
	declared []Declared,
) (InitiationResult, error) {
	var res InitiationResult
	fail := func(format string, a ...any) (InitiationResult, error) {
		return res, fmt.Errorf("%w: %s", ErrInitiation, fmt.Sprintf(format, a...))
	}

	if req.CallerID == "" && req.Caller != "" {
		req.CallerID = req.Caller
	}
	if req.Caller == "" && req.CallerID != "" {
		req.Caller = req.CallerID
	}
	if req.Metadata == nil {
		req.Metadata = map[string]any{}
	}

	body, _ := json.Marshal(req)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fail("URL inválida: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fail("timeout após %s", timeout)
		}
		return fail("%v", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fail("erro lendo resposta: %v", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fail("HTTP %d", resp.StatusCode)
	}

	trimmed := bytes.TrimSpace(raw)
	// Suporta também respostas de webhook embrulhadas em array JSON (ex: n8n Respond to Webhook)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var arr []json.RawMessage
		if err := json.Unmarshal(trimmed, &arr); err == nil && len(arr) > 0 {
			trimmed = bytes.TrimSpace(arr[0])
		}
	}

	var parsed struct {
		Type                       string                      `json:"type"`
		DynamicVariables           map[string]any              `json:"dynamic_variables"`
		ConversationConfigOverride *ConversationConfigOverride `json:"conversation_config_override"`
		BranchID                   string                      `json:"branch_id"`
		Environment                string                      `json:"environment"`
	}
	if err := json.Unmarshal(trimmed, &parsed); err != nil {
		return fail("JSON inválido: %v", err)
	}

	// Fallback: se o webhook retornou um JSON plano {"primNome": "João"} sem a chave dynamic_variables
	rawVarsMap := parsed.DynamicVariables
	if rawVarsMap == nil {
		var flatMap map[string]any
		if err := json.Unmarshal(trimmed, &flatMap); err == nil {
			delete(flatMap, "type")
			delete(flatMap, "conversation_config_override")
			delete(flatMap, "branch_id")
			delete(flatMap, "environment")
			if len(flatMap) > 0 {
				rawVarsMap = flatMap
			}
		}
	}

	out := map[string]string{}
	for k, v := range rawVarsMap {
		if !ValidName(k) || IsSystem(k) {
			continue
		}
		out[k] = scalar(v)
	}

	var missing []string
	for _, d := range declared {
		if _, ok := out[d.Name]; !ok {
			missing = append(missing, d.Name)
		}
	}

	resType := parsed.Type
	if resType == "" {
		resType = "conversation_initiation_client_data"
	}

	res = InitiationResult{
		Type:                       resType,
		DynamicVariables:           out,
		ConversationConfigOverride: parsed.ConversationConfigOverride,
		BranchID:                   parsed.BranchID,
		Environment:                parsed.Environment,
		MissingVariables:           missing,
	}
	return res, nil
}

// FetchDynamic chama o Initiation Webhook e exige que todas as Variables declaradas
// venham na resposta. Qualquer falha retorna um erro que envolve ErrInitiation.
func FetchDynamic(ctx context.Context, client *http.Client, url string, timeout time.Duration, req InitiationRequest, declared []Declared) (map[string]string, error) {
	res, err := FetchInitiationData(ctx, client, url, timeout, req, declared)
	if err != nil {
		return nil, err
	}
	if len(res.MissingVariables) > 0 {
		return res.DynamicVariables, fmt.Errorf("%w: Variables não devolvidas: %s", ErrInitiation, strings.Join(res.MissingVariables, ", "))
	}
	return res.DynamicVariables, nil
}

func scalar(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}
