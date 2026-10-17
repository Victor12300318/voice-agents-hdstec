import { useEffect, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { AnimatePresence, motion } from 'motion/react'
import { springStudio, transitionPopover } from '../components/motion'
import {
  Plus,
  Search,
  MoreHorizontal,
  ArrowDown,
  ArrowUp,
  Play,
  Settings,
  Copy,
  Power,
  X,
} from 'lucide-react'
import { api, type Agent, type AgentDetail, type Conversation, type Me } from '../api'
import Modal from '../components/Modal'
import Button from '../components/Button'

export default function Agents() {
  const [agents, setAgents] = useState<Agent[]>([])
  const [conversations, setConversations] = useState<Conversation[]>([])
  const [me, setMe] = useState<Me | null>(null)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [enableVersioning, setEnableVersioning] = useState(false)
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<'all' | 'active' | 'inactive'>('all')
  const [providerFilter, setProviderFilter] = useState<'all' | 'gemini' | 'openai'>('all')
  const [sortDesc, setSortDesc] = useState(true)
  const [openMenuId, setOpenMenuId] = useState<string | null>(null)
  const [error, setError] = useState('')
  const [creating, setCreating] = useState(false)
  const [isModalOpen, setIsModalOpen] = useState(false)
  const navigate = useNavigate()

  const load = async () => {
    try {
      const [ags, convs, meRes] = await Promise.all([
        api<Agent[]>('/api/agents'),
        api<Conversation[]>('/api/conversations').catch(() => []),
        api<Me>('/api/auth/me').catch(() => null),
      ])
      setAgents(ags || [])
      setConversations(convs || [])
      if (meRes) setMe(meRes)
    } catch (e: any) {
      setError(e.message)
    }
  }

  useEffect(() => {
    load()
  }, [])

  const create = async (e: FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    setError('')
    setCreating(true)
    try {
      const created = await api<Agent>('/api/agents', {
        method: 'POST',
        body: JSON.stringify({
          name: name.trim(),
          description: description.trim(),
          enable_versioning: enableVersioning,
        }),
      })
      navigate(`/agents/${created.id}`)
    } catch (err) {
      setError((err as Error).message)
      setCreating(false)
    }
  }

  const handleDuplicate = async (ag: Agent) => {
    setOpenMenuId(null)
    try {
      await api<Agent>(`/api/agents/${ag.id}/duplicate`, {
        method: 'POST',
        body: JSON.stringify({
          name: `${ag.name} (Cópia)`,
        }),
      })
      await load()
    } catch (err: any) {
      setError(err.message || 'Erro ao duplicar agente')
    }
  }

  const handleToggleActive = async (ag: Agent) => {
    setOpenMenuId(null)
    try {
      const full = await api<AgentDetail>(`/api/agents/${ag.id}`)
      await api(`/api/agents/${ag.id}`, {
        method: 'PUT',
        body: JSON.stringify({
          name: full.name,
          description: full.description || '',
          active: !full.active,
          provider: full.provider,
          model: full.model,
          voice: full.voice,
          language: full.language,
          timezone: full.timezone,
          system_prompt: full.system_prompt,
          first_message: full.first_message,
          first_message_interruptible: full.first_message_interruptible,
          max_duration_seconds: full.max_duration_seconds,
          recording_enabled: full.recording_enabled,
          variables: full.variables || [],
          initiation_webhook_url: full.initiation_webhook_url || '',
          initiation_webhook_timeout_ms: full.initiation_webhook_timeout_ms || 3000,
          system_tools: full.system_tools || ['encerrar_conversa'],
          tools: full.tools || [],
        }),
      })
      await load()
    } catch (err: any) {
      setError(err.message || 'Erro ao atualizar agente')
    }
  }

  // Contabiliza conversas dos últimos 7 dias por agente
  const sevenDaysAgo = Date.now() - 7 * 24 * 60 * 60 * 1000
  const convCount7dByAgent: Record<string, number> = {}
  for (const c of conversations) {
    const startedMs = new Date(c.started_at).getTime()
    if (!isNaN(startedMs) && startedMs >= sevenDaysAgo) {
      convCount7dByAgent[c.agent_id] = (convCount7dByAgent[c.agent_id] || 0) + 1
    }
  }

  const formatCreatedAt = (iso?: string) => {
    if (!iso) return '—'
    const d = new Date(iso)
    if (isNaN(d.getTime())) return '—'
    const dd = String(d.getDate()).padStart(2, '0')
    const mm = String(d.getMonth() + 1).padStart(2, '0')
    const yyyy = d.getFullYear()
    const hh = String(d.getHours()).padStart(2, '0')
    const min = String(d.getMinutes()).padStart(2, '0')
    return `${dd}/${mm}/${yyyy}, ${hh}:${min}`
  }

  const filtered = agents
    .filter((a) => {
      if (statusFilter === 'active' && !a.active) return false
      if (statusFilter === 'inactive' && a.active) return false
      if (providerFilter === 'gemini' && !(a.provider || '').includes('gemini')) return false
      if (providerFilter === 'openai' && !(a.provider || '').includes('openai')) return false
      if (!search.trim()) return true
      const q = search.toLowerCase()
      return (
        a.name.toLowerCase().includes(q) ||
        (a.description && a.description.toLowerCase().includes(q)) ||
        (a.provider && a.provider.toLowerCase().includes(q))
      )
    })
    .sort((a, b) => {
      const ca = convCount7dByAgent[a.id] || 0
      const cb = convCount7dByAgent[b.id] || 0
      if (ca !== cb) {
        return sortDesc ? cb - ca : ca - cb
      }
      return new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
    })

  return (
    <div
      className="max-w-5xl mx-auto py-4 px-2 sm:px-6 space-y-5 text-zinc-900"
      onClick={() => setOpenMenuId(null)}
    >
      {/* Topo: Título "Agentes" + Botão "+ Novo agente" */}
      <div className="flex items-center justify-between gap-4">
        <h1 className="text-2xl font-bold tracking-tight text-zinc-950">Agentes</h1>

        <button
          type="button"
          onClick={() => setIsModalOpen(true)}
          className="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl bg-zinc-950 hover:bg-zinc-800 text-white text-xs font-semibold shadow-xs transition-colors cursor-pointer"
        >
          <Plus className="w-3.5 h-3.5" />
          Novo agente
        </button>
      </div>

      {/* Barra de Busca "Buscar agentes..." */}
      <div className="space-y-2.5">
        <div className="relative">
          <Search className="w-4 h-4 absolute left-3.5 top-1/2 -translate-y-1/2 text-zinc-400" />
          <input
            type="text"
            placeholder="Buscar agentes..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full pl-10 pr-4 py-2.5 bg-white border border-zinc-200/90 rounded-xl text-xs text-zinc-900 placeholder-zinc-400 focus:outline-none focus:border-zinc-400 transition-colors"
          />
        </div>

        {/* Pílulas de Filtro Rápido (apenas com filtros reais da plataforma) */}
        <div className="flex flex-wrap items-center gap-1.5">
          <button
            type="button"
            onClick={() =>
              setStatusFilter(
                statusFilter === 'all'
                  ? 'active'
                  : statusFilter === 'active'
                    ? 'inactive'
                    : 'all',
              )
            }
            className={`inline-flex items-center gap-1 px-2.5 py-1 rounded-lg border text-xs font-medium transition-colors ${
              statusFilter !== 'all'
                ? 'bg-zinc-900 text-white border-zinc-900'
                : 'bg-white hover:bg-zinc-50 text-zinc-700 border-zinc-200/90'
            }`}
          >
            <Plus className="w-3 h-3" />
            {statusFilter === 'all'
              ? 'Status'
              : statusFilter === 'active'
                ? 'Status: Ativos'
                : 'Status: Inativos'}
          </button>

          <button
            type="button"
            onClick={() =>
              setProviderFilter(
                providerFilter === 'all'
                  ? 'gemini'
                  : providerFilter === 'gemini'
                    ? 'openai'
                    : 'all',
              )
            }
            className={`inline-flex items-center gap-1 px-2.5 py-1 rounded-lg border text-xs font-medium transition-colors ${
              providerFilter !== 'all'
                ? 'bg-zinc-900 text-white border-zinc-900'
                : 'bg-white hover:bg-zinc-50 text-zinc-700 border-zinc-200/90'
            }`}
          >
            <Plus className="w-3 h-3" />
            {providerFilter === 'all'
              ? 'Provedor'
              : providerFilter === 'gemini'
                ? 'Provedor: Gemini Live'
                : 'Provedor: OpenAI'}
          </button>
        </div>
      </div>

      {error && (
        <div className="p-3 bg-red-50 border border-red-200 rounded-xl text-red-700 text-xs">
          {error}
        </div>
      )}

      {/* Tabela Limpa Estilo ElevenLabs (Nome | Criado por | ↓ conversas de 7 dias | Criado em | •••) */}
      <div className="w-full">
        <table className="w-full text-left border-collapse">
          <thead>
            <tr className="border-b border-zinc-200/80 text-xs text-zinc-500">
              <th className="py-3 pr-4 font-normal w-[38%]">Nome</th>
              <th className="py-3 px-4 font-normal w-[28%]">Criado por</th>
              <th className="py-3 px-4 font-normal text-right w-[16%]">
                <button
                  type="button"
                  onClick={() => setSortDesc(!sortDesc)}
                  className="inline-flex items-center gap-1 hover:text-zinc-900 transition-colors cursor-pointer"
                >
                  {sortDesc ? (
                    <ArrowDown className="w-3 h-3 text-zinc-500" />
                  ) : (
                    <ArrowUp className="w-3 h-3 text-zinc-500" />
                  )}
                  <span>conversas de 7 dias</span>
                </button>
              </th>
              <th className="py-3 pl-4 pr-2 font-normal text-right w-[14%]">Criado em</th>
              <th className="py-3 pl-2 w-10" />
            </tr>
          </thead>
          <tbody className="divide-y divide-zinc-100 text-[13px]">
            {filtered.length === 0 ? (
              <tr>
                <td colSpan={5} className="py-12 text-center text-xs text-zinc-400">
                  Nenhum agente encontrado.
                </td>
              </tr>
            ) : (
              filtered.map((a, idx) => {
                const count7d = convCount7dByAgent[a.id] || 0
                const isMenuOpen = openMenuId === a.id
                return (
                  <motion.tr
                    key={a.id}
                    initial={{ opacity: 0, y: 6 }}
                    animate={{ opacity: 1, y: 0 }}
                    transition={{ ...springStudio, delay: Math.min(idx, 12) * 0.022 }}
                    onClick={() => navigate(`/agents/${a.id}`)}
                    className="group hover:bg-zinc-50/80 transition-colors cursor-pointer"
                  >
                    <td className="py-4 pr-4 font-semibold text-zinc-900">
                      <div className="flex items-center gap-2.5">
                        <span>{a.name}</span>
                        {!a.active && (
                          <span className="px-2 py-0.5 rounded-md bg-zinc-100 text-zinc-500 text-[10px] font-medium">
                            Inativo
                          </span>
                        )}
                      </div>
                    </td>

                    <td className="py-4 px-4 text-xs text-zinc-600 truncate max-w-[240px]">
                      {me?.email || 'admin@organizacao'}
                    </td>

                    <td className="py-4 px-4 text-xs text-zinc-800 text-right font-variant-numeric tabular-nums">
                      {count7d}
                    </td>

                    <td className="py-4 pl-4 pr-2 text-xs text-zinc-600 text-right whitespace-nowrap tabular-nums">
                      {formatCreatedAt(a.created_at)}
                    </td>

                    <td
                      className="py-4 pl-2 text-right relative"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <button
                        type="button"
                        onClick={() => setOpenMenuId(isMenuOpen ? null : a.id)}
                        className="p-1.5 rounded-lg text-zinc-400 hover:text-zinc-800 hover:bg-zinc-100 transition-colors"
                      >
                        <MoreHorizontal className="w-4 h-4" />
                      </button>

                      <AnimatePresence>
                        {isMenuOpen && (
                          <motion.div
                            key={`menu-${a.id}`}
                            initial={{ opacity: 0, scale: 0.96, y: -4 }}
                            animate={{ opacity: 1, scale: 1, y: 0 }}
                            exit={{ opacity: 0, scale: 0.96, y: -2 }}
                            transition={transitionPopover}
                            className="absolute right-0 top-11 z-30 w-48 bg-white border border-zinc-200/90 rounded-xl shadow-lg py-1 text-xs text-left origin-top-right"
                          >
                            <button
                              type="button"
                              onClick={() => navigate(`/agents/${a.id}`)}
                              className="w-full px-3.5 py-2 hover:bg-zinc-50 flex items-center gap-2 text-zinc-700 font-medium"
                            >
                              <Settings className="w-3.5 h-3.5 text-zinc-400" />
                              Configurar agente
                            </button>

                            <button
                              type="button"
                              onClick={() => navigate(`/agents/${a.id}/test`)}
                              className="w-full px-3.5 py-2 hover:bg-zinc-50 flex items-center gap-2 text-zinc-700 font-medium"
                            >
                              <Play className="w-3.5 h-3.5 text-zinc-400" />
                              Testar conversa
                            </button>

                            <button
                              type="button"
                              onClick={() => handleDuplicate(a)}
                              className="w-full px-3.5 py-2 hover:bg-zinc-50 flex items-center gap-2 text-zinc-700 font-medium"
                            >
                              <Copy className="w-3.5 h-3.5 text-zinc-400" />
                              Duplicar agente
                            </button>

                            <div className="my-1 border-t border-zinc-100" />

                            <button
                              type="button"
                              onClick={() => handleToggleActive(a)}
                              className="w-full px-3.5 py-2 hover:bg-zinc-50 flex items-center gap-2 text-zinc-700 font-medium"
                            >
                              <Power className="w-3.5 h-3.5 text-zinc-400" />
                              {a.active ? 'Desativar agente' : 'Ativar agente'}
                            </button>
                          </motion.div>
                        )}
                      </AnimatePresence>
                    </td>
                  </motion.tr>
                )
              })
            )}
          </tbody>
        </table>
      </div>

      {/* Modal Criar Agente */}
      <Modal open={isModalOpen} onClose={() => setIsModalOpen(false)}>
        <div className="p-6 space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="font-semibold text-zinc-900 text-sm">Novo agente</h3>
            <button
              onClick={() => setIsModalOpen(false)}
              className="text-zinc-400 hover:text-zinc-600"
            >
              <X className="w-4 h-4" />
            </button>
          </div>

          <form onSubmit={create} className="space-y-4">
            <div>
              <label className="block text-xs font-semibold text-zinc-700 mb-1">
                Nome do agente
              </label>
              <input
                placeholder="Ex: Jessica"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
                maxLength={100}
                className="w-full px-3.5 py-2 bg-white border border-zinc-200 rounded-xl text-sm text-zinc-900 focus:outline-none focus:border-zinc-400"
              />
            </div>

            <div>
              <label className="block text-xs font-semibold text-zinc-700 mb-1">
                Descrição (opcional)
              </label>
              <input
                placeholder="Ex: Consultora especialista em vendas e fidelização"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                maxLength={500}
                className="w-full px-3.5 py-2 bg-white border border-zinc-200 rounded-xl text-sm text-zinc-900 focus:outline-none focus:border-zinc-400"
              />
            </div>

            <label className="flex items-start gap-2.5 p-3 rounded-xl bg-zinc-50 border border-zinc-200/80 cursor-pointer text-xs">
              <input
                type="checkbox"
                checked={enableVersioning}
                onChange={(e) => setEnableVersioning(e.target.checked)}
                className="mt-0.5 rounded border-zinc-300 text-zinc-900"
              />
              <div>
                <span className="font-semibold text-zinc-900 block">
                  Habilitar controle de versão (Branches & Divisão de Tráfego)
                </span>
                <span className="text-[11px] text-zinc-500">
                  Cria a branch Main com snapshots imutáveis (agtvrsn_xxxx) e experimentos A/B.
                </span>
              </div>
            </label>

            <div className="flex justify-end gap-2 pt-2">
              <Button type="button" variant="secondary" onClick={() => setIsModalOpen(false)}>
                Cancelar
              </Button>
              <Button type="submit" disabled={creating || !name.trim()}>
                {creating ? 'Criando…' : 'Criar agente'}
              </Button>
            </div>
          </form>
        </div>
      </Modal>
    </div>
  )
}
