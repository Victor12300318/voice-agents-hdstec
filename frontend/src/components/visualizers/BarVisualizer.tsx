// frontend/src/components/visualizers/BarVisualizer.tsx
// Port do visualizador de barras de frequência com transições animadas da ElevenLabs UI
import { useEffect, useRef, useState, type HTMLAttributes } from 'react'

export type VisualizerState =
  | 'idle'
  | 'connecting'
  | 'initializing'
  | 'listening'
  | 'speaking'
  | 'thinking'

export interface BarVisualizerProps extends HTMLAttributes<HTMLDivElement> {
  state?: VisualizerState
  barCount?: number
  analyser?: AnalyserNode | null
  mediaStream?: MediaStream | null
  minHeight?: number
  maxHeight?: number
  centerAlign?: boolean
  demo?: boolean
}

export function BarVisualizer({
  state = 'idle',
  barCount = 20,
  analyser,
  mediaStream,
  minHeight = 15,
  maxHeight = 95,
  centerAlign = true,
  demo = false,
  className = '',
  ...props
}: BarVisualizerProps) {
  const [bands, setBands] = useState<number[]>(() => new Array(barCount).fill(0.1))
  const [highlighted, setHighlighted] = useState<number[]>([])
  const internalAnalyserRef = useRef<AnalyserNode | null>(null)
  const audioCtxRef = useRef<AudioContext | null>(null)
  const sourceRef = useRef<MediaStreamAudioSourceNode | null>(null)

  // Gerenciamento de MediaStream caso passado como prop
  useEffect(() => {
    if (!mediaStream) {
      if (sourceRef.current) {
        sourceRef.current.disconnect()
        sourceRef.current = null
      }
      if (audioCtxRef.current && audioCtxRef.current.state !== 'closed') {
        audioCtxRef.current.close()
        audioCtxRef.current = null
      }
      internalAnalyserRef.current = null
      return
    }

    try {
      const AudioCtx =
        window.AudioContext ||
        (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext
      const ctx = new AudioCtx()
      const an = ctx.createAnalyser()
      an.fftSize = 256
      an.smoothingTimeConstant = 0.8
      const src = ctx.createMediaStreamSource(mediaStream)
      src.connect(an)

      audioCtxRef.current = ctx
      internalAnalyserRef.current = an
      sourceRef.current = src
    } catch {
      /* ignore */
    }

    return () => {
      if (sourceRef.current) sourceRef.current.disconnect()
      if (audioCtxRef.current && audioCtxRef.current.state !== 'closed') audioCtxRef.current.close()
    }
  }, [mediaStream])

  // Animação de estados (thinking, connecting, listening)
  useEffect(() => {
    let interval: any = null
    let step = 0

    if (state === 'connecting') {
      interval = setInterval(() => {
        step = (step + 1) % barCount
        setHighlighted([step, barCount - 1 - step])
      }, 100)
    } else if (state === 'thinking') {
      const center = Math.floor(barCount / 2)
      interval = setInterval(() => {
        step = (step + 1) % 4
        setHighlighted([center - step, center, center + step].filter((i) => i >= 0 && i < barCount))
      }, 140)
    } else if (state === 'listening') {
      const center = Math.floor(barCount / 2)
      setHighlighted([center])
    } else {
      setHighlighted([])
    }

    return () => {
      if (interval) clearInterval(interval)
    }
  }, [state, barCount])

  // Leitura de áudio em tempo real ou simulação demo
  useEffect(() => {
    let rafId: number
    const activeAnalyser = analyser || internalAnalyserRef.current

    const updateAudio = () => {
      if (activeAnalyser && state === 'speaking') {
        const buffer = new Uint8Array(activeAnalyser.frequencyBinCount)
        activeAnalyser.getByteFrequencyData(buffer)

        const startIdx = Math.floor(buffer.length * 0.05)
        const endIdx = Math.floor(buffer.length * 0.6)
        const relevant = buffer.slice(startIdx, endIdx)
        const chunkSize = Math.max(1, Math.floor(relevant.length / barCount))

        const newBands = new Array(barCount)
        for (let i = 0; i < barCount; i++) {
          let sum = 0
          const s = i * chunkSize
          const e = Math.min(s + chunkSize, relevant.length)
          for (let j = s; j < e; j++) {
            sum += relevant[j]
          }
          const avg = (sum / Math.max(1, e - s)) / 255
          newBands[i] = Math.max(0.08, Math.min(1, avg * 1.3))
        }
        setBands(newBands)
      } else if (demo || state === 'speaking') {
        // Fallback dinâmico suave se não houver AnalyserNode direto
        const time = Date.now() / 300
        const newBands = new Array(barCount)
        for (let i = 0; i < barCount; i++) {
          const wave = Math.sin(time + i * 0.4) * 0.35 + 0.45
          newBands[i] = Math.max(0.1, Math.min(1, wave))
        }
        setBands(newBands)
      } else {
        setBands(new Array(barCount).fill(0.12))
      }

      rafId = requestAnimationFrame(updateAudio)
    }

    rafId = requestAnimationFrame(updateAudio)
    return () => cancelAnimationFrame(rafId)
  }, [analyser, state, barCount, demo])

  return (
    <div
      className={`relative flex justify-center gap-1.5 w-full h-20 px-3 py-2 rounded-2xl bg-slate-900/40 dark:bg-slate-950/40 backdrop-blur-md border border-white/10 ${
        centerAlign ? 'items-center' : 'items-end'
      } ${className}`}
      {...props}
    >
      {bands.map((val, idx) => {
        const heightPct = Math.min(maxHeight, Math.max(minHeight, val * 100))
        const isHigh = highlighted.includes(idx)

        let barColor = 'bg-slate-400/40 dark:bg-slate-600/40'
        if (state === 'speaking') {
          barColor = 'bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.5)]'
        } else if (state === 'listening' && isHigh) {
          barColor = 'bg-blue-500 shadow-[0_0_8px_rgba(59,130,246,0.5)]'
        } else if (state === 'thinking' && isHigh) {
          barColor = 'bg-purple-500 shadow-[0_0_8px_rgba(168,85,247,0.5)]'
        } else if (state === 'connecting' && isHigh) {
          barColor = 'bg-sky-400'
        }

        return (
          <div
            key={idx}
            className={`w-1.5 rounded-full transition-all duration-100 ${barColor}`}
            style={{
              height: `${heightPct}%`,
            }}
          />
        )
      })}
    </div>
  )
}
