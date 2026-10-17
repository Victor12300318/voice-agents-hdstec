import { useEffect, useMemo, useRef, useState } from 'react'
import {
  Search,
  Plus,
  ArrowUpDown,
  PanelLeft,
  Undo2,
  Redo2,
  Copy,
  BookOpen,
  Shield,
  Link2,
  Inbox,
  Zap,
  Heading1,
  Heading2,
  Heading3,
  ListOrdered,
  Wrench,
  Settings,
  FolderOpen,
  ClipboardList,
  ChevronLeft,
  MoreHorizontal,
  Check,
  AlertTriangle,
  FileUp,
  Sparkles,
  FileCode2,
  X,
} from 'lucide-react'
import {
  type Skill,
  type Tool,
  type KnowledgeBase,
  type Agent,
} from '../api'
import Toggle from './Toggle'
import ProcedureEditorModal from './ProcedureEditorModal'

interface NotionProcedureWorkspaceProps {
  agents: Agent[]
  selectedAgentId: string
  onSelectAgent?: (agentId: string) => void
  lockAgent?: boolean
  procedures: Skill[]
  tools: Tool[]
  knowledgeBases: KnowledgeBase[]
  onSaveProcedure: (payload: {
    id?: string
    agent_id: string
    name: string
    type: 'free_form' | 'deterministic'
    description: string
    trigger: string
    guardrails: string[]
    content: string
    active: boolean
  }) => Promise<void>
  onDeleteProcedure: (proc: Skill) => Promise<void>
  onOpenImportSOP?: () => void
}

type SlashView = 'root' | 'tools' | 'system_tools' | 'kb' | 'procedures'

const SYSTEM_TOOLS_CATALOG = [
  {
    id: 'transferir_numero',
    label: 'Transferir para número',
    sub: 'sys_tool_transferir_numero_sip',
  },
  {
    id: 'end_call',
    label: 'Encerrar conversa',
    sub: 'sys_tool_encerrar_conversa_padrao',
  },
]

