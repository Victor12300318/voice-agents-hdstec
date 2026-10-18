package recording

import (
	"bytes"
	"encoding/binary"
	"math"
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

	callerRem        []int16 // Resto (0 ou 1 amostra em 16kHz) para interpolação 2->3 (16kHz->24kHz)
	callerPrevSample int32   // Última amostra do chunk anterior para passo uniforme de 2/3 sem chiado de 8kHz
	hasCallerPrev    bool
	inAgentTurn      bool    // true enquanto o agente está transmitindo chunks contínuos de um mesmo turno
	lastCallerTime   time.Time
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

	s24 := upsample16to24Stateful(s16[:usable], &sr.callerPrevSample, &sr.hasCallerPrev)

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

	leftOut := sr.leftSamples
	if maxLen >= recordingSampleRate {
		leftOut = masterCallerTrack24k(sr.leftSamples)
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
		_ = binary.Write(&buf, binary.LittleEndian, leftOut[i])
		_ = binary.Write(&buf, binary.LittleEndian, sr.rightSamples[i])
	}

	return buf.Bytes()
}

// masterCallerTrack24k aplica pós-processamento de estúdio estilo ElevenLabs na trilha gravada do Caller:
// 1. Filtro High-Pass IIR de 1ª ordem (~80 Hz) para remover DC offset e grave abafado de linha.
// 2. Filtro De-Hiss de fase linear (5 taps) para eliminar chiado de alta frequência.
// 3. Envelope Noise Suppressor suave + Auto-Leveling AGC (alvo -4 dBFS / 20.000) para equiparar
//    o volume da voz do cliente ao volume da voz do Agente sem distorção.
func masterCallerTrack24k(in []int16) []int16 {
	n := len(in)
	if n < 16 {
		return in
	}

	// Passo 1: High-Pass (~80 Hz @ 24kHz) + De-Hiss FIR
	filtered := make([]float64, n)
	hpAlpha := 0.979 // ~80 Hz em 24000 Hz
	var prevIn, prevHP float64
	for i := 0; i < n; i++ {
		x := float64(in[i])
		hp := hpAlpha * (prevHP + x - prevIn)
		prevIn = x
		prevHP = hp
		filtered[i] = hp
	}

	smoothed := make([]float64, n)
	var peak float64
	for i := 2; i < n-2; i++ {
		v := 0.10*filtered[i-2] + 0.24*filtered[i-1] + 0.32*filtered[i] + 0.24*filtered[i+1] + 0.10*filtered[i+2]
		smoothed[i] = v
		av := math.Abs(v)
		if av > peak {
			peak = av
		}
	}

	// Passo 2: Calcula ganho automático de normalização (AGC de gravação)
	targetPeak := 20000.0
	gain := 1.0
	if peak > 350.0 && peak < targetPeak {
		gain = targetPeak / peak
		if gain > 3.2 {
			gain = 3.2
		}
	}

	// Passo 3: Envelope follower suave (5ms attack, 90ms release) para silenciar chiado de fundo entre falas
	out := make([]int16, n)
	var env float64
	attackCoeff := 0.02
	releaseCoeff := 0.0008
	noiseFloor := 85.0

	for i := 0; i < n; i++ {
		absSample := math.Abs(smoothed[i])
		if absSample > env {
			env += attackCoeff * (absSample - env)
		} else {
			env += releaseCoeff * (absSample - env)
		}

		gate := 1.0
		if env < noiseFloor {
			ratio := env / noiseFloor
			gate = ratio * ratio * 0.25
		}

		val := smoothed[i] * gain * gate
		if val > 23000.0 {
			val = 23000.0 + 8500.0*math.Tanh((val-23000.0)/8500.0)
		} else if val < -23000.0 {
			val = -23000.0 - 8500.0*math.Tanh((-val-23000.0)/8500.0)
		}
		if val > 32500.0 {
			val = 32500.0
		} else if val < -32500.0 {
			val = -32500.0
		}
		out[i] = int16(math.Round(val))
	}

	return out
}

// upsample16to24Stateful converte 16kHz -> 24kHz (2 amostras -> 3 amostras) com espaçamento temporal
// estritamente uniforme de 2/3 de amostra usando a última amostra do par/chunk anterior (sPrev),
// eliminando 100% do chiado de aliasing de 8kHz na faixa gravada do Caller.
func upsample16to24Stateful(in []int16, prevSample *int32, hasPrev *bool) []int16 {
	numPairs := len(in) / 2
	if numPairs == 0 {
		return nil
	}
	out := make([]int16, numPairs*3)
	sPrev := int32(in[0])
	if prevSample != nil && hasPrev != nil && *hasPrev {
		sPrev = *prevSample
	}

	for i := 0; i < numPairs; i++ {
		s0 := int32(in[i*2])
		s1 := int32(in[i*2+1])
		out[i*3] = int16((sPrev + 2*s0) / 3)
		out[i*3+1] = int16((2*s0 + s1) / 3)
		out[i*3+2] = int16(s1)
		sPrev = s1
	}

	if prevSample != nil && hasPrev != nil {
		*prevSample = sPrev
		*hasPrev = true
	}
	return out
}
