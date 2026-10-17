// Package guardian implementa o Guardião Jev (via OpenRouter `typesafe/jev-router`
// ou TypeSafe System One API `jev-latest`) para monitorar conversas de voz em tempo real:
// 1. Detecção de Caixa Postal / Secretária Eletrônica (is_voicemail)
// 2. Detecção de Cliente Mudo / Agente Falando Sozinho (talking_to_self)
// 3. Detecção de Alucinação do Agente / Desvio de Contexto (agent_hallucinating)
package guardian

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	OpenAIEndpoint     = "https://api.openai.com/v1/chat/completions"
	OpenRouterEndpoint = "https://openrouter.ai/api/v1/chat/completions"
	TypeSafeEndpoint   = "https://api.typesafe.ai/v1/systemone"
	DefaultModel       = "typesafe/jev-router"
	DefaultOpenAIModel = "gpt-4.1-mini"
)

// EvalInput reúne o estado atual da conversa para avaliação pelo Guardião Jev.
type EvalInput struct {
	AgentName                          string            `json:"agent_name"`
	AgentRoleSummary                   string            `json:"agent_role_summary"`
	ClientVariables                    map[string]string `json:"client_variables"`
	RecentTranscript                   []string          `json:"recent_transcript"`
	LastAgentTurn                      string            `json:"last_agent_turn"`
	LastCallerTurn                     string            `json:"last_caller_turn"`
	ConsecutiveAgentTurnsWithoutCaller int               `json:"consecutive_agent_turns_without_caller"`
	SecondsSinceLastCallerSpeech       int               `json:"seconds_since_last_caller_speech"`
	TotalCallerTurns                   int               `json:"total_caller_turns"`
}

// EvalResult representa a resposta estruturada (primitivas Noul e Score) do Guardião Jev.
type EvalResult struct {
	Model                 string  `json:"model"`
	IsVoicemail           float64 `json:"is_voicemail"`
	TalkingToSelf         float64 `json:"talking_to_self"`
	AgentHallucinating    float64 `json:"agent_hallucinating"`
	HallucinationSeverity float64 `json:"hallucination_severity"`
	Reason                string  `json:"reason"`
	CorrectionHint        string  `json:"correction_hint,omitempty"`
	InputTokens           int     `json:"input_tokens"`
	OutputTokens          int     `json:"output_tokens"`
	CostUSD               float64 `json:"cost_usd,omitempty"`
}

type Evaluator struct {
	HTTPClient       *http.Client
	OpenAIAPIKey     string
	OpenRouterAPIKey string
	OpenRouterModel  string
	TypeSafeAPIKey   string
}

// FastVoicemailHeuristic detecta instantaneamente frases clássicas de caixa postal das operadoras brasileiras
// (TIM, Vivo, Claro, Oi) para complementar a avaliação probabilística do Jev.
func FastVoicemailHeuristic(callerText string) (bool, string) {
	norm := strings.ToLower(strings.TrimSpace(callerText))
	if norm == "" {
		return false, ""
	}
	patterns := []string{
		"caixa postal",
		"caixa de mensagens",
		"grave seu recado",
		"grave sua mensagem",
		"deixe seu recado",
		"deixe sua mensagem",
		"após o sinal",
		"apos o sinal",
		"após o bip",
		"apos o bip",
		"número chamado não está disponível",
		"numero chamado nao esta disponivel",
		"fora da área de cobertura",
		"fora da area de cobertura",
		"desligado ou fora da área",
		"encaminhada para a caixa",
		"correio de voz",
	}
	for _, p := range patterns {
		if strings.Contains(norm, p) {
			return true, fmt.Sprintf("Padrão de caixa postal identificado na fala: %q", p)
		}
	}
	return false, ""
}

// IsConfigured informa se há chave disponível (no agente ou global no servidor).
func (e *Evaluator) IsConfigured(overrideKey string) bool {
	if strings.TrimSpace(overrideKey) != "" {
		return true
	}
	if e == nil {
		return false
	}
	return strings.TrimSpace(e.OpenAIAPIKey) != "" ||
		strings.TrimSpace(e.OpenRouterAPIKey) != "" ||
		strings.TrimSpace(e.TypeSafeAPIKey) != ""
}

