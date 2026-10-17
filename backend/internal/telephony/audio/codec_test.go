package audio

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestMuLawRoundTrip(t *testing.T) {
	// Testa todos os 256 valores de μ-law possíveis
	for i := 0; i < 256; i++ {
		origByte := byte(i)
		linear := DecodeMuLawSample(origByte)
		encoded := EncodeMuLawSample(linear)

		// Na decodificação e re-codificação de tabela μ-law padrão, o byte pode diferir no máximo em 1 por quantização
		decodedAgain := DecodeMuLawSample(encoded)
		diff := math.Abs(float64(linear - decodedAgain))
		if diff > 100 { // erro relativo mínimo esperado para sinal de 16-bit
			t.Errorf("Roundtrip byte %d: linear=%d, encoded=%d, decodedAgain=%d, diff=%f",
				origByte, linear, encoded, decodedAgain, diff)
		}
	}
}

func TestResamplePCM16(t *testing.T) {
	// 80 amostras a 8000 Hz = 10ms
	inSamples := make([]byte, 160)
	for i := 0; i < 80; i++ {
		// Onda senoidal de 400Hz
		val := int16(10000 * math.Sin(2*math.Pi*400*float64(i)/8000))
		binary.LittleEndian.PutUint16(inSamples[i*2:], uint16(val))
	}

	// 8000 -> 16000 (deve dobrar o número de amostras: 80 -> 160 amostras = 320 bytes)
	out16k := ResamplePCM16(inSamples, 8000, 16000)
	if len(out16k) != 320 {
		t.Fatalf("Resample 8k->16k: expected 320 bytes, got %d", len(out16k))
	}

	// 16000 -> 8000 (deve voltar para 160 bytes)
	out8k := ResamplePCM16(out16k, 16000, 8000)
	if len(out8k) != 160 {
		t.Fatalf("Resample 16k->8k: expected 160 bytes, got %d", len(out8k))
	}

	// 24000 -> 8000 (3:1 ratio)
	in24k := make([]byte, 480) // 240 amostras = 10ms
	outFrom24k := ResamplePCM16(in24k, 24000, 8000)
	if len(outFrom24k) != 160 { // 80 amostras = 160 bytes
		t.Fatalf("Resample 24k->8k: expected 160 bytes, got %d", len(outFrom24k))
	}
}

func TestTelephonyPipeline(t *testing.T) {
	// 160 bytes de μ-law 8kHz (20ms telefônico padrão)
	muLaw := make([]byte, 160)
	for i := range muLaw {
		muLaw[i] = byte(i)
	}

	// Conversão para Gemini (16kHz PCM16): 160 amostras -> 320 amostras PCM16 = 640 bytes
	geminiInput := TelephonyToProvider(muLaw, 16000)
	if len(geminiInput) != 640 {
		t.Fatalf("TelephonyToProvider 16k: expected 640 bytes, got %d", len(geminiInput))
	}

	// Conversão de saída do Gemini (24kHz PCM16, ex: 960 bytes = 480 amostras = 20ms) para telefonia (160 bytes)
	geminiOutput := make([]byte, 960)
	telephonyMuLaw := ProviderToTelephony(geminiOutput, 24000)
	if len(telephonyMuLaw) != 160 {
		t.Fatalf("ProviderToTelephony 24k->8k: expected 160 bytes, got %d", len(telephonyMuLaw))
	}
}
