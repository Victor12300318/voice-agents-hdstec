// frontend/src/audio.ts
// Pipeline de Áudio WebRTC Studio (estilo ElevenLabs Conversational AI):
// 1. AudioRecorder: captura microfone com WebRTC AEC/NS/AGC + cadeia C++ Biquad (High-Pass 85Hz,
//    Presença Vocal 2.8kHz, Anti-Aliasing Low-Pass 7.4kHz) + DynamicsCompressorNode + AudioWorklet
//    em thread de áudio dedicada.
// 2. AudioPlayer: reprodução 24kHz PCM16 LE com jitter buffer adaptativo (35ms) e flush sem cliques.

const WORKLET_CODE = `
class WebRTCPCMProcessor extends AudioWorkletProcessor {
  constructor() {
    super();
    this._buffer = new Float32Array(2048);
    this._writeIdx = 0;
  }
  process(inputs) {
    const input = inputs[0];
    if (!input || !input[0]) return true;
    const ch = input[0];
    for (let i = 0; i < ch.length; i++) {
      this._buffer[this._writeIdx++] = ch[i];
      if (this._writeIdx >= this._buffer.length) {
        const chunk = this._buffer.slice(0);
        this.port.postMessage(chunk, [chunk.buffer]);
        this._writeIdx = 0;
      }
    }
    return true;
  }
}
registerProcessor('webrtc-pcm-processor', WebRTCPCMProcessor);
`

export class AudioRecorder {
  private stream: MediaStream | null = null
  private audioCtx: AudioContext | null = null
  private workletNode: AudioWorkletNode | null = null
  private processor: ScriptProcessorNode | null = null
  private source: MediaStreamAudioSourceNode | null = null
  private highPass: BiquadFilterNode | null = null
  private presenceBoost: BiquadFilterNode | null = null
  private antiAliasLowPass: BiquadFilterNode | null = null
  private compressor: DynamicsCompressorNode | null = null
  private makeupGain: GainNode | null = null
  private muteGain: GainNode | null = null
  public isMuted = false

  constructor(private onAudioChunk: (pcm16: ArrayBuffer) => void) {}

