import { useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { AnimatePresence, motion } from 'motion/react'
import { springBubble, springTabPill, transitionPage } from '../components/motion'
import {
  ArrowLeft,
  Volume2,
  Download,
  Sparkles,
  Wrench,
  FileCode,
  Terminal,
  Coins,
  Copy,
  Check,
  User,
  Bot,
  ArrowRightLeft,
  ShieldAlert,
} from 'lucide-react'
import { api, type ConversationDetail } from '../api'

type TimelineItem =
  | {
      kind: 'message'
      id: string
      role: 'agent' | 'caller'
      speaker: string
      timeStr: string
      relTimeStr: string
      text: string
      trailingSpace?: boolean
    }
  | {
      kind: 'badge'
      id: string
      badgeType: 'procedure' | 'tool' | 'transfer' | 'interruption' | 'guardian'
      timeStr: string
      label: string
    }

const STANDALONE_PT_WORDS = new Set([
  'a', 'as', 'o', 'os', 'um', 'uma', 'de', 'da', 'do', 'das', 'dos', 'em', 'na', 'no', 'nas', 'nos',
  'por', 'para', 'pra', 'pro', 'com', 'sem', 'sob', 'sobre', 'que', 'se', 'mas', 'ou', 'e', 'é',
  'não', 'sim', 'já', 'só', 'mais', 'bem', 'bom', 'boa', 'muito', 'como', 'quando', 'onde',
  'eu', 'tu', 'ele', 'ela', 'nós', 'você', 'vocês', 'eles', 'elas', 'me', 'te', 'se', 'lhe', 'nos',
  'meu', 'minha', 'seu', 'sua', 'seus', 'suas', 'esse', 'essa', 'isso', 'este', 'esta', 'isto',
  'hoje', 'agora', 'então', 'lá', 'aqui', 'aí', 'ali', 'vou', 'vai', 'foi', 'são', 'está', 'tá',
  'tem', 'ter', 'falar', 'internet', 'dados', 'confirmados', 'encontra', 'saber', 'plano', 'gigas',
])

function appendTranscriptFragment(
  existing: string,
  fragment: string,
  hasSpaceBoundary = true,
): string {
  const prev = existing.trimEnd()
  const next = fragment.trim()
  if (!prev) return next
  if (!next) return prev
  // Se o novo fragmento começa com pontuação, cola sem espaço extra
  if (/^[.,!?;:)\]]/.test(next)) {
    return `${prev}${next}`
  }
  if (hasSpaceBoundary) {
    return `${prev} ${next}`
  }
  const lastWordMatch = prev.match(/([a-záàâãéêíóôõúçA-ZÁÀÂÃÉÊÍÓÔÕÚÇ]+)$/)
  const firstWordMatch = next.match(/^([a-záàâãéêíóôõúçA-ZÁÀÂÃÉÊÍÓÔÕÚÇ]+)/)
  if (lastWordMatch && firstWordMatch) {
    const lw = lastWordMatch[1].toLowerCase()
    const fw = firstWordMatch[1].toLowerCase()
    if (STANDALONE_PT_WORDS.has(lw) || STANDALONE_PT_WORDS.has(fw)) {
      return `${prev} ${next}`
    }
  }
  // Se o delta do modelo veio sem espaço nos dois lados e continua uma sub-palavra (ex: "gentile" + "za")
  if (
    /[a-záàâãéêíóôõúçA-ZÁÀÂÃÉÊÍÓÔÕÚÇ]$/.test(prev) &&
    /^[a-záàâãéêíóôõúç]/.test(next)
  ) {
    return `${prev}${next}`
  }
  return `${prev} ${next}`
}

function formatRelativeTime(startIso: string, currentIso: string): string {
  const s = new Date(startIso).getTime()
  const c = new Date(currentIso).getTime()
  if (isNaN(s) || isNaN(c) || c < s) return '0:00'
  const totalSecs = Math.floor((c - s) / 1000)
  const mins = Math.floor(totalSecs / 60)
  const secs = String(totalSecs % 60).padStart(2, '0')
  return `${mins}:${secs}`
}

