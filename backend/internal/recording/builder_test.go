package recording

import (
	"encoding/binary"
	"testing"
	"time"
)

func TestSessionRecorderBuildWAV(t *testing.T) {
	rec := NewSessionRecorder()
	t0 := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	rec.SetStartTime(t0)

	// 100 amostras do Caller a partir de t0 (16kHz -> 150 amostras a 24kHz)
	callerPCM := make([]byte, 200)
	for i := 0; i < 100; i++ {
		binary.LittleEndian.PutUint16(callerPCM[i*2:i*2+2], uint16(1000+i))
	}
	rec.AddCallerAudioAt(callerPCM, t0)

	// 150 amostras do Agent a partir de t0 (24kHz nativas)
	agentPCM := make([]byte, 300)
	for i := 0; i < 150; i++ {
		binary.LittleEndian.PutUint16(agentPCM[i*2:i*2+2], uint16(2000+i))
	}
	rec.AddAgentAudioAt(agentPCM, t0)

	wav := rec.BuildWAV()
	if len(wav) < 44 {
		t.Fatalf("tamanho inválido de WAV: %d", len(wav))
	}

	if string(wav[0:4]) != "RIFF" || string(wav[8:12]) != "WAVE" || string(wav[12:16]) != "fmt " {
		t.Fatalf("cabeçalho RIFF/WAVE inválido: %s", string(wav[:16]))
	}

	channels := binary.LittleEndian.Uint16(wav[22:24])
	if channels != 2 {
		t.Fatalf("esperava 2 canais (estéreo), veio %d", channels)
	}

	sampleRate := binary.LittleEndian.Uint32(wav[24:28])
	if sampleRate != 24000 {
		t.Fatalf("esperava taxa de 24000 Hz, veio %d", sampleRate)
	}

	// Verifica interleaving
	dataOffset := 44
	firstLeft := int16(binary.LittleEndian.Uint16(wav[dataOffset : dataOffset+2]))
	firstRight := int16(binary.LittleEndian.Uint16(wav[dataOffset+2 : dataOffset+4]))

	if firstLeft != 1000 {
		t.Fatalf("amostra do Caller incorreta: %d", firstLeft)
	}
	if firstRight != 2000 {
		t.Fatalf("amostra do Agent incorreta: %d", firstRight)
	}
}

func TestInterruptionTruncate(t *testing.T) {
	rec := NewSessionRecorder()
	t0 := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	rec.SetStartTime(t0)

	// Caller fala 1 segundo (16000 amostras -> 24000 amostras a 24kHz)
	rec.AddCallerAudioAt(make([]byte, 32000), t0)

	// Agent envia 5 segundos de áudio adiantado (24kHz * 5s = 120.000 amostras)
	rec.AddAgentAudioAt(make([]byte, 240000), t0)

	// Interrupção no segundo 2: descarta áudio do agente além do segundo 2
	tInterrupcao := t0.Add(2 * time.Second)
	rec.TruncateAgentAt(tInterrupcao)

	wav := rec.BuildWAV()
	dataLen := len(wav) - 44
	// Segundo 2 a 24kHz estéreo = 2 * 24000 * 4 bytes = 192.000 bytes
	if dataLen != 192000 {
		t.Fatalf("esperava 192000 bytes após truncar no segundo 2, veio %d", dataLen)
	}
}
