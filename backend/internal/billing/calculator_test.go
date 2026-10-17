package billing_test

import (
	"math"
	"testing"

	"voiceagent/internal/billing"
	"voiceagent/internal/voice"
)

func TestCalculateGeminiTokens(t *testing.T) {
	usage := voice.Usage{
		TotalTokens:       10000,
		InputTextTokens:   2000,
		InputAudioTokens:  4000,
		OutputTextTokens:  1000,
		OutputAudioTokens: 3000,
	}

	res := billing.Calculate("gemini_live", "gemini-3.8-live", usage, 30000)
	if res.Method != "tokens" {
		t.Fatalf("esperava method tokens, recebeu %s", res.Method)
	}

	expectedInputText := (2000.0 / 1e6) * 0.75   // 0.0015
	expectedInputAudio := (4000.0 / 1e6) * 3.00  // 0.012
	expectedOutputText := (1000.0 / 1e6) * 4.50  // 0.0045
	expectedOutputAudio := (3000.0 / 1e6) * 12.0 // 0.036
	expectedTotal := expectedInputText + expectedInputAudio + expectedOutputText + expectedOutputAudio

	if math.Abs(res.TotalCostUSD-expectedTotal) > 1e-9 {
		t.Errorf("esperava total USD %v, obteve %v", expectedTotal, res.TotalCostUSD)
	}
	if math.Abs(res.TotalCostBRL-expectedTotal*billing.DefaultUSDtoBRL) > 1e-9 {
		t.Errorf("esperava total BRL %v, obteve %v", expectedTotal*billing.DefaultUSDtoBRL, res.TotalCostBRL)
	}
}

func TestCalculateDurationFallback(t *testing.T) {
	// 2 minutos de conversa sem tokens
	res := billing.Calculate("gemini_live", "gemini-3.8-live", voice.Usage{}, 120000)
	if res.Method != "duration_estimate" {
		t.Fatalf("esperava method duration_estimate, recebeu %s", res.Method)
	}
	// 2 min * 0.023 = 0.046
	expectedUSD := 2.0 * 0.023
	if res.TotalCostUSD != expectedUSD {
		t.Errorf("esperava %v, obteve %v", expectedUSD, res.TotalCostUSD)
	}
}
