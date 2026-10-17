import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import {
  Calendar,
  ChevronLeft,
  ChevronRight,
  ChevronDown,
  ChevronUp,
  Download,
  ArrowUpRight,
  Wrench,
  FolderOpen,
  Cpu,
  Check,
  Pencil,
  Plus,
} from 'lucide-react'
import {
  api,
  type DashboardMetrics,
  type Conversation,
  type Agent,
  type ToolWithStats,
  type KnowledgeBase,
  type Me,
} from '../api'

type ActiveMetricTab =
  | 'conversations'
  | 'avg_duration'
  | 'total_cost'
  | 'avg_cost'
  | 'avg_latency'
  | 'success_rate'

type DashboardSectionTab = 'geral' | 'ferramentas' | 'llms' | 'kb'

type PresetId =
  | '15m'
  | '30m'
  | '1h'
  | '3h'
  | '6h'
  | '12h'
  | '24h'
  | 'yesterday'
  | 'today'
  | '7d'
  | '30d'
  | '90d'
  | 'custom'

interface PresetOption {
  id: PresetId
  label: string
}

const PRESET_OPTIONS: PresetOption[] = [
  { id: '15m', label: 'Últimos 15 minutos' },
  { id: '30m', label: 'Últimos 30 minutos' },
  { id: '1h', label: 'Última 1 hora' },
  { id: '3h', label: 'Últimas 3 horas' },
  { id: '6h', label: 'Últimas 6 horas' },
  { id: '12h', label: 'Últimas 12 horas' },
  { id: '24h', label: 'Últimas 24 horas' },
  { id: 'yesterday', label: 'Ontem' },
  { id: 'today', label: 'Hoje' },
  { id: '7d', label: 'Últimos 7 dias' },
  { id: '30d', label: 'Últimos 30 dias' },
  { id: '90d', label: 'Últimos 90 dias' },
]

const MONTH_NAMES_PT = [
  'janeiro',
  'fevereiro',
  'março',
  'abril',
  'maio',
  'junho',
  'julho',
  'agosto',
  'setembro',
  'outubro',
  'novembro',
  'dezembro',
]

const WEEKDAYS_PT = ['seg', 'ter', 'qua', 'qui', 'sex', 'sáb', 'dom']

function computePresetRange(preset: PresetId): { start: Date; end: Date } {
  const now = new Date()
  const end = new Date(now)
  const start = new Date(now)

  switch (preset) {
    case '15m':
      start.setTime(now.getTime() - 15 * 60 * 1000)
      break
    case '30m':
      start.setTime(now.getTime() - 30 * 60 * 1000)
      break
    case '1h':
      start.setTime(now.getTime() - 60 * 60 * 1000)
      break
    case '3h':
      start.setTime(now.getTime() - 3 * 60 * 60 * 1000)
      break
    case '6h':
      start.setTime(now.getTime() - 6 * 60 * 60 * 1000)
      break
    case '12h':
      start.setTime(now.getTime() - 12 * 60 * 60 * 1000)
      break
    case '24h':
      start.setTime(now.getTime() - 24 * 60 * 60 * 1000)
      break
    case 'yesterday': {
      const yStart = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1, 0, 0, 0, 0)
      const yEnd = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1, 23, 59, 59, 999)
      return { start: yStart, end: yEnd }
    }
    case 'today': {
      const tStart = new Date(now.getFullYear(), now.getMonth(), now.getDate(), 0, 0, 0, 0)
      return { start: tStart, end }
    }
    case '7d':
      start.setTime(now.getTime() - 7 * 24 * 60 * 60 * 1000)
      start.setHours(0, 0, 0, 0)
      break
    case '30d':
      start.setTime(now.getTime() - 30 * 24 * 60 * 60 * 1000)
      start.setHours(0, 0, 0, 0)
      break
    case '90d':
      start.setTime(now.getTime() - 90 * 24 * 60 * 60 * 1000)
      start.setHours(0, 0, 0, 0)
      break
    default:
      start.setTime(now.getTime() - 7 * 24 * 60 * 60 * 1000)
      break
  }
  return { start, end }
}

function formatDateTimeInput(d: Date): string {
  const dd = String(d.getDate()).padStart(2, '0')
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const yyyy = d.getFullYear()
  const hh = String(d.getHours()).padStart(2, '0')
  const min = String(d.getMinutes()).padStart(2, '0')
  return `${dd} / ${mm} / ${yyyy} , ${hh} : ${min}`
}

function parseDateTimeInput(str: string, fallback: Date): Date {
  const cleaned = str.replace(/\s+/g, '')
  const match = cleaned.match(/^(\d{2})\/(\d{2})\/(\d{4}),(\d{2}):(\d{2})$/)
  if (!match) return fallback
  const [, dd, mm, yyyy, hh, min] = match
  const parsed = new Date(
    Number(yyyy),
    Number(mm) - 1,
    Number(dd),
    Number(hh),
    Number(min),
    0,
    0,
  )
  return isNaN(parsed.getTime()) ? fallback : parsed
}

function isSameDay(a: Date, b: Date): boolean {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  )
}

function isBetweenDays(day: Date, start: Date, end: Date): boolean {
  const d = new Date(day.getFullYear(), day.getMonth(), day.getDate()).getTime()
  const s = new Date(start.getFullYear(), start.getMonth(), start.getDate()).getTime()
  const e = new Date(end.getFullYear(), end.getMonth(), end.getDate()).getTime()
  return d > Math.min(s, e) && d < Math.max(s, e)
}

function buildCalendarCells(year: number, month: number) {
  const firstOfMonth = new Date(year, month, 1)
  // Segunda-feira = 0 ... Domingo = 6
  const startDayOfWeek = (firstOfMonth.getDay() + 6) % 7
  const daysInMonth = new Date(year, month + 1, 0).getDate()
  const daysInPrevMonth = new Date(year, month, 0).getDate()

  const cells: { date: Date; inCurrentMonth: boolean }[] = []

  // Dias do mês anterior
  for (let i = startDayOfWeek - 1; i >= 0; i--) {
    const dayNum = daysInPrevMonth - i
    cells.push({
      date: new Date(year, month - 1, dayNum),
      inCurrentMonth: false,
    })
  }

  // Dias do mês atual
  for (let d = 1; d <= daysInMonth; d++) {
    cells.push({
      date: new Date(year, month, d),
      inCurrentMonth: true,
    })
  }

  // Completa 42 células (6 semanas)
  let nextMonthDay = 1
  while (cells.length < 42) {
    cells.push({
      date: new Date(year, month + 1, nextMonthDay++),
      inCurrentMonth: false,
    })
  }

  return cells
}

