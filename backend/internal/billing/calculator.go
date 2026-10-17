package billing

import (
	"strings"

	"voiceagent/internal/voice"
)

// DefaultUSDtoBRL é a taxa de conversão padrão para Real Brasileiro.
const DefaultUSDtoBRL = 5.75

// CostResult traz o cálculo detalhado de custos da conversa.
type CostResult struct {
	TotalCostUSD       float64     `json:"total_cost_usd"`
	TotalCostBRL       float64     `json:"total_cost_brl"`
	InputAudioCostUSD  float64     `json:"input_audio_cost_usd"`
	OutputAudioCostUSD float64     `json:"output_audio_cost_usd"`
	InputTextCostUSD   float64     `json:"input_text_cost_usd"`
	OutputTextCostUSD  float64     `json:"output_text_cost_usd"`
	TokensUsage        voice.Usage `json:"tokens_usage"`
	Method             string      `json:"method"` // "tokens" ou "duration_estimate"
}

// Calculate calcula o custo de uma conversa a partir dos tokens ou duração.
func Calculate(provider, model string, usage voice.Usage, durationMS int64) CostResult {
	res := CostResult{
		TokensUsage: usage,
		Method:      "tokens",
	}

	modelLower := strings.ToLower(model)
	providerLower := strings.ToLower(provider)

	// Se houver tokens registrados, calcula via tokens exatos
	if usage.TotalTokens > 0 || usage.InputAudioTokens > 0 || usage.OutputAudioTokens > 0 {
		switch {
		case providerLower == "gemini_live" || strings.Contains(modelLower, "gemini"):
			// Preços oficiais Gemini 3.8 Live / Gemini Live API (ai.google.dev/pricing):
			// Input Text: $0.75 / 1M
			// Input Audio: $3.00 / 1M
			// Output Text: $4.50 / 1M
			// Output Audio: $12.00 / 1M
			res.InputTextCostUSD = (float64(usage.InputTextTokens) / 1_000_000.0) * 0.75
			res.InputAudioCostUSD = (float64(usage.InputAudioTokens) / 1_000_000.0) * 3.00
			res.OutputTextCostUSD = (float64(usage.OutputTextTokens) / 1_000_000.0) * 4.50
			res.OutputAudioCostUSD = (float64(usage.OutputAudioTokens) / 1_000_000.0) * 12.00

		case strings.Contains(modelLower, "mini"):
			// OpenAI Realtime Mini (gpt-4o-mini-realtime-preview, gpt-realtime-mini):
			// Input Text: $0.60 / 1M | Audio Input: $10.00 / 1M ($0.30 cached)
			// Output Text: $2.40 / 1M | Audio Output: $20.00 / 1M
			res.InputTextCostUSD = (float64(usage.InputTextTokens) / 1_000_000.0) * 0.60
			res.InputAudioCostUSD = (float64(usage.InputAudioTokens) / 1_000_000.0) * 10.00
			res.OutputTextCostUSD = (float64(usage.OutputTextTokens) / 1_000_000.0) * 2.40
			res.OutputAudioCostUSD = (float64(usage.OutputAudioTokens) / 1_000_000.0) * 20.00

		default:
			// OpenAI Realtime Full (gpt-4o-realtime-preview, gpt-realtime-2.1):
			// Input Text: $4.00 / 1M | Audio Input: $32.00 / 1M
			// Output Text: $24.00 / 1M | Audio Output: $64.00 / 1M
			res.InputTextCostUSD = (float64(usage.InputTextTokens) / 1_000_000.0) * 4.00
			res.InputAudioCostUSD = (float64(usage.InputAudioTokens) / 1_000_000.0) * 32.00
			res.OutputTextCostUSD = (float64(usage.OutputTextTokens) / 1_000_000.0) * 24.00
			res.OutputAudioCostUSD = (float64(usage.OutputAudioTokens) / 1_000_000.0) * 64.00
		}

		res.TotalCostUSD = res.InputTextCostUSD + res.InputAudioCostUSD + res.OutputTextCostUSD + res.OutputAudioCostUSD
		res.TotalCostBRL = res.TotalCostUSD * DefaultUSDtoBRL
		return res
	}

	// Fallback por estimativa de duração (minutos) caso tokens venham zerados
	res.Method = "duration_estimate"
	durationMinutes := float64(durationMS) / 60000.0
	if durationMinutes <= 0 {
		return res
	}

	var ratePerMinute float64
	switch {
	case providerLower == "gemini_live" || strings.Contains(modelLower, "gemini"):
		// Gemini Live: ~$0.005/min áudio in + $0.018/min áudio out = ~$0.023/min
		ratePerMinute = 0.023
	case strings.Contains(modelLower, "gpt-live"):
		// OpenAI GPT-Live 1: $0.05/min (billed per second)
		ratePerMinute = 0.05
	case strings.Contains(modelLower, "mini"):
		// OpenAI Realtime Mini: ~$0.04/min
		ratePerMinute = 0.04
	default:
		// OpenAI Realtime Standard: ~$0.15/min
		ratePerMinute = 0.15
	}

	res.TotalCostUSD = durationMinutes * ratePerMinute
	res.TotalCostBRL = res.TotalCostUSD * DefaultUSDtoBRL
	return res
}
