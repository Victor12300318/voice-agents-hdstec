import { useState, useEffect } from 'react'
import { AnimatePresence, motion } from 'motion/react'
import { springStudio, springTabPill, transitionPopover } from './motion'
import {
  Search,
  Wrench,
  Webhook,
  Blocks,
  Code2,
  Trash2,
  Settings,
  BookOpen,
  ChevronDown,
  ArrowRightLeft,
  ShieldCheck,
  Plus,
  X,
} from 'lucide-react'
import Toggle from './Toggle'
import Sheet from './Sheet'
import {
  api,
  type ToolWithStats,
  type Agent,
  type SystemToolsConfig,
  type AgentTransferRule,
  type GuardianJevConfig,
} from '../api'
import { type ToolCategory } from './ElevenToolDrawer'

interface ElevenToolsPanelProps {
  currentAgentId?: string
  tools: ToolWithStats[]
  enabledToolIds: string[]
  systemTools: string[]
  systemToolsConfig?: SystemToolsConfig
  referencedByProceduresToolIds: string[]
  onToggleTool: (toolId: string) => void
  onToggleSystemTool: (sysToolName: string) => void
  onSaveSystemToolsConfig?: (nextConfig: SystemToolsConfig, enableToolName?: string) => void
  onOpenNewToolModal: (kind?: ToolCategory) => void
  onEditTool?: (tool: ToolWithStats) => void
  onDeleteTool?: (toolId: string) => void
}

const SYSTEM_TOOLS_LIST = [
  { id: 'encerrar_conversa', label: 'Encerrar conversa', hasSettings: true },
  { id: 'detectar_idioma', label: 'Detectar idioma', hasSettings: false },
  { id: 'pular_vez', label: 'Pular vez', hasSettings: false },
  { id: 'atualizar_estado', label: 'Atualizar estado', hasSettings: false },
  { id: 'transferir_agente', label: 'Transferir para agente', hasSettings: true },
  { id: 'transferir_numero', label: 'Transferir para número', hasSettings: true },
  { id: 'reproduzir_dtmf', label: 'Reproduzir tom de toque do teclado', hasSettings: false },
  { id: 'detectar_correio_voz', label: 'Detecção de correio de voz', hasSettings: true },
  { id: 'guardiao_jev', label: 'Guardião Jev (TypeSafe / OpenRouter)', hasSettings: true },
]

