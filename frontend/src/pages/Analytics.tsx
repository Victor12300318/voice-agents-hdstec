import { useEffect, useState } from 'react'
import { BarChart2, Clock, Activity, Zap, CheckCircle2, DollarSign } from 'lucide-react'
import { api, type DashboardMetrics, type Conversation } from '../api'

export default function AnalyticsPage() {
  const [metrics, setMetrics] = useState<DashboardMetrics | null>(null)
  const [conversations, setConversations] = useState<Conversation[]>([])

  useEffect(() => {
    Promise.all([
      api<DashboardMetrics>('/api/dashboard').catch(() => null),
      api<Conversation[]>('/api/conversations?limit=100').catch(() => []),
    ]).then(([m, c]) => {
      setMetrics(m)
      setConversations(c)
    })
  }, [])

  const totalConv = metrics?.total_conversations || conversations.length
  const failedConv = metrics?.failed_conversations || conversations.filter((c) => c.status === 'failed').length
  const successRate = totalConv > 0 ? (((totalConv - failedConv) / totalConv) * 100).toFixed(1) : '100.0'

  return (
    <div className="max-w-6xl mx-auto space-y-8">
      {/* Top Header */}
      <div className="border-b border-zinc-200/80 pb-5">
        <h1 className="text-2xl font-semibold tracking-tight text-zinc-900 flex items-center gap-2">
          <BarChart2 className="w-6 h-6 text-zinc-700" />
          Analytics & Performance de Voz
        </h1>
        <p className="text-sm text-zinc-500 mt-1">
          Métricas consolidadas de latência, taxa de conclusão e uso dos Voice Providers na Organization.
        </p>
      </div>

      {/* KPI Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-4">
        <div className="bg-white border border-zinc-200/80 rounded-xl p-5 shadow-xs">
          <div className="flex items-center justify-between text-zinc-500 text-xs font-medium uppercase tracking-wider mb-2">
            <span>Conversas Totais</span>
            <Activity className="w-4 h-4 text-zinc-400" />
          </div>
          <div className="text-2xl font-bold text-zinc-900">{totalConv}</div>
          <div className="text-xs text-zinc-500 mt-2">
            Sessões em produção
          </div>
        </div>

        <div className="bg-white border border-zinc-200/80 rounded-xl p-5 shadow-xs">
          <div className="flex items-center justify-between text-zinc-500 text-xs font-medium uppercase tracking-wider mb-2">
            <span>Latência Média</span>
            <Clock className="w-4 h-4 text-zinc-400" />
          </div>
          <div className="text-2xl font-bold text-zinc-900">
            {metrics?.avg_latency_ms ? `${metrics.avg_latency_ms}ms` : '—'}
          </div>
          <div className="text-xs text-zinc-500 mt-2">
            Fim fala → primeiro áudio
          </div>
        </div>

        <div className="bg-white border border-zinc-200/80 rounded-xl p-5 shadow-xs">
          <div className="flex items-center justify-between text-zinc-500 text-xs font-medium uppercase tracking-wider mb-2">
            <span>Duração Média</span>
            <Zap className="w-4 h-4 text-zinc-400" />
          </div>
          <div className="text-2xl font-bold text-zinc-900">
            {metrics?.avg_duration_ms ? `${Math.round(metrics.avg_duration_ms / 1000)}s` : '—'}
          </div>
          <div className="text-xs text-zinc-500 mt-2">
            Tempo por sessão
          </div>
        </div>

        <div className="bg-white border border-zinc-200/80 rounded-xl p-5 shadow-xs">
          <div className="flex items-center justify-between text-zinc-500 text-xs font-medium uppercase tracking-wider mb-2">
            <span>Taxa de Sucesso</span>
            <CheckCircle2 className="w-4 h-4 text-emerald-500" />
          </div>
          <div className="text-2xl font-bold text-zinc-900">{successRate}%</div>
          <div className="text-xs text-zinc-500 mt-2">
            Sessões sem falhas
          </div>
        </div>

        <div className="bg-white border border-zinc-200/80 rounded-xl p-5 shadow-xs">
          <div className="flex items-center justify-between text-zinc-500 text-xs font-medium uppercase tracking-wider mb-2">
            <span>Custo Total</span>
            <DollarSign className="w-4 h-4 text-emerald-600" />
          </div>
          <div className="text-2xl font-bold text-emerald-700">
            ${(metrics?.total_cost_usd || 0).toFixed(3)}
          </div>
          <div className="text-xs text-zinc-500 mt-2 font-mono">
            ≈ R$ {(metrics?.total_cost_brl || 0).toFixed(2)}
          </div>
        </div>
      </div>

      {/* Comparison Grid */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Provider Comparison Card */}
        <div className="bg-white border border-zinc-200/80 rounded-xl p-6 shadow-xs">
          <h3 className="font-semibold text-zinc-900 text-base mb-1">
            Voice Providers Suportados
          </h3>
          <p className="text-xs text-zinc-500 mb-6">
            Características dos motores de voz integrados ao Voice Agent Runtime.
          </p>

          <div className="space-y-4">
            <div className="p-4 bg-zinc-50 border border-zinc-200 rounded-xl">
              <div className="flex items-center justify-between mb-2">
                <span className="font-semibold text-sm text-zinc-900 flex items-center gap-2">
                  <span className="w-2.5 h-2.5 rounded-full bg-blue-600" />
                  Google Gemini Live
                </span>
                <span className="text-xs font-mono font-medium text-emerald-600 bg-emerald-50 px-2 py-0.5 rounded">
                  Nativo PCM16 Bidi
                </span>
              </div>
              <div className="grid grid-cols-3 gap-2 text-xs text-zinc-600 mt-3 pt-3 border-t border-zinc-200">
                <div>
                  <span className="block text-zinc-400 text-[10px] uppercase">Protocolo</span>
                  <span className="font-semibold text-zinc-800">WebSocket Bidi</span>
                </div>
                <div>
                  <span className="block text-zinc-400 text-[10px] uppercase">Amostragem</span>
                  <span className="font-semibold text-zinc-800">16kHz in / 24kHz out</span>
                </div>
                <div>
                  <span className="block text-zinc-400 text-[10px] uppercase">Interrupção</span>
                  <span className="font-semibold text-zinc-800">Automática</span>
                </div>
              </div>
            </div>

            <div className="p-4 bg-zinc-50 border border-zinc-200 rounded-xl">
              <div className="flex items-center justify-between mb-2">
                <span className="font-semibold text-sm text-zinc-900 flex items-center gap-2">
                  <span className="w-2.5 h-2.5 rounded-full bg-emerald-600" />
                  OpenAI Realtime
                </span>
                <span className="text-xs font-mono font-medium text-emerald-600 bg-emerald-50 px-2 py-0.5 rounded">
                  gpt-realtime-2.1
                </span>
              </div>
              <div className="grid grid-cols-3 gap-2 text-xs text-zinc-600 mt-3 pt-3 border-t border-zinc-200">
                <div>
                  <span className="block text-zinc-400 text-[10px] uppercase">Protocolo</span>
                  <span className="font-semibold text-zinc-800">WebSocket</span>
                </div>
                <div>
                  <span className="block text-zinc-400 text-[10px] uppercase">Amostragem</span>
                  <span className="font-semibold text-zinc-800">24kHz PCM16</span>
                </div>
                <div>
                  <span className="block text-zinc-400 text-[10px] uppercase">Tool Calling</span>
                  <span className="font-semibold text-zinc-800">Nativo</span>
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* Recent Activity Card */}
        <div className="bg-white border border-zinc-200/80 rounded-xl p-6 shadow-xs">
          <h3 className="font-semibold text-zinc-900 text-base mb-1">
            Status das Conversas Recentes
          </h3>
          <p className="text-xs text-zinc-500 mb-6">
            Distribuição dos status das últimas chamadas registradas.
          </p>

          {conversations.length === 0 ? (
            <div className="py-10 text-center text-zinc-400 text-xs">
              Nenhuma conversa registrada ainda para análise.
            </div>
          ) : (
            <div className="space-y-3">
              {[
                {
                  label: 'Concluídas com Sucesso',
                  count: conversations.filter((c) => c.status === 'completed').length,
                  pct: `${Math.round(
                    (conversations.filter((c) => c.status === 'completed').length /
                      conversations.length) *
                      100
                  )}%`,
                  barColor: 'bg-emerald-600',
                },
                {
                  label: 'Falhas de Provedor / Erro',
                  count: conversations.filter((c) => c.status === 'failed').length,
                  pct: `${Math.round(
                    (conversations.filter((c) => c.status === 'failed').length /
                      conversations.length) *
                      100
                  )}%`,
                  barColor: 'bg-red-500',
                },
              ].map((item, idx) => (
                <div key={idx} className="space-y-1.5">
                  <div className="flex items-center justify-between text-xs">
                    <span className="font-medium text-zinc-800">{item.label}</span>
                    <span className="text-zinc-500 font-mono">
                      {item.count} ({item.pct})
                    </span>
                  </div>
                  <div className="w-full h-2 bg-zinc-100 rounded-full overflow-hidden">
                    <div
                      className={`h-full ${item.barColor} rounded-full`}
                      style={{ width: item.pct }}
                    />
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