export default function NotionProcedureWorkspace({
  agents,
  selectedAgentId,
  onSelectAgent,
  lockAgent = false,
  procedures,
  tools,
  knowledgeBases,
  onSaveProcedure,
  onDeleteProcedure,
  onOpenImportSOP,
}: NotionProcedureWorkspaceProps) {
  const [sidebarOpen, setSidebarOpen] = useState(true)
  const [searchQuery, setSearchQuery] = useState('')
  const [sortAsc, setSortAsc] = useState(true)
  const [activeProcId, setActiveProcId] = useState<string | null>(null)

  // Estado editável do documento aberto (estilo Notion da ElevenLabs)
  const [title, setTitle] = useState('')
  const [guardrails, setGuardrails] = useState<string[]>([])
  const [addingGuardrail, setAddingGuardrail] = useState(false)
  const [newGuardrail, setNewGuardrail] = useState('')
  const [triggerEnabled, setTriggerEnabled] = useState(true)
  const [triggerText, setTriggerText] = useState('')
  const [content, setContent] = useState('')
  const [history, setHistory] = useState<string[]>([])
  const [historyIdx, setHistoryIdx] = useState(-1)
  const [dirty, setDirty] = useState(false)
  const [saving, setSaving] = useState(false)
  const [savedToast, setSavedToast] = useState(false)

  //Modo Visual Rico (Renderizado com Pílulas) vs Edição no Bloco
  const [isEditingContent, setIsEditingContent] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement | null>(null)

  // Menu Slash (/) Multi-nível igual às Imagens 2 e 3 da ElevenLabs
  const [slashOpen, setSlashOpen] = useState(false)
  const [slashView, setSlashView] = useState<SlashView>('root')
  const [slashSearch, setSlashSearch] = useState('')
  const [slashPosition, setSlashPosition] = useState<{ top: number; left: number }>({
    top: 120,
    left: 160,
  })
  const [slashCursorPos, setSlashCursorPos] = useState<number | null>(null)

  // Modal avançado para Structured Procedures
  // Modal avançado para Structured Procedures
  const [structuredModalOpen, setStructuredModalOpen] = useState(false)
  const [showNewMenu, setShowNewMenu] = useState(false)
  const [showAllUsed, setShowAllUsed] = useState(false)

  const agentProcedures = useMemo(() => {
    const list = procedures.filter((p) => !selectedAgentId || p.agent_id === selectedAgentId)
    const filtered = list.filter(
      (p) =>
        p.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        (p.trigger || '').toLowerCase().includes(searchQuery.toLowerCase()),
    )
    return [...filtered].sort((a, b) =>
      sortAsc ? a.name.localeCompare(b.name) : b.name.localeCompare(a.name),
    )
  }, [procedures, selectedAgentId, searchQuery, sortAsc])

  const activeProc = useMemo(
    () => procedures.find((p) => p.id === activeProcId) || null,
    [procedures, activeProcId],
  )

  // Seleciona automaticamente o primeiro procedure da lista quando disponível
  useEffect(() => {
    if (agentProcedures.length > 0 && (!activeProcId || !agentProcedures.some((p) => p.id === activeProcId))) {
      selectProcedure(agentProcedures[0])
    } else if (agentProcedures.length === 0) {
      setActiveProcId(null)
    }
  }, [agentProcedures])

  const selectProcedure = (p: Skill) => {
    setActiveProcId(p.id)
    setTitle(p.name)
    setGuardrails(p.guardrails || [])
    const hasTrig = Boolean(p.trigger && p.trigger.trim() !== '')
    setTriggerEnabled(hasTrig)
    setTriggerText(p.trigger || '')
    setContent(p.content || '')
    setHistory([p.content || ''])
    setHistoryIdx(0)
    setDirty(false)
    setSlashOpen(false)
    setIsEditingContent(false)
  }

  const pushContentHistory = (nextContent: string) => {
    setContent(nextContent)
    setDirty(true)
    const sliced = history.slice(0, historyIdx + 1)
    setHistory([...sliced, nextContent])
    setHistoryIdx(sliced.length)
  }

  const handleUndo = () => {
    if (historyIdx > 0) {
      const nextIdx = historyIdx - 1
      setHistoryIdx(nextIdx)
      setContent(history[nextIdx])
      setDirty(true)
    }
  }

  const handleRedo = () => {
    if (historyIdx < history.length - 1) {
      const nextIdx = historyIdx + 1
      setHistoryIdx(nextIdx)
      setContent(history[nextIdx])
      setDirty(true)
    }
  }

  // Calcula "Usado por" (quais outros procedures referenciam este procedure)
  const usedByProcedures = useMemo(() => {
    if (!activeProc) return []
    return procedures.filter(
      (other) =>
        other.id !== activeProc.id &&
        (other.referenced_procedure_ids?.includes(activeProc.id) ||
          other.content?.includes(activeProc.id) ||
          other.content?.includes(activeProc.name)),
    )
  }, [activeProc, procedures])

  // Calcula "Usa" (todas as Tools, System Tools, KBs e Sub-procedures referenciados no texto)
  const usedResources = useMemo(() => {
    const items: {
      key: string
      kind: 'procedure' | 'system_tool' | 'tool' | 'kb'
      label: string
      valid: boolean
    }[] = []
    const combined = `${triggerEnabled ? triggerText : ''}\n${content}`
    const regexes = [
      /\[(tool|kb|knowledge|procedure|system_tool)\s+id="([^"]+)"\]/g,
      /@\[(tool|procedure|knowledge|system_tool):([^\]]+)\]/g,
    ]
    const seen = new Set<string>()

    for (const re of regexes) {
      let m: RegExpExecArray | null
      while ((m = re.exec(combined)) !== null) {
        const rawKind = m[1] === 'kb' ? 'knowledge' : m[1]
        const id = m[2]
        const uKey = `${rawKind}:${id}`
        if (seen.has(uKey)) continue
        seen.add(uKey)

        if (rawKind === 'procedure') {
          const found = procedures.find((p) => p.id === id || p.name === id)
          items.push({
            key: uKey,
            kind: 'procedure',
            label: found ? found.name : id,
            valid: !!found,
          })
        } else if (rawKind === 'system_tool') {
          const sys = SYSTEM_TOOLS_CATALOG.find((s) => s.id === id)
          items.push({
            key: uKey,
            kind: 'system_tool',
            label: sys ? sys.label : id === 'end_call' ? 'Encerrar conversa' : 'Transferir para número',
            valid: true,
          })
        } else if (rawKind === 'tool') {
          const found = tools.find((t) => t.id === id || t.name === id)
          items.push({
            key: uKey,
            kind: 'tool',
            label: found ? found.name : id,
            valid: !!found,
          })
        } else if (rawKind === 'knowledge') {
          const found = knowledgeBases.find((k) => k.id === id || k.name === id)
          items.push({
            key: uKey,
            kind: 'kb',
            label: found ? found.name : id,
            valid: !!found,
          })
        }
      }
    }
    return items
  }, [content, triggerText, triggerEnabled, procedures, tools, knowledgeBases])

  const handleSave = async () => {
    if (!activeProc) return
    setSaving(true)
    try {
      await onSaveProcedure({
        id: activeProc.id,
        agent_id: activeProc.agent_id || selectedAgentId || agents[0]?.id || '',
        name: title.trim() || 'Procedimento sem título',
        type: activeProc.type || 'free_form',
        description: activeProc.description || '',
        trigger: triggerEnabled ? triggerText.trim() : '',
        guardrails,
        content,
        active: activeProc.active,
      })
      setDirty(false)
      setSavedToast(true)
      setTimeout(() => setSavedToast(false), 2000)
    } finally {
      setSaving(false)
    }
  }

  const handleCreateNewFreeForm = async () => {
    setShowNewMenu(false)
    const targetAg = selectedAgentId || agents[0]?.id || ''
    if (!targetAg) return
    await onSaveProcedure({
      agent_id: targetAg,
      name: 'Novo Procedimento',
      type: 'free_form',
      description: '',
      trigger: 'Quando o cliente solicitar atendimento sobre este assunto.',
      guardrails: [],
      content:
        '1. **Identificação e Contexto:**\n   * Confirme a solicitação com o cliente de forma cordial.\n   * Digite `/` a qualquer momento para inserir uma ferramenta ou sub-procedimento.',
      active: true,
    })
  }

  // Insere comando ou referência na posição exata onde o usuário digitou '/'
  const insertSlashCommand = (snippet: string) => {
    if (slashCursorPos !== null && content[slashCursorPos - 1] === '/') {
      const before = content.slice(0, slashCursorPos - 1)
      const after = content.slice(slashCursorPos)
      const updated = `${before}${snippet}${after}`
      pushContentHistory(updated)
    } else {
      const updated = `${content}${content.endsWith(' ') || content.endsWith('\n') || !content ? '' : ' '}${snippet}`
      pushContentHistory(updated)
    }
    setSlashOpen(false)
    setSlashView('root')
    setSlashSearch('')
    setIsEditingContent(true)
    setTimeout(() => textareaRef.current?.focus(), 20)
  }

  // Renderiza uma linha do documento convertendo tokens [tool id="..."], [procedure id="..."], [system_tool id="..."], [kb id="..."]
  // em Pílulas Visuais Atômicas idênticas à Imagem 1 da ElevenLabs!
  const renderRichLineWithPills = (line: string) => {
    const tokenRegex =
      /(\[(?:tool|kb|knowledge|procedure|system_tool)\s+id="[^"]+"\]|@\[(?:tool|kb|knowledge|procedure|system_tool):[^\]]+\]|\*\*[^*]+\*\*|\*[^*]+\*)/g
    const parts = line.split(tokenRegex)

    return parts.map((part, idx) => {
      if (!part) return null

      // Negrito **...**
      if (part.startsWith('**') && part.endsWith('**') && part.length > 4) {
        return (
          <strong key={idx} className="font-semibold text-zinc-900">
            {part.slice(2, -2)}
          </strong>
        )
      }
      // Itálico *...*
      if (part.startsWith('*') && part.endsWith('*') && part.length > 2) {
        return (
          <em key={idx} className="italic text-zinc-700">
            {part.slice(1, -1)}
          </em>
        )
      }

      // Token de Referência Inline
      const m1 = part.match(/^\[(tool|kb|knowledge|procedure|system_tool)\s+id="([^"]+)"\]$/)
      const m2 = part.match(/^@\[(tool|kb|knowledge|procedure|system_tool):([^\]]+)\]$/)
      const match = m1 || m2
      if (match) {
        const rawKind = match[1] === 'kb' ? 'knowledge' : match[1]
        const id = match[2]

        if (rawKind === 'system_tool') {
          const sys = SYSTEM_TOOLS_CATALOG.find((s) => s.id === id)
          const label = sys
            ? sys.label
            : id === 'end_call'
              ? 'Encerrar conversa'
              : 'Transferir para número'
          return (
            <span
              key={idx}
              className="inline-flex items-center gap-1.5 px-2.5 py-0.5 mx-1 rounded-full text-xs font-medium bg-zinc-100 text-zinc-800 border border-zinc-200/90 shadow-2xs align-baseline select-none"
            >
              <Settings className="w-3 h-3 text-zinc-600" />
              {label}
            </span>
          )
        }

        if (rawKind === 'procedure') {
          const found = procedures.find((p) => p.id === id || p.name === id)
          return (
            <span
              key={idx}
              onClick={(e) => {
                e.stopPropagation()
                if (found) selectProcedure(found)
              }}
              className={`inline-flex items-center gap-1.5 px-2.5 py-0.5 mx-1 rounded-full text-xs font-medium border shadow-2xs align-baseline cursor-pointer ${
                found
                  ? 'bg-zinc-100 hover:bg-zinc-200/80 text-zinc-800 border-zinc-200/90'
                  : 'bg-red-50 text-red-700 border-red-200'
              }`}
            >
              <ClipboardList className="w-3 h-3 text-zinc-600" />
              {found ? found.name : id}
            </span>
          )
        }

        if (rawKind === 'tool') {
          const found = tools.find((t) => t.id === id || t.name === id)
          return (
            <span
              key={idx}
              className={`inline-flex items-center gap-1.5 px-2.5 py-0.5 mx-1 rounded-full text-xs font-medium border shadow-2xs align-baseline select-none ${
                found
                  ? 'bg-zinc-100 text-zinc-800 border-zinc-200/90'
                  : 'bg-red-50 text-red-700 border-red-200'
              }`}
            >
              <Wrench className="w-3 h-3 text-zinc-600" />
              {found ? found.name : id}
            </span>
          )
        }

        if (rawKind === 'knowledge') {
          const found = knowledgeBases.find((k) => k.id === id || k.name === id)
          return (
            <span
              key={idx}
              className={`inline-flex items-center gap-1.5 px-2.5 py-0.5 mx-1 rounded-full text-xs font-medium border shadow-2xs align-baseline select-none ${
                found
                  ? 'bg-zinc-100 text-zinc-800 border-zinc-200/90'
                  : 'bg-red-50 text-red-700 border-red-200'
              }`}
            >
              <FolderOpen className="w-3 h-3 text-zinc-600" />
              {found ? found.name : id}
            </span>
          )
        }
      }

      return <span key={idx}>{part}</span>
    })
  }

  return (
    <div className="bg-white border border-zinc-200/90 rounded-2xl shadow-xs overflow-hidden flex min-h-[720px] relative">
      {/* COLUNA ESQUERDA: Lista de Procedimentos (Idêntica à Imagem 1) */}
      {sidebarOpen && (
        <aside className="w-64 sm:w-72 border-r border-zinc-200/80 bg-zinc-50/40 flex flex-col flex-shrink-0">
          {/* Topo da Sidebar */}
          <div className="px-4 py-3.5 border-b border-zinc-200/70 flex items-center justify-between">
            <span className="font-semibold text-xs text-zinc-900 tracking-tight">Procedimentos</span>
            <div className="flex items-center gap-1 relative">
              <button
                type="button"
                onClick={() => setSortAsc(!sortAsc)}
                title="Ordenar A-Z / Z-A"
                className="p-1.5 text-zinc-500 hover:text-zinc-900 hover:bg-zinc-200/60 rounded-lg transition-colors"
              >
                <ArrowUpDown className="w-3.5 h-3.5" />
              </button>
              <button
                type="button"
                onClick={() => setShowNewMenu(!showNewMenu)}
                title="Criar Procedimento"
                className="p-1.5 text-zinc-600 hover:text-zinc-900 hover:bg-zinc-200/60 rounded-lg transition-colors"
              >
                <Plus className="w-4 h-4" />
              </button>

              {showNewMenu && (
                <div className="absolute right-0 top-8 w-56 bg-white border border-zinc-200 rounded-xl shadow-lg z-40 p-1.5 text-xs space-y-0.5">
                  <button
                    type="button"
                    onClick={handleCreateNewFreeForm}
                    className="w-full text-left px-2.5 py-2 rounded-lg hover:bg-zinc-100 flex items-center gap-2 text-zinc-800 font-medium"
                  >
                    <Sparkles className="w-3.5 h-3.5 text-zinc-600" />
                    Novo Procedimento Livre
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setShowNewMenu(false)
                      setStructuredModalOpen(true)
                    }}
                    className="w-full text-left px-2.5 py-2 rounded-lg hover:bg-zinc-100 flex items-center gap-2 text-zinc-800 font-medium"
                  >
                    <FileCode2 className="w-3.5 h-3.5 text-purple-600" />
                    Procedimento Estruturado
                  </button>
                  {onOpenImportSOP && (
                    <button
                      type="button"
                      onClick={() => {
                        setShowNewMenu(false)
                        onOpenImportSOP()
                      }}
                      className="w-full text-left px-2.5 py-2 rounded-lg hover:bg-zinc-100 flex items-center gap-2 text-zinc-800 font-medium border-t border-zinc-100"
                    >
                      <FileUp className="w-3.5 h-3.5 text-blue-600" />
                      Importar de SOP (From SOP)
                    </button>
                  )}
                </div>
              )}
            </div>
          </div>

          {/* Seletor de Agente se não estiver travado num único agente */}
          {!lockAgent && agents.length > 1 && onSelectAgent && (
            <div className="px-3 pt-2.5">
              <select
                value={selectedAgentId}
                onChange={(e) => onSelectAgent(e.target.value)}
                className="w-full px-2.5 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs text-zinc-800 font-medium"
              >
                {agents.map((a) => (
                  <option key={a.id} value={a.id}>
                    Agente: {a.name}
                  </option>
                ))}
              </select>
            </div>
          )}

          {/* Campo Buscar procedimentos */}
          <div className="p-3">
            <div className="relative">
              <Search className="w-3.5 h-3.5 text-zinc-400 absolute left-3 top-1/2 -translate-y-1/2" />
              <input
                type="text"
                placeholder="Buscar procedimentos"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="w-full pl-8 pr-3 py-1.5 bg-white border border-zinc-200/90 rounded-lg text-xs text-zinc-900 placeholder-zinc-400 focus:outline-none focus:ring-1 focus:ring-zinc-400"
              />
            </div>
          </div>

          {/* Lista de Procedimentos */}
          <div className="flex-1 overflow-y-auto px-2 pb-4 space-y-0.5">
            {agentProcedures.length === 0 ? (
              <div className="px-3 py-8 text-center text-xs text-zinc-400 space-y-2">
                <p>Nenhum procedimento criado.</p>
                <button
                  type="button"
                  onClick={handleCreateNewFreeForm}
                  className="inline-flex items-center gap-1 px-3 py-1.5 rounded-lg bg-zinc-900 text-white text-xs font-medium"
                >
                  <Plus className="w-3.5 h-3.5" /> Criar primeiro
                </button>
              </div>
            ) : (
              agentProcedures.map((proc) => {
                const isSelected = proc.id === activeProcId
                const hasErr = (proc.validation_errors?.length || 0) > 0
                return (
                  <div
                    key={proc.id}
                    onClick={() => selectProcedure(proc)}
                    className={`group flex items-center justify-between px-2.5 py-2 rounded-lg text-xs cursor-pointer transition-colors ${
                      isSelected
                        ? 'bg-zinc-200/75 text-zinc-950 font-semibold'
                        : 'text-zinc-700 hover:bg-zinc-100'
                    }`}
                  >
                    <div className="flex items-center gap-2 min-w-0">
                      {/* Ícone de fluxo sinuoso igual à sidebar da ElevenLabs */}
                      <svg
                        className="w-3.5 h-3.5 text-zinc-500 flex-shrink-0"
                        viewBox="0 0 16 16"
                        fill="none"
                        stroke="currentColor"
                        strokeWidth="1.5"
                      >
                        <path
                          d="M3 4h6a2 2 0 0 1 0 4H5a2 2 0 0 0 0 4h8"
                          strokeLinecap="round"
                        />
                      </svg>
                      <span className="truncate">{proc.name}</span>
                    </div>

                    <div className="flex items-center gap-1.5 flex-shrink-0">
                      {isSelected && dirty && (
                        <span
                          className="w-2 h-2 rounded-full bg-amber-500 flex-shrink-0"
                          title="Alterações não salvas no rascunho"
                        />
                      )}
                      {hasErr && <AlertTriangle className="w-3.5 h-3.5 text-red-500" />}
                      <button
                        type="button"
                        onClick={(e) => {
                          e.stopPropagation()
                          onDeleteProcedure(proc)
                        }}
                        title="Excluir procedimento"
                        className="opacity-0 group-hover:opacity-100 p-0.5 text-zinc-400 hover:text-red-600 rounded transition-opacity"
                      >
                        <MoreHorizontal className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </div>
                )
              })
            )}
          </div>
        </aside>
      )}

      {/* COLUNA DIREITA: Editor Estilo Notion da ElevenLabs (Imagem 1) */}
      <div className="flex-1 flex flex-col min-w-0 bg-white">
        {/* Barra Superior de Ferramentas (Toggle Sidebar, Undo, Redo, Copy, Docs, Salvar) */}
        <div className="px-4 py-2.5 border-b border-zinc-200/80 flex items-center justify-between">
          <div className="flex items-center gap-1 text-zinc-500">
            <button
              type="button"
              onClick={() => setSidebarOpen(!sidebarOpen)}
              title="Alternar barra lateral"
              className="p-1.5 hover:bg-zinc-100 hover:text-zinc-900 rounded-lg transition-colors"
            >
              <PanelLeft className="w-4 h-4" />
            </button>
            <button
              type="button"
              onClick={handleUndo}
              disabled={historyIdx <= 0}
              title="Desfazer (Undo)"
              className="p-1.5 hover:bg-zinc-100 hover:text-zinc-900 disabled:opacity-30 rounded-lg transition-colors"
            >
              <Undo2 className="w-4 h-4" />
            </button>
            <button
              type="button"
              onClick={handleRedo}
              disabled={historyIdx >= history.length - 1}
              title="Refazer (Redo)"
              className="p-1.5 hover:bg-zinc-100 hover:text-zinc-900 disabled:opacity-30 rounded-lg transition-colors"
            >
              <Redo2 className="w-4 h-4" />
            </button>
            {activeProc && (
              <button
                type="button"
                onClick={() => navigator.clipboard.writeText(content)}
                title="Copiar conteúdo do procedimento"
                className="p-1.5 hover:bg-zinc-100 hover:text-zinc-900 rounded-lg transition-colors"
              >
                <Copy className="w-4 h-4" />
              </button>
            )}
            <a
              href="https://elevenlabs.io/docs/eleven-agents/customization/procedures/free-form-procedures"
              target="_blank"
              rel="noreferrer"
              title="Documentação de Procedures"
              className="p-1.5 hover:bg-zinc-100 hover:text-zinc-900 rounded-lg transition-colors"
            >
              <BookOpen className="w-4 h-4" />
            </a>
          </div>

          {activeProc && (
            <div className="flex items-center gap-2">
              {activeProc.type === 'deterministic' && (
                <button
                  type="button"
                  onClick={() => setStructuredModalOpen(true)}
                  className="inline-flex items-center gap-1.5 px-3 py-1 rounded-lg bg-purple-50 text-purple-800 border border-purple-200 text-xs font-semibold"
                >
                  <FileCode2 className="w-3.5 h-3.5" />
                  Abrir Construtor de Passos
                </button>
              )}
              {savedToast && (
                <span className="text-xs text-emerald-600 font-medium flex items-center gap-1">
                  <Check className="w-3.5 h-3.5" /> Salvo
                </span>
              )}
              <button
                type="button"
                onClick={handleSave}
                disabled={saving}
                className={`px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-all ${
                  dirty
                    ? 'bg-zinc-900 text-white hover:bg-zinc-800 shadow-xs'
                    : 'bg-zinc-100 text-zinc-700 hover:bg-zinc-200'
                }`}
              >
                {saving ? 'Salvando…' : dirty ? 'Salvar alterações' : 'Salvo'}
              </button>
            </div>
          )}
        </div>

        {/* Corpo do Documento */}
        {!activeProc ? (
          <div className="flex-1 flex flex-col items-center justify-center p-12 text-center text-zinc-400 text-sm">
            Selecione um procedimento na barra lateral ou clique em <strong>+</strong> para criar um novo.
          </div>
        ) : (
          <div className="flex-1 overflow-y-auto px-8 sm:px-16 py-10 max-w-4xl mx-auto w-full space-y-8">
            {/* Título Principal Editável Inline (Igual à Imagem 1) */}
            <div className="flex items-center gap-2">
              <input
                type="text"
                value={title}
                onChange={(e) => {
                  setTitle(e.target.value)
                  setDirty(true)
                }}
                placeholder="Nome do Procedimento..."
                className="w-full text-2xl font-bold tracking-tight text-zinc-900 bg-transparent border-0 focus:outline-none focus:ring-0"
              />
            </div>

            {/* Tabela de Propriedades Estilo Notion (Diretrizes, Usado por, Usa, Acionar) */}
            <div className="space-y-3.5 text-xs">
              {/* Linha 1: Diretrizes (Guardrails) */}
              <div className="grid grid-cols-12 items-start gap-4">
                <div className="col-span-4 sm:col-span-3 flex items-center gap-2 text-zinc-500 py-1">
                  <Shield className="w-3.5 h-3.5 text-zinc-400" />
                  <span className="underline decoration-dotted underline-offset-4">Diretrizes</span>
                </div>
                <div className="col-span-8 sm:col-span-9 flex flex-wrap items-center gap-1.5 py-0.5">
                  {guardrails.map((g, i) => (
                    <span
                      key={i}
                      className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full bg-zinc-100 text-zinc-800 border border-zinc-200 text-xs"
                    >
                      {g}
                      <button
                        type="button"
                        onClick={() => {
                          setGuardrails(guardrails.filter((_, idx) => idx !== i))
                          setDirty(true)
                        }}
                        className="text-zinc-400 hover:text-red-600"
                      >
                        <X className="w-3 h-3" />
                      </button>
                    </span>
                  ))}
                  {addingGuardrail ? (
                    <div className="flex items-center gap-1.5">
                      <input
                        type="text"
                        autoFocus
                        value={newGuardrail}
                        onChange={(e) => setNewGuardrail(e.target.value)}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter' && newGuardrail.trim()) {
                            e.preventDefault()
                            setGuardrails([...guardrails, newGuardrail.trim()])
                            setNewGuardrail('')
                            setAddingGuardrail(false)
                            setDirty(true)
                          } else if (e.key === 'Escape') {
                            setAddingGuardrail(false)
                          }
                        }}
                        placeholder="Digite a diretriz e tecle Enter..."
                        className="px-2.5 py-1 border border-zinc-300 rounded-lg text-xs text-zinc-900 focus:outline-none"
                      />
                    </div>
                  ) : (
                    <button
                      type="button"
                      onClick={() => setAddingGuardrail(true)}
                      className="inline-flex items-center gap-1 text-zinc-500 hover:text-zinc-900 py-0.5 px-1.5 rounded hover:bg-zinc-100 transition-colors"
                    >
                      <Plus className="w-3.5 h-3.5" /> Adicionar
                    </button>
                  )}
                </div>
              </div>

              {/* Linha 2: Usado por */}
              <div className="grid grid-cols-12 items-center gap-4">
                <div className="col-span-4 sm:col-span-3 flex items-center gap-2 text-zinc-500 py-1">
                  <Link2 className="w-3.5 h-3.5 text-zinc-400" />
                  <span>Usado por</span>
                </div>
                <div className="col-span-8 sm:col-span-9 flex flex-wrap items-center gap-1.5">
                  {usedByProcedures.length === 0 ? (
                    <span className="text-zinc-400">Nenhum</span>
                  ) : (
                    usedByProcedures.map((up) => (
                      <button
                        key={up.id}
                        type="button"
                        onClick={() => selectProcedure(up)}
                        className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full bg-zinc-100 hover:bg-zinc-200 text-zinc-800 border border-zinc-200/80 text-xs font-medium"
                      >
                        <ClipboardList className="w-3 h-3 text-zinc-600" />
                        {up.name}
                      </button>
                    ))
                  )}
                </div>
              </div>

              {/* Linha 3: Usa (Pílulas automáticas de recursos referenciados no conteúdo!) */}
              <div className="grid grid-cols-12 items-center gap-4">
                <div className="col-span-4 sm:col-span-3 flex items-center gap-2 text-zinc-500 py-1">
                  <Inbox className="w-3.5 h-3.5 text-zinc-400" />
                  <span>Usa</span>
                </div>
                <div className="col-span-8 sm:col-span-9 flex flex-wrap items-center gap-1.5">
                  {usedResources.length === 0 ? (
                    <span className="text-zinc-400">Nenhum recurso referenciado</span>
                  ) : (
                    <>
                      {(showAllUsed ? usedResources : usedResources.slice(0, 3)).map((res) => (
                        <span
                          key={res.key}
                          className={`inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium border ${
                            res.valid
                              ? 'bg-zinc-100 text-zinc-800 border-zinc-200/90'
                              : 'bg-red-50 text-red-700 border-red-200'
                          }`}
                        >
                          {res.kind === 'procedure' && <ClipboardList className="w-3 h-3 text-zinc-600" />}
                          {res.kind === 'system_tool' && <Settings className="w-3 h-3 text-zinc-600" />}
                          {res.kind === 'tool' && <Wrench className="w-3 h-3 text-zinc-600" />}
                          {res.kind === 'kb' && <FolderOpen className="w-3 h-3 text-zinc-600" />}
                          {res.label}
                        </span>
                      ))}
                      {usedResources.length > 3 && (
                        <button
                          type="button"
                          onClick={() => setShowAllUsed(!showAllUsed)}
                          className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-semibold bg-zinc-100 hover:bg-zinc-200 text-zinc-700 border border-zinc-200"
                        >
                          {showAllUsed ? 'Recolher' : `+${usedResources.length - 3}`}
                        </button>
                      )}
                    </>
                  )}
                </div>
              </div>

              {/* Linha 4: Acionar (Toggle Switch + Card Arredondado de Trigger igual à Imagem 1) */}
              <div className="space-y-2.5">
                <div className="grid grid-cols-12 items-center gap-4">
                  <div className="col-span-4 sm:col-span-3 flex items-center gap-2 text-zinc-500 py-1">
                    <Zap className="w-3.5 h-3.5 text-zinc-400" />
                    <span className="underline decoration-dotted underline-offset-4">Acionar</span>
                  </div>
                  <div className="col-span-8 sm:col-span-9 flex items-center gap-3">
                    <Toggle
                      checked={triggerEnabled}
                      onChange={(next) => {
                        setTriggerEnabled(next)
                        setDirty(true)
                      }}
                      label="Habilitar acionamento por gatilho"
                    />
                    {!triggerEnabled && (
                      <span className="text-[11px] text-zinc-400">
                        Desligado (Sub-procedimento acionado apenas por referência)
                      </span>
                    )}
                  </div>
                </div>

                {triggerEnabled && (
                  <div className="p-3.5 bg-white border border-zinc-200/90 rounded-xl shadow-2xs">
                    <textarea
                      rows={2}
                      value={triggerText}
                      onChange={(e) => {
                        setTriggerText(e.target.value)
                        setDirty(true)
                      }}
                      placeholder="Quando o cliente solicitar atendimento sobre..."
                      className="w-full text-xs text-zinc-800 leading-relaxed bg-transparent border-0 focus:outline-none resize-none"
                    />
                  </div>
                )}
              </div>
            </div>

            {/* Seção Conteúdo com Editor Rico e Comando '/' Multi-Nível */}
            <div className="space-y-3 pt-2 relative">
              <div className="flex items-center justify-between">
                <span className="text-xs font-medium text-zinc-400">Conteúdo</span>
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    onClick={() => {
                      setSlashView('root')
                      setSlashSearch('')
                      setSlashPosition({ top: 40, left: 80 })
                      setSlashCursorPos(content.length)
                      setSlashOpen(!slashOpen)
                    }}
                    className="text-[11px] font-medium text-zinc-500 hover:text-zinc-900 bg-zinc-100 hover:bg-zinc-200/70 px-2.5 py-1 rounded-lg transition-colors"
                  >
                    + Comando / Referência (/)
                  </button>
                  <button
                    type="button"
                    onClick={() => setIsEditingContent(!isEditingContent)}
                    className="text-[11px] font-medium text-zinc-500 hover:text-zinc-900 px-2 py-1 rounded-lg hover:bg-zinc-100"
                  >
                    {isEditingContent ? 'Visualizar Pílulas' : 'Editar Texto'}
                  </button>
                </div>
              </div>

              {/* Visualização rica com Pílulas Inline clicáveis (idêntica à Imagem 1) ou Edição fluida */}
              {!isEditingContent ? (
                <div
                  onClick={() => {
                    setIsEditingContent(true)
                    setTimeout(() => textareaRef.current?.focus(), 20)
                  }}
                  className="min-h-[340px] p-4 rounded-xl border border-transparent hover:border-zinc-200/80 transition-colors cursor-text space-y-2.5 text-[13px] text-zinc-800 leading-relaxed"
                >
                  {content.split('\n').map((line, lIdx) => {
                    const trimmed = line.trim()
                    if (!trimmed) return <div key={lIdx} className="h-2" />

                    if (trimmed.startsWith('# ')) {
                      return (
                        <h2 key={lIdx} className="text-lg font-bold text-zinc-900 pt-2">
                          {renderRichLineWithPills(trimmed.slice(2))}
                        </h2>
                      )
                    }
                    if (trimmed.startsWith('## ')) {
                      return (
                        <h3 key={lIdx} className="text-base font-bold text-zinc-900 pt-1.5">
                          {renderRichLineWithPills(trimmed.slice(3))}
                        </h3>
                      )
                    }
                    if (trimmed.startsWith('### ')) {
                      return (
                        <h4 key={lIdx} className="text-sm font-semibold text-zinc-900 pt-1">
                          {renderRichLineWithPills(trimmed.slice(4))}
                        </h4>
                      )
                    }
                    if (trimmed.startsWith('* ') || trimmed.startsWith('- ') || trimmed.startsWith('• ')) {
                      return (
                        <div key={lIdx} className="flex items-start gap-2.5 pl-5">
                          <span className="text-zinc-900 font-bold mt-0.5">•</span>
                          <div className="flex-1">{renderRichLineWithPills(trimmed.slice(2))}</div>
                        </div>
                      )
                    }
                    const numMatch = trimmed.match(/^(\d+\.)\s+(.*)$/)
                    if (numMatch) {
                      return (
                        <div key={lIdx} className="flex items-start gap-2 pt-1 font-medium text-zinc-900">
                          <span>{numMatch[1]}</span>
                          <div className="flex-1">{renderRichLineWithPills(numMatch[2])}</div>
                        </div>
                      )
                    }

                    return <div key={lIdx}>{renderRichLineWithPills(line)}</div>
                  })}
                </div>
              ) : (
                <div className="relative">
                  <textarea
                    ref={textareaRef}
                    rows={16}
                    value={content}
                    onBlur={() => {
                      // Mantém aberto se o menu slash estiver ativo
                      if (!slashOpen) {
                        setIsEditingContent(false)
                      }
                    }}
                    onChange={(e) => {
                      const val = e.target.value
                      const caret = e.target.selectionStart
                      pushContentHistory(val)

                      if (val[caret - 1] === '/') {
                        setSlashCursorPos(caret)
                        setSlashView('root')
                        setSlashSearch('')
                        setSlashPosition({ top: 64, left: 48 })
                        setSlashOpen(true)
                      } else if (slashOpen && !val.includes('/')) {
                        setSlashOpen(false)
                      }
                    }}
                    onKeyDown={(e) => {
                      if (e.key === 'Escape' && slashOpen) {
                        e.preventDefault()
                        setSlashOpen(false)
                      }
                    }}
                    placeholder="Digite '/' para abrir o menu de formatação, etapas, ferramentas, base de conhecimento e sub-procedimentos..."
                    className="w-full p-4 bg-zinc-50/60 border border-zinc-200 rounded-xl text-[13px] text-zinc-900 leading-relaxed focus:outline-none focus:bg-white focus:ring-1 focus:ring-zinc-300"
                  />
                </div>
              )}

              {/* POPOVER FLUTUANTE DO COMANDO '/' (Idêntico às Imagens 2 e 3 da ElevenLabs!) */}
              {slashOpen && (
                <div
                  style={{ top: slashPosition.top, left: slashPosition.left }}
                  className="absolute z-50 w-72 bg-white border border-zinc-200/90 rounded-2xl shadow-2xl overflow-hidden text-xs animate-in fade-in zoom-in-95 duration-100"
                >
                  {slashView === 'root' ? (
                    /* NÍVEL 1 DO MENU SLASH (Imagem 2: Formatação + Comandos) */
                    <div className="divide-y divide-zinc-100">
                      <div className="p-2.5">
                        <input
                          type="text"
                          autoFocus
                          placeholder="Pesquisar..."
                          value={slashSearch}
                          onChange={(e) => setSlashSearch(e.target.value)}
                          onKeyDown={(e) => {
                            if (e.key === 'Escape') setSlashOpen(false)
                          }}
                          className="w-full px-2 py-1 text-xs text-zinc-900 placeholder-zinc-400 bg-transparent focus:outline-none"
                        />
                      </div>

                      <div className="p-1.5 max-h-80 overflow-y-auto space-y-3">
                        {/* Grupo Formatação */}
                        <div>
                          <span className="px-2.5 py-1 text-[11px] font-medium text-zinc-400 block">
                            Formatação
                          </span>
                          {[
                            {
                              label: 'Cabeçalho 1',
                              icon: Heading1,
                              snippet: '\n# ',
                            },
                            {
                              label: 'Cabeçalho 2',
                              icon: Heading2,
                              snippet: '\n## ',
                            },
                            {
                              label: 'Cabeçalho 3',
                              icon: Heading3,
                              snippet: '\n### ',
                            },
                          ]
                            .filter((item) =>
                              item.label.toLowerCase().includes(slashSearch.toLowerCase()),
                            )
                            .map((item) => {
                              const Icon = item.icon
                              return (
                                <button
                                  key={item.label}
                                  type="button"
                                  onClick={() => insertSlashCommand(item.snippet)}
                                  className="w-full text-left px-2.5 py-2 rounded-xl hover:bg-zinc-100 flex items-center gap-2.5 text-zinc-800 font-medium transition-colors"
                                >
                                  <Icon className="w-4 h-4 text-zinc-600" />
                                  {item.label}
                                </button>
                              )
                            })}
                        </div>

                        {/* Grupo Comandos */}
                        <div>
                          <span className="px-2.5 py-1 text-[11px] font-medium text-zinc-400 block">
                            Comandos
                          </span>
                          <button
                            type="button"
                            onClick={() =>
                              insertSlashCommand(
                                '\n1. **Nova Etapa:**\n   * Descreva a instrução desta etapa.',
                              )
                            }
                            className="w-full text-left px-2.5 py-2 rounded-xl hover:bg-zinc-100 flex items-center gap-2.5 text-zinc-800 font-medium transition-colors"
                          >
                            <ListOrdered className="w-4 h-4 text-zinc-600" />
                            Etapa
                          </button>

                          <button
                            type="button"
                            onClick={() => {
                              setSlashView('tools')
                              setSlashSearch('')
                            }}
                            className="w-full text-left px-2.5 py-2 rounded-xl hover:bg-zinc-100 flex items-center gap-2.5 text-zinc-800 font-medium transition-colors"
                          >
                            <Wrench className="w-4 h-4 text-zinc-600" />
                            Ferramentas
                          </button>

                          <button
                            type="button"
                            onClick={() => {
                              setSlashView('system_tools')
                              setSlashSearch('')
                            }}
                            className="w-full text-left px-2.5 py-2 rounded-xl hover:bg-zinc-100 flex items-center gap-2.5 text-zinc-800 font-medium transition-colors"
                          >
                            <Settings className="w-4 h-4 text-zinc-600" />
                            Ferramentas do sistema
                          </button>

                          <button
                            type="button"
                            onClick={() => {
                              setSlashView('kb')
                              setSlashSearch('')
                            }}
                            className="w-full text-left px-2.5 py-2 rounded-xl hover:bg-zinc-100 flex items-center gap-2.5 text-zinc-800 font-medium transition-colors"
                          >
                            <FolderOpen className="w-4 h-4 text-zinc-600" />
                            Base de conhecimento
                          </button>

                          <button
                            type="button"
                            onClick={() => {
                              setSlashView('procedures')
                              setSlashSearch('')
                            }}
                            className="w-full text-left px-2.5 py-2 rounded-xl hover:bg-zinc-100 flex items-center gap-2.5 text-zinc-800 font-medium transition-colors"
                          >
                            <ClipboardList className="w-4 h-4 text-zinc-600" />
                            Procedimentos
                          </button>
                        </div>
                      </div>
                    </div>
                  ) : (
                    /* NÍVEL 2 DO MENU SLASH (Imagem 3: Drill-down com "< Ferramentas", busca, filtros "+ Tipo" "+ Criador", nome + ID) */
                    <div className="w-80 divide-y divide-zinc-100">
                      {/* Header com botão Voltar (< Ferramentas) */}
                      <div className="px-3 py-2.5 flex items-center gap-2">
                        <button
                          type="button"
                          onClick={() => {
                            setSlashView('root')
                            setSlashSearch('')
                          }}
                          className="p-1 hover:bg-zinc-100 rounded-lg text-zinc-600"
                        >
                          <ChevronLeft className="w-4 h-4" />
                        </button>
                        <span className="font-semibold text-zinc-900">
                          {slashView === 'tools'
                            ? 'Ferramentas'
                            : slashView === 'system_tools'
                              ? 'Ferramentas do sistema'
                              : slashView === 'kb'
                                ? 'Base de conhecimento'
                                : 'Procedimentos'}
                        </span>
                      </div>

                      {/* Busca + Pílulas de Filtro (+ Tipo, + Criador) iguais à Imagem 3 */}
                      <div className="p-2.5 space-y-2">
                        <input
                          type="text"
                          autoFocus
                          placeholder={`Pesquisar ${
                            slashView === 'tools'
                              ? 'ferramentas'
                              : slashView === 'system_tools'
                                ? 'ferramentas do sistema'
                                : slashView === 'kb'
                                  ? 'bases de conhecimento'
                                  : 'procedimentos'
                          }...`}
                          value={slashSearch}
                          onChange={(e) => setSlashSearch(e.target.value)}
                          className="w-full px-1.5 py-1 text-xs text-zinc-900 placeholder-zinc-400 bg-transparent focus:outline-none"
                        />
                        <div className="flex items-center gap-1.5">
                          <span className="px-2 py-0.5 rounded-md border border-zinc-200 text-[11px] text-zinc-600 font-medium">
                            + Tipo
                          </span>
                          <span className="px-2 py-0.5 rounded-md border border-zinc-200 text-[11px] text-zinc-600 font-medium">
                            + Criador
                          </span>
                        </div>
                      </div>

                      {/* Lista de itens com Nome em Negrito + ID Técnico em Monospace */}
                      <div className="p-1.5 max-h-64 overflow-y-auto space-y-1">
                        {slashView === 'tools' && (
                          <>
                            <span className="px-2.5 py-1 text-[11px] font-medium text-zinc-400 block">
                              Ferramentas personalizadas
                            </span>
                            {tools
                              .filter((t) =>
                                t.name.toLowerCase().includes(slashSearch.toLowerCase()),
                              )
                              .map((t) => (
                                <button
                                  key={t.id}
                                  type="button"
                                  onClick={() => insertSlashCommand(`[tool id="${t.id}"]`)}
                                  className="w-full text-left px-2.5 py-2 rounded-xl hover:bg-zinc-100 flex items-start gap-2.5 transition-colors"
                                >
                                  <Wrench className="w-4 h-4 text-zinc-700 mt-0.5 flex-shrink-0" />
                                  <div className="min-w-0">
                                    <div className="font-semibold text-zinc-900 truncate">
                                      {t.name}
                                    </div>
                                    <div className="text-[11px] font-mono text-zinc-400 truncate">
                                      tool_{t.id.replace(/-/g, '').slice(0, 24)}
                                    </div>
                                  </div>
                                </button>
                              ))}
                          </>
                        )}

                        {slashView === 'system_tools' && (
                          <>
                            <span className="px-2.5 py-1 text-[11px] font-medium text-zinc-400 block">
                              Ferramentas nativas do sistema
                            </span>
                            {SYSTEM_TOOLS_CATALOG.filter((s) =>
                              s.label.toLowerCase().includes(slashSearch.toLowerCase()),
                            ).map((s) => (
                              <button
                                key={s.id}
                                type="button"
                                onClick={() => insertSlashCommand(`[system_tool id="${s.id}"]`)}
                                className="w-full text-left px-2.5 py-2 rounded-xl hover:bg-zinc-100 flex items-start gap-2.5 transition-colors"
                              >
                                <Settings className="w-4 h-4 text-zinc-700 mt-0.5 flex-shrink-0" />
                                <div className="min-w-0">
                                  <div className="font-semibold text-zinc-900 truncate">
                                    {s.label}
                                  </div>
                                  <div className="text-[11px] font-mono text-zinc-400 truncate">
                                    {s.sub}
                                  </div>
                                </div>
                              </button>
                            ))}
                          </>
                        )}

                        {slashView === 'kb' && (
                          <>
                            <span className="px-2.5 py-1 text-[11px] font-medium text-zinc-400 block">
                              Documentos da Base de Conhecimento
                            </span>
                            {knowledgeBases
                              .filter((k) =>
                                k.name.toLowerCase().includes(slashSearch.toLowerCase()),
                              )
                              .map((k) => (
                                <button
                                  key={k.id}
                                  type="button"
                                  onClick={() => insertSlashCommand(`[kb id="${k.id}"]`)}
                                  className="w-full text-left px-2.5 py-2 rounded-xl hover:bg-zinc-100 flex items-start gap-2.5 transition-colors"
                                >
                                  <FolderOpen className="w-4 h-4 text-zinc-700 mt-0.5 flex-shrink-0" />
                                  <div className="min-w-0">
                                    <div className="font-semibold text-zinc-900 truncate">
                                      {k.name}
                                    </div>
                                    <div className="text-[11px] font-mono text-zinc-400 truncate">
                                      kb_{k.id.replace(/-/g, '').slice(0, 24)}
                                    </div>
                                  </div>
                                </button>
                              ))}
                          </>
                        )}

                        {slashView === 'procedures' && (
                          <>
                            <span className="px-2.5 py-1 text-[11px] font-medium text-zinc-400 block">
                              Procedimentos do Agente
                            </span>
                            {agentProcedures
                              .filter(
                                (p) =>
                                  p.id !== activeProc.id &&
                                  p.name.toLowerCase().includes(slashSearch.toLowerCase()),
                              )
                              .map((p) => (
                                <button
                                  key={p.id}
                                  type="button"
                                  onClick={() => insertSlashCommand(`[procedure id="${p.id}"]`)}
                                  className="w-full text-left px-2.5 py-2 rounded-xl hover:bg-zinc-100 flex items-start gap-2.5 transition-colors"
                                >
                                  <ClipboardList className="w-4 h-4 text-zinc-700 mt-0.5 flex-shrink-0" />
                                  <div className="min-w-0">
                                    <div className="font-semibold text-zinc-900 truncate">
                                      {p.name}
                                    </div>
                                    <div className="text-[11px] font-mono text-zinc-400 truncate">
                                      agtprc_{p.id.replace(/-/g, '').slice(0, 22)}
                                    </div>
                                  </div>
                                </button>
                              ))}
                          </>
                        )}
                      </div>
                    </div>
                  )}
                </div>
              )}
            </div>
          </div>
        )}
      </div>

      {/* Modal Construtor de Structured Procedures */}
      <ProcedureEditorModal
        open={structuredModalOpen}
        onClose={() => setStructuredModalOpen(false)}
        initialSkill={activeProc?.type === 'deterministic' ? activeProc : null}
        initialType="deterministic"
        agents={agents}
        defaultAgentId={selectedAgentId || agents[0]?.id}
        lockAgent={lockAgent}
        tools={tools}
        knowledgeBases={knowledgeBases}
        siblingProcedures={agentProcedures}
        onSave={onSaveProcedure}
      />
    </div>
  )
}
