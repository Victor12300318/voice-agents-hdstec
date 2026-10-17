package audio

import "encoding/binary"

const (
	bias = 0x84
	clip = 32635
)

var muLawToLinear [256]int16

func init() {
	for i := 0; i < 256; i++ {
		muLawToLinear[i] = decodeMuLawSample(byte(i))
	}
}

// DecodeMuLawSample decodifica um byte G.711 μ-law em amostra PCM16 linear.
func DecodeMuLawSample(u byte) int16 {
	return muLawToLinear[u]
}

func decodeMuLawSample(u byte) int16 {
	u = ^u
	sign := u & 0x80
	exponent := (u >> 4) & 0x07
	mantissa := u & 0x0F
	sample := int16((int32(mantissa)<<3 | 0x84) << exponent) - bias
	if sign != 0 {
		return -sample
	}
	return sample
}

// EncodeMuLawSample converte uma amostra linear PCM16 em byte G.711 μ-law.
func EncodeMuLawSample(pcm int16) byte {
	sign := byte(0)
	if pcm < 0 {
		sign = 0x80
		// Trata overflow de -32768
		if pcm == -32768 {
			pcm = -32767
		}
		pcm = -pcm
	}
	if pcm > clip {
		pcm = clip
	}
	pcm += bias

	exponent := byte(7)
	for mask := int16(0x4000); (pcm&mask) == 0 && exponent > 0; mask >>= 1 {
		exponent--
	}
	mantissa := byte((pcm >> (exponent + 3)) & 0x0F)
	return ^(sign | (exponent << 4) | mantissa)
}

// DecodeMuLaw converte buffer G.711 μ-law em buffer PCM16 little-endian.
func DecodeMuLaw(muLaw []byte) []byte {
	out := make([]byte, len(muLaw)*2)
	for i, b := range muLaw {
		sample := muLawToLinear[b]
		binary.LittleEndian.PutUint16(out[i*2:], uint16(sample))
	}
	return out
}

// EncodeMuLaw converte buffer PCM16 little-endian em buffer G.711 μ-law.
func EncodeMuLaw(pcm16 []byte) []byte {
	numSamples := len(pcm16) / 2
	out := make([]byte, numSamples)
	for i := 0; i < numSamples; i++ {
		sample := int16(binary.LittleEndian.Uint16(pcm16[i*2:]))
		out[i] = EncodeMuLawSample(sample)
	}
	return out
}
