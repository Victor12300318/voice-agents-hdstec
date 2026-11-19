package audio

import (
	"encoding/binary"
	"math"
)

// Kernel FIR passa-baixas de 45 taps (janela Blackman-Harris de 4 termos, >72 dB de rejeição de stopband)
// para decimação 24.000 Hz -> 8.000 Hz com corte em 3.580 Hz, preservando brilho vocal sem aliasing.
var (
	lowPass45_24kTo8k = buildBlackmanHarrisLowPass(45, 3580.0/24000.0, 1.0)
	lowPass31_16kTo8k = buildBlackmanHarrisLowPass(31, 3580.0/16000.0, 1.0)

	// Sub-filtros polifásicos de 16 taps por fase no domínio de 8kHz para upsampling 8kHz -> 16kHz (1:2)
	// e 8kHz -> 24kHz (1:3) em passo único, sem degraus lineares e sem cliques de fronteira.
	polyUp8kTo16k = buildPolyphaseUpsampler(16, 2, 3580.0/16000.0)
	polyUp8kTo24k = buildPolyphaseUpsampler(16, 3, 3580.0/24000.0)
)

func buildBlackmanHarrisLowPass(taps int, normCutoff, gain float64) []float64 {
	kernel := make([]float64, taps)
	center := float64(taps-1) / 2.0
	var sum float64
	for i := 0; i < taps; i++ {
		n := float64(i) - center
		var sinc float64
		if math.Abs(n) < 1e-9 {
			sinc = 2.0 * normCutoff
		} else {
			sinc = math.Sin(2.0*math.Pi*normCutoff*n) / (math.Pi * n)
		}
		// Janela de Blackman-Harris de 4 termos (~92 dB sidelobe attenuation)
		wArg := 2.0 * math.Pi * float64(i) / float64(taps-1)
		window := 0.35875 -
			0.48829*math.Cos(wArg) +
			0.14128*math.Cos(2.0*wArg) -
			0.01168*math.Cos(3.0*wArg)
		val := sinc * window
		kernel[i] = val
		sum += val
	}
	if sum != 0 {
		scale := gain / sum
		for i := range kernel {
			kernel[i] *= scale
		}
	}
	return kernel
}

// buildPolyphaseUpsampler decompõe um filtro protótipo de (tapsPerPhase * factor) coeficientes
// em `factor` sub-filtros normalizados (DC gain = 1.0 em cada fase) aplicados sobre a linha de atraso 8kHz.
func buildPolyphaseUpsampler(tapsPerPhase, factor int, normCutoffHighRate float64) [][]float64 {
	totalTaps := tapsPerPhase * factor
	proto := buildBlackmanHarrisLowPass(totalTaps, normCutoffHighRate, float64(factor))
	phases := make([][]float64, factor)
	for p := 0; p < factor; p++ {
		sub := make([]float64, tapsPerPhase)
		var phaseSum float64
		// Convolução causal: delay[tapsPerPhase-1] é a amostra 8kHz mais recente (k=0)
		for k := 0; k < tapsPerPhase; k++ {
			tapIdx := k*factor + p
			val := proto[totalTaps-1-tapIdx]
			sub[tapsPerPhase-1-k] = val
			phaseSum += val
		}
		if phaseSum != 0 {
			for k := range sub {
				sub[k] /= phaseSum
			}
		}
		phases[p] = sub
	}
	return phases
}

// StreamDSP mantém todo o estado contínuo de filtragem e reamostragem bidirecional de uma chamada
// telefônica (PSTN/SIP G.711 μ-law 8kHz <-> Provider PCM16 16kHz/24kHz), eliminando 100% dos ruídos
// de fronteira de pacotes de 20ms, distorção de cruzamento por zero e aliasing.
type StreamDSP struct {
	// Estado Uplink (Caller 8kHz -> Provider 16kHz/24kHz)
	inRawPrev float64
	inHPPrev  float64
	inEnv     float64
	upDelay   [16]float64
	upInit    bool

	// Estado Downlink (Provider 24kHz/16kHz -> Caller 8kHz μ-law)
	outRawPrev  float64
	outHPPrev   float64
	outPrePrev  float64
	downBuf     []float64
	downInit    bool
	ditherState uint32
}

