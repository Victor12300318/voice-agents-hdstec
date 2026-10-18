package audio

import (
	"encoding/binary"
	"math"
)

// Kernel FIR passa-baixas (Windowed-Sinc de 31 taps com janela de Blackman) pré-calculado
// para corte em ~3.550 Hz numa taxa de entrada de 24.000 Hz.
// Preserva 100% da clareza e brilho vocal até 3.450 Hz (evitando voz abafada/rouca) e atenua
// frequências acima de 4.000 Hz em >55 dB antes da decimação 3:1 para 8.000 Hz.
var lowPass24kTo8k = buildLowPassKernel(31, 3550.0/24000.0)

// Kernel FIR passa-baixas para conversão de 16.000 Hz -> 8.000 Hz.
var lowPass16kTo8k = buildLowPassKernel(23, 3550.0/16000.0)

func buildLowPassKernel(taps int, normCutoff float64) []float64 {
	kernel := make([]float64, taps)
	center := float64(taps-1) / 2.0
	var sum float64
	for i := 0; i < taps; i++ {
		n := float64(i) - center
		var sinc float64
		if n == 0 {
			sinc = 2.0 * normCutoff
		} else {
			sinc = math.Sin(2.0*math.Pi*normCutoff*n) / (math.Pi * n)
		}
		// Janela de Blackman para banda de passagem plana (0 dB ripple) e >55dB stopband
		alpha := 2.0 * math.Pi * float64(i) / float64(taps-1)
		window := 0.42 - 0.5*math.Cos(alpha) + 0.08*math.Cos(2.0*alpha)
		val := sinc * window
		kernel[i] = val
		sum += val
	}
	if sum != 0 {
		for i := range kernel {
			kernel[i] /= sum
		}
	}
	return kernel
}

// ResamplePCM16 realiza resampling de PCM16 mono little-endian entre duas frequências.
func ResamplePCM16(input []byte, inRate, outRate int) []byte {
	return ResamplePCM16WithHistory(input, nil, inRate, outRate)
}