  async start(targetSampleRate = 16000) {
    this.stream = await navigator.mediaDevices.getUserMedia({
      audio: {
        echoCancellation: { ideal: true },
        noiseSuppression: { ideal: true },
        autoGainControl: { ideal: true },
        sampleRate: { ideal: 48000 },
        channelCount: { ideal: 1 },
        // @ts-ignore - Flags específicas do Chromium WebRTC APM
        googEchoCancellation: true,
        googAutoGainControl: true,
        googNoiseSuppression: true,
        googHighpassFilter: true,
      } as any,
    })

    const AudioContextClass =
      window.AudioContext ||
      (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext
    this.audioCtx = new AudioContextClass({ latencyHint: 'interactive' })
    this.source = this.audioCtx.createMediaStreamSource(this.stream)

    // 1. Filtro High-Pass Butterworth em 85 Hz: elimina rumble, pop de microfone e zumbido 50/60 Hz
    this.highPass = this.audioCtx.createBiquadFilter()
    this.highPass.type = 'highpass'
    this.highPass.frequency.value = 85
    this.highPass.Q.value = 0.707

    // 2. Filtro de Presença Vocal em 2800 Hz (+2.5 dB): destaca consoantes (s, f, t, p, c) para o ASR
    this.presenceBoost = this.audioCtx.createBiquadFilter()
    this.presenceBoost.type = 'peaking'
    this.presenceBoost.frequency.value = 2800
    this.presenceBoost.Q.value = 1.0
    this.presenceBoost.gain.value = 2.5

    // 3. Filtro Anti-Aliasing Low-Pass em 7400 Hz (antes do downsampling para 16kHz):
    // impede que ruídos de alta frequência (8kHz–24kHz) dobrem para dentro da voz causando chiado!
    this.antiAliasLowPass = this.audioCtx.createBiquadFilter()
    this.antiAliasLowPass.type = 'lowpass'
    this.antiAliasLowPass.frequency.value = Math.min(7400, targetSampleRate * 0.46)
    this.antiAliasLowPass.Q.value = 0.707

    // 4. WebRTC DynamicsCompressor + Makeup Gain (1.8x linear em C++ 32-bit float):
    // nivela vozes baixas sem distorcer e sem estourar quando o usuário fala perto do microfone
    this.compressor = this.audioCtx.createDynamicsCompressor()
    this.compressor.threshold.value = -26
    this.compressor.knee.value = 12
    this.compressor.ratio.value = 3.0
    this.compressor.attack.value = 0.003
    this.compressor.release.value = 0.15

    this.makeupGain = this.audioCtx.createGain()
    this.makeupGain.gain.value = 1.8

    this.muteGain = this.audioCtx.createGain()
    this.muteGain.gain.value = 0

    // Conecta a cadeia DSP WebRTC
    this.source.connect(this.highPass)
    this.highPass.connect(this.presenceBoost)
    this.presenceBoost.connect(this.antiAliasLowPass)
    this.antiAliasLowPass.connect(this.compressor)
    this.compressor.connect(this.makeupGain)

    const inputRate = this.audioCtx.sampleRate

    const handleFloatChunk = (input: Float32Array) => {
      const resampled = downsample(input, inputRate, targetSampleRate)
      if (this.isMuted) {
        const silent = new Int16Array(resampled.length)
        this.onAudioChunk(silent.buffer as ArrayBuffer)
        return
      }
      const pcm16 = floatTo16BitPCM(resampled)
      this.onAudioChunk(pcm16.buffer as ArrayBuffer)
    }

    // Usa AudioWorklet na thread dedicada de áudio se suportado, com fallback automático
    if (this.audioCtx.audioWorklet) {
      try {
        const blob = new Blob([WORKLET_CODE], { type: 'application/javascript' })
        const url = URL.createObjectURL(blob)
        await this.audioCtx.audioWorklet.addModule(url)
        URL.revokeObjectURL(url)

        this.workletNode = new AudioWorkletNode(this.audioCtx, 'webrtc-pcm-processor')
        this.workletNode.port.onmessage = (ev) => {
          if (ev.data instanceof Float32Array) {
            handleFloatChunk(ev.data)
          }
        }
        this.makeupGain.connect(this.workletNode)
        this.workletNode.connect(this.muteGain)
        this.muteGain.connect(this.audioCtx.destination)
        return
      } catch {
        // Fallback para ScriptProcessorNode abaixo
      }
    }

    const bufferSize = 2048
    this.processor = this.audioCtx.createScriptProcessor(bufferSize, 1, 1)
    this.processor.onaudioprocess = (e) => {
      const out = e.outputBuffer.getChannelData(0)
      out.fill(0)
      handleFloatChunk(e.inputBuffer.getChannelData(0))
    }

    this.makeupGain.connect(this.processor)
    this.processor.connect(this.muteGain)
    this.muteGain.connect(this.audioCtx.destination)
  }

  setMuted(muted: boolean) {
    this.isMuted = muted
  }

  getStream(): MediaStream | null {
    return this.stream
  }

  stop() {
    if (this.workletNode) {
      this.workletNode.port.onmessage = null
      this.workletNode.disconnect()
      this.workletNode = null
    }
    if (this.processor) {
      this.processor.disconnect()
      this.processor.onaudioprocess = null
      this.processor = null
    }
    for (const node of [
      this.makeupGain,
      this.compressor,
      this.antiAliasLowPass,
      this.presenceBoost,
      this.highPass,
      this.muteGain,
      this.source,
    ]) {
      if (node) {
        try {
          node.disconnect()
        } catch {}
      }
    }
    this.makeupGain = null
    this.compressor = null
    this.antiAliasLowPass = null
    this.presenceBoost = null
    this.highPass = null
    this.muteGain = null
    this.source = null

    if (this.audioCtx && this.audioCtx.state !== 'closed') {
      this.audioCtx.close().catch(() => {})
      this.audioCtx = null
    }
    if (this.stream) {
      this.stream.getTracks().forEach((t) => t.stop())
      this.stream = null
    }
  }
}

export class AudioPlayer {
  private audioCtx: AudioContext | null = null
  private analyser: AnalyserNode | null = null
  private nextPlayTime = 0
  private activeNodes: AudioBufferSourceNode[] = []
  private sampleRate = 24000
  private speakingTimer: any = null
  public onSpeakingChange?: (speaking: boolean) => void