// ResetOutbound limpa o buffer de saída do agente imediatamente em caso de barge-in / interrupção.
func (d *StreamDSP) ResetOutbound() {
	d.outRawPrev = 0
	d.outHPPrev = 0
	d.outPrePrev = 0
	d.downBuf = d.downBuf[:0]
	d.downInit = false
}

// InboundMuLawToPCM decodifica G.711 μ-law 8kHz do Caller, remove DC/hum de 60Hz, aplica
// expander suave por envelope (sem distorção de cruzamento por zero) e faz upsampling polifásico.
func (d *StreamDSP) InboundMuLawToPCM(muLaw8k []byte, targetSampleRate int) []byte {
	n8 := len(muLaw8k)
	if n8 == 0 {
		return nil
	}

	cleaned8k := make([]float64, n8)
	const hpAlpha8k = 0.933 // ~90 Hz @ 8000 Hz
	const attack = 0.08     // ~1.5 ms @ 8000 Hz (preserva consoantes plosivas p, t, c, d)
	const release = 0.0025  // ~50 ms @ 8000 Hz (sem bombeamento entre sílabas)
	const noiseFloor = 58.0 // Piso típico de quantização G.711 em linha silenciosa

	for i, b := range muLaw8k {
		x := float64(muLawToLinear[b])
		hp := hpAlpha8k * (d.inHPPrev + x - d.inRawPrev)
		d.inRawPrev = x
		d.inHPPrev = hp

		absHP := math.Abs(hp)
		if absHP > d.inEnv {
			d.inEnv += attack * (absHP - d.inEnv)
		} else {
			d.inEnv += release * (absHP - d.inEnv)
		}

		// Expander contínuo baseado no envelope (nunca por amostra instantânea, evitando distorção harmônica)
		gate := 1.0
		if d.inEnv < noiseFloor {
			r := d.inEnv / noiseFloor
			gate = 0.12 + 0.88*r*r
		}

		val := hp * 1.50 * gate
		if val > 22000.0 {
			val = 22000.0 + 9500.0*math.Tanh((val-22000.0)/9500.0)
		} else if val < -22000.0 {
			val = -22000.0 - 9500.0*math.Tanh((-val-22000.0)/9500.0)
		}
		cleaned8k[i] = val
	}

	if targetSampleRate == 8000 {
		out := make([]byte, n8*2)
		for i, v := range cleaned8k {
			binary.LittleEndian.PutUint16(out[i*2:], uint16(clampPCM16(v)))
		}
		return out
	}

	var phases [][]float64
	var factor int
	switch targetSampleRate {
	case 16000:
		phases = polyUp8kTo16k
		factor = 2
	case 24000:
		phases = polyUp8kTo24k
		factor = 3
	default:
		raw8k := make([]byte, n8*2)
		for i, v := range cleaned8k {
			binary.LittleEndian.PutUint16(raw8k[i*2:], uint16(clampPCM16(v)))
		}
		return ResamplePCM16(raw8k, 8000, targetSampleRate)
	}

	if !d.upInit {
		for i := range d.upDelay {
			d.upDelay[i] = cleaned8k[0]
		}
		d.upInit = true
	}

	out := make([]byte, n8*factor*2)
	outIdx := 0
	for _, x := range cleaned8k {
		copy(d.upDelay[0:15], d.upDelay[1:16])
		d.upDelay[15] = x

		for p := 0; p < factor; p++ {
			sub := phases[p]
			var acc float64
			for k := 0; k < 16; k++ {
				acc += d.upDelay[k] * sub[k]
			}
			binary.LittleEndian.PutUint16(out[outIdx:], uint16(clampPCM16(acc)))
			outIdx += 2
		}
	}
	return out
}

