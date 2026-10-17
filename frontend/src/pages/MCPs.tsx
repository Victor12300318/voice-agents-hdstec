import { useEffect, useState } from 'react'
import { AnimatePresence, motion } from 'motion/react'
import { springStudio, transitionPopover } from '../components/motion'
import {
  BookOpen,
  Webhook,
  Wrench,
  Blocks,
  Code2,
  Search,
  Plus,
  MoreHorizontal,
  Play,
  Settings,
  Copy,
  Trash2,
  X,
  Check,
  AlertCircle,
} from 'lucide-react'
import { api, type ToolWithStats, type Agent, type Me } from '../api'
import ElevenToolDrawer, { type ToolCategory } from '../components/ElevenToolDrawer'
import Modal from '../components/Modal'
import Button from '../components/Button'

export default function MCPsPage() {
  const [tools, setTools] = useState<ToolWithStats[]>([])
  const [agents, setAgents] = useState<Agent[]>([])
  const [me, setMe] = useState<Me | null>(null)
  const [loading, setLoading] = useState(true)
  const [errorMsg, setErrorMsg] = useState('')
  const [search, setSearch] = useState('')
  const [kindFilter, setKindFilter] = useState<'all' | ToolCategory>('all')
  const [selectedAgentFilter, setSelectedAgentFilter] = useState<string>('all')
  const [openMenuId, setOpenMenuId] = useState<string | null>(null)

  // Gaveta Oficial ElevenLabs (Webhook / Client / MCP / Code + Editar como JSON)
  const [isDrawerOpen, setIsDrawerOpen] = useState(false)
  const [drawerInitialKind, setDrawerInitialKind] = useState<ToolCategory>('webhook')
  const [editingTool, setEditingTool] = useState<ToolWithStats | null>(null)

  // Modal de Teste Interativo da Ferramenta
  const [testModalTool, setTestModalTool] = useState<ToolWithStats | null>(null)
  const [testArgsJson, setTestArgsJson] = useState('{\n  "ddd": "11",\n  "operadora": "TIM"\n}')
  const [testingTool, setTestingTool] = useState(false)
  const [testResult, setTestResult] = useState<any>(null)

  const loadData = async () => {
    setLoading(true)
    try {
      const [tls, ags, meRes] = await Promise.all([
        api<ToolWithStats[]>('/api/tools'),
        api<Agent[]>('/api/agents').catch(() => []),
        api<Me>('/api/auth/me').catch(() => null),
      ])
      setTools(tls || [])
      setAgents(ags || [])
      if (meRes) setMe(meRes)
    } catch (err: any) {
      setErrorMsg(err.message || 'Erro ao carregar ferramentas')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadData()
  }, [])

  const openNewToolDrawer = (kind: ToolCategory) => {
    setEditingTool(null)
    setDrawerInitialKind(kind)
    setIsDrawerOpen(true)
  }

  const openEditToolDrawer = (t: ToolWithStats) => {
    setOpenMenuId(null)
    setEditingTool(t)
    setDrawerInitialKind(((t.kind as ToolCategory) || 'webhook'))
    setIsDrawerOpen(true)
  }

  const handleDuplicateTool = async (t: ToolWithStats) => {
    setOpenMenuId(null)
    try {
      await api('/api/tools', {
        method: 'POST',
        body: JSON.stringify({
          name: `${t.name}_copy`,
          description: t.description || '',
          kind: t.kind || 'webhook',
          input_schema: t.input_schema || [],
          config: t.config || { url: 'https://api.example.com/v1', method: 'GET' },
          timeout_ms: t.timeout_ms || 20000,
          active: true,
        }),
      })
      await loadData()
    } catch (err: any) {
      setErrorMsg(err.message)
    }
  }

  const handleDeleteTool = async (id: string) => {
    setOpenMenuId(null)
    try {
      await api(`/api/tools/${id}`, { method: 'DELETE' })
      setTools(tools.filter((t) => t.id !== id))
    } catch (err: any) {
      setErrorMsg(err.message)
    }
  }

  const handleRunTest = async () => {
    if (!testModalTool) return
    setTestingTool(true)
    setTestResult(null)
    try {
      let parsedArgs = {}
      if (testArgsJson.trim()) {
        parsedArgs = JSON.parse(testArgsJson)
      }
      const res = await api<any>(`/api/tools/${testModalTool.id}/test`, {
        method: 'POST',
        body: JSON.stringify({ args: parsedArgs }),
      })
      setTestResult({ ok: res.status === 'success', data: res })
    } catch (err: any) {
      setTestResult({ ok: false, error: err.message || 'Erro ao executar ferramenta' })
    } finally {
      setTestingTool(false)
    }
  }

  const getToolKindIcon = (t: ToolWithStats) => {
    if (t.kind === 'client') return <Wrench className="w-4 h-4 text-zinc-600 mt-0.5 flex-shrink-0" />
    if (t.kind === 'code') return <Code2 className="w-4 h-4 text-zinc-600 mt-0.5 flex-shrink-0" />
    if (t.kind === 'mcp' || (t.config?.url || '').includes('/mcp')) {
      return <Blocks className="w-4 h-4 text-zinc-600 mt-0.5 flex-shrink-0" />
    }
    return <Webhook className="w-4 h-4 text-zinc-600 mt-0.5 flex-shrink-0" />
  }

  const getToolSubtitle = (t: ToolWithStats) => {
    if (t.kind === 'client') {
      return t.config?.expects_response
        ? 'Client Tool · Aguarda resposta do cliente'
        : 'Client Tool · Execução no navegador/SDK'
    }
    if (t.kind === 'code') {
      return 'Code Tool · JavaScript Sandbox (export default async (ctx) => ...)'
    }
    if (t.kind === 'mcp' || (t.config?.url || '').includes('/mcp')) {
      return `${t.config?.transport || 'SSE'} · ${t.config?.url || 'MCP Server'}`
    }
    return t.config?.api_schema?.url || t.config?.url || '—'
  }

  const filteredTools = tools.filter((t) => {
    if (kindFilter !== 'all') {
      const effectiveKind =
        t.kind === 'mcp' || (t.config?.url || '').includes('/mcp')
          ? 'mcp'
          : (t.kind || 'webhook')
      if (effectiveKind !== kindFilter) return false
    }
    if (selectedAgentFilter !== 'all') {
      const ag = agents.find((a) => a.id === selectedAgentFilter)
      if (ag && !(ag.tools || []).includes(t.id)) return false
    }
    if (!search.trim()) return true
    const q = search.toLowerCase()
    return (
      t.name.toLowerCase().includes(q) ||
      (t.description || '').toLowerCase().includes(q) ||
      (t.config?.url || '').toLowerCase().includes(q)
    )
  })

  return (
    <div
      className="max-w-5xl mx-auto py-4 px-2 sm:px-6 space-y-5 text-zinc-900"
      onClick={() => setOpenMenuId(null)}
    >
      {/* Topo: Título "Ferramentas 📖" */}
      <div className="flex items-center gap-2">
        <h1 className="text-2xl font-bold tracking-tight text-zinc-950">Ferramentas</h1>
        <a
          href="https://elevenlabs.io/docs/eleven-agents/customization/tools"
          target="_blank"
          rel="noreferrer"
          title="Documentação de Ferramentas da ElevenLabs"
          className="text-zinc-400 hover:text-zinc-700 transition-colors"
        >
          <BookOpen className="w-4 h-4" />
        </a>
      </div>

      {/* Linha de 4 Cards de Criação de Ferramentas (Webhook, Cliente, Integração MCP, Código) */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
        <button
          type="button"
          onClick={() => openNewToolDrawer('webhook')}
          className="p-4 bg-white hover:bg-zinc-50 border border-zinc-200/90 rounded-2xl text-left flex flex-col justify-between gap-4 shadow-2xs transition-colors cursor-pointer"
        >
          <Webhook className="w-4 h-4 text-zinc-700" />
          <span className="text-xs font-semibold text-zinc-900">Adicionar ferramenta webhook</span>
        </button>

        <button
          type="button"
          onClick={() => openNewToolDrawer('client')}
          className="p-4 bg-white hover:bg-zinc-50 border border-zinc-200/90 rounded-2xl text-left flex flex-col justify-between gap-4 shadow-2xs transition-colors cursor-pointer"
        >
          <Wrench className="w-4 h-4 text-zinc-700" />
          <span className="text-xs font-semibold text-zinc-900">Adicionar ferramenta cliente</span>
        </button>

        <button
          type="button"
          onClick={() => openNewToolDrawer('mcp')}
          className="p-4 bg-white hover:bg-zinc-50 border border-zinc-200/90 rounded-2xl text-left flex flex-col justify-between gap-4 shadow-2xs transition-colors cursor-pointer"
        >
          <Blocks className="w-4 h-4 text-zinc-700" />
          <span className="text-xs font-semibold text-zinc-900">
            Adicionar ferramenta de integração
          </span>
        </button>

        <button
          type="button"
          onClick={() => openNewToolDrawer('code')}
          className="p-4 bg-white hover:bg-zinc-50 border border-zinc-200/90 rounded-2xl text-left flex flex-col justify-between gap-4 shadow-2xs transition-colors cursor-pointer"
        >
          <Code2 className="w-4 h-4 text-zinc-700" />
          <span className="text-xs font-semibold text-zinc-900">Adicionar ferramenta código</span>
        </button>
      </div>

      {/* Barra de Busca + Pílulas "+ Tipo", "+ Criador", "+ Agente" */}
      <div className="space-y-2.5">
        <div className="relative">
          <Search className="w-4 h-4 absolute left-3.5 top-1/2 -translate-y-1/2 text-zinc-400" />
          <input
            type="text"
            placeholder="Buscar ferramentas..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full pl-10 pr-4 py-2.5 bg-white border border-zinc-200/90 rounded-xl text-xs text-zinc-900 placeholder-zinc-400 focus:outline-none focus:border-zinc-400 transition-colors"
          />
        </div>

        <div className="flex flex-wrap items-center gap-1.5">
          <button
            type="button"
            onClick={() => {
              const order: ('all' | ToolCategory)[] = ['all', 'webhook', 'client', 'mcp', 'code']
              const nextIdx = (order.indexOf(kindFilter) + 1) % order.length
              setKindFilter(order[nextIdx])
            }}
            className={`inline-flex items-center gap-1 px-2.5 py-1 rounded-lg border text-xs font-medium transition-colors cursor-pointer ${
              kindFilter !== 'all'
                ? 'bg-zinc-900 text-white border-zinc-900'
                : 'bg-white hover:bg-zinc-50 text-zinc-700 border-zinc-200/90'
            }`}
          >
            <Plus className="w-3 h-3" />
            {kindFilter === 'all'
              ? 'Tipo'
              : kindFilter === 'webhook'
                ? 'Tipo: Webhook'
                : kindFilter === 'client'
                  ? 'Tipo: Cliente'
                  : kindFilter === 'mcp'
                    ? 'Tipo: Integração MCP'
                    : 'Tipo: Código'}
          </button>

          <span className="inline-flex items-center gap-1 px-2.5 py-1 rounded-lg border border-zinc-200/90 bg-white text-xs font-medium text-zinc-700">
            <Plus className="w-3 h-3" />
            Criador
          </span>

          <button
            type="button"
            onClick={() => {
              if (agents.length === 0) return
              if (selectedAgentFilter === 'all') {
                setSelectedAgentFilter(agents[0].id)
              } else {
                const idx = agents.findIndex((a) => a.id === selectedAgentFilter)
                if (idx >= 0 && idx + 1 < agents.length) {
                  setSelectedAgentFilter(agents[idx + 1].id)
                } else {
                  setSelectedAgentFilter('all')
                }
              }
            }}
            className={`inline-flex items-center gap-1 px-2.5 py-1 rounded-lg border text-xs font-medium transition-colors cursor-pointer ${
              selectedAgentFilter !== 'all'
                ? 'bg-zinc-900 text-white border-zinc-900'
                : 'bg-white hover:bg-zinc-50 text-zinc-700 border-zinc-200/90'
            }`}
          >
            <Plus className="w-3 h-3" />
            {selectedAgentFilter === 'all'
              ? 'Agente'
              : `Agente: ${agents.find((a) => a.id === selectedAgentFilter)?.name || ''}`}
          </button>
        </div>
      </div>

      {errorMsg && (
        <div className="p-3 bg-red-50 border border-red-200 rounded-xl text-red-700 text-xs flex items-center justify-between">
          <span>{errorMsg}</span>
          <button onClick={() => setErrorMsg('')} className="text-red-400 hover:text-red-700">
            <X className="w-3.5 h-3.5" />
          </button>
        </div>
      )}

      {/* Tabela de Ferramentas (Nome + URL/Subtítulo | Descrição | Criado por | •••) */}
      <div className="w-full">
        <table className="w-full text-left border-collapse">
          <thead>
            <tr className="border-b border-zinc-200/80 text-xs text-zinc-500">
              <th className="py-3 pr-4 font-normal w-[36%]">Nome</th>
              <th className="py-3 px-4 font-normal w-[42%]">Descrição</th>
              <th className="py-3 pl-4 pr-2 font-normal w-[18%]">Criado por</th>
              <th className="py-3 pl-2 w-10" />
            </tr>
          </thead>
          <tbody className="divide-y divide-zinc-100 text-xs">
            {loading ? (
              <tr>
                <td colSpan={4} className="py-12 text-center text-zinc-400">
                  Carregando ferramentas…
                </td>
              </tr>
            ) : filteredTools.length === 0 ? (
              <tr>
                <td colSpan={4} className="py-12 text-center text-zinc-400">
                  Nenhuma ferramenta encontrada. Clique em um dos cards acima para adicionar.
                </td>
              </tr>
            ) : (
              filteredTools.map((t, idx) => {
                const isMenuOpen = openMenuId === t.id
                return (
                  <motion.tr
                    key={t.id}
                    initial={{ opacity: 0, y: 6 }}
                    animate={{ opacity: 1, y: 0 }}
                    transition={{ ...springStudio, delay: Math.min(idx, 12) * 0.022 }}
                    onClick={() => openEditToolDrawer(t)}
                    className="group hover:bg-zinc-50/80 transition-colors cursor-pointer"
                  >
                    <td className="py-4 pr-4 align-top">
                      <div className="flex items-start gap-2.5">
                        {getToolKindIcon(t)}
                        <div className="min-w-0">
                          <div className="font-semibold text-zinc-900 text-[13px] truncate">
                            {t.name}
                          </div>
                          <div className="text-[11px] text-zinc-400 truncate max-w-[330px] mt-0.5">
                            {getToolSubtitle(t)}
                          </div>
                        </div>
                      </div>
                    </td>

                    <td className="py-4 px-4 align-top text-zinc-700 leading-relaxed">
                      <div className="line-clamp-2 max-w-md">
                        {t.description || 'Sem descrição configurada.'}
                      </div>
                    </td>

                    <td className="py-4 pl-4 pr-2 align-top text-zinc-600 truncate max-w-[200px]">
                      {me?.email || 'admin@organizacao'}
                    </td>

                    <td
                      className="py-4 pl-2 text-right align-top relative"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <button
                        type="button"
                        onClick={() => setOpenMenuId(isMenuOpen ? null : t.id)}
                        className="p-1.5 rounded-lg text-zinc-400 hover:text-zinc-800 hover:bg-zinc-100 transition-colors cursor-pointer"
                      >
                        <MoreHorizontal className="w-4 h-4" />
                      </button>

                      <AnimatePresence>
                        {isMenuOpen && (
                          <motion.div
                            key={`tool-menu-${t.id}`}
                            initial={{ opacity: 0, scale: 0.96, y: -4 }}
                            animate={{ opacity: 1, scale: 1, y: 0 }}
                            exit={{ opacity: 0, scale: 0.96, y: -2 }}
                            transition={transitionPopover}
                            className="absolute right-0 top-11 z-30 w-48 bg-white border border-zinc-200/90 rounded-xl shadow-lg py-1 text-xs text-left origin-top-right"
                          >
                            <button
                              type="button"
                              onClick={() => openEditToolDrawer(t)}
                              className="w-full px-3.5 py-2 hover:bg-zinc-50 flex items-center gap-2 text-zinc-700 font-medium"
                            >
                              <Settings className="w-3.5 h-3.5 text-zinc-400" />
                              Editar ferramenta
                            </button>

                            <button
                              type="button"
                              onClick={() => {
                                setOpenMenuId(null)
                                const sampleObj: Record<string, any> = {}
                                for (const p of t.input_schema || []) {
                                  sampleObj[p.name] = p.type === 'number' ? 1 : `exemplo_${p.name}`
                                }
                                setTestArgsJson(JSON.stringify(sampleObj, null, 2))
                                setTestResult(null)
                                setTestModalTool(t)
                              }}
                              className="w-full px-3.5 py-2 hover:bg-zinc-50 flex items-center gap-2 text-zinc-700 font-medium"
                            >
                              <Play className="w-3.5 h-3.5 text-zinc-400" />
                              Testar execução
                            </button>

                            <button
                              type="button"
                              onClick={() => handleDuplicateTool(t)}
                              className="w-full px-3.5 py-2 hover:bg-zinc-50 flex items-center gap-2 text-zinc-700 font-medium"
                            >
                              <Copy className="w-3.5 h-3.5 text-zinc-400" />
                              Duplicar
                            </button>

                            <div className="my-1 border-t border-zinc-100" />

                            <button
                              type="button"
                              onClick={() => handleDeleteTool(t.id)}
                              className="w-full px-3.5 py-2 hover:bg-red-50 flex items-center gap-2 text-red-600 font-medium"
                            >
                              <Trash2 className="w-3.5 h-3.5" />
                              Excluir ferramenta
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

      {/* Gaveta ElevenLabs de Criação/Edição de Ferramenta */}
      <ElevenToolDrawer
        open={isDrawerOpen}
        onClose={() => setIsDrawerOpen(false)}
        initialKind={drawerInitialKind}
        editingTool={editingTool}
        onSaved={() => {
          loadData()
        }}
      />

      {/* Modal de Teste de Execução da Ferramenta */}
      <Modal open={Boolean(testModalTool)} onClose={() => setTestModalTool(null)}>
        {testModalTool && (
          <div className="p-6 space-y-4 text-xs">
            <div className="flex items-center justify-between">
              <div className="font-semibold text-sm text-zinc-900 flex items-center gap-2">
                <Play className="w-4 h-4 text-zinc-700" />
                Testar ferramenta: <code className="font-mono">{testModalTool.name}</code>
              </div>
              <button
                onClick={() => setTestModalTool(null)}
                className="text-zinc-400 hover:text-zinc-700"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            <div>
              <label className="block font-semibold text-zinc-700 mb-1">
                Argumentos JSON de Teste (args)
              </label>
              <textarea
                rows={5}
                value={testArgsJson}
                onChange={(e) => setTestArgsJson(e.target.value)}
                className="w-full p-3 bg-zinc-950 text-zinc-100 rounded-xl font-mono text-xs focus:outline-none"
              />
            </div>

            {testResult && (
              <div
                className={`p-3 rounded-xl border font-mono text-[11px] whitespace-pre-wrap max-h-60 overflow-y-auto ${
                  testResult.ok
                    ? 'bg-emerald-50 border-emerald-200 text-emerald-950'
                    : 'bg-red-50 border-red-200 text-red-900'
                }`}
              >
                <div className="flex items-center gap-1.5 font-sans font-semibold mb-1">
                  {testResult.ok ? (
                    <>
                      <Check className="w-3.5 h-3.5 text-emerald-600" /> Execução concluída
                    </>
                  ) : (
                    <>
                      <AlertCircle className="w-3.5 h-3.5 text-red-600" /> Erro na execução
                    </>
                  )}
                </div>
                {JSON.stringify(testResult.data || testResult.error, null, 2)}
              </div>
            )}

            <div className="flex justify-end gap-2 pt-2">
              <Button variant="secondary" size="sm" onClick={() => setTestModalTool(null)}>
                Fechar
              </Button>
              <Button size="sm" onClick={handleRunTest} disabled={testingTool}>
                {testingTool ? 'Executando…' : 'Disparar Requisição'}
              </Button>
            </div>
          </div>
        )}
      </Modal>
    </div>
  )
}