export default function ElevenToolsPanel({
  currentAgentId,
  tools,
  enabledToolIds,
  systemTools,
  systemToolsConfig,
  referencedByProceduresToolIds,
  onToggleTool,
  onToggleSystemTool,
  onSaveSystemToolsConfig,
  onOpenNewToolModal,
  onEditTool,
}: ElevenToolsPanelProps) {
  const [search, setSearch] = useState('')
  const [subTab, setSubTab] = useState<'ferramentas' | 'mcp'>('ferramentas')
  const [kindFilter, setKindFilter] = useState<'all' | ToolCategory>('all')
  const [addMenuOpen, setAddMenuOpen] = useState(false)
  const [existingSearch, setExistingSearch] = useState('')

  // Estado da Gaveta de Configuração de "Transferir para agente" (transfer_to_agent)
  const [isTransferDrawerOpen, setIsTransferDrawerOpen] = useState(false)
  const [workspaceAgents, setWorkspaceAgents] = useState<Agent[]>([])
  const [transferDescription, setTransferDescription] = useState('')
  const [transferRules, setTransferRules] = useState<AgentTransferRule[]>([])
  const [isTransferJsonMode, setIsTransferJsonMode] = useState(false)
  const [transferJsonText, setTransferJsonText] = useState('')
  const [transferJsonError, setTransferJsonError] = useState('')

  // Estado da Gaveta de Configuração do "Guardião Jev" (OpenRouter `typesafe/jev-router` / TypeSafe `jev-latest`)
  const [isGuardianDrawerOpen, setIsGuardianDrawerOpen] = useState(false)
  const [guardianForm, setGuardianForm] = useState<GuardianJevConfig>({
    enabled: true,
    provider: 'openrouter',
    model: 'typesafe/jev-router',
    api_key: '',
    check_voicemail: true,
    check_talking_to_self: true,
    check_hallucination: true,
    voicemail_threshold: 0.8,
    talking_to_self_threshold: 0.78,
    hallucination_threshold: 0.75,
  })

  const openGuardianSettings = () => {
    const existing = systemToolsConfig?.guardian_jev
    setGuardianForm({
      enabled: existing ? existing.enabled : true,
      provider: existing?.provider || 'openrouter',
      model: existing?.model || 'typesafe/jev-router',
      api_key: existing?.api_key || '',
      check_voicemail: existing ? existing.check_voicemail : true,
      check_talking_to_self: existing ? existing.check_talking_to_self : true,
      check_hallucination: existing ? existing.check_hallucination : true,
      voicemail_threshold: existing?.voicemail_threshold ?? 0.8,
      talking_to_self_threshold: existing?.talking_to_self_threshold ?? 0.78,
      hallucination_threshold: existing?.hallucination_threshold ?? 0.75,
    })
    setIsGuardianDrawerOpen(true)
  }

  const handleSaveGuardianConfig = () => {
    if (onSaveSystemToolsConfig) {
      onSaveSystemToolsConfig(
        {
          ...(systemToolsConfig || {}),
          guardian_jev: {
            ...guardianForm,
            enabled: true,
          },
        },
        'guardiao_jev',
      )
    }
    setIsGuardianDrawerOpen(false)
  }

  useEffect(() => {
    api<Agent[]>('/api/agents')
      .then((list) => setWorkspaceAgents(list || []))
      .catch(() => {})
  }, [])

  const openTransferAgentSettings = () => {
    const existing = systemToolsConfig?.transfer_to_agent
    setTransferDescription(existing?.description || '')
    setTransferRules(
      existing?.transfers && existing.transfers.length > 0
        ? existing.transfers
        : [
            {
              agent_id:
                workspaceAgents.find((a) => a.id !== currentAgentId)?.id ||
                workspaceAgents[0]?.id ||
                '',
              condition: '',
              delay_ms: 0,
              transfer_message: '',
              enable_transferred_agent_first_message: false,
            },
          ],
    )
    setIsTransferJsonMode(false)
    setTransferJsonError('')
    setIsTransferDrawerOpen(true)
  }

  const buildTransferJsonObject = () => ({
    type: 'system',
    name: 'transfer_to_agent',
    description:
      transferDescription ||
      'Transfer the user to a specialized agent based on their request.',
    params: {
      system_tool_type: 'transfer_to_agent',
      transfers: transferRules.map((r) => ({
        agent_id: r.agent_id,
        condition: r.condition,
        delay_ms: r.delay_ms || 0,
        transfer_message: r.transfer_message ? r.transfer_message : null,
        enable_transferred_agent_first_message: Boolean(
          r.enable_transferred_agent_first_message,
        ),
      })),
    },
  })

  const applyTransferJsonToState = (raw: string): boolean => {
    try {
      const parsed = JSON.parse(raw)
      setTransferJsonError('')
      const root = parsed.transfer_to_agent || parsed.transferToAgent || parsed
      if (typeof root.description === 'string') {
        setTransferDescription(root.description)
      }
      const transfersArr = root.params?.transfers || root.transfers
      if (Array.isArray(transfersArr)) {
        setTransferRules(
          transfersArr.map((t: any) => ({
            agent_id: t.agent_id || t.agentId || '',
            condition: t.condition || '',
            delay_ms: Number(t.delay_ms ?? t.delayMs ?? 0),
            transfer_message: t.transfer_message ?? t.transferMessage ?? '',
            enable_transferred_agent_first_message: Boolean(
              t.enable_transferred_agent_first_message ??
                t.enableTransferredAgentFirstMessage ??
                false,
            ),
          })),
        )
      }
      return true
    } catch (e: any) {
      setTransferJsonError(`JSON inválido: ${e.message}`)
      return false
    }
  }

  const handleSaveTransferConfig = () => {
    let rulesToSave = transferRules
    let descToSave = transferDescription
    if (isTransferJsonMode) {
      if (!applyTransferJsonToState(transferJsonText)) return
      const parsed = JSON.parse(transferJsonText)
      const root = parsed.transfer_to_agent || parsed.transferToAgent || parsed
      descToSave = root.description || ''
      const arr = root.params?.transfers || root.transfers || []
      rulesToSave = arr.map((t: any) => ({
        agent_id: t.agent_id || t.agentId || '',
        agent_name: workspaceAgents.find((a) => a.id === (t.agent_id || t.agentId))?.name,
        condition: t.condition || '',
        delay_ms: Number(t.delay_ms ?? t.delayMs ?? 0),
        transfer_message: t.transfer_message ?? t.transferMessage ?? null,
        enable_transferred_agent_first_message: Boolean(
          t.enable_transferred_agent_first_message ??
            t.enableTransferredAgentFirstMessage ??
            false,
        ),
      }))
    } else {
      rulesToSave = transferRules.map((r) => ({
        ...r,
        agent_name: workspaceAgents.find((a) => a.id === r.agent_id)?.name,
        transfer_message: r.transfer_message?.trim() ? r.transfer_message.trim() : null,
      }))
    }

    if (onSaveSystemToolsConfig) {
      onSaveSystemToolsConfig(
        {
          ...systemToolsConfig,
          transfer_to_agent: {
            type: 'system',
            name: 'transfer_to_agent',
            description: descToSave.trim(),
            transfers: rulesToSave.filter((r) => r.agent_id),
          },
        },
        'transferir_agente',
      )
    }
    setIsTransferDrawerOpen(false)
  }

  // APENAS ferramentas que pertencem a este agente (vinculadas no agente ou referenciadas em procedimentos do agente)
  const agentTools = tools.filter(
    (t) => enabledToolIds.includes(t.id) || referencedByProceduresToolIds.includes(t.id),
  )

  // Ferramentas existentes no workspace que ainda NÃO estão vinculadas ao agente
  const unlinkedWorkspaceTools = tools.filter(
    (t) =>
      !enabledToolIds.includes(t.id) &&
      (t.name.toLowerCase().includes(existingSearch.toLowerCase()) ||
        (t.description || '').toLowerCase().includes(existingSearch.toLowerCase())),
  )

  const filteredTools = agentTools.filter((t) => {
    const isMcp = t.kind === 'mcp' || (t.config?.url || '').includes('/mcp')
    if (subTab === 'mcp' && !isMcp) return false
    if (kindFilter !== 'all') {
      const effectiveKind = isMcp ? 'mcp' : t.kind || 'webhook'
      if (effectiveKind !== kindFilter) return false
    }
    if (!search.trim()) return true
    const q = search.toLowerCase()
    return (
      t.name.toLowerCase().includes(q) ||
      (t.description || '').toLowerCase().includes(q) ||
      (t.config?.url || '').toLowerCase().includes(q)
    )
  })

  const activeSysCount = systemTools.length
  const configuredTransfersCount =
    systemToolsConfig?.transfer_to_agent?.transfers?.length || 0

  const getToolIcon = (t: ToolWithStats) => {
    if (t.kind === 'client') return <Wrench className="w-4 h-4 text-zinc-700 mt-0.5 flex-shrink-0" />
    if (t.kind === 'code') return <Code2 className="w-4 h-4 text-zinc-700 mt-0.5 flex-shrink-0" />
    if (t.kind === 'mcp' || (t.config?.url || '').includes('/mcp')) {
      return <Blocks className="w-4 h-4 text-zinc-700 mt-0.5 flex-shrink-0" />
    }
    return <Webhook className="w-4 h-4 text-zinc-700 mt-0.5 flex-shrink-0" />
  }

  const otherAgents = workspaceAgents.filter((a) => a.id !== currentAgentId)

  return (
    <div
      className="grid grid-cols-1 lg:grid-cols-12 gap-8 items-start"
      onClick={() => setAddMenuOpen(false)}
    >
      {/* Coluna Principal (8 colunas): Tabela de Ferramentas do Agente */}
      <div className="lg:col-span-8 space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="space-y-2">
            <h2 className="text-xl font-bold tracking-tight text-zinc-900 flex items-center gap-2">
              Ferramentas
              <BookOpen className="w-4 h-4 text-zinc-400" />
            </h2>
            <div className="inline-flex p-0.5 bg-zinc-100 rounded-xl border border-zinc-200/80">
              {(['ferramentas', 'mcp'] as const).map((tabKey) => {
                const isActive = subTab === tabKey
                return (
                  <button
                    key={tabKey}
                    type="button"
                    onClick={() => setSubTab(tabKey)}
                    className={`relative px-3 py-1 rounded-lg text-xs font-semibold transition-colors cursor-pointer ${
                      isActive ? 'text-zinc-900' : 'text-zinc-500 hover:text-zinc-800'
                    }`}
                  >
                    {isActive && (
                      <motion.span
                        layoutId="agent-tools-subtab-pill"
                        transition={springTabPill}
                        className="absolute inset-0 rounded-lg bg-white shadow-2xs"
                        aria-hidden="true"
                      />
                    )}
                    <span className="relative z-10">
                      {tabKey === 'ferramentas' ? 'Ferramentas' : 'MCP'}
                    </span>
                  </button>
                )
              })}
            </div>
          </div>

          {/* Botão "Adicionar ferramenta" com menu para criar nova ou vincular existente do workspace */}
          <div className="relative" onClick={(e) => e.stopPropagation()}>
            <button
              type="button"
              onClick={() => setAddMenuOpen(!addMenuOpen)}
              className="px-4 py-2 bg-zinc-950 hover:bg-zinc-800 text-white text-xs font-semibold rounded-xl shadow-xs transition-colors inline-flex items-center gap-1.5 cursor-pointer"
            >
              <span>Adicionar ferramenta</span>
              <ChevronDown className="w-3.5 h-3.5 text-zinc-300" />
            </button>

            <AnimatePresence>
              {addMenuOpen && (
                <motion.div
                  key="add-tool-dropdown"
                  initial={{ opacity: 0, scale: 0.96, y: -4 }}
                  animate={{ opacity: 1, scale: 1, y: 0 }}
                  exit={{ opacity: 0, scale: 0.96, y: -2 }}
                  transition={transitionPopover}
                  className="absolute right-0 top-11 z-30 w-80 bg-white border border-zinc-200/90 rounded-2xl shadow-xl p-2 text-xs space-y-2 origin-top-right"
                >
                <div className="px-2 py-1 text-[10px] font-semibold uppercase tracking-wider text-zinc-400">
                  Criar nova ferramenta para este agente
                </div>

                <div className="space-y-0.5">
                  <button
                    type="button"
                    onClick={() => {
                      setAddMenuOpen(false)
                      onOpenNewToolModal('webhook')
                    }}
                    className="w-full px-3 py-2 rounded-xl hover:bg-zinc-50 flex items-center gap-2.5 text-left text-zinc-800 font-medium cursor-pointer"
                  >
                    <Webhook className="w-4 h-4 text-zinc-600" />
                    <div>
                      <div>Adicionar ferramenta webhook</div>
                      <div className="text-[10px] text-zinc-400 font-normal">
                        Chamar API HTTP externa ou fluxo n8n
                      </div>
                    </div>
                  </button>

                  <button
                    type="button"
                    onClick={() => {
                      setAddMenuOpen(false)
                      onOpenNewToolModal('client')
                    }}
                    className="w-full px-3 py-2 rounded-xl hover:bg-zinc-50 flex items-center gap-2.5 text-left text-zinc-800 font-medium cursor-pointer"
                  >
                    <Wrench className="w-4 h-4 text-zinc-600" />
                    <div>
                      <div>Adicionar ferramenta cliente</div>
                      <div className="text-[10px] text-zinc-400 font-normal">
                        Executar ação no navegador ou SDK cliente
                      </div>
                    </div>
                  </button>

                  <button
                    type="button"
                    onClick={() => {
                      setAddMenuOpen(false)
                      onOpenNewToolModal('mcp')
                    }}
                    className="w-full px-3 py-2 rounded-xl hover:bg-zinc-50 flex items-center gap-2.5 text-left text-zinc-800 font-medium cursor-pointer"
                  >
                    <Blocks className="w-4 h-4 text-zinc-600" />
                    <div>
                      <div>Adicionar ferramenta de integração (MCP)</div>
                      <div className="text-[10px] text-zinc-400 font-normal">
                        Conectar servidor Model Context Protocol
                      </div>
                    </div>
                  </button>

                  <button
                    type="button"
                    onClick={() => {
                      setAddMenuOpen(false)
                      onOpenNewToolModal('code')
                    }}
                    className="w-full px-3 py-2 rounded-xl hover:bg-zinc-50 flex items-center gap-2.5 text-left text-zinc-800 font-medium cursor-pointer"
                  >
                    <Code2 className="w-4 h-4 text-zinc-600" />
                    <div>
                      <div>Adicionar ferramenta código</div>
                      <div className="text-[10px] text-zinc-400 font-normal">
                        Executar JavaScript customizado em sandbox
                      </div>
                    </div>
                  </button>

                  <button
                    type="button"
                    onClick={() => {
                      setAddMenuOpen(false)
                      openTransferAgentSettings()
                    }}
                    className="w-full px-3 py-2 rounded-xl hover:bg-zinc-50 flex items-center gap-2.5 text-left text-zinc-800 font-medium cursor-pointer"
                  >
                    <ArrowRightLeft className="w-4 h-4 text-zinc-600" />
                    <div>
                      <div>Transferir para agente de IA (transfer_to_agent)</div>
                      <div className="text-[10px] text-zinc-400 font-normal">
                        Encaminhar conversa para outro agente por condição
                      </div>
                    </div>
                  </button>
                </div>

                {/* Vincular ferramenta existente do workspace */}
                <div className="border-t border-zinc-100 pt-2 space-y-1.5">
                  <div className="px-2 text-[10px] font-semibold uppercase tracking-wider text-zinc-400">
                    Vincular ferramenta existente do workspace
                  </div>
                  <div className="px-2">
                    <input
                      type="text"
                      value={existingSearch}
                      onChange={(e) => setExistingSearch(e.target.value)}
                      placeholder="Filtrar ferramentas do workspace..."
                      className="w-full px-2.5 py-1.5 bg-zinc-50 border border-zinc-200 rounded-lg text-xs focus:outline-none focus:border-zinc-400"
                    />
                  </div>
                  <div className="max-h-44 overflow-y-auto divide-y divide-zinc-50">
                    {unlinkedWorkspaceTools.length === 0 ? (
                      <div className="py-3 text-center text-[11px] text-zinc-400">
                        Nenhuma outra ferramenta disponível no workspace.
                      </div>
                    ) : (
                      unlinkedWorkspaceTools.map((ut) => (
                        <button
                          key={ut.id}
                          type="button"
                          onClick={() => {
                            onToggleTool(ut.id)
                            setAddMenuOpen(false)
                          }}
                          className="w-full px-2.5 py-2 hover:bg-zinc-50 flex items-center justify-between gap-2 text-left rounded-lg cursor-pointer"
                        >
                          <div className="min-w-0">
                            <div className="font-semibold text-zinc-800 truncate">{ut.name}</div>
                            <div className="text-[10px] text-zinc-400 truncate">
                              {ut.description || ut.config?.url || '—'}
                            </div>
                          </div>
                          <span className="px-2 py-0.5 rounded-md bg-zinc-900 text-white text-[10px] font-medium flex-shrink-0">
                            + Vincular
                          </span>
                        </button>
                      ))
                    )}
                  </div>
                </div>
                </motion.div>
              )}
            </AnimatePresence>
          </div>
        </div>

        {/* Busca + Filtros (+ Tipo, + Criador) */}
        <div className="space-y-2.5">
          <div className="relative">
            <Search className="w-4 h-4 text-zinc-400 absolute left-3.5 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              placeholder="Pesquisar ferramentas deste agente..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-full pl-10 pr-4 py-2 bg-white border border-zinc-200/90 rounded-xl text-xs text-zinc-900 placeholder-zinc-400 focus:outline-none focus:ring-1 focus:ring-zinc-900"
            />
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => {
                const order: ('all' | ToolCategory)[] = ['all', 'webhook', 'client', 'mcp', 'code']
                setKindFilter(order[(order.indexOf(kindFilter) + 1) % order.length])
              }}
              className={`px-2.5 py-1 rounded-lg border text-[11px] font-medium transition-colors cursor-pointer ${
                kindFilter !== 'all'
                  ? 'bg-zinc-900 text-white border-zinc-900'
                  : 'border-zinc-200 bg-white text-zinc-600 hover:bg-zinc-50'
              }`}
            >
              +{' '}
              {kindFilter === 'all'
                ? 'Tipo'
                : kindFilter === 'webhook'
                  ? 'Tipo: Webhook'
                  : kindFilter === 'client'
                    ? 'Tipo: Cliente'
                    : kindFilter === 'mcp'
                      ? 'Tipo: MCP'
                      : 'Tipo: Código'}
            </button>
            <span className="px-2.5 py-1 rounded-lg border border-zinc-200 bg-white text-[11px] font-medium text-zinc-600">
              + Criador
            </span>
          </div>
        </div>

        {/* Tabela de Ferramentas vinculadas a ESTE Agente */}
        <div className="bg-white border border-zinc-200/80 rounded-2xl overflow-hidden shadow-2xs">
          <table className="w-full text-left text-xs">
            <thead className="border-b border-zinc-100 text-[11px] text-zinc-400 font-medium">
              <tr>
                <th className="py-3 px-4">Nome</th>
                <th className="py-3 px-3">Descrição</th>
                <th className="py-3 px-3 text-center underline decoration-dotted underline-offset-4">
                  Taxa de sucesso
                </th>
                <th className="py-3 px-3 text-center underline decoration-dotted underline-offset-4">
                  Execuções
                </th>
                <th className="py-3 px-3 text-center">Disponível em</th>
                <th className="py-3 px-4 text-right" />
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-100">
              {filteredTools.length === 0 ? (
                <tr>
                  <td colSpan={6} className="py-12 text-center text-zinc-400">
                    Nenhuma ferramenta vinculada a este agente. Clique em{' '}
                    <strong className="text-zinc-700">"Adicionar ferramenta"</strong> acima para
                    criar ou vincular uma ferramenta existente.
                  </td>
                </tr>
              ) : (
                filteredTools.map((t, idx) => {
                  const isGlobal = enabledToolIds.includes(t.id)
                  const inProc = referencedByProceduresToolIds.includes(t.id)
                  const execs = t.total_executions ?? 0
                  const rate = execs > 0 ? Math.round(t.success_rate ?? 100) : 100

                  let rateBadge = 'bg-emerald-50 text-emerald-700 border-emerald-200'
                  if (rate < 75) {
                    rateBadge = 'bg-red-50 text-red-700 border-red-200'
                  } else if (rate < 95) {
                    rateBadge = 'bg-amber-50 text-amber-800 border-amber-200'
                  }

                  return (
                    <motion.tr
                      key={t.id}
                      initial={{ opacity: 0, y: 6 }}
                      animate={{ opacity: 1, y: 0 }}
                      transition={{ ...springStudio, delay: Math.min(idx, 12) * 0.022 }}
                      onClick={() => onEditTool && onEditTool(t)}
                      className="hover:bg-zinc-50/70 transition-colors cursor-pointer"
                    >
                      <td className="py-3.5 px-4">
                        <div className="flex items-start gap-2.5">
                          {getToolIcon(t)}
                          <div className="min-w-0 max-w-[190px]">
                            <div className="font-semibold text-zinc-900 truncate">{t.name}</div>
                            <div className="text-[11px] font-mono text-zinc-400 truncate">
                              {t.config?.url ||
                                (t.kind === 'client'
                                  ? 'Client Tool'
                                  : t.kind === 'code'
                                    ? 'JavaScript Sandbox'
                                    : '—')}
                            </div>
                          </div>
                        </div>
                      </td>

                      <td className="py-3.5 px-3 text-zinc-600 max-w-[180px]">
                        <span className="line-clamp-2">{t.description || '—'}</span>
                      </td>

                      <td className="py-3.5 px-3 text-center">
                        <span
                          className={`inline-flex items-center px-2 py-0.5 rounded-md text-[11px] font-semibold border ${rateBadge}`}
                        >
                          {rate}%
                        </span>
                      </td>

                      <td className="py-3.5 px-3 text-center font-medium text-zinc-800">
                        {execs}
                      </td>

                      <td
                        className="py-3.5 px-3 text-center"
                        onClick={(e) => e.stopPropagation()}
                      >
                        <div className="inline-flex items-center gap-1">
                          {isGlobal && (
                            <span className="inline-flex items-center px-2.5 py-1 rounded-lg text-[11px] font-semibold border bg-emerald-50 text-emerald-800 border-emerald-200">
                              Agente
                            </span>
                          )}
                          {inProc && (
                            <span className="inline-flex items-center px-2.5 py-1 rounded-lg text-[11px] font-semibold border bg-zinc-100 text-zinc-800 border-zinc-200">
                              Procedimento
                            </span>
                          )}
                        </div>
                      </td>

                      <td
                        className="py-3.5 px-4 text-right"
                        onClick={(e) => e.stopPropagation()}
                      >
                        {isGlobal && (
                          <button
                            type="button"
                            onClick={() => onToggleTool(t.id)}
                            className="p-1.5 text-zinc-400 hover:text-red-600 hover:bg-red-50 rounded-lg transition-colors cursor-pointer"
                            title="Desvincular ferramenta deste agente"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </button>
                        )}
                      </td>
                    </motion.tr>
                  )
                })
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Coluna Direita (4 colunas): Card "Ferramentas do sistema" */}
      <div className="lg:col-span-4 space-y-3">
        <div>
          <h3 className="text-xs font-semibold text-zinc-900 flex items-center gap-1.5">
            Ferramentas do sistema
            <BookOpen className="w-3.5 h-3.5 text-zinc-400" />
          </h3>
          <p className="text-[11px] text-zinc-500 mt-0.5">
            Permite que o agente execute ações integradas.
          </p>
        </div>

        <div className="bg-white border border-zinc-200/90 rounded-2xl overflow-hidden shadow-2xs divide-y divide-zinc-100 text-xs">
          <div className="px-4 py-2.5 bg-zinc-50/50 font-semibold text-zinc-700 text-[11px]">
            {activeSysCount} {activeSysCount === 1 ? 'ferramenta ativa' : 'ferramentas ativas'}
          </div>

          {SYSTEM_TOOLS_LIST.map((st) => {
            const isChecked = systemTools.includes(st.id)
            return (
              <div
                key={st.id}
                className="px-4 py-3 flex items-center justify-between hover:bg-zinc-50/60 transition-colors"
              >
                <div className="flex items-center gap-3 min-w-0">
                  <Toggle
                    checked={isChecked}
                    onChange={() => {
                      if (
                        st.id === 'transferir_agente' &&
                        !isChecked &&
                        configuredTransfersCount === 0
                      ) {
                        onToggleSystemTool(st.id)
                        openTransferAgentSettings()
                      } else if (st.id === 'guardiao_jev' && !isChecked) {
                        onToggleSystemTool(st.id)
                        openGuardianSettings()
                      } else {
                        onToggleSystemTool(st.id)
                      }
                    }}
                  />
                  <div className="min-w-0">
                    <button
                      type="button"
                      onClick={() => {
                        if (st.id === 'transferir_agente') {
                          openTransferAgentSettings()
                        } else if (st.id === 'guardiao_jev' || st.id === 'detectar_correio_voz') {
                          openGuardianSettings()
                        } else {
                          onToggleSystemTool(st.id)
                        }
                      }}
                      className={`font-medium text-left cursor-pointer ${
                        isChecked
                          ? 'text-zinc-900 underline decoration-dotted underline-offset-4'
                          : 'text-zinc-600'
                      }`}
                    >
                      {st.label}
                    </button>
                    {st.id === 'transferir_agente' && configuredTransfersCount > 0 && (
                      <div className="text-[10px] text-emerald-700 font-medium">
                        {configuredTransfersCount}{' '}
                        {configuredTransfersCount === 1
                          ? 'regra de transferência configurada'
                          : 'regras de transferência configuradas'}
                      </div>
                    )}
                    {st.id === 'guardiao_jev' && (
                      <div className="text-[10px] text-emerald-700 font-medium">
                        {systemToolsConfig?.guardian_jev?.model || 'typesafe/jev-router'} • Alucinação, Mudo e Caixa Postal
                      </div>
                    )}
                  </div>
                </div>

                {st.hasSettings && (
                  <button
                    type="button"
                    onClick={() => {
                      if (st.id === 'transferir_agente') {
                        openTransferAgentSettings()
                      } else if (st.id === 'guardiao_jev' || st.id === 'detectar_correio_voz') {
                        openGuardianSettings()
                      }
                    }}
                    title={
                      st.id === 'transferir_agente'
                        ? 'Configurar regras de transferência entre agentes (transfer_to_agent)'
                        : st.id === 'guardiao_jev' || st.id === 'detectar_correio_voz'
                          ? 'Configurar Guardião Jev via OpenRouter'
                          : 'Configuração da ferramenta de sistema'
                    }
                    className="p-1.5 text-zinc-400 hover:text-zinc-800 hover:bg-zinc-100 rounded-lg transition-colors cursor-pointer"
                  >
                    <Settings className="w-3.5 h-3.5" />
                  </button>
                )}
              </div>
            )
          })}
        </div>
      </div>

      {/* =====================================================================
          GAVETA DE CONFIGURAÇÃO: TRANSFERÊNCIA DE AGENTE (transfer_to_agent)
         ===================================================================== */}
      <Sheet
        open={isTransferDrawerOpen}
        onClose={() => setIsTransferDrawerOpen(false)}
        side="right"
        sizeClass="max-w-2xl"
      >
        <div className="h-full flex flex-col overflow-hidden bg-white text-zinc-900 font-sans">
          {/* Cabeçalho */}
          <div className="px-6 py-4 flex items-center justify-between border-b border-zinc-100 flex-shrink-0">
            <div className="flex items-center gap-3">
              <div className="w-8 h-8 rounded-lg border border-zinc-200/90 bg-white flex items-center justify-center shadow-2xs text-zinc-800">
                <ArrowRightLeft className="w-4 h-4" />
              </div>
              <div className="flex items-center gap-2">
                <h2 className="text-[15px] font-semibold text-zinc-950 tracking-tight">
                  Transferência de agente (transfer_to_agent)
                </h2>
                <a
                  href="https://elevenlabs.io/docs/eleven-agents/customization/tools/system-tools/agent-transfer"
                  target="_blank"
                  rel="noreferrer"
                  title="Documentação oficial de Transferência de Agente"
                  className="text-zinc-400 hover:text-zinc-700"
                >
                  <BookOpen className="w-4 h-4" />
                </a>
              </div>
            </div>

            <button
              type="button"
              onClick={() => setIsTransferDrawerOpen(false)}
              className="p-1.5 rounded-lg text-zinc-400 hover:text-zinc-800 hover:bg-zinc-100 cursor-pointer"
            >
              <X className="w-4 h-4" />
            </button>
          </div>

          {/* Corpo rolável */}
          <div className="flex-1 overflow-y-auto overscroll-contain px-6 py-5 space-y-4">
            {isTransferJsonMode ? (
              <div className="space-y-3 h-full flex flex-col">
                <div>
                  <h3 className="text-xs font-semibold text-zinc-900">
                    built_in_tools.transfer_to_agent (JSON)
                  </h3>
                  <p className="text-[11px] text-zinc-500">
                    Estrutura JSON compatível com a API e SDK da ElevenLabs.
                  </p>
                </div>
                {transferJsonError && (
                  <div className="p-2.5 rounded-lg bg-red-50 border border-red-200 text-xs text-red-700">
                    {transferJsonError}
                  </div>
                )}
                <textarea
                  rows={22}
                  spellCheck={false}
                  value={transferJsonText}
                  onChange={(e) => {
                    setTransferJsonText(e.target.value)
                    setTransferJsonError('')
                  }}
                  className="w-full flex-1 p-4 rounded-2xl bg-zinc-950 text-zinc-100 font-mono text-xs leading-relaxed focus:outline-none border border-zinc-800"
                />
              </div>
            ) : (
              <>
                {/* Passo 1: Descrição da ferramenta (opcional) */}
                <div className="bg-[#f7f7f8] border border-zinc-200/70 rounded-2xl p-4 space-y-3">
                  <div>
                    <h3 className="text-[13px] font-semibold text-zinc-900">
                      Descrição da ferramenta (opcional)
                    </h3>
                    <p className="text-xs text-zinc-500 mt-0.5">
                      Forneça uma descrição personalizada para orientar o LLM sobre quando acionar
                      uma transferência. Se for deixado em branco, será usada uma descrição padrão
                      que abrange as regras de transferência definidas.
                    </p>
                  </div>

                  <div className="bg-white border border-zinc-200/80 rounded-xl p-3.5 shadow-2xs">
                    <textarea
                      rows={2}
                      value={transferDescription}
                      onChange={(e) => setTransferDescription(e.target.value)}
                      placeholder="Transfira o usuário para um agente especializado com base na solicitação dele."
                      className="w-full text-xs text-zinc-900 placeholder-zinc-400 focus:outline-none resize-y"
                    />
                  </div>
                </div>

                {/* Passo 2: Regras de transferência */}
                <div className="bg-[#f7f7f8] border border-zinc-200/70 rounded-2xl p-4 space-y-3">
                  <div className="flex items-center justify-between gap-4">
                    <div>
                      <h3 className="text-[13px] font-semibold text-zinc-900">
                        Regras de transferência
                      </h3>
                      <p className="text-xs text-zinc-500 mt-0.5">
                        Mapeie condições para agentes de destino. Os agentes são referenciados por{' '}
                        <code>agent_number</code> indexado a partir de zero (0, 1, 2...).
                      </p>
                    </div>
                  </div>

                  {transferRules.length === 0 ? (
                    <div className="bg-white border border-zinc-200/80 rounded-xl p-6 text-center text-xs text-zinc-400">
                      Nenhuma regra de transferência adicionada. Clique em "+ Adicionar regra de
                      transferência" abaixo.
                    </div>
                  ) : (
                    <div className="space-y-3">
                      {transferRules.map((rule, idx) => (
                        <div
                          key={idx}
                          className="bg-white border border-zinc-200/90 rounded-xl p-4 space-y-3.5 shadow-2xs"
                        >
                          <div className="flex items-center justify-between border-b border-zinc-100 pb-2.5">
                            <div className="flex items-center gap-2">
                              <span className="px-2 py-0.5 rounded-md bg-zinc-900 text-white font-mono text-[10px] font-semibold">
                                agent_number: {idx}
                              </span>
                              <span className="text-xs font-semibold text-zinc-800">
                                Regra de transferência #{idx + 1}
                              </span>
                            </div>
                            <button
                              type="button"
                              onClick={() =>
                                setTransferRules(transferRules.filter((_, i) => i !== idx))
                              }
                              className="p-1 text-zinc-400 hover:text-red-600 rounded-lg cursor-pointer"
                              title="Remover regra"
                            >
                              <Trash2 className="w-3.5 h-3.5" />
                            </button>
                          </div>

                          {/* Agente de destino */}
                          <div>
                            <label className="block text-xs font-semibold text-zinc-900 mb-1">
                              Agente de destino
                            </label>
                            <select
                              value={rule.agent_id}
                              onChange={(e) => {
                                const next = [...transferRules]
                                const chosen = workspaceAgents.find((a) => a.id === e.target.value)
                                next[idx] = {
                                  ...rule,
                                  agent_id: e.target.value,
                                  agent_name: chosen?.name,
                                }
                                setTransferRules(next)
                              }}
                              className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-xs text-zinc-900 focus:outline-none focus:border-zinc-400"
                            >
                              <option value="">Selecione um agente...</option>
                              {(otherAgents.length > 0 ? otherAgents : workspaceAgents).map(
                                (ag) => (
                                  <option key={ag.id} value={ag.id}>
                                    {ag.name} ({ag.provider || 'gemini_live'})
                                  </option>
                                ),
                              )}
                            </select>
                          </div>

                          {/* Condição */}
                          <div>
                            <label className="block text-xs font-semibold text-zinc-900 mb-1">
                              Condição
                            </label>
                            <textarea
                              rows={2}
                              value={rule.condition}
                              onChange={(e) => {
                                const next = [...transferRules]
                                next[idx] = { ...rule, condition: e.target.value }
                                setTransferRules(next)
                              }}
                              placeholder="Ex: Quando o usuário perguntar sobre detalhes de cobrança ou pedir suporte técnico para o produto X."
                              className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-xs text-zinc-900 focus:outline-none focus:border-zinc-400 resize-y"
                            />
                          </div>

                          {/* Atraso antes da transferência (delay_ms) + Mensagem de transferência */}
                          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                            <div>
                              <label className="block text-xs font-semibold text-zinc-900 mb-1">
                                Atraso antes da transferência (ms)
                              </label>
                              <input
                                type="number"
                                min={0}
                                max={15000}
                                step={100}
                                value={rule.delay_ms ?? 0}
                                onChange={(e) => {
                                  const next = [...transferRules]
                                  next[idx] = {
                                    ...rule,
                                    delay_ms: Math.max(0, parseInt(e.target.value) || 0),
                                  }
                                  setTransferRules(next)
                                }}
                                className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-xs font-mono text-zinc-900"
                              />
                              <span className="text-[10px] text-zinc-400 block mt-0.5">
                                Padrão: 0 (imediata)
                              </span>
                            </div>

                            <div className="sm:col-span-2">
                              <label className="block text-xs font-semibold text-zinc-900 mb-1">
                                Mensagem de transferência (opcional)
                              </label>
                              <input
                                type="text"
                                value={rule.transfer_message || ''}
                                onChange={(e) => {
                                  const next = [...transferRules]
                                  next[idx] = { ...rule, transfer_message: e.target.value }
                                  setTransferRules(next)
                                }}
                                placeholder="Ex: Vou conectar você com nosso especialista em cobrança."
                                className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-xs text-zinc-900"
                              />
                              <span className="text-[10px] text-zinc-400 block mt-0.5">
                                Em branco = transferência silenciosa
                              </span>
                            </div>
                          </div>

                          {/* Ativar primeira mensagem (enable_transferred_agent_first_message) */}
                          <div className="pt-1 border-t border-zinc-100 flex items-center justify-between gap-4">
                            <div>
                              <span className="text-xs font-semibold text-zinc-900 block">
                                Ativar primeira mensagem
                              </span>
                              <span className="text-[11px] text-zinc-500">
                                Define se o agente transferido deve reproduzir sua primeira mensagem
                                após a transferência.
                              </span>
                            </div>
                            <Toggle
                              checked={Boolean(rule.enable_transferred_agent_first_message)}
                              onChange={(val) => {
                                const next = [...transferRules]
                                next[idx] = {
                                  ...rule,
                                  enable_transferred_agent_first_message: val,
                                }
                                setTransferRules(next)
                              }}
                            />
                          </div>
                        </div>
                      ))}
                    </div>
                  )}

                  <button
                    type="button"
                    onClick={() =>
                      setTransferRules([
                        ...transferRules,
                        {
                          agent_id: otherAgents[0]?.id || workspaceAgents[0]?.id || '',
                          agent_name: otherAgents[0]?.name || workspaceAgents[0]?.name,
                          condition: '',
                          delay_ms: 0,
                          transfer_message: '',
                          enable_transferred_agent_first_message: false,
                        },
                      ])
                    }
                    className="w-full py-2 px-3 rounded-xl bg-white hover:bg-zinc-50 border border-zinc-200/90 text-xs font-semibold text-zinc-900 inline-flex items-center justify-center gap-1.5 shadow-2xs transition-colors cursor-pointer"
                  >
                    <Plus className="w-3.5 h-3.5" />
                    Adicionar regra de transferência
                  </button>
                </div>
              </>
            )}
          </div>

          {/* Rodapé */}
          <div className="px-6 py-3.5 border-t border-zinc-200/80 bg-white flex items-center justify-between flex-shrink-0">
            <button
              type="button"
              onClick={() => {
                if (!isTransferJsonMode) {
                  setTransferJsonText(JSON.stringify(buildTransferJsonObject(), null, 2))
                  setTransferJsonError('')
                  setIsTransferJsonMode(true)
                } else {
                  if (applyTransferJsonToState(transferJsonText)) {
                    setIsTransferJsonMode(false)
                  }
                }
              }}
              className="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg hover:bg-zinc-100 text-xs font-semibold text-zinc-800 transition-colors cursor-pointer"
            >
              <Code2 className="w-3.5 h-3.5 text-zinc-600" />
              <span>{isTransferJsonMode ? 'Formulário visual' : 'Editar como JSON'}</span>
            </button>

            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={() => setIsTransferDrawerOpen(false)}
                className="px-4 py-2 rounded-xl border border-zinc-200/90 bg-white hover:bg-zinc-50 text-xs font-semibold text-zinc-800 cursor-pointer"
              >
                Cancelar
              </button>
              <button
                type="button"
                onClick={handleSaveTransferConfig}
                className="px-4 py-2 rounded-xl bg-zinc-950 hover:bg-zinc-800 text-white text-xs font-semibold cursor-pointer"
              >
                Salvar regras de transferência
              </button>
            </div>
          </div>
        </div>
      </Sheet>

      {/* =====================================================================
          GAVETA DE CONFIGURAÇÃO: GUARDIÃO JEV (OpenRouter `typesafe/jev-router`)
         ===================================================================== */}
      <Sheet
        open={isGuardianDrawerOpen}
        onClose={() => setIsGuardianDrawerOpen(false)}
        side="right"
        sizeClass="max-w-xl"
      >
        <div className="h-full flex flex-col overflow-hidden bg-white text-zinc-900 font-sans">
          <div className="px-6 py-4 flex items-center justify-between border-b border-zinc-100 flex-shrink-0">
            <div className="flex items-center gap-3">
              <div className="w-8 h-8 rounded-lg border border-zinc-200/90 bg-white flex items-center justify-center shadow-2xs text-emerald-700">
                <ShieldCheck className="w-4 h-4" />
              </div>
              <div>
                <h2 className="text-[15px] font-semibold text-zinc-950 tracking-tight">
                  Guardião Jev (OpenRouter)
                </h2>
                <p className="text-[11px] text-zinc-500">
                  Monitoramento System One em tempo real da integridade da conversa
                </p>
              </div>
            </div>
            <button
              type="button"
              onClick={() => setIsGuardianDrawerOpen(false)}
              className="p-1.5 rounded-lg text-zinc-400 hover:text-zinc-800 hover:bg-zinc-100 cursor-pointer"
            >
              <X className="w-4 h-4" />
            </button>
          </div>

          <div className="flex-1 overflow-y-auto overscroll-contain px-6 py-5 space-y-4">
            {/* Provedor + Modelo + API Key */}
            <div className="bg-[#f7f7f8] border border-zinc-200/70 rounded-2xl p-4 space-y-3">
              <div>
                <h3 className="text-[13px] font-semibold text-zinc-900">
                  Conexão OpenRouter (`typesafe/jev-router`)
                </h3>
                <p className="text-xs text-zinc-500 mt-0.5">
                  Usa o modelo Jev na OpenRouter para avaliar cada turno em paralelo sem adicionar latência no áudio.
                </p>
              </div>

              <div className="bg-white border border-zinc-200/80 rounded-xl p-3.5 space-y-3 shadow-2xs">
                <div>
                  <label className="block text-xs font-semibold text-zinc-900 mb-1">
                    Modelo OpenRouter
                  </label>
                  <input
                    type="text"
                    value={guardianForm.model || 'typesafe/jev-router'}
                    onChange={(e) => setGuardianForm({ ...guardianForm, model: e.target.value })}
                    placeholder="typesafe/jev-router"
                    className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-xs font-mono text-zinc-900 focus:outline-none focus:border-zinc-400"
                  />
                </div>

                <div>
                  <label className="block text-xs font-semibold text-zinc-900 mb-1">
                    OpenRouter API Key (opcional se já definida em OPENROUTER_API_KEY no .env)
                  </label>
                  <input
                    type="password"
                    value={guardianForm.api_key || ''}
                    onChange={(e) => setGuardianForm({ ...guardianForm, api_key: e.target.value })}
                    placeholder="sk-or-v1-..."
                    className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-xs font-mono text-zinc-900 focus:outline-none focus:border-zinc-400"
                  />
                  <span className="text-[10px] text-zinc-400 block mt-1">
                    Se preenchida aqui, sobrescreve a variável <code>OPENROUTER_API_KEY</code> do servidor para este agente.
                  </span>
                </div>
              </div>
            </div>

            {/* Verificadores Atômicos (Noul + Score) */}
            <div className="bg-[#f7f7f8] border border-zinc-200/70 rounded-2xl p-4 space-y-3">
              <div>
                <h3 className="text-[13px] font-semibold text-zinc-900">
                  Proteções ativas em tempo real
                </h3>
                <p className="text-xs text-zinc-500 mt-0.5">
                  Todas as 3 perguntas são avaliadas na mesma requisição a cada turno.
                </p>
              </div>

              <div className="space-y-2.5">
                {/* 1. Caixa Postal */}
                <div className="bg-white border border-zinc-200/90 rounded-xl p-3.5 space-y-2.5 shadow-2xs">
                  <div className="flex items-center justify-between gap-3">
                    <div>
                      <div className="text-xs font-semibold text-zinc-900">
                        1. Detecção de Caixa Postal (`is_voicemail`)
                      </div>
                      <div className="text-[11px] text-zinc-500">
                        Encerra a chamada na hora assim que identifica secretária eletrônica, bip ou mensagem automática da operadora.
                      </div>
                    </div>
                    <Toggle
                      checked={guardianForm.check_voicemail}
                      onChange={(val) => setGuardianForm({ ...guardianForm, check_voicemail: val })}
                    />
                  </div>
                  <div className="flex items-center justify-between pt-2 border-t border-zinc-100 text-[11px]">
                    <span className="text-zinc-600">Limiar de certeza (Noul 0–1):</span>
                    <input
                      type="number"
                      min={0.5}
                      max={0.99}
                      step={0.05}
                      value={guardianForm.voicemail_threshold ?? 0.8}
                      onChange={(e) =>
                        setGuardianForm({
                          ...guardianForm,
                          voicemail_threshold: parseFloat(e.target.value) || 0.8,
                        })
                      }
                      className="w-20 px-2 py-1 border border-zinc-200 rounded-md font-mono text-right text-zinc-900"
                    />
                  </div>
                </div>

                {/* 2. Cliente Mudo / Falando Sozinho */}
                <div className="bg-white border border-zinc-200/90 rounded-xl p-3.5 space-y-2.5 shadow-2xs">
                  <div className="flex items-center justify-between gap-3">
                    <div>
                      <div className="text-xs font-semibold text-zinc-900">
                        2. Cliente Mudo / Agente Falando Sozinho (`talking_to_self`)
                      </div>
                      <div className="text-[11px] text-zinc-500">
                        No 1º alerta pergunta se o cliente ainda está na linha; se continuar mudo no turno seguinte, encerra a chamada automaticamente.
                      </div>
                    </div>
                    <Toggle
                      checked={guardianForm.check_talking_to_self}
                      onChange={(val) =>
                        setGuardianForm({ ...guardianForm, check_talking_to_self: val })
                      }
                    />
                  </div>
                  <div className="flex items-center justify-between pt-2 border-t border-zinc-100 text-[11px]">
                    <span className="text-zinc-600">Limiar de certeza (Noul 0–1):</span>
                    <input
                      type="number"
                      min={0.5}
                      max={0.99}
                      step={0.05}
                      value={guardianForm.talking_to_self_threshold ?? 0.78}
                      onChange={(e) =>
                        setGuardianForm({
                          ...guardianForm,
                          talking_to_self_threshold: parseFloat(e.target.value) || 0.78,
                        })
                      }
                      className="w-20 px-2 py-1 border border-zinc-200 rounded-md font-mono text-right text-zinc-900"
                    />
                  </div>
                </div>

                {/* 3. Alucinação do Agente */}
                <div className="bg-white border border-zinc-200/90 rounded-xl p-3.5 space-y-2.5 shadow-2xs">
                  <div className="flex items-center justify-between gap-3">
                    <div>
                      <div className="text-xs font-semibold text-zinc-900">
                        3. Agente Alucinando (`agent_hallucinating`)
                      </div>
                      <div className="text-[11px] text-zinc-500">
                        Injeta correção silenciosa imediata se o agente inventar dados fora das variáveis ou sair do contexto, e registra alerta na transcrição.
                      </div>
                    </div>
                    <Toggle
                      checked={guardianForm.check_hallucination}
                      onChange={(val) =>
                        setGuardianForm({ ...guardianForm, check_hallucination: val })
                      }
                    />
                  </div>
                  <div className="flex items-center justify-between pt-2 border-t border-zinc-100 text-[11px]">
                    <span className="text-zinc-600">Limiar de certeza (Noul 0–1):</span>
                    <input
                      type="number"
                      min={0.5}
                      max={0.99}
                      step={0.05}
                      value={guardianForm.hallucination_threshold ?? 0.75}
                      onChange={(e) =>
                        setGuardianForm({
                          ...guardianForm,
                          hallucination_threshold: parseFloat(e.target.value) || 0.75,
                        })
                      }
                      className="w-20 px-2 py-1 border border-zinc-200 rounded-md font-mono text-right text-zinc-900"
                    />
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div className="px-6 py-3.5 border-t border-zinc-200/80 bg-white flex items-center justify-end gap-2 flex-shrink-0">
            <button
              type="button"
              onClick={() => setIsGuardianDrawerOpen(false)}
              className="px-4 py-2 rounded-xl border border-zinc-200/90 bg-white hover:bg-zinc-50 text-xs font-semibold text-zinc-800 cursor-pointer"
            >
              Cancelar
            </button>
            <button
              type="button"
              onClick={handleSaveGuardianConfig}
              className="px-4 py-2 rounded-xl bg-zinc-950 hover:bg-zinc-800 text-white text-xs font-semibold cursor-pointer"
            >
              Salvar Guardião Jev
            </button>
          </div>
        </div>
      </Sheet>
    </div>
  )
}