// OutboundPCMToMuLaw condicionada o áudio PCM16 do Agente (high-pass 110Hz + presença vocal 2.6kHz),
// aplica decimação FIR Blackman-Harris 100% causal sem truncamento de borda e codifica em G.711 μ-law 8kHz.
func (d *StreamDSP) OutboundPCMToMuLaw(pcmData []byte, providerSampleRate int) []byte {
	numIn := len(pcmData) / 2
	if numIn == 0 {
		return nil
	}

	if providerSampleRate == 8000 {
		out := make([]byte, numIn)
		for i := 0; i < numIn; i++ {
			s := int16(binary.LittleEndian.Uint16(pcmData[i*2:]))
			out[i] = EncodeMuLawSample(s)
		}
		return out
	}

	var kernel []float64
	var decim int
	var hpAlpha float64
	switch providerSampleRate {
	case 24000:
		kernel = lowPass45_24kTo8k
		decim = 3
		hpAlpha = 0.9716 // ~110 Hz @ 24kHz
	case 16000:
		kernel = lowPass31_16kTo8k
		decim = 2
		hpAlpha = 0.9577 // ~110 Hz @ 16kHz
	default:
		pcm8k := ResamplePCM16(pcmData, providerSampleRate, 8000)
		return EncodeMuLaw(pcm8k)
	}

	taps := len(kernel)
	if !d.downInit {
		if d.ditherState == 0 {
			d.ditherState = 0x6D2B79F5
		}
		first := float64(int16(binary.LittleEndian.Uint16(pcmData[0:2])))
		d.outRawPrev = first
		d.outPrePrev = first
		// Pré-carrega (taps - 1) amostras na linha de atraso para que N amostras de entrada gerem
		// exatamente N/decim amostras de saída desde o primeiro pacote sem nunca grampear bordas!
		d.downBuf = append(d.downBuf[:0], make([]float64, taps-1)...)
		d.downInit = true
	}

	for i := 0; i < numIn; i++ {
		x := float64(int16(binary.LittleEndian.Uint16(pcmData[i*2:])))
		// 1. High-Pass @ 110Hz (remove sub-grave inútil na telefonia que satura passos do μ-law)
		hp := hpAlpha * (d.outHPPrev + x - d.outRawPrev)
		d.outRawPrev = x
		d.outHPPrev = hp

		// 2. Realce leve de presença vocal (+2.0 dB acima de 2kHz) para dicção cristalina no telefone
		emph := hp + 0.22*(hp-d.outPrePrev)
		d.outPrePrev = hp

		d.downBuf = append(d.downBuf, emph)
	}

	avail := len(d.downBuf) - taps + 1
	if avail <= 0 {
		return nil
	}
	numOut := (avail + decim - 1) / decim
	out := make([]byte, numOut)

	pos := 0
	for m := 0; m < numOut; m++ {
		var acc float64
		window := d.downBuf[pos : pos+taps]
		for k := 0; k < taps; k++ {
			acc += window[k] * kernel[k]
		}

		// Dither TPDF mínimo (±2 unidades em 16-bit) para suavizar caudas silenciosas no G.711
		acc += d.nextTPDFDither() * 2.0

		// Soft-knee limiter em 29.000 para nunca clipar no teto de 32.635 do G.711 μ-law
		if acc > 29000.0 {
			acc = 29000.0 + 3200.0*math.Tanh((acc-29000.0)/3200.0)
		} else if acc < -29000.0 {
			acc = -29000.0 - 3200.0*math.Tanh((-acc-29000.0)/3200.0)
		}

		out[m] = EncodeMuLawSample(clampPCM16(acc))
		pos += decim
	}

	// Mantém o histórico exato não consumido para continuidade perfeita no próximo pacote
	remaining := len(d.downBuf) - pos
	copy(d.downBuf[0:remaining], d.downBuf[pos:])
	d.downBuf = d.downBuf[:remaining]

	return out
}

func (d *StreamDSP) nextTPDFDither() float64 {
	d.ditherState = d.ditherState*1664525 + 1013904223
	r1 := float64(d.ditherState&0xFFFF) / 65535.0
	d.ditherState = d.ditherState*1664525 + 1013904223
	r2 := float64(d.ditherState&0xFFFF) / 65535.0
	return r1 - r2
}

func clampPCM16(v float64) int16 {
	if v > 32635.0 {
		return 32635
	}
	if v < -32635.0 {
		return -32635
	}
	return int16(math.Round(v))
}

// ============================================================================
// Funções utilitárias compatíveis com chamadas avulsas e testes unitários
// ============================================================================

// ResamplePCM16 realiza resampling de PCM16 mono little-endian entre duas frequências.
func ResamplePCM16(input []byte, inRate, outRate int) []byte {
	return ResamplePCM16WithHistory(input, nil, inRate, outRate)
}