// Evaluate executa a avaliação System One no Guardião via OpenAI (`gpt-4.1-mini`),
// OpenRouter (`typesafe/jev-router`) ou TypeSafe nativa (`jev-latest`).
func (e *Evaluator) Evaluate(
	ctx context.Context,
	in EvalInput,
	overrideProvider string,
	overrideModel string,
	overrideAPIKey string,
) (EvalResult, error) {
	// 1. Checagem instantânea de frases inequívocas de caixa postal nas primeiras falas
	if in.TotalCallerTurns <= 3 && in.LastCallerTurn != "" {
		if hit, reason := FastVoicemailHeuristic(in.LastCallerTurn); hit {
			return EvalResult{
				Model:       "jev-fast-voicemail",
				IsVoicemail: 0.99,
				Reason:      reason,
			}, nil
		}
	}

	apiKey := strings.TrimSpace(overrideAPIKey)
	provider := strings.ToLower(strings.TrimSpace(overrideProvider))
	model := strings.TrimSpace(overrideModel)

	if apiKey != "" {
		if strings.HasPrefix(apiKey, "sk-or-") {
			provider = "openrouter"
		} else if strings.HasPrefix(apiKey, "sk-") {
			provider = "openai"
		} else if provider == "" {
			provider = "typesafe"
		}
	} else if e != nil {
		// Prioridade 1: OpenRouter Jev (`typesafe/jev-router`)
		if strings.TrimSpace(e.OpenRouterAPIKey) != "" {
			apiKey = strings.TrimSpace(e.OpenRouterAPIKey)
			provider = "openrouter"
		} else if strings.TrimSpace(e.TypeSafeAPIKey) != "" {
			// Prioridade 2: TypeSafe Jev nativo (`jev-latest`)
			apiKey = strings.TrimSpace(e.TypeSafeAPIKey)
			if strings.HasPrefix(apiKey, "sk-or-") {
				provider = "openrouter"
			} else {
				provider = "typesafe"
			}
		} else if strings.TrimSpace(e.OpenAIAPIKey) != "" {
			// Fallback caso apenas OPENAI_API_KEY esteja definida no ambiente
			apiKey = strings.TrimSpace(e.OpenAIAPIKey)
			provider = "openai"
		}
	}

	if apiKey == "" {
		return EvalResult{}, fmt.Errorf("nenhuma API key configurada para o Guardião Jev (defina OPENROUTER_API_KEY ou TYPESAFE_API_KEY)")
	}

	client := &http.Client{Timeout: 6 * time.Second}
	if e != nil && e.HTTPClient != nil {
		client = e.HTTPClient
	}

	if provider == "typesafe" && !strings.HasPrefix(apiKey, "sk-") {
		if model == "" || strings.Contains(model, "/") {
			model = "jev-latest"
		}
		return evaluateTypeSafeNative(ctx, client, apiKey, model, in)
	}

	if provider == "openai" {
		if model == "" || strings.Contains(model, "/") {
			model = DefaultOpenAIModel
		}
		return evaluateChatCompletions(ctx, client, OpenAIEndpoint, apiKey, model, in)
	}

	if model == "" {
		if e != nil && strings.TrimSpace(e.OpenRouterModel) != "" {
			model = strings.TrimSpace(e.OpenRouterModel)
		} else {
			model = DefaultModel
		}
	}
	return evaluateChatCompletions(ctx, client, OpenRouterEndpoint, apiKey, model, in)
}