  init(sampleRate = 24000) {
    this.sampleRate = sampleRate
    const AudioContextClass =
      window.AudioContext ||
      (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext
    this.audioCtx = new AudioContextClass({ sampleRate, latencyHint: 'interactive' })
    this.analyser = this.audioCtx.createAnalyser()
    this.analyser.fftSize = 256
    this.analyser.smoothingTimeConstant = 0.8
    this.analyser.connect(this.audioCtx.destination)
    this.nextPlayTime = this.audioCtx.currentTime
  }

  getAnalyser(): AnalyserNode | null {
    return this.analyser
  }

  getVolume(): number {
    if (!this.analyser || !this.isPlaying()) return 0
    const data = new Uint8Array(this.analyser.frequencyBinCount)
    this.analyser.getByteFrequencyData(data)
    let sum = 0
    for (let i = 0; i < data.length; i++) {
      sum += data[i]
    }
    return Math.min(1, (sum / data.length / 255) * 1.6)
  }

  async resume() {
    if (this.audioCtx && this.audioCtx.state === 'suspended') {
      await this.audioCtx.resume()
    }
  }

  isPlaying(): boolean {
    if (!this.audioCtx) return false
    return this.nextPlayTime > this.audioCtx.currentTime + 0.03
  }

  playChunk(pcm16Bytes: ArrayBuffer) {
    if (!this.audioCtx) return

    const int16 = new Int16Array(pcm16Bytes)
    if (int16.length === 0) return
    const float32 = new Float32Array(int16.length)
    for (let i = 0; i < int16.length; i++) {
      float32[i] = int16[i] / 32768.0
    }

    const now = this.audioCtx.currentTime
    const isFirstChunkOfBurst = this.nextPlayTime <= now + 0.005

    // Micro-fade-in de 24 amostras (1ms) apenas no 1º chunk de cada fala para evitar clique inicial
    if (isFirstChunkOfBurst && float32.length > 24) {
      for (let i = 0; i < 24; i++) {
        float32[i] *= i / 24
      }
    }

    const buffer = this.audioCtx.createBuffer(1, float32.length, this.sampleRate)
    buffer.getChannelData(0).set(float32)

    const source = this.audioCtx.createBufferSource()
    source.buffer = buffer
    if (this.analyser) {
      source.connect(this.analyser)
    } else {
      source.connect(this.audioCtx.destination)
    }

    // Jitter buffer de 35ms no início do turno para eliminar estalos por variação de rede WebSocket
    const startTime = isFirstChunkOfBurst ? now + 0.035 : Math.max(now, this.nextPlayTime)
    source.start(startTime)
    this.nextPlayTime = startTime + buffer.duration

    this.activeNodes.push(source)
    if (this.onSpeakingChange) {
      this.onSpeakingChange(true)
    }

    if (this.speakingTimer) {
      clearTimeout(this.speakingTimer)
    }
    const waitMs = Math.max(80, Math.round((this.nextPlayTime - now) * 1000) + 40)
    this.speakingTimer = setTimeout(() => {
      this.activeNodes = []
      if (this.onSpeakingChange) {
        this.onSpeakingChange(false)
      }
    }, waitMs)

    source.onended = () => {
      const idx = this.activeNodes.indexOf(source)
      if (idx !== -1) {
        this.activeNodes.splice(idx, 1)
      }
      if (this.activeNodes.length === 0 && this.onSpeakingChange) {
        this.onSpeakingChange(false)
      }
    }
  }

  flush() {
    if (this.speakingTimer) {
      clearTimeout(this.speakingTimer)
      this.speakingTimer = null
    }
    if (!this.audioCtx) return
    for (const node of this.activeNodes) {
      try {
        node.stop()
        node.disconnect()
      } catch (_) {}
    }
    this.activeNodes = []
    this.nextPlayTime = this.audioCtx.currentTime
    if (this.onSpeakingChange) {
      this.onSpeakingChange(false)
    }
  }

  close() {
    this.flush()
    if (this.analyser) {
      try {
        this.analyser.disconnect()
      } catch (_) {}
      this.analyser = null
    }
    if (this.audioCtx && this.audioCtx.state !== 'closed') {
      this.audioCtx.close().catch(() => {})
      this.audioCtx = null
    }
  }
}

// Resampling linear sobre sinal já filtrado pelo Biquad anti-aliasing (fc 7.4kHz)
function downsample(buffer: Float32Array, inputRate: number, outputRate: number): Float32Array {
  if (outputRate === inputRate) {
    return buffer
  }
  const ratio = inputRate / outputRate
  const newLength = Math.round(buffer.length / ratio)
  const result = new Float32Array(newLength)
  for (let i = 0; i < newLength; i++) {
    const pos = i * ratio
    const idx = Math.floor(pos)
    const frac = pos - idx
    const s0 = buffer[idx] || 0
    const s1 = idx + 1 < buffer.length ? buffer[idx + 1] : s0
    result[i] = s0 + frac * (s1 - s0)
  }
  return result
}

function floatTo16BitPCM(input: Float32Array): Int16Array {
  const output = new Int16Array(input.length)
  for (let i = 0; i < input.length; i++) {
    const s = Math.max(-1, Math.min(1, input[i]))
    output[i] = s < 0 ? s * 0x8000 : s * 0x7fff
  }
  return output
}