export default function Dashboard() {
  const navigate = useNavigate()
  const [me, setMe] = useState<Me | null>(null)
  const [metrics, setMetrics] = useState<DashboardMetrics | null>(null)
  const [conversations, setConversations] = useState<Conversation[]>([])
  const [agents, setAgents] = useState<Agent[]>([])
  const [tools, setTools] = useState<ToolWithStats[]>([])
  const [kbs, setKbs] = useState<KnowledgeBase[]>([])
  const [errorMsg, setErrorMsg] = useState('')

  // Controles de navegação e filtros reais (Estilo Imagem 1)
  const [sectionTab, setSectionTab] = useState<DashboardSectionTab>('geral')
  const [activeMetric, setActiveMetric] = useState<ActiveMetricTab>('conversations')

  // Estado do seletor de período com Popover estilo ElevenLabs (Imagem 1)
  const initialRange = useMemo(() => computePresetRange('7d'), [])
  const [preset, setPreset] = useState<PresetId>('7d')
  const [rangeStart, setRangeStart] = useState<Date>(initialRange.start)
  const [rangeEnd, setRangeEnd] = useState<Date>(initialRange.end)

  const [isDatePopoverOpen, setIsDatePopoverOpen] = useState(false)
  const [draftPreset, setDraftPreset] = useState<PresetId>('7d')
  const [draftStart, setDraftStart] = useState<Date>(initialRange.start)
  const [draftEnd, setDraftEnd] = useState<Date>(initialRange.end)
  const [draftStartInput, setDraftStartInput] = useState<string>(
    formatDateTimeInput(initialRange.start),
  )
  const [draftEndInput, setDraftEndInput] = useState<string>(
    formatDateTimeInput(initialRange.end),
  )
  const [selectingSecondDay, setSelectingSecondDay] = useState(false)
  const [calYear, setCalYear] = useState<number>(initialRange.start.getFullYear())
  const [calMonth, setCalMonth] = useState<number>(initialRange.start.getMonth())

  // Outros filtros da barra (Granularidade, Agente, Agrupar por, Visualização empilhada)
  const [granularity, setGranularity] = useState<'minute' | 'hour' | 'day'>('hour')
  const [selectedAgentId, setSelectedAgentId] = useState<string>('all')
  const [isAgentMenuOpen, setIsAgentMenuOpen] = useState(false)
  const [groupBy, setGroupBy] = useState<'none' | 'agent' | 'provider'>('none')
  const [stackedView, setStackedView] = useState(false)
  const [scaleMode, setScaleMode] = useState<'linear' | 'log'>('linear')
  const [hoverIdx, setHoverIdx] = useState<number | null>(null)

  useEffect(() => {
    Promise.all([
      api<Me>('/api/auth/me').catch(() => null),
      api<DashboardMetrics>('/api/dashboard').catch(() => null),
      api<Conversation[]>('/api/conversations?include_test=true&limit=500').catch(() => []),
      api<Agent[]>('/api/agents').catch(() => []),
      api<ToolWithStats[]>('/api/tools').catch(() => []),
      api<KnowledgeBase[]>('/api/knowledge-bases').catch(() => []),
    ])
      .then(([meRes, dashRes, convRes, agRes, toolRes, kbRes]) => {
        if (meRes) setMe(meRes)
        if (dashRes) setMetrics(dashRes)
        setConversations(convRes || [])
        setAgents(agRes || [])
        setTools(toolRes || [])
        setKbs(kbRes || [])
      })
      .catch((e) => setErrorMsg(e.message))
  }, [])

  const openDatePopover = () => {
    setDraftPreset(preset)
    setDraftStart(rangeStart)
    setDraftEnd(rangeEnd)
    setDraftStartInput(formatDateTimeInput(rangeStart))
    setDraftEndInput(formatDateTimeInput(rangeEnd))
    setCalYear(rangeStart.getFullYear())
    setCalMonth(rangeStart.getMonth())
    setSelectingSecondDay(false)
    setIsDatePopoverOpen(true)
    setIsAgentMenuOpen(false)
  }

  const handleSelectPreset = (p: PresetId) => {
    const r = computePresetRange(p)
    setDraftPreset(p)
    setDraftStart(r.start)
    setDraftEnd(r.end)
    setDraftStartInput(formatDateTimeInput(r.start))
    setDraftEndInput(formatDateTimeInput(r.end))
    setCalYear(r.start.getFullYear())
    setCalMonth(r.start.getMonth())
    setSelectingSecondDay(false)
  }

  const handleClickCalendarDay = (day: Date) => {
    setDraftPreset('custom')
    if (!selectingSecondDay) {
      const s = new Date(day.getFullYear(), day.getMonth(), day.getDate(), 0, 0, 0)
      const e = new Date(day.getFullYear(), day.getMonth(), day.getDate(), 23, 59, 0)
      setDraftStart(s)
      setDraftEnd(e)
      setDraftStartInput(formatDateTimeInput(s))
      setDraftEndInput(formatDateTimeInput(e))
      setSelectingSecondDay(true)
    } else {
      const clickedStart = new Date(day.getFullYear(), day.getMonth(), day.getDate(), 0, 0, 0)
      if (clickedStart.getTime() < draftStart.getTime()) {
        const nextEnd = new Date(
          draftStart.getFullYear(),
          draftStart.getMonth(),
          draftStart.getDate(),
          23,
          59,
          0,
        )
        setDraftStart(clickedStart)
        setDraftEnd(nextEnd)
        setDraftStartInput(formatDateTimeInput(clickedStart))
        setDraftEndInput(formatDateTimeInput(nextEnd))
      } else {
        const nextEnd = new Date(day.getFullYear(), day.getMonth(), day.getDate(), 23, 59, 0)
        setDraftEnd(nextEnd)
        setDraftEndInput(formatDateTimeInput(nextEnd))
      }
      setSelectingSecondDay(false)
    }
  }

  const handleApplyDatePopover = () => {
    const finalStart = parseDateTimeInput(draftStartInput, draftStart)
    const finalEnd = parseDateTimeInput(draftEndInput, draftEnd)
    setPreset(draftPreset)
    setRangeStart(finalStart <= finalEnd ? finalStart : finalEnd)
    setRangeEnd(finalEnd >= finalStart ? finalEnd : finalStart)
    setIsDatePopoverOpen(false)
  }

  const shiftPeriodWindow = (dir: -1 | 1) => {
    const span = Math.max(15 * 60 * 1000, rangeEnd.getTime() - rangeStart.getTime())
    const nextStart = new Date(rangeStart.getTime() + dir * span)
    const nextEnd = new Date(rangeEnd.getTime() + dir * span)
    setPreset('custom')
    setRangeStart(nextStart)
    setRangeEnd(nextEnd)
  }

  const activePeriodLabel = useMemo(() => {
    if (preset !== 'custom') {
      return PRESET_OPTIONS.find((o) => o.id === preset)?.label || 'Últimos 7 dias'
    }
    const fmt = (d: Date) =>
      `${String(d.getDate()).padStart(2, '0')}/${String(d.getMonth() + 1).padStart(2, '0')}`
    return `${fmt(rangeStart)} – ${fmt(rangeEnd)}`
  }, [preset, rangeStart, rangeEnd])

  // Saudação dinâmica
  const greeting = useMemo(() => {
    const h = new Date().getHours()
    if (h < 12) return 'Bom dia'
    if (h < 18) return 'Boa tarde'
    return 'Boa noite'
  }, [])

  const userDisplayName = useMemo(() => {
    if (!me?.email) return 'Victor'
    const raw = me.email.split('@')[0].split('.')[0] || 'Victor'
    return raw.charAt(0).toUpperCase() + raw.slice(1)
  }, [me])

  // Conversas filtradas pelo intervalo [rangeStart, rangeEnd] e pelo agente selecionado
  const filteredConversations = useMemo(() => {
    const startMs = rangeStart.getTime()
    const endMs = rangeEnd.getTime()
    return conversations.filter((c) => {
      if (selectedAgentId !== 'all' && c.agent_id !== selectedAgentId) return false
      const started = new Date(c.started_at).getTime()
      if (!isNaN(started) && (started < startMs || started > endMs)) return false
      return true
    })
  }, [conversations, selectedAgentId, rangeStart, rangeEnd])

  // Cálculo dos KPIs reais sobre o recorte filtrado
  const kpiStats = useMemo(() => {
    const total = filteredConversations.length
    let sumDurationMs = 0
    let countWithDuration = 0
    let sumCostUsd = 0
    let sumCostBrl = 0
    let failedCount = 0

    for (const c of filteredConversations) {
      if (c.duration_ms && c.duration_ms > 0) {
        sumDurationMs += c.duration_ms
        countWithDuration++
      } else if (c.started_at && c.ended_at) {
        const diff = new Date(c.ended_at).getTime() - new Date(c.started_at).getTime()
        if (diff > 0) {
          sumDurationMs += diff
          countWithDuration++
        }
      }
      sumCostUsd += c.cost_usd || 0
      sumCostBrl += c.cost_brl || 0
      if (c.status === 'failed') failedCount++
    }

    const avgDurMs =
      countWithDuration > 0
        ? Math.round(sumDurationMs / countWithDuration)
        : total > 0
          ? metrics?.avg_duration_ms || 0
          : 0
    const totalCostUsd = sumCostUsd
    const totalCostBrl = sumCostBrl
    const avgCostUsd = total > 0 ? totalCostUsd / total : 0
    const avgLatencyMs = metrics?.avg_latency_ms || 0
    const successRate =
      total > 0 ? Math.round(((total - failedCount) / total) * 100) : 100

    return {
      total,
      avgDurMs,
      totalCostUsd,
      totalCostBrl,
      avgCostUsd,
      avgLatencyMs,
      successRate,
    }
  }, [filteredConversations, metrics])

  const formatMinSec = (ms: number) => {
    if (!ms || ms <= 0) return '0:00'
    const totalSecs = Math.round(ms / 1000)
    const mins = Math.floor(totalSecs / 60)
    const secs = String(totalSecs % 60).padStart(2, '0')
    return `${mins}:${secs}`
  }

  // Gera os buckets temporais para o gráfico de linha
  const chartSeries = useMemo(() => {
    const startMs = rangeStart.getTime()
    const endMs = Math.max(startMs + 60 * 1000, rangeEnd.getTime())
    const totalSpanMs = endMs - startMs

    let targetStepMs = 3 * 60 * 60 * 1000
    if (granularity === 'minute') targetStepMs = 15 * 60 * 1000
    else if (granularity === 'hour') targetStepMs = 60 * 60 * 1000
    else if (granularity === 'day') targetStepMs = 24 * 60 * 60 * 1000

    const bucketCount = Math.max(8, Math.min(72, Math.round(totalSpanMs / targetStepMs)))
    const actualStep = totalSpanMs / bucketCount

    const buckets = Array.from({ length: bucketCount }, (_, i) => {
      const bStart = startMs + i * actualStep
      const bEnd = bStart + actualStep
      return {
        startMs: bStart,
        endMs: bEnd,
        count: 0,
        durSumMs: 0,
        durCount: 0,
        costSumUsd: 0,
        failedCount: 0,
      }
    })

    for (const c of filteredConversations) {
      const t = new Date(c.started_at).getTime()
      if (isNaN(t) || t < startMs || t > endMs) continue
      const idx = Math.min(
        bucketCount - 1,
        Math.max(0, Math.floor((t - startMs) / actualStep)),
      )
      buckets[idx].count++
      buckets[idx].costSumUsd += c.cost_usd || 0
      if (c.status === 'failed') buckets[idx].failedCount++
      const d =
        c.duration_ms && c.duration_ms > 0
          ? c.duration_ms
          : c.ended_at
            ? Math.max(0, new Date(c.ended_at).getTime() - t)
            : 0
      if (d > 0) {
        buckets[idx].durSumMs += d
        buckets[idx].durCount++
      }
    }

    return buckets.map((b) => {
      let value = 0
      if (activeMetric === 'conversations') value = b.count
      else if (activeMetric === 'avg_duration')
        value = b.durCount > 0 ? Math.round(b.durSumMs / b.durCount / 1000) : 0
      else if (activeMetric === 'total_cost') value = Number(b.costSumUsd.toFixed(4))
      else if (activeMetric === 'avg_cost')
        value = b.count > 0 ? Number((b.costSumUsd / b.count).toFixed(4)) : 0
      else if (activeMetric === 'avg_latency')
        value = b.count > 0 ? kpiStats.avgLatencyMs : 0
      else if (activeMetric === 'success_rate')
        value =
          b.count > 0
            ? Math.round(((b.count - b.failedCount) / b.count) * 100)
            : 0

      const d = new Date(b.startMs)
      const dd = String(d.getDate()).padStart(2, '0')
      const mm = String(d.getMonth() + 1).padStart(2, '0')
      const yyyy = d.getFullYear()
      const hh = String(d.getHours()).padStart(2, '0')
      const min = String(d.getMinutes()).padStart(2, '0')
      return {
        label: `${dd}/${mm}/${yyyy}, ${hh}:${min}`,
        value,
      }
    })
  }, [filteredConversations, granularity, rangeStart, rangeEnd, activeMetric, kpiStats.avgLatencyMs])

  const maxChartVal = useMemo(() => {
    const m = Math.max(...chartSeries.map((s) => s.value), 0)
    if (m <= 0) return 10
    return m
  }, [chartSeries])

  const handleExportCsv = () => {
    const header = 'Data_Hora,Valor\n'
    const rows = chartSeries.map((pt) => `"${pt.label}",${pt.value}`).join('\n')
    const blob = new Blob([header + rows], { type: 'text/csv;charset=utf-8;' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `metricas_${activeMetric}.csv`
    a.click()
    URL.revokeObjectURL(url)
  }

  const svgWidth = 920
  const svgHeight = 230
  const padLeft = 42
  const padRight = 18
  const padTop = 18
  const padBottom = 26
  const plotW = svgWidth - padLeft - padRight
  const plotH = svgHeight - padTop - padBottom

  const getY = (val: number) => {
    if (maxChartVal <= 0) return padTop + plotH
    if (scaleMode === 'log') {
      const logMax = Math.log10(maxChartVal + 1)
      const logVal = Math.log10(Math.max(0, val) + 1)
      return padTop + plotH - (logVal / (logMax || 1)) * plotH
    }
    return padTop + plotH - (Math.max(0, val) / maxChartVal) * plotH
  }

  const polylinePoints = chartSeries
    .map((pt, i) => {
      const x =
        padLeft +
        (chartSeries.length > 1 ? (i / (chartSeries.length - 1)) * plotW : plotW / 2)
      const y = getY(pt.value)
      return `${x.toFixed(1)},${y.toFixed(1)}`
    })
    .join(' ')

  const areaPoints = `${padLeft},${padTop + plotH} ${polylinePoints} ${svgWidth - padRight},${padTop + plotH}`

  const activeCount =
    metrics?.active_conversations ??
    conversations.filter((c) => c.status === 'active' || c.status === 'starting').length

  // Meses exibidos lado a lado no calendário duplo (ex: outubro 2026 e novembro 2026)
  const leftMonthDate = new Date(calYear, calMonth, 1)
  const rightMonthDate = new Date(calYear, calMonth + 1, 1)
  const leftCells = useMemo(
    () => buildCalendarCells(leftMonthDate.getFullYear(), leftMonthDate.getMonth()),
    [calYear, calMonth],
  )
  const rightCells = useMemo(
    () => buildCalendarCells(rightMonthDate.getFullYear(), rightMonthDate.getMonth()),
    [calYear, calMonth],
  )

  return (
    <div
      className="max-w-6xl mx-auto py-2 px-2 sm:px-6 space-y-6 text-zinc-900"
      onClick={() => {
        setIsDatePopoverOpen(false)
        setIsAgentMenuOpen(false)
      }}
    >
      {/* 1. Pílula Superior: "• Conversas ativas: X" */}
      <div>
        <div className="inline-flex items-center gap-2 px-3.5 py-1.5 rounded-full border border-zinc-200/90 bg-white text-xs font-medium text-zinc-800 shadow-2xs">
          <span
            className={`w-2 h-2 rounded-full ${
              activeCount > 0 ? 'bg-emerald-500 animate-pulse' : 'bg-zinc-950'
            }`}
          />
          <span>
            Conversas ativas: <strong className="font-semibold">{activeCount}</strong>
          </span>
        </div>
      </div>

      {/* 2. Subtítulo do Espaço de Trabalho + Saudação */}
      <div className="space-y-1">
        <div className="text-xs text-zinc-500 font-medium">
          {me?.organization?.name || 'TIM'} espaço de trabalho
        </div>
        <h1 className="text-3xl font-bold tracking-tight text-zinc-950">
          {greeting}, {userDisplayName}
        </h1>
      </div>

      {/* 3. Barra de Sub-abas */}
      <div className="border-b border-zinc-200/80 flex items-center gap-6 text-xs font-medium overflow-x-auto">
        {(
          [
            { id: 'geral', label: 'Geral' },
            { id: 'ferramentas', label: 'Ferramentas' },
            { id: 'llms', label: 'LLMs' },
            { id: 'kb', label: 'Base de conhecimento' },
          ] as { id: DashboardSectionTab; label: string }[]
        ).map((tab) => (
          <button
            key={tab.id}
            type="button"
            onClick={() => setSectionTab(tab.id)}
            className={`pb-2.5 border-b-2 transition-colors whitespace-nowrap cursor-pointer ${
              sectionTab === tab.id
                ? 'border-zinc-950 text-zinc-950 font-semibold'
                : 'border-transparent text-zinc-500 hover:text-zinc-800'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {errorMsg && (
        <div className="p-3 bg-red-50 border border-red-200 rounded-xl text-xs text-red-700">
          {errorMsg}
        </div>
      )}

      {/* =====================================================================
          ABA 1: GERAL (Barra de Filtros com Popover de Calendário Duplo + Gráfico)
         ===================================================================== */}
      {sectionTab === 'geral' && (
        <div className="space-y-6">
          {/* BARRA DE FILTROS IDÊNTICA À IMAGEM 1 */}
          <div className="flex flex-wrap items-center gap-2 text-xs relative">
            {/* Seletor de Período com Popover de Presets + Calendário Duplo */}
            <div className="relative" onClick={(e) => e.stopPropagation()}>
              <div className="inline-flex items-center rounded-xl border border-zinc-200/90 bg-white shadow-2xs divide-x divide-zinc-200/80">
                <button
                  type="button"
                  onClick={() => shiftPeriodWindow(-1)}
                  className="px-2 py-1.5 text-zinc-500 hover:text-zinc-900 cursor-pointer"
                  title="Período anterior"
                >
                  <ChevronLeft className="w-3.5 h-3.5" />
                </button>

                <button
                  type="button"
                  onClick={() =>
                    isDatePopoverOpen ? setIsDatePopoverOpen(false) : openDatePopover()
                  }
                  className="px-3 py-1.5 flex items-center gap-1.5 font-medium text-zinc-800 hover:bg-zinc-50 transition-colors cursor-pointer"
                >
                  <Calendar className="w-3.5 h-3.5 text-zinc-500" />
                  <span className="font-semibold text-zinc-900">{activePeriodLabel}</span>
                  <span className="text-zinc-400">· UTC-3</span>
                  {isDatePopoverOpen ? (
                    <ChevronUp className="w-3.5 h-3.5 text-zinc-500" />
                  ) : (
                    <ChevronDown className="w-3.5 h-3.5 text-zinc-500" />
                  )}
                </button>

                <button
                  type="button"
                  onClick={() => shiftPeriodWindow(1)}
                  className="px-2 py-1.5 text-zinc-500 hover:text-zinc-900 cursor-pointer"
                  title="Próximo período"
                >
                  <ChevronRight className="w-3.5 h-3.5" />
                </button>
              </div>

              {/* POPOVER DO CALENDÁRIO DUPLO E PRESETS (Idêntico à Imagem 1) */}
              {isDatePopoverOpen && (
                <div className="absolute left-0 top-10 z-40 w-[690px] max-w-[94vw] bg-white border border-zinc-200/90 rounded-2xl shadow-2xl overflow-hidden text-xs select-none">
                  <div className="flex flex-col md:flex-row">
                    {/* Coluna Esquerda: Presets Rápidos */}
                    <div className="w-full md:w-48 border-b md:border-b-0 md:border-r border-zinc-100 p-1.5 space-y-0.5 max-h-[380px] overflow-y-auto">
                      {PRESET_OPTIONS.map((opt) => {
                        const isSelected = draftPreset === opt.id
                        return (
                          <button
                            key={opt.id}
                            type="button"
                            onClick={() => handleSelectPreset(opt.id)}
                            className={`w-full px-3 py-1.5 rounded-lg text-left flex items-center justify-between transition-colors cursor-pointer ${
                              isSelected
                                ? 'bg-zinc-100 text-zinc-950 font-semibold'
                                : 'text-zinc-700 hover:bg-zinc-50'
                            }`}
                          >
                            <span>{opt.label}</span>
                            {isSelected && <Check className="w-3.5 h-3.5 text-zinc-900" />}
                          </button>
                        )
                      })}
                    </div>

                    {/* Coluna Direita: Campos Início / Fim + Calendário de 2 Meses + Botões Cancelar / Aplicar */}
                    <div className="flex-1 p-4 flex flex-col justify-between space-y-4">
                      {/* Linha de Inputs: Início e Fim */}
                      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                        <div>
                          <label className="block text-[11px] text-zinc-500 mb-1">Início</label>
                          <div className="relative">
                            <input
                              type="text"
                              value={draftStartInput}
                              onChange={(e) => {
                                setDraftStartInput(e.target.value)
                                setDraftPreset('custom')
                              }}
                              onBlur={() => {
                                const parsed = parseDateTimeInput(draftStartInput, draftStart)
                                setDraftStart(parsed)
                                setDraftStartInput(formatDateTimeInput(parsed))
                              }}
                              className="w-full pl-3 pr-8 py-1.5 bg-white border border-zinc-300 focus:border-zinc-900 rounded-lg text-xs font-medium text-zinc-900 focus:outline-none"
                            />
                            <Calendar className="w-3.5 h-3.5 text-zinc-400 absolute right-2.5 top-1/2 -translate-y-1/2 pointer-events-none" />
                          </div>
                        </div>

                        <div>
                          <label className="block text-[11px] text-zinc-500 mb-1">Fim</label>
                          <div className="relative">
                            <input
                              type="text"
                              value={draftEndInput}
                              onChange={(e) => {
                                setDraftEndInput(e.target.value)
                                setDraftPreset('custom')
                              }}
                              onBlur={() => {
                                const parsed = parseDateTimeInput(draftEndInput, draftEnd)
                                setDraftEnd(parsed)
                                setDraftEndInput(formatDateTimeInput(parsed))
                              }}
                              className="w-full pl-3 pr-8 py-1.5 bg-white border border-zinc-200 focus:border-zinc-900 rounded-lg text-xs font-medium text-zinc-900 focus:outline-none"
                            />
                            <Calendar className="w-3.5 h-3.5 text-zinc-400 absolute right-2.5 top-1/2 -translate-y-1/2 pointer-events-none" />
                          </div>
                        </div>
                      </div>

                      {/* Grade Dupla de Calendário (Mês 1 e Mês 2) */}
                      <div className="grid grid-cols-1 sm:grid-cols-2 gap-6 pt-1">
                        {[
                          {
                            title: `${MONTH_NAMES_PT[leftMonthDate.getMonth()]} ${leftMonthDate.getFullYear()}`,
                            cells: leftCells,
                            showPrev: true,
                            showNext: false,
                          },
                          {
                            title: `${MONTH_NAMES_PT[rightMonthDate.getMonth()]} ${rightMonthDate.getFullYear()}`,
                            cells: rightCells,
                            showPrev: false,
                            showNext: true,
                          },
                        ].map((mObj, mIdx) => (
                          <div key={mIdx} className="space-y-2">
                            <div className="flex items-center justify-between px-1">
                              {mObj.showPrev ? (
                                <button
                                  type="button"
                                  onClick={() => {
                                    const prev = new Date(calYear, calMonth - 1, 1)
                                    setCalYear(prev.getFullYear())
                                    setCalMonth(prev.getMonth())
                                  }}
                                  className="w-6 h-6 rounded-lg border border-zinc-200 flex items-center justify-center text-zinc-500 hover:text-zinc-900 hover:bg-zinc-50 cursor-pointer"
                                >
                                  <ChevronLeft className="w-3.5 h-3.5" />
                                </button>
                              ) : (
                                <span className="w-6" />
                              )}

                              <span className="text-xs font-semibold text-zinc-900">
                                {mObj.title}
                              </span>

                              {mObj.showNext ? (
                                <button
                                  type="button"
                                  onClick={() => {
                                    const next = new Date(calYear, calMonth + 1, 1)
                                    setCalYear(next.getFullYear())
                                    setCalMonth(next.getMonth())
                                  }}
                                  className="w-6 h-6 rounded-lg border border-zinc-200 flex items-center justify-center text-zinc-500 hover:text-zinc-900 hover:bg-zinc-50 cursor-pointer"
                                >
                                  <ChevronRight className="w-3.5 h-3.5" />
                                </button>
                              ) : (
                                <span className="w-6" />
                              )}
                            </div>

                            <div className="grid grid-cols-7 text-center text-[10px] text-zinc-400 font-medium">
                              {WEEKDAYS_PT.map((wd) => (
                                <div key={wd} className="py-1">
                                  {wd}
                                </div>
                              ))}
                            </div>

                            <div className="grid grid-cols-7 gap-y-0.5 text-center text-xs">
                              {mObj.cells.map((cell, cIdx) => {
                                const isStart = isSameDay(cell.date, draftStart)
                                const isEnd = isSameDay(cell.date, draftEnd)
                                const inBetween = isBetweenDays(cell.date, draftStart, draftEnd)

                                return (
                                  <button
                                    key={cIdx}
                                    type="button"
                                    onClick={() => handleClickCalendarDay(cell.date)}
                                    className={`h-7 flex items-center justify-center transition-colors cursor-pointer ${
                                      isStart || isEnd
                                        ? 'bg-zinc-900 text-white font-semibold rounded-lg'
                                        : inBetween
                                          ? 'bg-zinc-100 text-zinc-900 font-medium'
                                          : cell.inCurrentMonth
                                            ? 'text-zinc-800 hover:bg-zinc-100 rounded-lg'
                                            : 'text-zinc-300 hover:bg-zinc-50 rounded-lg'
                                    }`}
                                  >
                                    {cell.date.getDate()}
                                  </button>
                                )
                              })}
                            </div>
                          </div>
                        ))}
                      </div>

                      {/* Botões Cancelar / Aplicar */}
                      <div className="flex items-center justify-end gap-2 pt-2">
                        <button
                          type="button"
                          onClick={() => setIsDatePopoverOpen(false)}
                          className="px-3.5 py-1.5 rounded-xl border border-zinc-200/90 bg-white hover:bg-zinc-50 text-xs font-semibold text-zinc-800 cursor-pointer"
                        >
                          Cancelar
                        </button>
                        <button
                          type="button"
                          onClick={handleApplyDatePopover}
                          className="px-4 py-1.5 rounded-xl bg-zinc-950 hover:bg-zinc-800 text-white text-xs font-semibold cursor-pointer"
                        >
                          Aplicar
                        </button>
                      </div>
                    </div>
                  </div>

                  {/* Barra Inferior de Fuso Horário ("America/Sao_Paulo (UTC-3)") */}
                  <div className="border-t border-zinc-100 bg-zinc-50/40 px-4 py-2.5 flex items-center gap-2 text-xs text-zinc-600">
                    <span className="w-6 h-6 rounded-lg border border-zinc-200 bg-white flex items-center justify-center text-zinc-600">
                      <Pencil className="w-3 h-3" />
                    </span>
                    <strong className="font-semibold text-zinc-900">America/Sao_Paulo</strong>
                    <span className="text-zinc-400">(UTC-3)</span>
                  </div>
                </div>
              )}
            </div>

            {/* Pílula: Granularidade | Por Hora v */}
            <div className="inline-flex items-center rounded-xl border border-zinc-200/90 bg-white shadow-2xs divide-x divide-zinc-200/80">
              <span className="px-2.5 py-1.5 text-zinc-600 font-medium">Granularidade</span>
              <div className="px-2.5 py-1.5">
                <select
                  value={granularity}
                  onChange={(e) => setGranularity(e.target.value as any)}
                  className="bg-transparent font-semibold text-zinc-900 focus:outline-none cursor-pointer"
                >
                  <option value="minute">Por Minuto</option>
                  <option value="hour">Por Hora</option>
                  <option value="day">Por Dia</option>
                </select>
              </div>
            </div>

            {/* Pílula: Agente (Popover de seleção de agente) */}
            <div className="relative" onClick={(e) => e.stopPropagation()}>
              <button
                type="button"
                onClick={() => {
                  setIsAgentMenuOpen(!isAgentMenuOpen)
                  setIsDatePopoverOpen(false)
                }}
                className={`px-3 py-1.5 rounded-xl border text-xs font-medium shadow-2xs transition-colors cursor-pointer ${
                  selectedAgentId !== 'all'
                    ? 'bg-zinc-900 text-white border-zinc-900'
                    : 'bg-white hover:bg-zinc-50 text-zinc-800 border-zinc-200/90'
                }`}
              >
                {selectedAgentId === 'all'
                  ? 'Agente'
                  : `Agente: ${agents.find((a) => a.id === selectedAgentId)?.name || ''}`}
              </button>

              {isAgentMenuOpen && (
                <div className="absolute left-0 top-10 z-30 w-56 bg-white border border-zinc-200/90 rounded-xl shadow-xl p-1.5 space-y-0.5">
                  <button
                    type="button"
                    onClick={() => {
                      setSelectedAgentId('all')
                      setIsAgentMenuOpen(false)
                    }}
                    className={`w-full px-3 py-1.5 rounded-lg text-left flex items-center justify-between cursor-pointer ${
                      selectedAgentId === 'all'
                        ? 'bg-zinc-100 font-semibold text-zinc-950'
                        : 'text-zinc-700 hover:bg-zinc-50'
                    }`}
                  >
                    <span>Todos os agentes</span>
                    {selectedAgentId === 'all' && <Check className="w-3.5 h-3.5" />}
                  </button>
                  {agents.map((ag) => (
                    <button
                      key={ag.id}
                      type="button"
                      onClick={() => {
                        setSelectedAgentId(ag.id)
                        setIsAgentMenuOpen(false)
                      }}
                      className={`w-full px-3 py-1.5 rounded-lg text-left flex items-center justify-between cursor-pointer ${
                        selectedAgentId === ag.id
                          ? 'bg-zinc-100 font-semibold text-zinc-950'
                          : 'text-zinc-700 hover:bg-zinc-50'
                      }`}
                    >
                      <span className="truncate">{ag.name}</span>
                      {selectedAgentId === ag.id && <Check className="w-3.5 h-3.5" />}
                    </button>
                  ))}
                </div>
              )}
            </div>

            {/* Pílula: + Agrupar por */}
            <button
              type="button"
              onClick={() =>
                setGroupBy(
                  groupBy === 'none' ? 'agent' : groupBy === 'agent' ? 'provider' : 'none',
                )
              }
              className={`inline-flex items-center gap-1 px-3 py-1.5 rounded-xl border text-xs font-medium shadow-2xs transition-colors cursor-pointer ${
                groupBy !== 'none'
                  ? 'bg-zinc-900 text-white border-zinc-900'
                  : 'bg-white hover:bg-zinc-50 text-zinc-800 border-zinc-200/90'
              }`}
            >
              <Plus className="w-3 h-3" />
              <span>
                {groupBy === 'none'
                  ? 'Agrupar por'
                  : groupBy === 'agent'
                    ? 'Agrupar por: Agente'
                    : 'Agrupar por: Modelo'}
              </span>
            </button>

            {/* Pílula: + Visualização empilhada */}
            <button
              type="button"
              onClick={() => setStackedView(!stackedView)}
              className={`inline-flex items-center gap-1 px-3 py-1.5 rounded-xl border text-xs font-medium shadow-2xs transition-colors cursor-pointer ${
                stackedView
                  ? 'bg-zinc-900 text-white border-zinc-900'
                  : 'bg-white hover:bg-zinc-50 text-zinc-800 border-zinc-200/90'
              }`}
            >
              <Plus className="w-3 h-3" />
              <span>Visualização empilhada</span>
            </button>
          </div>

          {/* CARD PRINCIPAL: Faixa Superior de Métricas + Gráfico de Linha Azul */}
          <div className="bg-white border border-zinc-200/90 rounded-2xl overflow-hidden shadow-2xs">
            {/* Faixa Superior com 6 Colunas Clicáveis */}
            <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 bg-[#f8f8f9] border-b border-zinc-200/80">
              {[
                {
                  id: 'conversations' as ActiveMetricTab,
                  label: 'Número de conversas',
                  value: String(kpiStats.total),
                },
                {
                  id: 'avg_duration' as ActiveMetricTab,
                  label: 'Duração média',
                  value: formatMinSec(kpiStats.avgDurMs),
                },
                {
                  id: 'total_cost' as ActiveMetricTab,
                  label: 'Custo total',
                  value: `$${kpiStats.totalCostUsd.toFixed(3)}`,
                  sub: `≈ R$ ${kpiStats.totalCostBrl.toFixed(2)}`,
                },
                {
                  id: 'avg_cost' as ActiveMetricTab,
                  label: 'Custo médio',
                  value: `$${kpiStats.avgCostUsd.toFixed(4)}`,
                },
                {
                  id: 'avg_latency' as ActiveMetricTab,
                  label: 'Latência média',
                  value: kpiStats.avgLatencyMs > 0 ? `${kpiStats.avgLatencyMs} ms` : '0 ms',
                },
                {
                  id: 'success_rate' as ActiveMetricTab,
                  label: 'Taxa de sucesso',
                  value: `${kpiStats.successRate}%`,
                },
              ].map((m) => {
                const isSelected = activeMetric === m.id
                return (
                  <button
                    key={m.id}
                    type="button"
                    onClick={() => setActiveMetric(m.id)}
                    className={`p-4 text-left transition-colors relative cursor-pointer ${
                      isSelected
                        ? 'bg-white text-zinc-950'
                        : 'hover:bg-zinc-100/70 text-zinc-600'
                    }`}
                  >
                    <div className="text-[11px] text-zinc-500 font-normal truncate">{m.label}</div>
                    <div className="text-xl font-bold text-zinc-950 mt-1 tabular-nums">
                      {m.value}
                    </div>
                    {m.sub && (
                      <div className="text-[10px] text-zinc-400 font-mono mt-0.5">{m.sub}</div>
                    )}
                    {isSelected && (
                      <div className="absolute bottom-0 left-0 right-0 h-0.5 bg-zinc-950" />
                    )}
                  </button>
                )
              })}
            </div>

            {/* Área do Gráfico de Linha */}
            <div className="p-6 space-y-4">
              <div className="relative w-full overflow-x-auto">
                <svg
                  viewBox={`0 0 ${svgWidth} ${svgHeight}`}
                  className="w-full h-56 select-none overflow-visible"
                  onMouseLeave={() => setHoverIdx(null)}
                >
                  {[1, 0.5, 0].map((ratio, idx) => {
                    const y = padTop + (1 - ratio) * plotH
                    const labelVal =
                      maxChartVal <= 1
                        ? (maxChartVal * ratio).toFixed(2)
                        : Math.round(maxChartVal * ratio)
                    return (
                      <g key={idx}>
                        <line
                          x1={padLeft}
                          y1={y}
                          x2={svgWidth - padRight}
                          y2={y}
                          stroke="#e4e4e7"
                          strokeWidth="1"
                        />
                        <text
                          x={padLeft - 8}
                          y={y + 4}
                          textAnchor="end"
                          className="fill-zinc-400 text-[10px] font-sans"
                        >
                          {labelVal}
                        </text>
                      </g>
                    )
                  })}

                  {stackedView && (
                    <polygon points={areaPoints} fill="rgba(37, 99, 235, 0.12)" />
                  )}

                  <polyline
                    fill="none"
                    stroke="#2563eb"
                    strokeWidth="2"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    points={polylinePoints}
                  />

                  {chartSeries.map((pt, i) => {
                    const x =
                      padLeft +
                      (chartSeries.length > 1
                        ? (i / (chartSeries.length - 1)) * plotW
                        : plotW / 2)
                    const y = getY(pt.value)
                    return (
                      <g key={i}>
                        {hoverIdx === i && (
                          <>
                            <line
                              x1={x}
                              y1={padTop}
                              x2={x}
                              y2={padTop + plotH}
                              stroke="#94a3b8"
                              strokeDasharray="3 3"
                              strokeWidth="1"
                            />
                            <circle
                              cx={x}
                              cy={y}
                              r="4"
                              fill="#2563eb"
                              stroke="#fff"
                              strokeWidth="1.5"
                            />
                          </>
                        )}
                        <rect
                          x={x - plotW / chartSeries.length / 2}
                          y={padTop}
                          width={plotW / chartSeries.length}
                          height={plotH}
                          fill="transparent"
                          onMouseEnter={() => setHoverIdx(i)}
                        />
                      </g>
                    )
                  })}

                  {chartSeries.length > 0 && (
                    <>
                      <text
                        x={padLeft}
                        y={svgHeight - 4}
                        textAnchor="start"
                        className="fill-zinc-500 text-[10px]"
                      >
                        {chartSeries[0].label}
                      </text>
                      <text
                        x={svgWidth - padRight}
                        y={svgHeight - 4}
                        textAnchor="end"
                        className="fill-zinc-500 text-[10px]"
                      >
                        {chartSeries[chartSeries.length - 1].label}
                      </text>
                    </>
                  )}
                </svg>

                {hoverIdx !== null && chartSeries[hoverIdx] && (
                  <div className="absolute top-2 right-4 px-3 py-1.5 rounded-xl bg-zinc-900 text-white text-xs shadow-lg pointer-events-none">
                    <span className="text-zinc-400 mr-2">{chartSeries[hoverIdx].label}:</span>
                    <strong className="font-mono">{chartSeries[hoverIdx].value}</strong>
                  </div>
                )}
              </div>

              {/* Rodapé do Gráfico: Linear | Logarítmica + Download CSV + Link Histórico */}
              <div className="flex flex-wrap items-center justify-between gap-3 pt-2">
                <div className="flex items-center gap-2">
                  <div className="inline-flex p-0.5 rounded-xl bg-zinc-100 border border-zinc-200/80 text-xs">
                    <button
                      type="button"
                      onClick={() => setScaleMode('linear')}
                      className={`px-3 py-1 rounded-lg font-medium transition-colors cursor-pointer ${
                        scaleMode === 'linear'
                          ? 'bg-white text-zinc-950 shadow-2xs'
                          : 'text-zinc-500 hover:text-zinc-800'
                      }`}
                    >
                      Linear
                    </button>
                    <button
                      type="button"
                      onClick={() => setScaleMode('log')}
                      className={`px-3 py-1 rounded-lg font-medium transition-colors cursor-pointer ${
                        scaleMode === 'log'
                          ? 'bg-white text-zinc-950 shadow-2xs'
                          : 'text-zinc-500 hover:text-zinc-800'
                      }`}
                    >
                      Logarítmica
                    </button>
                  </div>

                  <button
                    type="button"
                    onClick={handleExportCsv}
                    title="Exportar CSV"
                    className="p-2 rounded-xl border border-zinc-200/90 bg-white hover:bg-zinc-50 text-zinc-700 shadow-2xs cursor-pointer"
                  >
                    <Download className="w-3.5 h-3.5" />
                  </button>
                </div>

                <Link
                  to="/conversations"
                  className="inline-flex items-center gap-1 text-xs font-medium text-zinc-600 hover:text-zinc-950"
                >
                  <span>Histórico de conversas filtrado</span>
                  <ArrowUpRight className="w-3.5 h-3.5" />
                </Link>
              </div>
            </div>
          </div>

          {/* Tabela Compacta das Últimas Conversas do Período */}
          <div className="bg-white border border-zinc-200/90 rounded-2xl overflow-hidden shadow-2xs">
            <div className="px-5 py-3.5 border-b border-zinc-100 flex items-center justify-between">
              <span className="text-xs font-semibold text-zinc-900">
                Conversas no período selecionado ({filteredConversations.slice(0, 8).length})
              </span>
              <Link
                to="/conversations"
                className="text-xs text-zinc-500 hover:text-zinc-900 font-medium"
              >
                Ver todas →
              </Link>
            </div>
            <table className="w-full text-left text-xs">
              <thead className="border-b border-zinc-100 text-zinc-400 font-normal">
                <tr>
                  <th className="px-5 py-2.5 font-normal">Data</th>
                  <th className="px-4 py-2.5 font-normal">Agente</th>
                  <th className="px-4 py-2.5 font-normal">Canal</th>
                  <th className="px-4 py-2.5 font-normal text-right">Duração</th>
                  <th className="px-4 py-2.5 font-normal text-right">Custo</th>
                  <th className="px-5 py-2.5 font-normal text-right">Status</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-zinc-100">
                {filteredConversations.slice(0, 8).map((c) => {
                  const agName =
                    (c as any).agent_name ||
                    agents.find((a) => a.id === c.agent_id)?.name ||
                    'Agente'
                  const durMs =
                    c.duration_ms ||
                    (c.ended_at
                      ? Math.max(
                          0,
                          new Date(c.ended_at).getTime() - new Date(c.started_at).getTime(),
                        )
                      : 0)
                  return (
                    <tr
                      key={c.id}
                      onClick={() => navigate(`/conversations/${c.id}`)}
                      className="hover:bg-zinc-50/80 transition-colors cursor-pointer"
                    >
                      <td className="px-5 py-3 text-zinc-700 tabular-nums">
                        {new Date(c.started_at).toLocaleString('pt-BR')}
                      </td>
                      <td className="px-4 py-3 font-semibold text-zinc-900">{agName}</td>
                      <td className="px-4 py-3 text-zinc-500">
                        {c.is_test ? 'Pré-visualização' : 'Telefonia / Produção'}
                      </td>
                      <td className="px-4 py-3 text-right font-mono text-zinc-700">
                        {formatMinSec(durMs)}
                      </td>
                      <td className="px-4 py-3 text-right font-mono text-zinc-700">
                        ${(c.cost_usd || 0).toFixed(4)}
                      </td>
                      <td className="px-5 py-3 text-right">
                        <span
                          className={`inline-flex px-2 py-0.5 rounded-md text-[11px] font-medium ${
                            c.status === 'completed'
                              ? 'bg-emerald-50 text-emerald-700'
                              : c.status === 'failed'
                                ? 'bg-red-50 text-red-700'
                                : 'bg-amber-50 text-amber-800'
                          }`}
                        >
                          {c.status === 'completed'
                            ? 'Bem-sucedida'
                            : c.status === 'failed'
                              ? 'Falhou'
                              : 'Ativa'}
                        </span>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* =====================================================================
          ABA 2: FERRAMENTAS
         ===================================================================== */}
      {sectionTab === 'ferramentas' && (
        <div className="bg-white border border-zinc-200/90 rounded-2xl overflow-hidden shadow-2xs">
          <div className="px-5 py-4 border-b border-zinc-100 flex items-center justify-between">
            <div>
              <h3 className="text-sm font-semibold text-zinc-900">Desempenho das Ferramentas</h3>
              <p className="text-xs text-zinc-500">
                Total de chamadas e taxa de sucesso das ferramentas configuradas no espaço de
                trabalho.
              </p>
            </div>
            <Link
              to="/mcps"
              className="px-3 py-1.5 rounded-xl border border-zinc-200 text-xs font-semibold text-zinc-800 hover:bg-zinc-50"
            >
              Gerenciar ferramentas →
            </Link>
          </div>
          <table className="w-full text-left text-xs">
            <thead className="border-b border-zinc-100 text-zinc-400">
              <tr>
                <th className="px-5 py-3 font-normal">Ferramenta</th>
                <th className="px-4 py-3 font-normal">Tipo</th>
                <th className="px-4 py-3 font-normal text-right">Execuções</th>
                <th className="px-5 py-3 font-normal text-right">Taxa de sucesso</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-100">
              {tools.map((t) => (
                <tr
                  key={t.id}
                  onClick={() => navigate('/mcps')}
                  className="hover:bg-zinc-50 cursor-pointer"
                >
                  <td className="px-5 py-3.5 font-semibold text-zinc-900 flex items-center gap-2">
                    <Wrench className="w-3.5 h-3.5 text-zinc-400" />
                    {t.name}
                  </td>
                  <td className="px-4 py-3.5 text-zinc-500 uppercase font-mono text-[11px]">
                    {t.kind || 'webhook'}
                  </td>
                  <td className="px-4 py-3.5 text-right font-mono text-zinc-800">
                    {t.total_executions || 0}
                  </td>
                  <td className="px-5 py-3.5 text-right">
                    <span className="px-2 py-0.5 rounded-md bg-emerald-50 text-emerald-700 font-semibold">
                      {Math.round(t.success_rate ?? 100)}%
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* =====================================================================
          ABA 3: LLMs
         ===================================================================== */}
      {sectionTab === 'llms' && (
        <div className="bg-white border border-zinc-200/90 rounded-2xl overflow-hidden shadow-2xs">
          <div className="px-5 py-4 border-b border-zinc-100">
            <h3 className="text-sm font-semibold text-zinc-900">Uso e Custos por Modelo (LLM)</h3>
            <p className="text-xs text-zinc-500">
              Distribuição real das conversas e custos por modelo de voz em tempo real.
            </p>
          </div>
          <table className="w-full text-left text-xs">
            <thead className="border-b border-zinc-100 text-zinc-400">
              <tr>
                <th className="px-5 py-3 font-normal">Modelo / Provedor</th>
                <th className="px-4 py-3 font-normal text-right">Conversas</th>
                <th className="px-4 py-3 font-normal text-right">Custo Total (USD)</th>
                <th className="px-5 py-3 font-normal text-right">Custo Médio / Conversa</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-100">
              {Object.entries(
                filteredConversations.reduce(
                  (acc, c) => {
                    const key = c.model || c.provider || 'gemini-2.5-flash-native-audio'
                    if (!acc[key]) acc[key] = { count: 0, costUsd: 0 }
                    acc[key].count++
                    acc[key].costUsd += c.cost_usd || 0
                    return acc
                  },
                  {} as Record<string, { count: number; costUsd: number }>,
                ),
              ).map(([modelName, info]) => (
                <tr key={modelName} className="hover:bg-zinc-50">
                  <td className="px-5 py-3.5 font-semibold text-zinc-900 flex items-center gap-2">
                    <Cpu className="w-3.5 h-3.5 text-zinc-400" />
                    {modelName}
                  </td>
                  <td className="px-4 py-3.5 text-right font-mono text-zinc-800">{info.count}</td>
                  <td className="px-4 py-3.5 text-right font-mono text-zinc-800">
                    ${info.costUsd.toFixed(4)}
                  </td>
                  <td className="px-5 py-3.5 text-right font-mono text-zinc-600">
                    ${(info.count > 0 ? info.costUsd / info.count : 0).toFixed(4)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* =====================================================================
          ABA 4: BASE DE CONHECIMENTO
         ===================================================================== */}
      {sectionTab === 'kb' && (
        <div className="bg-white border border-zinc-200/90 rounded-2xl overflow-hidden shadow-2xs">
          <div className="px-5 py-4 border-b border-zinc-100 flex items-center justify-between">
            <div>
              <h3 className="text-sm font-semibold text-zinc-900">Bases de Conhecimento</h3>
              <p className="text-xs text-zinc-500">
                Documentos e catálogos indexados para consulta dos agentes.
              </p>
            </div>
            <Link
              to="/knowledge"
              className="px-3 py-1.5 rounded-xl border border-zinc-200 text-xs font-semibold text-zinc-800 hover:bg-zinc-50"
            >
              Abrir Base de conhecimento →
            </Link>
          </div>
          <table className="w-full text-left text-xs">
            <thead className="border-b border-zinc-100 text-zinc-400">
              <tr>
                <th className="px-5 py-3 font-normal">Nome</th>
                <th className="px-4 py-3 font-normal">Descrição</th>
                <th className="px-4 py-3 font-normal text-right">Documentos</th>
                <th className="px-5 py-3 font-normal text-right">Tamanho</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-100">
              {kbs.map((kb) => (
                <tr
                  key={kb.id}
                  onClick={() => navigate('/knowledge')}
                  className="hover:bg-zinc-50 cursor-pointer"
                >
                  <td className="px-5 py-3.5 font-semibold text-zinc-900 flex items-center gap-2">
                    <FolderOpen className="w-3.5 h-3.5 text-zinc-400" />
                    {kb.name}
                  </td>
                  <td className="px-4 py-3.5 text-zinc-500">{kb.description || '—'}</td>
                  <td className="px-4 py-3.5 text-right font-mono text-zinc-800">
                    {kb.documents_count}
                  </td>
                  <td className="px-5 py-3.5 text-right font-mono text-zinc-500">
                    {Math.max(1, Math.round((kb.total_bytes || 0) / 1024))} KB
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
