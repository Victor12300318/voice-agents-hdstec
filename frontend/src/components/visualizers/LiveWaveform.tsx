// frontend/src/components/visualizers/LiveWaveform.tsx
// Port do visualizador de áudio em tempo real da ElevenLabs UI com suporte a MediaStream externo
import { useEffect, useRef, type HTMLAttributes } from 'react'

export interface LiveWaveformProps extends HTMLAttributes<HTMLDivElement> {
  active?: boolean
  processing?: boolean
  mediaStream?: MediaStream | null
  barWidth?: number
  barHeight?: number
  barGap?: number
  barRadius?: number
  barColor?: string
  fadeEdges?: boolean
  fadeWidth?: number
  height?: string | number
  sensitivity?: number
  smoothingTimeConstant?: number
  fftSize?: number
  historySize?: number
  updateRate?: number
  mode?: 'scrolling' | 'static'
}

export function LiveWaveform({
  active = false,
  processing = false,
  mediaStream,
  barWidth = 3,
  barGap = 1.5,
  barRadius = 1.5,
  barColor,
  fadeEdges = true,
  fadeWidth = 24,
  barHeight: baseBarHeight = 4,
  height = 64,
  sensitivity = 1.2,
  smoothingTimeConstant = 0.8,
  fftSize = 256,
  historySize = 60,
  updateRate = 30,
  mode = 'static',
  className = '',
  ...props
}: LiveWaveformProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const historyRef = useRef<number[]>([])
  const analyserRef = useRef<AnalyserNode | null>(null)
  const audioContextRef = useRef<AudioContext | null>(null)
  const sourceRef = useRef<MediaStreamAudioSourceNode | null>(null)
  const lastUpdateRef = useRef<number>(0)
  const processingAnimationRef = useRef<number | null>(null)
  const staticBarsRef = useRef<number[]>([])
  const needsRedrawRef = useRef(true)

  const heightStyle = typeof height === 'number' ? `${height}px` : height

  // Redimensionamento do canvas com HiDPI
  useEffect(() => {
    const canvas = canvasRef.current
    const container = containerRef.current
    if (!canvas || !container) return

    const resizeObserver = new ResizeObserver(() => {
      const rect = container.getBoundingClientRect()
      const dpr = window.devicePixelRatio || 1

      canvas.width = rect.width * dpr
      canvas.height = rect.height * dpr
      canvas.style.width = `${rect.width}px`
      canvas.style.height = `${rect.height}px`

      const ctx = canvas.getContext('2d')
      if (ctx) {
        ctx.scale(dpr, dpr)
      }
      needsRedrawRef.current = true
    })

    resizeObserver.observe(container)
    return () => resizeObserver.disconnect()
  }, [])

  // Animação de processamento (ondas senoidais enquanto o agent processa)
  useEffect(() => {
    if (processing && !active) {
      let time = 0
      const animateProcessing = () => {
        time += 0.04
        const barCount = Math.floor(
          (containerRef.current?.getBoundingClientRect().width || 200) / (barWidth + barGap),
        )
        const processingData: number[] = []

        if (mode === 'static') {
          const halfCount = Math.floor(barCount / 2)
          for (let i = 0; i < barCount; i++) {
            const normalized = (i - halfCount) / Math.max(1, halfCount)
            const centerWeight = 1 - Math.abs(normalized) * 0.4
            const w1 = Math.sin(time * 1.5 + normalized * 3) * 0.25
            const w2 = Math.sin(time * 0.8 - normalized * 2) * 0.2
            const val = (0.25 + w1 + w2) * centerWeight
            processingData.push(Math.max(0.06, Math.min(1, val)))
          }
          staticBarsRef.current = processingData
        }

        needsRedrawRef.current = true
        processingAnimationRef.current = requestAnimationFrame(animateProcessing)
      }

      animateProcessing()
      return () => {
        if (processingAnimationRef.current) {
          cancelAnimationFrame(processingAnimationRef.current)
        }
      }
    }
  }, [processing, active, barWidth, barGap, mode])

  // Vinculação com o MediaStream de áudio
  useEffect(() => {
    if (!active || !mediaStream) {
      if (sourceRef.current) {
        sourceRef.current.disconnect()
        sourceRef.current = null
      }
      if (audioContextRef.current && audioContextRef.current.state !== 'closed') {
        audioContextRef.current.close()
        audioContextRef.current = null
      }
      analyserRef.current = null
      return
    }

    try {
      const AudioCtx =
        window.AudioContext ||
        (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext
      const ctx = new AudioCtx()
      const analyser = ctx.createAnalyser()
      analyser.fftSize = fftSize
      analyser.smoothingTimeConstant = smoothingTimeConstant

      const source = ctx.createMediaStreamSource(mediaStream)
      source.connect(analyser)

      audioContextRef.current = ctx
      analyserRef.current = analyser
      sourceRef.current = source
      historyRef.current = []
    } catch {
      /* ignore */
    }

    return () => {
      if (sourceRef.current) {
        sourceRef.current.disconnect()
        sourceRef.current = null
      }
      if (audioContextRef.current && audioContextRef.current.state !== 'closed') {
        audioContextRef.current.close()
        audioContextRef.current = null
      }
      analyserRef.current = null
    }
  }, [active, mediaStream, fftSize, smoothingTimeConstant])

  // Loop de desenho no canvas
  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    let rafId: number

    const render = (currentTime: number) => {
      const rect = canvas.getBoundingClientRect()
      if (rect.width === 0 || rect.height === 0) {
        rafId = requestAnimationFrame(render)
        return
      }

      if (active && analyserRef.current && currentTime - lastUpdateRef.current > updateRate) {
        lastUpdateRef.current = currentTime
        const data = new Uint8Array(analyserRef.current.frequencyBinCount)
        analyserRef.current.getByteFrequencyData(data)

        if (mode === 'static') {
          const start = Math.floor(data.length * 0.05)
          const end = Math.floor(data.length * 0.45)
          const relevant = data.slice(start, end)
          const barCount = Math.floor(rect.width / (barWidth + barGap))
          const half = Math.floor(barCount / 2)
          const bars: number[] = []

          for (let i = half - 1; i >= 0; i--) {
            const idx = Math.floor((i / Math.max(1, half)) * relevant.length)
            bars.push(Math.max(0.06, (relevant[idx] / 255) * sensitivity))
          }
          for (let i = 0; i < half; i++) {
            const idx = Math.floor((i / Math.max(1, half)) * relevant.length)
            bars.push(Math.max(0.06, (relevant[idx] / 255) * sensitivity))
          }
          staticBarsRef.current = bars
        }
        needsRedrawRef.current = true
      }

      if (needsRedrawRef.current || active || processing) {
        ctx.clearRect(0, 0, rect.width, rect.height)
        const computedColor = barColor || '#38bdf8'
        const step = barWidth + barGap
        const barCount = Math.floor(rect.width / step)
        const centerY = rect.height / 2
        const data = staticBarsRef.current

        for (let i = 0; i < barCount && i < data.length; i++) {
          const val = data[i] || 0.06
          const x = i * step
          const bHeight = Math.max(baseBarHeight, val * rect.height * 0.85)
          const y = centerY - bHeight / 2

          ctx.fillStyle = computedColor
          ctx.globalAlpha = 0.35 + val * 0.65

          if (barRadius > 0 && 'roundRect' in ctx) {
            ctx.beginPath()
            ctx.roundRect(x, y, barWidth, bHeight, barRadius)
            ctx.fill()
          } else {
            ctx.fillRect(x, y, barWidth, bHeight)
          }
        }

        // Suavização das bordas com fade
        if (fadeEdges && fadeWidth > 0) {
          const grad = ctx.createLinearGradient(0, 0, rect.width, 0)
          const pct = Math.min(0.25, fadeWidth / rect.width)
          grad.addColorStop(0, 'rgba(255,255,255,1)')
          grad.addColorStop(pct, 'rgba(255,255,255,0)')
          grad.addColorStop(1 - pct, 'rgba(255,255,255,0)')
          grad.addColorStop(1, 'rgba(255,255,255,1)')

          ctx.globalCompositeOperation = 'destination-out'
          ctx.fillStyle = grad
          ctx.fillRect(0, 0, rect.width, rect.height)
          ctx.globalCompositeOperation = 'source-over'
        }
        ctx.globalAlpha = 1
      }

      rafId = requestAnimationFrame(render)
    }

    rafId = requestAnimationFrame(render)
    return () => cancelAnimationFrame(rafId)
  }, [active, processing, barColor, barWidth, barGap, barRadius, baseBarHeight, fadeEdges, fadeWidth, mode, sensitivity, updateRate])

  return (
    <div
      ref={containerRef}
      className={`relative w-full ${className}`}
      style={{ height: heightStyle }}
      {...props}
    >
      {!active && !processing && (
        <div className="absolute top-1/2 right-0 left-0 -translate-y-1/2 border-t border-dashed border-slate-300 dark:border-slate-700/60" />
      )}
      <canvas ref={canvasRef} className="block w-full h-full" />
    </div>
  )
}