// ResamplePCM16WithHistory realiza resampling mantendo histórico entre chunks.
func ResamplePCM16WithHistory(input []byte, history *[]int16, inRate, outRate int) []byte {
	if inRate == outRate || len(input) < 2 {
		return input
	}
	numInSamples := len(input) / 2
	if numInSamples == 0 {
		return nil
	}
	ratio := float64(outRate) / float64(inRate)
	numOutSamples := int(math.Round(float64(numInSamples) * ratio))
	if numOutSamples == 0 {
		return nil
	}

	var histLen int
	if history != nil {
		histLen = len(*history)
	}
	inSamples := make([]int16, histLen+numInSamples)
	if histLen > 0 {
		copy(inSamples, *history)
	}
	for i := 0; i < numInSamples; i++ {
		inSamples[histLen+i] = int16(binary.LittleEndian.Uint16(input[i*2:]))
	}

	out := make([]byte, numOutSamples*2)
	step := float64(inRate) / float64(outRate)

	var kernel []float64
	if inRate > outRate {
		switch {
		case inRate == 24000 && outRate == 8000:
			kernel = lowPass45_24kTo8k
		case inRate == 16000 && outRate == 8000:
			kernel = lowPass31_16kTo8k
		default:
			kernel = buildBlackmanHarrisLowPass(31, 0.44*float64(outRate)/float64(inRate), 1.0)
		}
	}

	halfTap := len(kernel) / 2
	totalLen := len(inSamples)

	for i := 0; i < numOutSamples; i++ {
		var sample int16
		if len(kernel) > 0 {
			centerIdx := histLen + int(math.Round(float64(i)*step))
			if histLen >= len(kernel)-1 {
				centerIdx = (histLen - halfTap) + int(math.Round(float64(i)*step))
			}
			var acc float64
			for k := 0; k < len(kernel); k++ {
				srcIdx := centerIdx + k - halfTap
				if srcIdx < 0 {
					srcIdx = 0
				} else if srcIdx >= totalLen {
					srcIdx = totalLen - 1
				}
				acc += float64(inSamples[srcIdx]) * kernel[k]
			}
			sample = clampPCM16(acc)
		} else {
			var pos float64
			if histLen > 0 {
				pos = float64(histLen-1) + float64(i)*step
			} else {
				pos = float64(i) * step
			}
			idx := int(pos)
			frac := pos - float64(idx)
			if idx >= totalLen-1 {
				sample = inSamples[totalLen-1]
			} else {
				s0 := float64(inSamples[idx])
				s1 := float64(inSamples[idx+1])
				sample = clampPCM16(s0 + frac*(s1-s0))
			}
		}
		binary.LittleEndian.PutUint16(out[i*2:], uint16(sample))
	}

	if history != nil {
		keep := len(kernel) - 1
		if keep <= 0 {
			keep = 1
		}
		if totalLen > keep {
			*history = append((*history)[:0], inSamples[totalLen-keep:]...)
		} else {
			*history = append((*history)[:0], inSamples...)
		}
	}
	return out
}

// TelephonyToProvider converte áudio telefônico G.711 μ-law (8kHz) para PCM16 (16kHz ou 24kHz).
func TelephonyToProvider(muLaw8k []byte, providerInputSampleRate int) []byte {
	var dsp StreamDSP
	return dsp.InboundMuLawToPCM(muLaw8k, providerInputSampleRate)
}

// TelephonyToProviderWithHistory mantém compatibilidade delegando para StreamDSP quando sem estado dedicado.
func TelephonyToProviderWithHistory(muLaw8k []byte, history *[]int16, providerInputSampleRate int) []byte {
	pcm8k := DecodeMuLaw(muLaw8k)
	if providerInputSampleRate == 8000 {
		return pcm8k
	}
	return ResamplePCM16WithHistory(pcm8k, history, 8000, providerInputSampleRate)
}

// ProviderToTelephony converte áudio PCM16 retornado pelo modelo de IA (24kHz) para G.711 μ-law (8kHz).
func ProviderToTelephony(pcmProvider []byte, providerOutputSampleRate int) []byte {
	var dsp StreamDSP
	return dsp.OutboundPCMToMuLaw(pcmProvider, providerOutputSampleRate)
}

// ProviderToTelephonyWithHistory converte áudio PCM16 para G.711 μ-law (8kHz).
func ProviderToTelephonyWithHistory(pcmProvider []byte, history *[]int16, providerOutputSampleRate int) []byte {
	pcm8k := ResamplePCM16WithHistory(pcmProvider, history, providerOutputSampleRate, 8000)
	return EncodeMuLaw(pcm8k)
}