export default function ConversationDetailPage() {
  const { id } = useParams<{ id: string }>()
  const [detail, setDetail] = useState<ConversationDetail | null>(null)
  const [errorMsg, setErrorMsg] = useState('')
  const [activeTab, setActiveTab] = useState<'transcript' | 'logs'>('transcript')
  const [copiedAll, setCopiedAll] = useState(false)
  const [copiedBubbleId, setCopiedBubbleId] = useState<string | null>(null)

  useEffect(() => {
    api<ConversationDetail>(`/api/conversations/${id}`)
      .then(setDetail)
      .catch((e) => setErrorMsg(e.message))
  }, [id])

  // Agrupa fragmentos consecutivos de fala do mesmo interlocutor em mensagens completas e legíveis
  const timeline = useMemo<TimelineItem[]>(() => {
    if (!detail) return []
    const { conversation: c, agent, events } = detail
    const items: TimelineItem[] = []
    let currentAgentName = agent?.name || 'Agente'
    let forceNewBubble = false

    for (const ev of events || []) {
      const timeStr = new Date(ev.created_at).toLocaleTimeString('pt-BR')
      const relTimeStr = formatRelativeTime(c.started_at, ev.created_at)
      const p = ev.payload || {}

      if (ev.type === 'turn_complete') {
        forceNewBubble = true
        continue
      }

      if (ev.type === 'caller_transcript' || ev.type === 'agent_transcript') {
        const role: 'caller' | 'agent' = ev.type === 'caller_transcript' ? 'caller' : 'agent'
        const rawOrig = String(p.text || '')
        const hasLeadingSpace = /^\s/.test(rawOrig)
        const hasTrailingSpace = /\s$/.test(rawOrig)
        const rawText = rawOrig.trim()
        if (!rawText) {
          const prevItem = items[items.length - 1]
          if (prevItem && prevItem.kind === 'message' && prevItem.role === role) {
            prevItem.trailingSpace = true
          }
          continue
        }
        if (
          role === 'agent' &&
          (/call has been terminated|call is terminated|ending the call/i.test(rawText) ||
            rawText.includes('**') ||
            rawText.includes('*   '))
        ) {
          continue
        }

        const speaker = role === 'caller' ? 'Cliente (Caller)' : currentAgentName
        const last = items[items.length - 1]

        if (
          !forceNewBubble &&
          last &&
          last.kind === 'message' &&
          last.role === role &&
          last.speaker === speaker
        ) {
          const boundary = Boolean(last.trailingSpace || hasLeadingSpace)
          last.text = appendTranscriptFragment(last.text, rawText, boundary)
          last.trailingSpace = hasTrailingSpace
        } else {
          items.push({
            kind: 'message',
            id: String(ev.id),
            role,
            speaker,
            timeStr,
            relTimeStr,
            text: rawText,
            trailingSpace: hasTrailingSpace,
          })
          forceNewBubble = false
        }
        continue
      }

      if (ev.type === 'procedure_activated' || ev.type === 'skill_activated') {
        forceNewBubble = true
        items.push({
          kind: 'badge',
          id: String(ev.id),
          badgeType: 'procedure',
          timeStr,
          label: `Procedimento ativado: ${p.name || ''}`,
        })
        continue
      }

      if (ev.type === 'transfer_to_agent') {
        forceNewBubble = true
        if (p.to_agent_name) {
          currentAgentName = String(p.to_agent_name)
        }
        items.push({
          kind: 'badge',
          id: String(ev.id),
          badgeType: 'transfer',
          timeStr,
          label: `Conversa transferida para ${p.to_agent_name || 'outro agente'}`,
        })
        continue
      }

      if (ev.type === 'tool_call') {
        forceNewBubble = true
        items.push({
          kind: 'badge',
          id: String(ev.id),
          badgeType: 'tool',
          timeStr,
          label: `Ferramenta acionada: ${p.name || ''}`,
        })
        continue
      }

      if (ev.type === 'interruption') {
        forceNewBubble = true
        items.push({
          kind: 'badge',
          id: String(ev.id),
          badgeType: 'interruption',
          timeStr,
          label: 'Interrupção do cliente (barge-in)',
        })
        continue
      }

      if (ev.type === 'guardian_alert') {
        forceNewBubble = true
        let label = `Guardião Jev: ${p.reason || 'Alerta detectado'}`
        if (p.kind === 'voicemail') {
          const pct = Math.round(Number(p.is_voicemail ?? 0.99) * 100)
          label = `Guardião Jev: Caixa Postal detectada (${pct}%) — Chamada encerrada na hora`
        } else if (p.kind === 'hallucination') {
          const pct = Math.round(Number(p.agent_hallucinating ?? 0.85) * 100)
          const act =
            p.action === 'hangup'
              ? 'Chamada encerrada por segurança'
              : 'Correção injetada em tempo real'
          label = `Guardião Jev: Alucinação detectada (${pct}%) — ${act}`
        } else if (p.kind === 'talking_to_self') {
          const pct = Math.round(Number(p.talking_to_self ?? 0.85) * 100)
          const act =
            p.action === 'hangup'
              ? 'Chamada encerrada por silêncio do cliente'
              : 'Solicitada confirmação na linha'
          label = `Guardião Jev: Cliente mudo / Agente falando sozinho (${pct}%) — ${act}`
        }
        items.push({
          kind: 'badge',
          id: String(ev.id),
          badgeType: 'guardian',
          timeStr,
          label,
        })
      }
    }

    return items
  }, [detail])

  // Gera texto formatado completo da transcrição para copiar em 1 clique
  const formattedTranscriptText = useMemo(() => {
    if (!detail) return ''
    const { conversation: c, agent } = detail
    const header = `Transcrição da Conversa — ${agent?.name || 'Agente'} (${new Date(
      c.started_at,
    ).toLocaleString('pt-BR')})`
    const lines = timeline.map((item) => {
      if (item.kind === 'message') {
        return `[${item.timeStr}] ${item.speaker}: ${item.text}`
      }
      return `[${item.timeStr}] --- ${item.label} ---`
    })
    return [header, '----------------------------------------', ...lines].join('\n')
  }, [detail, timeline])

  const handleCopyFullTranscript = () => {
    if (!formattedTranscriptText) return
    navigator.clipboard.writeText(formattedTranscriptText)
    setCopiedAll(true)
    setTimeout(() => setCopiedAll(false), 2000)
  }

  const handleCopyBubble = (id: string, speaker: string, text: string) => {
    navigator.clipboard.writeText(`${speaker}: ${text}`)
    setCopiedBubbleId(id)
    setTimeout(() => setCopiedBubbleId(null), 1500)
  }

  if (!detail) {
    return (
      <div className="flex items-center justify-center p-12 text-zinc-500 text-sm">
        {errorMsg || 'Carregando detalhes da conversa…'}
      </div>
    )
  }

  const { conversation: c, agent, events, tool_executions: execs } = detail

  const formatDuration = (ms: number | null) => {
    if (!ms) return '0s'
    const totalSecs = Math.round(ms / 1000)
    const mins = Math.floor(totalSecs / 60)
    const secs = totalSecs % 60
    return mins > 0 ? `${mins}m ${secs}s` : `${secs}s`
  }

  return (
    <div className="max-w-5xl mx-auto space-y-6">
      {/* Top Header */}
      <div className="space-y-2 border-b border-zinc-200/80 pb-5">
        <Link
          to="/conversations"
          className="inline-flex items-center gap-1.5 text-xs font-medium text-zinc-500 hover:text-zinc-900 transition-colors"
        >
          <ArrowLeft className="w-3.5 h-3.5" />
          Voltar para Conversas
        </Link>

        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
          <div>
            <div className="flex items-center gap-2.5">
              <h1 className="text-2xl font-bold tracking-tight text-zinc-900">
                Conversa com {agent?.name || 'Agente'}
              </h1>
              <span
                className={`inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium ${
                  c.status === 'completed'
                    ? 'bg-emerald-50 text-emerald-700 border border-emerald-200'
                    : 'bg-zinc-100 text-zinc-700'
                }`}
              >
                <span
                  className={`w-1.5 h-1.5 rounded-full ${
                    c.status === 'completed' ? 'bg-emerald-500' : 'bg-zinc-400'
                  }`}
                />
                {c.status === 'completed' ? 'Finalizada' : c.status}
              </span>
              <span
                className={`text-[10px] px-2 py-0.5 rounded font-mono font-medium ${
                  c.is_test
                    ? 'bg-amber-50 text-amber-800 border border-amber-200'
                    : 'bg-zinc-100 text-zinc-700'
                }`}
              >
                {c.is_test ? 'Teste (Draft)' : 'Produção'}
              </span>
            </div>
            <p className="text-xs text-zinc-500 mt-1">
              Iniciada em {new Date(c.started_at).toLocaleString('pt-BR')} • Duração:{' '}
              {formatDuration(c.duration_ms)} • Motivo: {c.ended_reason}
            </p>
          </div>

          {/* Botão rápido de copiar transcrição em 1 clique no topo */}
          <button
            type="button"
            onClick={handleCopyFullTranscript}
            className={`inline-flex items-center gap-2 px-3.5 py-2 rounded-xl border text-xs font-semibold shadow-2xs transition-colors cursor-pointer ${
              copiedAll
                ? 'bg-emerald-50 border-emerald-200 text-emerald-700'
                : 'bg-white hover:bg-zinc-50 border-zinc-200/90 text-zinc-900'
            }`}
          >
            {copiedAll ? (
              <>
                <Check className="w-3.5 h-3.5 text-emerald-600" />
                <span>Transcrição copiada!</span>
              </>
            ) : (
              <>
                <Copy className="w-3.5 h-3.5 text-zinc-600" />
                <span>Copiar transcrição</span>
              </>
            )}
          </button>
        </div>
      </div>

      {errorMsg && (
        <div className="p-3 bg-red-50 border border-red-200 rounded-lg text-red-700 text-xs">
          {errorMsg}
        </div>
      )}

      {/* Cost & Tokens Card */}
      <div className="bg-white border border-zinc-200/80 rounded-xl p-5 shadow-xs space-y-4">
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 border-b border-zinc-100 pb-3">
          <div className="flex items-center gap-2">
            <Coins className="w-4 h-4 text-emerald-600" />
            <h3 className="text-xs font-semibold text-zinc-900 uppercase tracking-wider">
              Consumo de Tokens & Custo da Chamada
            </h3>
          </div>
          <div className="flex items-center gap-3">
            <span className="text-[11px] font-mono text-zinc-500">
              Método:{' '}
              {c.tokens_usage?.method === 'tokens'
                ? 'Tokens Reais da API'
                : 'Estimativa por Duração'}
            </span>
            <div className="font-mono text-sm font-bold text-emerald-700 bg-emerald-50 px-2.5 py-1 rounded border border-emerald-200">
              ${(c.cost_usd || 0).toFixed(4)}{' '}
              <span className="text-xs font-normal text-zinc-600">
                (R$ {(c.cost_brl || 0).toFixed(2)})
              </span>
            </div>
          </div>
        </div>

        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 text-xs">
          <div className="bg-zinc-50/80 border border-zinc-200/60 rounded-lg p-3">
            <span className="text-zinc-500 block text-[11px] mb-1">Áudio Entrada (Caller)</span>
            <div className="font-mono font-semibold text-zinc-900">
              {c.tokens_usage?.tokens_usage?.input_audio_tokens?.toLocaleString() || 0} tokens
            </div>
            <div className="text-[10px] text-zinc-400 mt-1 font-mono">
              ${(c.tokens_usage?.input_audio_cost_usd || 0).toFixed(4)}
            </div>
          </div>

          <div className="bg-zinc-50/80 border border-zinc-200/60 rounded-lg p-3">
            <span className="text-zinc-500 block text-[11px] mb-1">Áudio Saída (Agente)</span>
            <div className="font-mono font-semibold text-zinc-900">
              {c.tokens_usage?.tokens_usage?.output_audio_tokens?.toLocaleString() || 0} tokens
            </div>
            <div className="text-[10px] text-zinc-400 mt-1 font-mono">
              ${(c.tokens_usage?.output_audio_cost_usd || 0).toFixed(4)}
            </div>
          </div>

          <div className="bg-zinc-50/80 border border-zinc-200/60 rounded-lg p-3">
            <span className="text-zinc-500 block text-[11px] mb-1">Texto Entrada (Prompt / MCP)</span>
            <div className="font-mono font-semibold text-zinc-900">
              {c.tokens_usage?.tokens_usage?.input_text_tokens?.toLocaleString() || 0} tokens
            </div>
            <div className="text-[10px] text-zinc-400 mt-1 font-mono">
              ${(c.tokens_usage?.input_text_cost_usd || 0).toFixed(4)}
            </div>
          </div>

          <div className="bg-zinc-50/80 border border-zinc-200/60 rounded-lg p-3">
            <span className="text-zinc-500 block text-[11px] mb-1">Total de Tokens</span>
            <div className="font-mono font-semibold text-zinc-900">
              {(c.tokens_usage?.tokens_usage?.total_tokens || 0).toLocaleString()} tokens
            </div>
            <div className="text-[10px] text-emerald-600 font-medium mt-1">{c.model}</div>
          </div>
        </div>
      </div>

      {/* Audio Player Card */}
      <div className="bg-white border border-zinc-200/80 rounded-xl p-5 shadow-xs space-y-3">
        <div className="flex items-center justify-between">
          <h3 className="text-xs font-semibold text-zinc-700 uppercase tracking-wider flex items-center gap-1.5">
            <Volume2 className="w-4 h-4 text-zinc-500" />
            Gravação de Áudio Estéreo (PCM16 WAV 24 kHz)
          </h3>
          {c.recording_key && (
            <a
              href={`/api/conversations/${c.id}/audio`}
              download={`conversa-${c.id}.wav`}
              className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-zinc-100 hover:bg-zinc-200 border border-zinc-200 rounded-lg text-xs font-medium text-zinc-800 transition-colors"
            >
              <Download className="w-3.5 h-3.5" />
              Baixar WAV
            </a>
          )}
        </div>

        {c.recording_key ? (
          <div className="space-y-2">
            <audio controls src={`/api/conversations/${c.id}/audio`} className="w-full h-10" />
            <p className="text-[11px] text-zinc-400">
              Canal Esquerdo: Áudio do Cliente • Canal Direito: Áudio Nativo do Agente (24 kHz)
            </p>
          </div>
        ) : (
          <p className="text-xs text-zinc-400 italic">
            Nenhuma gravação de áudio registrada para esta conversa.
          </p>
        )}
      </div>

      {/* Tabs: Transcript vs Execution Logs */}
      <div className="border-b border-zinc-200 flex items-center justify-between gap-4 text-xs font-medium">
        <div className="flex gap-4">
          <button
            onClick={() => setActiveTab('transcript')}
            className={`relative pb-2.5 -mb-px transition-colors cursor-pointer ${
              activeTab === 'transcript'
                ? 'text-zinc-950 font-semibold'
                : 'text-zinc-500 hover:text-zinc-800'
            }`}
          >
            Transcrição da Conversa
            {activeTab === 'transcript' && (
              <motion.span
                layoutId="conv-detail-tab-indicator"
                transition={springTabPill}
                className="absolute inset-x-0 bottom-0 h-0.5 bg-zinc-900 rounded-full"
                aria-hidden="true"
              />
            )}
          </button>
          <button
            onClick={() => setActiveTab('logs')}
            className={`relative pb-2.5 -mb-px transition-colors cursor-pointer flex items-center gap-1.5 ${
              activeTab === 'logs'
                ? 'text-zinc-950 font-semibold'
                : 'text-zinc-500 hover:text-zinc-800'
            }`}
          >
            <Terminal className="w-3.5 h-3.5" />
            Logs de Execução & Tools ({execs?.length || 0})
            {activeTab === 'logs' && (
              <motion.span
                layoutId="conv-detail-tab-indicator"
                transition={springTabPill}
                className="absolute inset-x-0 bottom-0 h-0.5 bg-zinc-900 rounded-full"
                aria-hidden="true"
              />
            )}
          </button>
        </div>

        {activeTab === 'transcript' && timeline.length > 0 && (
          <button
            type="button"
            onClick={handleCopyFullTranscript}
            className="mb-1.5 inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg text-xs font-medium text-zinc-600 hover:text-zinc-950 hover:bg-zinc-100 transition-colors cursor-pointer"
          >
            {copiedAll ? (
              <>
                <Check className="w-3.5 h-3.5 text-emerald-600" />
                <span className="text-emerald-700 font-semibold">Copiado!</span>
              </>
            ) : (
              <>
                <Copy className="w-3.5 h-3.5" />
                <span>Copiar tudo em 1 clique</span>
              </>
            )}
          </button>
        )}
      </div>

      {/* Tab 1: Transcript Consolidada e Legível */}
      <AnimatePresence mode="wait">
      {activeTab === 'transcript' && (
        <motion.div
          key="tab-transcript"
          initial={{ opacity: 0, y: 6 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, transition: { duration: 0.12 } }}
          transition={transitionPage}
          className="bg-white border border-zinc-200/80 rounded-2xl p-6 shadow-xs space-y-4"
        >
          {timeline.length === 0 ? (
            <div className="py-12 text-center text-xs text-zinc-400">
              Nenhuma fala registrada na transcrição desta conversa.
            </div>
          ) : (
            <div className="space-y-4">
              {timeline.map((item, idx) => {
                if (item.kind === 'badge') {
                  return (
                    <motion.div
                      key={item.id}
                      initial={{ opacity: 0, scale: 0.94, y: 4 }}
                      animate={{ opacity: 1, scale: 1, y: 0 }}
                      transition={{ ...springBubble, delay: Math.min(idx, 12) * 0.018 }}
                      className="flex justify-center my-1.5"
                    >
                      <span
                        className={`inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-[11px] font-medium border ${
                          item.badgeType === 'procedure'
                            ? 'bg-amber-50 text-amber-800 border-amber-200'
                            : item.badgeType === 'transfer'
                              ? 'bg-indigo-50 text-indigo-800 border-indigo-200'
                              : item.badgeType === 'tool'
                                ? 'bg-sky-50 text-sky-800 border-sky-200'
                                : item.badgeType === 'guardian'
                                  ? 'bg-rose-50 text-rose-800 border-rose-200 font-semibold'
                                  : 'bg-zinc-100 text-zinc-600 border-zinc-200'
                        }`}
                      >
                        {item.badgeType === 'procedure' && (
                          <Sparkles className="w-3 h-3 text-amber-600" />
                        )}
                        {item.badgeType === 'transfer' && (
                          <ArrowRightLeft className="w-3 h-3 text-indigo-600" />
                        )}
                        {item.badgeType === 'tool' && <Wrench className="w-3 h-3 text-sky-600" />}
                        {item.badgeType === 'guardian' && (
                          <ShieldAlert className="w-3.5 h-3.5 text-rose-600" />
                        )}
                        <span>{item.label}</span>
                        <span className="opacity-60 font-mono text-[10px]">• {item.timeStr}</span>
                      </span>
                    </motion.div>
                  )
                }

                const isCaller = item.role === 'caller'
                const isCopied = copiedBubbleId === item.id

                return (
                  <motion.div
                    key={item.id}
                    initial={{ opacity: 0, x: isCaller ? 8 : -8, y: 4 }}
                    animate={{ opacity: 1, x: 0, y: 0 }}
                    transition={{ ...springBubble, delay: Math.min(idx, 12) * 0.018 }}
                    className={`flex items-start gap-3 group ${
                      isCaller ? 'justify-end' : 'justify-start'
                    }`}
                  >
                    {/* Avatar do Agente à esquerda */}
                    {!isCaller && (
                      <div className="w-7 h-7 rounded-full bg-gradient-to-tr from-indigo-500 via-purple-400 to-sky-300 flex items-center justify-center text-white flex-shrink-0 mt-0.5 shadow-2xs">
                        <Bot className="w-3.5 h-3.5" />
                      </div>
                    )}

                    <div
                      className={`relative max-w-[78%] rounded-2xl px-4 py-3 shadow-2xs transition-colors ${
                        isCaller
                          ? 'bg-zinc-900 text-white rounded-br-xs'
                          : 'bg-[#f4f4f6] border border-zinc-200/70 text-zinc-900 rounded-bl-xs'
                      }`}
                    >
                      <div className="flex items-center justify-between gap-4 mb-1">
                        <div className="flex items-center gap-1.5 text-[11px]">
                          <span
                            className={`font-semibold ${
                              isCaller ? 'text-zinc-200' : 'text-zinc-800'
                            }`}
                          >
                            {item.speaker}
                          </span>
                          <span className={isCaller ? 'text-zinc-400' : 'text-zinc-400'}>•</span>
                          <span
                            className={`font-mono text-[10px] ${
                              isCaller ? 'text-zinc-400' : 'text-zinc-500'
                            }`}
                          >
                            {item.timeStr} ({item.relTimeStr})
                          </span>
                        </div>

                        <button
                          type="button"
                          onClick={() => handleCopyBubble(item.id, item.speaker, item.text)}
                          title="Copiar esta mensagem"
                          className={`opacity-0 group-hover:opacity-100 transition-opacity p-1 rounded cursor-pointer ${
                            isCaller
                              ? 'text-zinc-400 hover:text-white hover:bg-zinc-800'
                              : 'text-zinc-400 hover:text-zinc-800 hover:bg-zinc-200/70'
                          }`}
                        >
                          {isCopied ? (
                            <Check className="w-3 h-3 text-emerald-500" />
                          ) : (
                            <Copy className="w-3 h-3" />
                          )}
                        </button>
                      </div>

                      <p className="text-[13.5px] leading-relaxed whitespace-pre-wrap break-words">
                        {item.text}
                      </p>
                    </div>

                    {/* Avatar do Cliente à direita */}
                    {isCaller && (
                      <div className="w-7 h-7 rounded-full bg-zinc-900 text-white flex items-center justify-center flex-shrink-0 mt-0.5 shadow-2xs">
                        <User className="w-3.5 h-3.5" />
                      </div>
                    )}
                  </motion.div>
                )
              })}
            </div>
          )}
        </motion.div>
      )}

      {/* Tab 2: Execution Logs */}
      {activeTab === 'logs' && (
        <motion.div
          key="tab-logs"
          initial={{ opacity: 0, y: 6 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, transition: { duration: 0.12 } }}
          transition={transitionPage}
          className="space-y-4"
        >
          <div className="bg-white border border-zinc-200/80 rounded-xl p-5 shadow-xs space-y-3">
            <h4 className="text-xs font-semibold text-zinc-700 uppercase tracking-wider flex items-center gap-1.5">
              <Wrench className="w-4 h-4 text-zinc-500" />
              Chamadas de Ferramentas ({execs?.length || 0})
            </h4>

            {!execs || execs.length === 0 ? (
              <p className="text-xs text-zinc-400 italic">
                Nenhuma ferramenta foi acionada nesta chamada.
              </p>
            ) : (
              <div className="space-y-3">
                {execs.map((ex) => (
                  <div
                    key={ex.id}
                    className="p-4 bg-zinc-50 border border-zinc-200 rounded-xl text-xs space-y-2"
                  >
                    <div className="flex items-center justify-between">
                      <span className="font-mono font-bold text-zinc-900 flex items-center gap-1.5">
                        <Wrench className="w-3.5 h-3.5 text-zinc-500" />@{ex.tool_name}
                      </span>
                      <span className="text-[11px] font-mono text-zinc-500">
                        {ex.execution_time_ms}ms • Status: {ex.status}
                      </span>
                    </div>

                    <div className="grid grid-cols-1 md:grid-cols-2 gap-3 pt-2">
                      <div>
                        <span className="text-[10px] text-zinc-500 uppercase font-semibold block mb-1">
                          Request Payload
                        </span>
                        <pre className="bg-zinc-900 text-zinc-100 p-2.5 rounded-lg text-[11px] font-mono overflow-x-auto">
                          {JSON.stringify(ex.request, null, 2)}
                        </pre>
                      </div>
                      <div>
                        <span className="text-[10px] text-zinc-500 uppercase font-semibold block mb-1">
                          Response
                        </span>
                        <pre className="bg-zinc-900 text-zinc-100 p-2.5 rounded-lg text-[11px] font-mono overflow-x-auto">
                          {JSON.stringify(ex.response, null, 2)}
                        </pre>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          <div className="bg-white border border-zinc-200/80 rounded-xl p-5 shadow-xs space-y-2">
            <h4 className="text-xs font-semibold text-zinc-700 uppercase tracking-wider flex items-center gap-1.5">
              <FileCode className="w-4 h-4 text-zinc-500" />
              Eventos Brutos do WebSocket ({events.length})
            </h4>
            <div className="bg-zinc-900 text-zinc-200 p-4 rounded-xl text-[11px] font-mono max-h-72 overflow-y-auto overscroll-contain space-y-1">
              {events.map((ev) => (
                <div key={ev.id} className="truncate">
                  <span className="text-zinc-500">
                    [{new Date(ev.created_at).toLocaleTimeString('pt-BR')}]
                  </span>{' '}
                  <span className="text-emerald-400 font-semibold">{ev.type}</span>:{' '}
                  <span className="text-zinc-400">{JSON.stringify(ev.payload)}</span>
                </div>
              ))}
            </div>
          </div>
        </motion.div>
      )}
      </AnimatePresence>
    </div>
  )
}