func evaluateChatCompletions(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	apiKey string,
	model string,
	in EvalInput,
) (EvalResult, error) {
	stateBytes, _ := json.MarshalIndent(in, "", "  ")

	systemPrompt := `Você é o Jev (System One Guardian), o guardião de integridade em tempo real de chamadas telefônicas de IA.
Avalie o objeto JSON de estado (state) da chamada e retorne EXCLUSIVAMENTE um objeto JSON válido com as seguintes primitivas calibradas entre 0.0 e 1.0:
{
  "is_voicemail": <float 0.0 a 1.0>,
  "talking_to_self": <float 0.0 a 1.0>,
  "agent_hallucinating": <float 0.0 a 1.0>,
  "hallucination_severity": <float 0.0 a 2.0>,
  "reason": "<resumo curto em português do diagnóstico>",
  "correction_hint": "<se agent_hallucinating >= 0.70, instrução curta para o agente corrigir imediatamente>"
}

Regras de calibração (Noul / Score):
1. "is_voicemail" (0.0 a 1.0):
   - Próximo de 1.0 se a fala do cliente (last_caller_turn ou recent_transcript) for uma caixa postal, secretária eletrônica, URA ou mensagem automática de operadora (ex: "grave seu recado após o sinal", "caixa postal", "após o bip", "não está disponível", "deixe sua mensagem").
   - Próximo de 0.0 se for uma pessoa real interagindo ("Alô", "Sim", "Oi", etc.).
2. "talking_to_self" (0.0 a 1.0):
   - Próximo de 1.0 se o cliente estiver mudo/ausente (consecutive_agent_turns_without_caller >= 2 ou apenas ruídos curtos sem sentido no ASR) E o agente estiver falando sozinho, avançando no roteiro/oferta como se o cliente tivesse respondido, ou preso falando para o vazio.
   - Próximo de 0.0 se o cliente estiver participando normalmente ou se for apenas a saudação inicial da chamada.
3. "agent_hallucinating" (0.0 a 1.0):
   - Próximo de 1.0 se em last_agent_turn o agente:
     (a) mudou de assunto/idioma sem relação com agent_role_summary (ex: textos médicos em inglês, markdown, código),
     (b) contradisse os dados de client_variables (ex: falou CPF, nome ou valores diferentes dos declarados no prompt/variáveis),
     (c) inventou benefícios, preços ou informações fora do seu papel.
   - Próximo de 0.0 se o agente seguiu o roteiro em português do Brasil e respeitou as variáveis do cliente.
4. "hallucination_severity" (0.0 a 2.0):
   - 0.0: Sem alucinação.
   - 1.0: Desvio leve que pode ser corrigido na próxima fala.
   - 2.0: Alucinação severa, vazamento de prompt/raciocínio interno em outra língua ou perda total de contexto.`

	reqBody := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": string(stateBytes)},
		},
		"temperature": 0,
		"response_format": map[string]any{
			"type": "json_object",
		},
	}

	rawReq, err := json.Marshal(reqBody)
	if err != nil {
		return EvalResult{}, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(rawReq))
	if err != nil {
		return EvalResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "https://voiceagent.local")
	req.Header.Set("X-Title", "VoiceAgent Guardian Jev")

	resp, err := client.Do(req)
	if err != nil {
		return EvalResult{}, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return EvalResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return EvalResult{}, fmt.Errorf("openrouter HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	var orResp struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int     `json:"prompt_tokens"`
			CompletionTokens int     `json:"completion_tokens"`
			Cost             float64 `json:"cost"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBytes, &orResp); err != nil {
		return EvalResult{}, fmt.Errorf("decode openrouter response: %w", err)
	}
	if len(orResp.Choices) == 0 {
		return EvalResult{}, fmt.Errorf("openrouter retornou choices vazio")
	}

	content := cleanJSONBlock(orResp.Choices[0].Message.Content)
	var parsed EvalResult
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return EvalResult{}, fmt.Errorf("parse jev json (%s): %w", content, err)
	}
	if orResp.Model != "" {
		parsed.Model = orResp.Model
	} else {
		parsed.Model = model
	}
	parsed.InputTokens = orResp.Usage.PromptTokens
	parsed.OutputTokens = orResp.Usage.CompletionTokens
	parsed.CostUSD = orResp.Usage.Cost
	return parsed, nil
}

func evaluateTypeSafeNative(
	ctx context.Context,
	client *http.Client,
	apiKey string,
	model string,
	in EvalInput,
) (EvalResult, error) {
	reqBody := map[string]any{
		"model": model,
		"state": in,
		"questions": map[string]any{
			"is_voicemail": map[string]any{
				"type":         "noul",
				"instructions": "Does the caller's speech (`last_caller_turn` or `recent_transcript`) indicate an automated voicemail greeting, answering machine, or telecom carrier unavailable message?",
				"criteria": map[string]any{
					"true":  "Voicemail greeting ('caixa postal', 'grave seu recado após o sinal', 'após o bip', 'não está disponível')",
					"false": "A real human answering or speaking on the phone",
				},
			},
			"talking_to_self": map[string]any{
				"type":         "noul",
				"instructions": "Is the caller mute/absent (`consecutive_agent_turns_without_caller` >= 2 or only meaningless noise) while the agent keeps talking to itself and advancing the script without real caller participation?",
				"criteria": map[string]any{
					"true":  "Agent spoke multiple turns without meaningful caller response or advanced the sales pitch while caller is silent",
					"false": "Caller is actively participating and responding coherently",
				},
			},
			"agent_hallucinating": map[string]any{
				"type":         "noul",
				"instructions": "In `last_agent_turn`, did the agent hallucinate facts contradicting `client_variables`, switch to an unrelated topic/language (e.g., English medical text or markdown), or break `agent_role_summary`?",
				"criteria": map[string]any{
					"true":  "Agent leaked unrelated English/markdown text, contradicted `client_variables`, or invented unauthorized data",
					"false": "Agent stayed in character in Portuguese and respected `client_variables`",
				},
			},
			"hallucination_severity": map[string]any{
				"type":         "score",
				"instructions": "How severe is any hallucination or off-script deviation in `last_agent_turn`?",
				"criteria": []string{
					"No hallucination or deviation",
					"Minor deviation that can be corrected in the next turn",
					"Severe hallucination, wrong language/topic leak, or critical data fabrication",
				},
			},
		},
	}

	rawReq, err := json.Marshal(reqBody)
	if err != nil {
		return EvalResult{}, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, TypeSafeEndpoint, bytes.NewReader(rawReq))
	if err != nil {
		return EvalResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return EvalResult{}, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return EvalResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return EvalResult{}, fmt.Errorf("typesafe HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	var tsResp struct {
		Model   string `json:"model"`
		Answers map[string]struct {
			Type  string  `json:"type"`
			Noul  float64 `json:"noul"`
			Score float64 `json:"score"`
		} `json:"answers"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBytes, &tsResp); err != nil {
		return EvalResult{}, err
	}

	out := EvalResult{
		Model:                 tsResp.Model,
		IsVoicemail:           tsResp.Answers["is_voicemail"].Noul,
		TalkingToSelf:         tsResp.Answers["talking_to_self"].Noul,
		AgentHallucinating:    tsResp.Answers["agent_hallucinating"].Noul,
		HallucinationSeverity: tsResp.Answers["hallucination_severity"].Score,
		InputTokens:           tsResp.Usage.InputTokens,
		OutputTokens:          tsResp.Usage.OutputTokens,
		CostUSD:               float64(tsResp.Usage.InputTokens) * 0.000000042, // $0.042 / 1M input tokens
	}
	if out.IsVoicemail >= 0.75 {
		out.Reason = "Caixa postal ou mensagem automática detectada pelo Jev"
	} else if out.AgentHallucinating >= 0.70 {
		out.Reason = "Desvio de contexto ou alucinação detectada na última fala do agente"
	} else if out.TalkingToSelf >= 0.70 {
		out.Reason = "Cliente em silêncio enquanto o agente fala sozinho"
	} else {
		out.Reason = "Conversa dentro dos parâmetros normais"
	}
	return out, nil
}

func cleanJSONBlock(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx != -1 {
			s = s[idx+1:]
		}
		if last := strings.LastIndex(s, "```"); last != -1 {
			s = s[:last]
		}
	}
	return strings.TrimSpace(s)
}
