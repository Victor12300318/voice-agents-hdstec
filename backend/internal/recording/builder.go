package recording

import (
	"bytes"
	"encoding/binary"
	"sync"
	"time"
)

// recordingSampleRate é 24000 Hz (taxa nativa de saída do Gemini Live e OpenAI Realtime).
// Manter o WAV em 24000 Hz preserva o áudio do Agente 100% bit-a-bit original, sem qualquer
// downsampling ou filtragem, eliminando chiados e rouquidão.
const recordingSampleRate = 24000

type SessionRecorder struct {
	mu        sync.Mutex
	startedAt time.Time

	leftSamples  []int16 // Caller (convertido de 16kHz para 24kHz)
	rightSamples []int16 // Agent (24kHz nativo, bit-a-bit intocado)

	callerRem      []int16 // Resto (0 ou 1 amostra em 16kHz) para interpolação 2->3 (16kHz->24kHz)
	inAgentTurn    bool    // true enquanto o agente está transmitindo chunks contínuos de um mesmo turno
	lastCallerTime time.Time
}

func NewSessionRecorder() *SessionRecorder {
	return &SessionRecorder{
		startedAt:    time.Now(),
		leftSamples:  make([]int16, 0, recordingSampleRate*30),
		rightSamples: make([]int16, 0, recordingSampleRate*30),
	}
}

func (sr *SessionRecorder) SetStartTime(t time.Time) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.startedAt = t
}

func (sr *SessionRecorder) AddCallerAudio(pcm16 []byte) {
	sr.AddCallerAudioAt(pcm16, time.Now())
}

func (sr *SessionRecorder) AddCallerAudioAt(pcm16 []byte, at time.Time) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	num16 := len(pcm16) / 2
	if num16 == 0 {
		return
	}

	s16 := make([]int16, len(sr.callerRem)+num16)
	copy(s16, sr.callerRem)
	base := len(sr.callerRem)
	for i := 0; i < num16; i++ {
		s16[base+i] = int16(binary.LittleEndian.Uint16(pcm16[i*2 : i*2+2]))
	}

	numPairs := len(s16) / 2
	usable := numPairs * 2
	sr.callerRem = append(sr.callerRem[:0], s16[usable:]...)
	if numPairs == 0 {
		return
	}

	s24 := upsample16to24(s16[:usable])

	// Só preenche silêncio no início da chamada ou após pausa > 300ms entre envios do Caller
	offset := int(at.Sub(sr.startedAt).Seconds() * float64(recordingSampleRate))
	if offset < 0 {
		offset = 0
	}
	if len(sr.leftSamples) == 0 || (!sr.lastCallerTime.IsZero() && at.Sub(sr.lastCallerTime) > 300*time.Millisecond) {
		for len(sr.leftSamples) < offset {
			sr.leftSamples = append(sr.leftSamples, 0)
		}
	}
	sr.lastCallerTime = at
	sr.leftSamples = append(sr.leftSamples, s24...)
}

func (sr *SessionRecorder) AddAgentAudio(pcm24kHz []byte) {
	sr.AddAgentAudioAt(pcm24kHz, time.Now())
}

func (sr *SessionRecorder) AddAgentAudioAt(pcm24kHz []byte, at time.Time) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	num24 := len(pcm24kHz) / 2
	if num24 == 0 {
		return
	}

	// Copia as amostras 24kHz nativas do provedor 100% intocadas (bit-exact)
	s24 := make([]int16, num24)
	for i := 0; i < num24; i++ {
		s24[i] = int16(binary.LittleEndian.Uint16(pcm24kHz[i*2 : i*2+2]))
	}

	// Apenas no 1º chunk do turno do agente alinhamos com o relógio da conversa.
	// Todos os chunks seguintes do mesmo turno são concatenados de forma 100% contígua,
	// sem qualquer buraco de zeros nem reamostragem.
	if !sr.inAgentTurn {
		offset := int(at.Sub(sr.startedAt).Seconds() * float64(recordingSampleRate))
		if offset < 0 {
			offset = 0
		}
		for len(sr.rightSamples) < offset {
			sr.rightSamples = append(sr.rightSamples, 0)
		}
		sr.inAgentTurn = true
	}

	sr.rightSamples = append(sr.rightSamples, s24...)
}

// EndAgentTurn sinaliza que o turno atual de fala do agente terminou,
// para que o próximo turno alinhe seu início com o relógio real da sessão.
func (sr *SessionRecorder) EndAgentTurn() {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.inAgentTurn = false
}

func (sr *SessionRecorder) TruncateAgentOnInterruption() {
	sr.TruncateAgentAt(time.Now())
}

func (sr *SessionRecorder) TruncateAgentAt(at time.Time) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	limit := int(at.Sub(sr.startedAt).Seconds() * float64(recordingSampleRate))
	if limit < 0 {
		limit = 0
	}
	if len(sr.rightSamples) > limit {
		sr.rightSamples = sr.rightSamples[:limit]
	}
	sr.inAgentTurn = false
}

// BuildWAV gera o arquivo binário WAV Estéreo completo (24kHz, 16-bit, 2 canais).
func (sr *SessionRecorder) BuildWAV() []byte {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	maxLen := len(sr.leftSamples)
	if len(sr.rightSamples) > maxLen {
		maxLen = len(sr.rightSamples)
	}
	if maxLen == 0 {
		return nil
	}

	for len(sr.leftSamples) < maxLen {
		sr.leftSamples = append(sr.leftSamples, 0)
	}
	for len(sr.rightSamples) < maxLen {
		sr.rightSamples = append(sr.rightSamples, 0)
	}

	var buf bytes.Buffer

	sampleRate := uint32(recordingSampleRate)
	numChannels := uint16(2)
	bitsPerSample := uint16(16)
	byteRate := sampleRate * uint32(numChannels) * uint32(bitsPerSample/8)
	blockAlign := numChannels * (bitsPerSample / 8)
	dataSize := uint32(maxLen * int(blockAlign))
	chunkSize := 36 + dataSize

	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, chunkSize)
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, numChannels)
	_ = binary.Write(&buf, binary.LittleEndian, sampleRate)
	_ = binary.Write(&buf, binary.LittleEndian, byteRate)
	_ = binary.Write(&buf, binary.LittleEndian, blockAlign)
	_ = binary.Write(&buf, binary.LittleEndian, bitsPerSample)
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, dataSize)

	for i := 0; i < maxLen; i++ {
		_ = binary.Write(&buf, binary.LittleEndian, sr.leftSamples[i])
		_ = binary.Write(&buf, binary.LittleEndian, sr.rightSamples[i])
	}

	return buf.Bytes()
}

// upsample16to24 converte 16kHz -> 24kHz (2 amostras -> 3 amostras) por interpolação linear suave.
func upsample16to24(in []int16) []int16 {
	numPairs := len(in) / 2
	if numPairs == 0 {
		return nil
	}
	out := make([]int16, numPairs*3)
	for i := 0; i < numPairs; i++ {
		s0 := int32(in[i*2])
		s1 := int32(in[i*2+1])
		out[i*3] = int16(s0)
		out[i*3+1] = int16((s0 + s1) / 2)
		out[i*3+2] = int16(s1)
	}
	return out
}
