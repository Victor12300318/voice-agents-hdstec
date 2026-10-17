// frontend/src/pages/AgentTest.tsx
import { useEffect, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { motion } from 'motion/react'
import { ArrowLeft, Mic, MicOff, Headphones, Play, Square } from 'lucide-react'
import { api, type AgentDetail } from '../api'
import { AudioPlayer, AudioRecorder } from '../audio'
import Button from '../components/Button'
import { springSnappy } from '../components/motion'
import { Orb, LiveWaveform, BarVisualizer, ShimmeringText } from '../components/visualizers'

type TranscriptMessage = {
  id: string
  role: 'caller' | 'agent' | 'system'
  text: string
  timestamp: string
  trailingSpace?: boolean
}

type VersionSummary = {
  version: number
  version_id?: string
  branch_name?: string
  created_at: string
}

type CallStatus = 'idle' | 'connecting' | 'listening' | 'speaking' | 'thinking' | 'interrupted' | 'ended'

export default function AgentTest() {
  const { id } = useParams<{ id: string }>()
  const [detail, setDetail] = useState<AgentDetail | null>(null)
  const [versions, setVersions] = useState<VersionSummary[]>([])
  const [selectedVersion, setSelectedVersion] = useState<string>('draft')
  const [caller, setCaller] = useState<string>('5511999999999')

  const [status, setStatus] = useState<CallStatus>('idle')
  const [statusDetail, setStatusDetail] = useState<string>('')
  const [transcripts, setTranscripts] = useState<TranscriptMessage[]>([])
  const [errorMsg, setErrorMsg] = useState<string>('')
  const [isMuted, setIsMuted] = useState<boolean>(false)
  const [antiEcho, setAntiEcho] = useState<boolean>(false)
  const [resolvedVars, setResolvedVars] = useState<Record<string, string> | null>(null)
  const forceNewBubbleRef = useRef<boolean>(false)

  // Volumes em tempo real para os visualizadores da ElevenLabs
  const [callerVolume, setCallerVolume] = useState<number>(0)
  const [agentVolume, setAgentVolume] = useState<number>(0)
  const [micStream, setMicStream] = useState<MediaStream | null>(null)

  const wsRef = useRef<WebSocket | null>(null)
  const recorderRef = useRef<AudioRecorder | null>(null)
  const playerRef = useRef<AudioPlayer | null>(null)
  const transcriptsEndRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    Promise.all([
      api<AgentDetail>(`/api/agents/${id}`),
      api<VersionSummary[]>(`/api/agents/${id}/versions`).catch(() => []),
    ])
      .then(([ag, vers]) => {
        setDetail(ag)
        setVersions(vers)
        if (ag.latest_version) {
          // Mantém draft como padrão para teste, mas permite escolher a versão publicada
        }
      })
      .catch((e) => setErrorMsg(e.message))

    return () => {
      hangup()
    }
  }, [id])

  useEffect(() => {
    transcriptsEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [transcripts])

  const addMessage = (role: 'caller' | 'agent' | 'system', text: string) => {
    const rawOrig = String(text || '')
    const hasLeadingSpace = /^\s/.test(rawOrig)
    const hasTrailingSpace = /\s$/.test(rawOrig)
    const frag = rawOrig.trim()
    if (!frag) return
    setTranscripts((prev) => {
      const last = prev[prev.length - 1]
      if (!forceNewBubbleRef.current && role !== 'system' && last && last.role === role) {
        const isPunct = /^[.,!?;:]/.test(frag)
        const needsSpace =
          !isPunct &&
          (last.trailingSpace ||
            hasLeadingSpace ||
            !/[a-záàâãéêíóôõúçA-ZÁÀÂÃÉÊÍÓÔÕÚÇ]$/.test(last.text) ||
            !/^[a-záàâãéêíóôõúç]/.test(frag))
        return [
          ...prev.slice(0, -1),
          {
            ...last,
            text: `${last.text.trimEnd()}${needsSpace ? ' ' : ''}${frag}`,
            trailingSpace: hasTrailingSpace,
          },
        ]
      }
      forceNewBubbleRef.current = false
      return [
        ...prev,
        {
          id: Math.random().toString(36).slice(2),
          role,
          text: frag,
          timestamp: new Date().toLocaleTimeString('pt-BR'),
          trailingSpace: hasTrailingSpace,
        },
      ]
    })
  }

  const startCall = async () => {
    if (!detail) return
    setErrorMsg('')
    setStatus('connecting')
    setStatusDetail('Iniciando áudio e conectando ao WebSocket...')
    setTranscripts([])
    setResolvedVars(null)

    try {
      // 1. Inicializa o player de áudio (24kHz para Gemini)
      const player = new AudioPlayer()
      player.init(24000)
      await player.resume()
      player.onSpeakingChange = (speaking) => {
        if (speaking) {
          setStatus('speaking')
          if (antiEcho && recorderRef.current) {
            recorderRef.current.setMuted(true)
          }
        } else {
          setStatus('listening')
          if (recorderRef.current) {
            recorderRef.current.setMuted(isMuted)
          }
        }
      }
      playerRef.current = player

      // 2. Conecta o WebSocket via proxy do nginx (mesma origem)
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const wsUrl = `${protocol}//${window.location.host}/ws/conversations/${detail.id}?version=${encodeURIComponent(
        selectedVersion,
      )}&caller=${encodeURIComponent(caller)}`

      const ws = new WebSocket(wsUrl)
      ws.binaryType = 'arraybuffer'
      wsRef.current = ws

      ws.onopen = async () => {
        setStatusDetail('Conexão aberta. Aguardando Voice Provider...')
      }

      ws.onmessage = async (event) => {
        if (typeof event.data === 'string') {
          try {
            const msg = JSON.parse(event.data)
            switch (msg.type) {
              case 'ready':
                setStatus('listening')
                setStatusDetail('Pronto! Fale com o agente a qualquer momento.')
                if (msg.variables) {
                  setResolvedVars(msg.variables)
                }

                // 3. Inicia a gravação do microfone após a confirmação de pronto
                try {
                  const recorder = new AudioRecorder((pcm) => {
                    if (ws.readyState === WebSocket.OPEN) {
                      ws.send(pcm)
                    }
                  })
                  await recorder.start(msg.input_sample_rate || 16000)
                  recorderRef.current = recorder
                  setMicStream(recorder.getStream())
                } catch (micErr) {
                  setErrorMsg('Erro ao acessar microfone: ' + (micErr as Error).message)
                  hangup()
                }
                break

              case 'caller_transcript':
                if (msg.text) {
                  addMessage('caller', msg.text)
                }
                break

              case 'agent_transcript':
                if (msg.text) {
                  addMessage('agent', msg.text)
                }
                break

              case 'interrupted':
                // INTERRUPÇÃO: para a reprodução imediatamente
                player.flush()
                setStatus('interrupted')
                setStatusDetail('Você interrompeu o agente.')
                addMessage('system', '⚡ Interrupção do usuário')
                setTimeout(() => {
                  setStatus((prev) => (prev === 'interrupted' ? 'listening' : prev))
                }, 1000)
                break

              case 'tool_call':
                setStatus('thinking')
                setStatusDetail(`Executando Tool: ${msg.text}...`)
                addMessage('system', `⚙️ Agente chamou a ferramenta: ${msg.text}`)
                break

              case 'tool_result':
                setStatusDetail(`Tool ${msg.text} concluída.`)
                addMessage('system', `✅ Resposta da ferramenta ${msg.text} enviada ao agente`)
                break

              case 'turn_complete':
                setStatus('listening')
                setStatusDetail('Ouvindo você...')
                break

              case 'ended':
                setStatus('ended')
                setStatusDetail(`Chamada encerrada (${msg.reason || 'finalizada'})`)
                addMessage('system', `Chamada encerrada: ${msg.reason}`)
                hangup()
                break

              case 'error':
                setErrorMsg(msg.message || 'Erro durante a chamada')
                setStatus('ended')
                addMessage('system', `Erro: ${msg.message}`)
                hangup()
                break
            }
          } catch (e) {
            console.error('Erro ao processar JSON do WS:', e)
          }
        } else if (event.data instanceof ArrayBuffer) {
          // Áudio recebido do agente (PCM16 24kHz)
          setStatus('speaking')
          setStatusDetail('Agente falando...')
          player.playChunk(event.data)
        }
      }

      ws.onerror = (e) => {
        console.error('WebSocket error:', e)
        setErrorMsg('Erro na conexão com o servidor de voz.')
        setStatus('ended')
      }

      ws.onclose = () => {
        if (status !== 'ended') {
          setStatus('ended')
          setStatusDetail('Conexão encerrada.')
        }
        cleanup()
      }
    } catch (err) {
      setErrorMsg((err as Error).message)
      setStatus('idle')
      cleanup()
    }
  }

  const cleanup = () => {
    if (recorderRef.current) {
      recorderRef.current.stop()
      recorderRef.current = null
    }
    if (playerRef.current) {
      playerRef.current.close()
      playerRef.current = null
    }
    if (wsRef.current) {
      wsRef.current = null
    }
    setMicStream(null)
    setCallerVolume(0)
    setAgentVolume(0)
  }

  // Monitora volumes em tempo real para os componentes da ElevenLabs
  useEffect(() => {
    if (status === 'idle' || status === 'ended') {
      setCallerVolume(0)
      setAgentVolume(0)
      return
    }

    let rafId: number
    let audioContext: AudioContext | null = null
    let analyser: AnalyserNode | null = null
    let source: MediaStreamAudioSourceNode | null = null

    if (micStream && !isMuted) {
      try {
        const AudioCtx =
          window.AudioContext ||
          (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext
        audioContext = new AudioCtx()
        analyser = audioContext.createAnalyser()
        analyser.fftSize = 64
        analyser.smoothingTimeConstant = 0.5
        source = audioContext.createMediaStreamSource(micStream)
        source.connect(analyser)
      } catch {
        /* ignore */
      }
    }

    const loop = () => {
      // 1. Volume do microfone do Caller
      if (analyser && !isMuted) {
        const data = new Uint8Array(analyser.frequencyBinCount)
        analyser.getByteFrequencyData(data)
        let sum = 0
        for (let i = 0; i < data.length; i++) sum += data[i]
        const vol = Math.min(1, (sum / data.length / 255) * 1.8)
        setCallerVolume(vol)
      } else {
        setCallerVolume(0)
      }

      // 2. Volume de saída do Agent
      if (playerRef.current) {
        setAgentVolume(playerRef.current.getVolume())
      } else {
        setAgentVolume(0)
      }

      rafId = requestAnimationFrame(loop)
    }

    rafId = requestAnimationFrame(loop)

    return () => {
      cancelAnimationFrame(rafId)
      if (source) source.disconnect()
      if (audioContext && audioContext.state !== 'closed') {
        audioContext.close().catch(() => {})
      }
    }
  }, [status, micStream, isMuted])

  const hangup = () => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      try {
        wsRef.current.send(JSON.stringify({ type: 'hangup' }))
      } catch (_) {}
      wsRef.current.close()
    }
    setStatus('ended')
    setStatusDetail('Chamada encerrada.')
    cleanup()
  }

  if (!detail) {
    return (
      <div className="max-w-5xl mx-auto text-sm text-zinc-500">{errorMsg || 'Carregando agente…'}</div>
    )
  }

  const statusLabel =
    status === 'idle'
      ? 'Pronto para iniciar'
      : status === 'connecting'
        ? 'Conectando ao modelo…'
        : status === 'listening'
          ? 'Ouvindo você (fale no microfone)'
          : status === 'thinking'
            ? 'Processando resposta…'
            : status === 'speaking'
              ? 'Agente falando (você pode interromper)'
              : status === 'interrupted'
                ? 'Interrupção detectada!'
                : 'Conversa encerrada'

  return (
    <div className="max-w-5xl mx-auto space-y-6">
      {/* Header */}
      <header className="flex flex-col sm:flex-row sm:items-start sm:justify-between gap-4 border-b border-zinc-200/80 pb-5">
        <div>
          <Link
            to={`/agents/${detail.id}`}
            className="inline-flex items-center gap-1.5 text-xs text-zinc-500 hover:text-zinc-900 transition-colors mb-1"
          >
            <ArrowLeft className="w-3.5 h-3.5" />
            Voltar para {detail.name}
          </Link>
          <div className="flex items-center gap-2">
            <h2 className="text-2xl font-semibold tracking-tight text-zinc-900">Testar: {detail.name}</h2>
            <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-blue-50 text-blue-700 border border-blue-200 font-mono">
              {detail.provider || 'gemini_live'}
            </span>
          </div>
        </div>

        <div className="flex flex-wrap items-end gap-3">
          <label className="flex flex-col gap-1 text-xs font-medium text-zinc-600">
            Versão para teste
            <select
              value={selectedVersion}
              onChange={(e) => setSelectedVersion(e.target.value)}
              disabled={status !== 'idle' && status !== 'ended'}
              className="px-3 py-2 bg-white border border-zinc-200 rounded-lg text-xs text-zinc-900 focus:outline-none focus:ring-1 focus:ring-zinc-900 disabled:opacity-50"
            >
              <option value="draft">Draft (Rascunho atual)</option>
              <option value="latest">Roteamento de Tráfego (Produção / Branches)</option>
              {versions.map((v) => (
                <option key={v.version_id || v.version} value={v.version_id || String(v.version)}>
                  v{v.version} {v.version_id ? `(${v.version_id})` : ''} {v.branch_name ? `• ${v.branch_name}` : ''}
                </option>
              ))}
            </select>
          </label>

          <label className="flex flex-col gap-1 text-xs font-medium text-zinc-600">
            Caller (telefone/origem)
            <input
              type="text"
              value={caller}
              onChange={(e) => setCaller(e.target.value)}
              disabled={status !== 'idle' && status !== 'ended'}
              placeholder="5511999999999"
              className="w-36 px-3 py-2 bg-white border border-zinc-200 rounded-lg text-xs font-mono text-zinc-900 focus:outline-none focus:ring-1 focus:ring-zinc-900 disabled:opacity-50"
            />
          </label>
        </div>
      </header>

      {errorMsg && (
        <div className="p-3 bg-red-50 border border-red-200 rounded-lg text-red-700 text-xs">{errorMsg}</div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-12 gap-6 items-start">
        {/* Console de chamada com Palco ElevenLabs UI */}
        <section className="lg:col-span-5 bg-slate-950 text-white rounded-2xl shadow-[var(--shadow-sheet)] p-6 sm:p-8 flex flex-col items-center gap-6 border border-white/10 relative overflow-hidden">
          {/* Ambient backlight glow */}
          <div
            className={`absolute -top-20 -left-20 w-64 h-64 rounded-full blur-3xl pointer-events-none transition-all duration-700 opacity-25 ${
              status === 'speaking'
                ? 'bg-emerald-500'
                : status === 'thinking'
                  ? 'bg-purple-600'
                  : status === 'interrupted'
                    ? 'bg-rose-500'
                    : status === 'connecting'
                      ? 'bg-sky-400'
                      : 'bg-blue-600'
            }`}
          />

          {/* 3D WebGL Shader Orb da ElevenLabs */}
          <Orb
            agentState={
              status === 'idle' || status === 'ended'
                ? 'idle'
                : status === 'connecting'
                  ? 'connecting'
                  : status === 'thinking'
                    ? 'thinking'
                    : status === 'speaking'
                      ? 'speaking'
                      : status === 'interrupted'
                        ? 'interrupted'
                        : 'listening'
            }
            inputVolume={callerVolume}
            outputVolume={agentVolume}
            className="w-56 h-56 sm:w-60 sm:h-60"
          />

          {/* Status e Feedback textual com ShimmeringText */}
          <div className="text-center space-y-1.5 min-h-[44px] flex flex-col items-center justify-center">
            {status === 'thinking' || status === 'connecting' ? (
              <ShimmeringText
                text={statusDetail || (status === 'thinking' ? 'Processando raciocínio…' : 'Conectando ao modelo…')}
                className="text-sm font-semibold tracking-tight"
                color={status === 'thinking' ? '#c084fc' : '#38bdf8'}
                shimmerColor="#ffffff"
              />
            ) : (
              <span className="font-semibold text-sm block tracking-tight text-white">{statusLabel}</span>
            )}
            {status !== 'thinking' && status !== 'connecting' && (
              <span className="text-[11px] text-zinc-400 block max-w-xs">{statusDetail}</span>
            )}
          </div>

          {/* Visualizador de Áudio (LiveWaveform do Caller ou BarVisualizer do Agent) */}
          <div className="w-full max-w-xs flex flex-col items-center">
            {status === 'speaking' ? (
              <BarVisualizer
                state="speaking"
                analyser={playerRef.current?.getAnalyser()}
                barCount={22}
                className="h-12 w-full"
              />
            ) : (
              <LiveWaveform
                active={status === 'listening' && !isMuted}
                processing={status === 'thinking'}
                mediaStream={micStream}
                barColor={status === 'listening' ? '#60a5fa' : '#a855f7'}
                height={48}
                className="w-full"
              />
            )}
          </div>

          <div className="w-full flex flex-col items-center gap-2">
            {status === 'idle' || status === 'ended' ? (
              <Button onClick={startCall} variant="primary" className="w-full max-w-xs bg-emerald-600 hover:bg-emerald-700 py-3 rounded-full">
                <Play className="w-4 h-4 fill-current" />
                Iniciar Conversa de Voz
              </Button>
            ) : (
              <div className="flex flex-wrap justify-center gap-2 w-full">
                <button
                  type="button"
                  onClick={() => {
                    const next = !isMuted
                    setIsMuted(next)
                    if (recorderRef.current) recorderRef.current.setMuted(next)
                  }}
                  className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full text-xs font-medium transition-colors border ${
                    isMuted
                      ? 'bg-amber-500/20 text-amber-300 border-amber-500/30'
                      : 'bg-zinc-800 text-zinc-300 border-zinc-700'
                  }`}
                >
                  {isMuted ? <MicOff className="w-3.5 h-3.5" /> : <Mic className="w-3.5 h-3.5" />}
                  {isMuted ? 'Desmutar Mic' : 'Mutar Mic'}
                </button>
                <button
                  type="button"
                  onClick={() => setAntiEcho(!antiEcho)}
                  title="Protege contra vazamento de som das caixas no microfone"
                  className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full text-xs font-medium transition-colors border ${
                    antiEcho
                      ? 'bg-emerald-500/20 text-emerald-300 border-emerald-500/30'
                      : 'bg-zinc-800 text-zinc-400 border-zinc-700'
                  }`}
                >
                  <Headphones className="w-3.5 h-3.5" />
                  Anti-Eco: {antiEcho ? 'ON' : 'OFF'}
                </button>
                <button
                  type="button"
                  onClick={hangup}
                  className="inline-flex items-center gap-1.5 px-4 py-1.5 rounded-full text-xs font-semibold bg-red-600 hover:bg-red-700 text-white transition-colors"
                >
                  <Square className="w-3.5 h-3.5 fill-current" />
                  Encerrar
                </button>
              </div>
            )}
          </div>
        </section>

        {/* Transcrição + variáveis */}
        <div className="lg:col-span-7 space-y-6">
          <section className="bg-white border border-zinc-200/80 rounded-xl shadow-xs flex flex-col min-h-[420px]">
            <h3 className="px-5 py-3.5 border-b border-zinc-100 text-xs font-semibold text-zinc-700 uppercase tracking-wider">
              Transcrição ao Vivo
            </h3>
            <div className="flex-1 p-5 overflow-y-auto overscroll-contain space-y-3 max-h-[480px]">
              {transcripts.length === 0 && (
                <p className="text-center text-xs text-zinc-400 py-10">
                  A transcrição aparecerá aqui em tempo real assim que você ou o agente começarem a falar.
                </p>
              )}
              {transcripts.map((m) => (
                <motion.div
                  key={m.id}
                  initial={{ opacity: 0, y: 10, scale: 0.98 }}
                  animate={{ opacity: 1, y: 0, scale: 1 }}
                  transition={springSnappy}
                  className={`flex flex-col ${
                    m.role === 'caller' ? 'items-end' : m.role === 'agent' ? 'items-start' : 'items-center'
                  }`}
                >
                  {m.role === 'system' ? (
                    <span className="text-[10px] text-zinc-400 bg-zinc-100 px-2.5 py-1 rounded-full">{m.text}</span>
                  ) : (
                    <div
                      className={`max-w-[85%] rounded-2xl px-4 py-2.5 text-xs ${
                        m.role === 'caller'
                          ? 'bg-zinc-900 text-white rounded-br-xs'
                          : 'bg-zinc-100 text-zinc-900 rounded-bl-xs border border-zinc-200/60'
                      }`}
                    >
                      <span className="block text-[10px] opacity-70 mb-0.5">
                        {m.role === 'caller' ? 'Você' : detail.name} • {m.timestamp}
                      </span>
                      <p className="leading-relaxed">{m.text}</p>
                    </div>
                  )}
                </motion.div>
              ))}
              <div ref={transcriptsEndRef} />
            </div>
          </section>

          {resolvedVars && (
            <section className="bg-white border border-zinc-200/80 rounded-xl shadow-xs p-5">
              <h3 className="text-xs font-semibold text-zinc-700 uppercase tracking-wider mb-3">
                Variáveis Resolvidas Nesta Sessão
              </h3>
              <div className="space-y-1.5">
                {Object.entries(resolvedVars).map(([k, v]) => (
                  <div key={k} className="flex items-center gap-2 text-xs">
                    <code className="font-mono bg-zinc-100 text-zinc-700 px-1.5 py-0.5 rounded">{`{{${k}}}`}</code>
                    <span className="text-zinc-600">{v}</span>
                  </div>
                ))}
              </div>
            </section>
          )}
        </div>
      </div>
    </div>
  )
}