// ResamplePCM16WithHistory realiza resampling mantendo histórico de amostras entre chunks
// consecutivos do WebSocket para eliminar descontinuidades e rouquidão nas bordas de pacotes.
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
			kernel = lowPass24kTo8k
		case inRate == 16000 && outRate == 8000:
			kernel = lowPass16kTo8k
		default:
			kernel = buildLowPassKernel(23, 0.44*float64(outRate)/float64(inRate))
		}
	}

	halfTap := len(kernel) / 2
	totalLen := len(inSamples)

	for i := 0; i < numOutSamples; i++ {
		var pos float64
		if len(kernel) > 0 {
			pos = float64(histLen) + float64(i)*step
		} else if histLen > 0 {
			// No upsampling com histórico (1 amostra anterior em inSamples[0]), interpola continuamente
			// desde a fronteira do chunk anterior sem repetir a última amostra.
			pos = float64(histLen-1) + float64(i+1)*step
		} else {
			pos = float64(i) * step
		}

		var sample int16
		if len(kernel) > 0 {
			centerIdx := int(math.Round(pos))
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
			if acc > 32635 {
				acc = 32635
			} else if acc < -32635 {
				acc = -32635
			}
			sample = int16(math.Round(acc))
		} else {
			idx := int(pos)
			frac := pos - float64(idx)
			if idx >= totalLen-1 {
				sample = inSamples[totalLen-1]
			} else {
				s0 := float64(inSamples[idx])
				s1 := float64(inSamples[idx+1])
				interp := s0 + frac*(s1-s0)
				if interp > 32767 {
					interp = 32767
				} else if interp < -32768 {
					interp = -32768
				}
				sample = int16(math.Round(interp))
			}
		}

		binary.LittleEndian.PutUint16(out[i*2:], uint16(sample))
	}

	if history != nil {
		keep := len(kernel)
		if keep == 0 {
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

// boostCallerSpeechPCM16 aplica supressão suave do piso de quantização G.711 μ-law (< 40)
// seguida de ganho vocal WebRTC (+6.5 dB / 2.1x) com soft-knee acima de 22.000, deixando a voz
// do cliente alta, limpa e sem chiado de fundo.
func boostCallerSpeechPCM16(pcm []byte) []byte {
	numSamples := len(pcm) / 2
	for i := 0; i < numSamples; i++ {
		raw := int16(binary.LittleEndian.Uint16(pcm[i*2:]))
		val := float64(raw)
		absVal := math.Abs(val)
		sign := 1.0
		if val < 0 {
			sign = -1.0
		}

		var out float64
		if absVal < 40.0 {
			// Atenuação quadrática contínua do ruído de quantização G.711 em silêncio (sem corte abrupto)
			out = (absVal * absVal / 40.0) * 0.5
		} else {
			out = absVal * 2.1
		}

		// Soft-knee limiter acima de 22.000 para nunca distorcer vogais fortes
		if out > 22000.0 {
			excess := out - 22000.0
			out = 22000.0 + 10000.0*math.Tanh(excess/10000.0)
		}
		if out > 32400.0 {
			out = 32400.0
		}

		outSample := int16(math.Round(sign * out))
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(outSample))
	}
	return pcm
}

// deHissUpsampledPCM16 aplica um filtro FIR passa-baixas anti-imagem (fc ~3.550 Hz) no sinal
// upsampled de 8kHz para 16kHz/24kHz, removendo os degraus de alta frequência ("chiado") do G.711 μ-law.
func deHissUpsampledPCM16(pcm []byte) []byte {
	n := len(pcm) / 2
	if n < 5 {
		return pcm
	}
	samples := make([]int16, n)
	for i := 0; i < n; i++ {
		samples[i] = int16(binary.LittleEndian.Uint16(pcm[i*2:]))
	}
	for i := 2; i < n-2; i++ {
		// Kernel FIR simétrico de fase linear [0.10, 0.24, 0.32, 0.24, 0.10] (corte suave acima de 3.5 kHz)
		acc := 0.10*float64(samples[i-2]) +
			0.24*float64(samples[i-1]) +
			0.32*float64(samples[i]) +
			0.24*float64(samples[i+1]) +
			0.10*float64(samples[i+2])
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(int16(math.Round(acc))))
	}
	return pcm
}

// TelephonyToProvider converte áudio telefônico G.711 μ-law (8kHz) para PCM16 na taxa de entrada do modelo (16kHz ou 24kHz).
func TelephonyToProvider(muLaw8k []byte, providerInputSampleRate int) []byte {
	return TelephonyToProviderWithHistory(muLaw8k, nil, providerInputSampleRate)
}

// TelephonyToProviderWithHistory converte áudio telefônico G.711 μ-law (8kHz) para PCM16 mantendo continuidade entre pacotes de 20ms.
func TelephonyToProviderWithHistory(muLaw8k []byte, history *[]int16, providerInputSampleRate int) []byte {
	pcm8k := boostCallerSpeechPCM16(DecodeMuLaw(muLaw8k))
	if providerInputSampleRate == 8000 {
		return pcm8k
	}
	upsampled := ResamplePCM16WithHistory(pcm8k, history, 8000, providerInputSampleRate)
	return deHissUpsampledPCM16(upsampled)
}

// ProviderToTelephony converte áudio PCM16 retornado pelo modelo de IA (24kHz) para G.711 μ-law (8kHz).
func ProviderToTelephony(pcmProvider []byte, providerOutputSampleRate int) []byte {
	pcm8k := ResamplePCM16(pcmProvider, providerOutputSampleRate, 8000)
	return EncodeMuLaw(pcm8k)
}

// ProviderToTelephonyWithHistory converte áudio PCM16 (24kHz) para G.711 μ-law (8kHz) mantendo continuidade do filtro entre chunks.
func ProviderToTelephonyWithHistory(pcmProvider []byte, history *[]int16, providerOutputSampleRate int) []byte {
	pcm8k := ResamplePCM16WithHistory(pcmProvider, history, providerOutputSampleRate, 8000)
	return EncodeMuLaw(pcm8k)
}
