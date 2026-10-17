// frontend/src/audio.ts
// Gerenciamento de áudio no navegador:
// 1. AudioRecorder: captura microfone com cancelamento de eco e resample para 16kHz PCM16 LE.
// 2. AudioPlayer: reproduz áudio 24kHz PCM16 LE com flush imediato em interrupções (barge-in).

export class AudioRecorder {
  private stream: MediaStream | null = null
  private audioCtx: AudioContext | null = null
  private processor: ScriptProcessorNode | null = null
  private source: MediaStreamAudioSourceNode | null = null
  private muteGain: GainNode | null = null
  public isMuted = false

  constructor(private onAudioChunk: (pcm16: ArrayBuffer) => void) {}

  async start(targetSampleRate = 16000) {
    this.stream = await navigator.mediaDevices.getUserMedia({
      audio: {
        echoCancellation: true,
        noiseSuppression: true,
        autoGainControl: true,
        channelCount: 1,
        // @ts-ignore - Flags específicas do Chromium para supressão rigorosa de eco
        googEchoCancellation: true,
        googAutoGainControl: true,
        googNoiseSuppression: true,
        googHighpassFilter: true,
      } as any,
    })

    const AudioContextClass =
      window.AudioContext ||
      (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext
    this.audioCtx = new AudioContextClass()
    this.source = this.audioCtx.createMediaStreamSource(this.stream)

    const bufferSize = 2048
    this.processor = this.audioCtx.createScriptProcessor(bufferSize, 1, 1)

    const inputRate = this.audioCtx.sampleRate

    this.processor.onaudioprocess = (e) => {
      // 1. Zera o buffer de saída para impedir loopback acústico do microfone para os alto-falantes
      const out = e.outputBuffer.getChannelData(0)
      out.fill(0)

      if (this.isMuted) {
        return
      }

      const input = e.inputBuffer.getChannelData(0)
      const resampled = downsample(input, inputRate, targetSampleRate)
      const pcm16 = floatTo16BitPCM(resampled)
      this.onAudioChunk(pcm16.buffer as ArrayBuffer)
    }

    // 2. Conecta o processor através de um GainNode mutado (ganho 0)
    // Isso garante que o navegador continue disparando onaudioprocess sem tocar nos alto-falantes
    this.muteGain = this.audioCtx.createGain()
    this.muteGain.gain.value = 0

    this.source.connect(this.processor)
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
    if (this.processor) {
      this.processor.disconnect()
      this.processor.onaudioprocess = null
      this.processor = null
    }
    if (this.muteGain) {
      this.muteGain.disconnect()
      this.muteGain = null
    }
    if (this.source) {
      this.source.disconnect()
      this.source = null
    }
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
    this.audioCtx = new AudioContextClass({ sampleRate })
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
    return this.nextPlayTime > this.audioCtx.currentTime + 0.05
  }

  playChunk(pcm16Bytes: ArrayBuffer) {
    if (!this.audioCtx) return

    const int16 = new Int16Array(pcm16Bytes)
    const float32 = new Float32Array(int16.length)
    for (let i = 0; i < int16.length; i++) {
      float32[i] = int16[i] / 32768.0
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

    const now = this.audioCtx.currentTime
    const startTime = Math.max(now, this.nextPlayTime)
    source.start(startTime)
    this.nextPlayTime = startTime + buffer.duration

    this.activeNodes.push(source)
    if (this.onSpeakingChange) {
      this.onSpeakingChange(true)
    }

    // Timer determinístico para sinalizar fim da fala quando o buffer terminar
    if (this.speakingTimer) {
      clearTimeout(this.speakingTimer)
    }
    const waitMs = Math.max(100, Math.round((this.nextPlayTime - now) * 1000) + 80)
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

  // Interrupção: para todos os nós tocando agora e descarta áudio agendado
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

// Resampling linear de inputRate para targetRate
function downsample(buffer: Float32Array, inputRate: number, outputRate: number): Float32Array {
  if (outputRate === inputRate) {
    return buffer
  }
  const ratio = inputRate / outputRate
  const newLength = Math.round(buffer.length / ratio)
  const result = new Float32Array(newLength)
  let offsetResult = 0
  let offsetBuffer = 0
  while (offsetResult < result.length) {
    const nextOffsetBuffer = Math.round((offsetResult + 1) * ratio)
    let accum = 0
    let count = 0
    for (let i = offsetBuffer; i < nextOffsetBuffer && i < buffer.length; i++) {
      accum += buffer[i]
      count++
    }
    result[offsetResult] = count > 0 ? accum / count : buffer[offsetBuffer] || 0
    offsetResult++
    offsetBuffer = nextOffsetBuffer
  }
  return result
}

function floatTo16BitPCM(input: Float32Array): Int16Array {
  const output = new Int16Array(input.length)
  for (let i = 0; i < input.length; i++) {
    // Ganho vocal de 1.8x com soft-limiting (tanh) para que microfones baixos cheguem claros ao modelo e na gravação
    const boosted = Math.tanh(input[i] * 1.8)
    const s = Math.max(-1, Math.min(1, boosted))
    output[i] = s < 0 ? s * 0x8000 : s * 0x7fff
  }
  return output
}
