import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { MessageSquare, Search, Clock, Bot, ArrowRight } from 'lucide-react'
import { api, type Conversation } from '../api'

export default function Conversations() {
  const [conversations, setConversations] = useState<Conversation[]>([])
  const [includeTest, setIncludeTest] = useState(true)
  const [search, setSearch] = useState('')
  const [errorMsg, setErrorMsg] = useState('')

  const load = () => {
    api<Conversation[]>(`/api/conversations?include_test=${includeTest}&limit=100`)
      .then(setConversations)
      .catch((e) => setErrorMsg(e.message))
  }

  useEffect(() => {
    load()
  }, [includeTest])

  const formatDuration = (ms: number | null) => {
    if (!ms) return '0s'
    const totalSecs = Math.round(ms / 1000)
    const mins = Math.floor(totalSecs / 60)
    const secs = totalSecs % 60
    return mins > 0 ? `${mins}m ${secs}s` : `${secs}s`
  }

  const formatReason = (reason: string) => {
    switch (reason) {
      case 'caller_hangup':
        return 'Caller desligou'
      case 'agent_ended':
        return 'Agente encerrou'
      case 'provider_error':
        return 'Erro no provedor'
      case 'initiation_failed':
        return 'Falha no webhook'
      case 'max_duration':
        return 'Duração máxima'
      default:
        return reason || 'Concluída'
    }
  }

  const filtered = conversations.filter(
    (c) =>
      !search ||
      (c.agent_name && c.agent_name.toLowerCase().includes(search.toLowerCase())) ||
      c.id.toLowerCase().includes(search.toLowerCase()) ||
      c.provider.toLowerCase().includes(search.toLowerCase())
  )

  return (
    <div className="max-w-6xl mx-auto space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 border-b border-zinc-200/80 pb-5">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight text-zinc-900 flex items-center gap-2">
            <MessageSquare className="w-6 h-6 text-zinc-700" />
            Histórico de Conversas
          </h1>
          <p className="text-sm text-zinc-500 mt-1">
            Todas as sessões de voz registradas com transcrições, áudio PCM gravado e logs técnicos de execução.
          </p>
        </div>

        <label className="inline-flex items-center gap-2 text-xs font-medium text-zinc-700 bg-white px-3 py-2 border border-zinc-200 rounded-lg cursor-pointer shadow-2xs">
          <input
            type="checkbox"
            checked={includeTest}
            onChange={(e) => setIncludeTest(e.target.checked)}
            className="rounded border-zinc-300 text-zinc-900 focus:ring-zinc-900"
          />
          <span>Incluir Testes de Draft</span>
        </label>
      </div>

      {/* Search & Stats */}
      <div className="flex items-center justify-between gap-3">
        <div className="relative flex-1 max-w-md">
          <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-zinc-400" />
          <input
            type="text"
            placeholder="Buscar por agente, ID ou provedor..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full pl-9 pr-4 py-2 bg-white border border-zinc-200 rounded-lg text-sm text-zinc-900 placeholder-zinc-400 focus:outline-none focus:ring-1 focus:ring-zinc-900"
          />
        </div>
        <span className="text-xs text-zinc-500">
          {filtered.length} {filtered.length === 1 ? 'conversa' : 'conversas'}
        </span>
      </div>

      {errorMsg && (
        <div className="p-3 bg-red-50 border border-red-200 rounded-lg text-red-700 text-xs">
          {errorMsg}
        </div>
      )}

      {/* Table */}
      <div className="bg-white border border-zinc-200/80 rounded-xl overflow-hidden shadow-xs">
        <table className="w-full text-left text-xs">
          <thead className="bg-zinc-50/70 border-b border-zinc-200 text-zinc-600 font-semibold uppercase tracking-wider text-[11px]">
            <tr>
              <th className="px-5 py-3">Status</th>
              <th className="px-5 py-3">Agente</th>
              <th className="px-5 py-3">Provedor / Modelo</th>
              <th className="px-5 py-3">Tipo</th>
              <th className="px-5 py-3">Duração</th>
              <th className="px-5 py-3">Custo Estimado</th>
              <th className="px-5 py-3">Desfecho</th>
              <th className="px-5 py-3">Data / Hora</th>
              <th className="px-5 py-3 text-right"></th>
            </tr>
          </thead>
          <tbody className="divide-y divide-zinc-100">
            {filtered.length === 0 ? (
              <tr>
                <td colSpan={9} className="px-5 py-10 text-center text-zinc-400">
                  Nenhuma conversa encontrada.
                </td>
              </tr>
            ) : (
              filtered.map((c) => (
                <tr key={c.id} className="hover:bg-zinc-50/50 transition-colors">
                  <td className="px-5 py-3.5">
                    <span
                      className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-[11px] font-medium ${
                        c.status === 'completed'
                          ? 'bg-emerald-50 text-emerald-700 border border-emerald-200'
                          : c.status === 'failed'
                          ? 'bg-red-50 text-red-700 border border-red-200'
                          : 'bg-zinc-100 text-zinc-700'
                      }`}
                    >
                      <span
                        className={`w-1.5 h-1.5 rounded-full ${
                          c.status === 'completed'
                            ? 'bg-emerald-500'
                            : c.status === 'failed'
                            ? 'bg-red-500'
                            : 'bg-zinc-400'
                        }`}
                      />
                      {c.status === 'completed' ? 'Concluída' : c.status}
                    </span>
                  </td>

                  <td className="px-5 py-3.5 font-medium text-zinc-900">
                    <div className="flex items-center gap-1.5">
                      <Bot className="w-3.5 h-3.5 text-zinc-400" />
                      {c.agent_name || 'Agente'}
                    </div>
                  </td>

                  <td className="px-5 py-3.5 text-zinc-600 font-mono text-[11px]">
                    <span className="uppercase font-semibold text-zinc-800">{c.provider}</span> • {c.model}
                  </td>

                  <td className="px-5 py-3.5">
                    <div className="flex flex-col gap-1 items-start">
                      <span
                        className={`text-[10px] px-2 py-0.5 rounded font-mono font-medium ${
                          c.direction === 'inbound'
                            ? 'bg-blue-50 text-blue-700 border border-blue-200'
                            : c.is_test
                            ? 'bg-amber-50 text-amber-800 border border-amber-200'
                            : 'bg-zinc-100 text-zinc-700'
                        }`}
                      >
                        {c.direction === 'inbound' ? 'SIP Inbound' : c.is_test ? 'Draft Test' : 'Web Console'}
                      </span>
                      {c.from_number && (
                        <span className="text-[10px] font-mono text-zinc-500" title={`Para: ${c.to_number || 'DID'}`}>
                          {c.from_number}
                        </span>
                      )}
                    </div>
                  </td>

                  <td className="px-5 py-3.5 font-mono text-zinc-700 flex items-center gap-1">
                    <Clock className="w-3 h-3 text-zinc-400" />
                    {formatDuration(c.duration_ms)}
                  </td>

                  <td className="px-5 py-3.5">
                    <span className="font-mono text-[11px] font-medium text-emerald-700 bg-emerald-50 px-2 py-0.5 rounded border border-emerald-200">
                      ${(c.cost_usd || 0).toFixed(4)}
                    </span>
                    <span className="text-[10px] text-zinc-400 block mt-0.5">
                      R$ {(c.cost_brl || 0).toFixed(2)}
                    </span>
                  </td>

                  <td className="px-5 py-3.5 text-zinc-500">{formatReason(c.ended_reason)}</td>

                  <td className="px-5 py-3.5 text-zinc-500 text-[11px]">
                    {new Date(c.started_at).toLocaleString('pt-BR')}
                  </td>

                  <td className="px-5 py-3.5 text-right">
                    <Link
                      to={`/conversations/${c.id}`}
                      className="inline-flex items-center gap-1 text-zinc-600 hover:text-zinc-950 font-medium transition-colors"
                    >
                      Ver detalhes
                      <ArrowRight className="w-3 h-3" />
                    </Link>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}
