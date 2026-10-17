import { useEffect, useState } from 'react'
import {
  Sparkles,
  Plus,
  X,
  FileCode2,
  FileUp,
  Check,
  LayoutGrid,
  PanelLeft,
} from 'lucide-react'
import { api, type Skill, type Tool, type KnowledgeBase, type Agent } from '../api'
import Modal from '../components/Modal'
import Button from '../components/Button'
import ProcedureEditorModal from '../components/ProcedureEditorModal'
import NotionProcedureWorkspace from '../components/NotionProcedureWorkspace'

export default function SkillsPage() {
  const [skills, setSkills] = useState<Skill[]>([])
  const [agents, setAgents] = useState<Agent[]>([])
  const [tools, setTools] = useState<Tool[]>([])
  const [knowledgeBases, setKnowledgeBases] = useState<KnowledgeBase[]>([])
  const [selectedAgentId, setSelectedAgentId] = useState<string>('')
  const [viewMode, setViewMode] = useState<'workspace' | 'cards'>('workspace')
  const [loading, setLoading] = useState(true)
  const [selectedSkill, setSelectedSkill] = useState<Skill | null>(null)
  const [initialCreateType, setInitialCreateType] = useState<'free_form' | 'deterministic'>('free_form')
  const [isEditorOpen, setIsEditorOpen] = useState(false)
  const [error, setError] = useState('')

  // Modal From SOP (Importador de SOP da ElevenLabs)
  const [isSOPModalOpen, setIsSOPModalOpen] = useState(false)
  const [sopAgentId, setSopAgentId] = useState('')
  const [sopText, setSopText] = useState('')
  const [importingSOP, setImportingSOP] = useState(false)

  const loadData = async () => {
    setLoading(true)
    setError('')
    try {
      const [allTools, allAgents, allKBs] = await Promise.all([
        api<Tool[]>('/api/tools').catch(() => []),
        api<Agent[]>('/api/agents').catch(() => []),
        api<KnowledgeBase[]>('/api/knowledge-bases').catch(() => []),
      ])
      setTools(allTools || [])
      setAgents(allAgents || [])
      setKnowledgeBases(allKBs || [])
      if (allAgents && allAgents.length > 0) {
        if (!selectedAgentId) setSelectedAgentId(allAgents[0].id)
        if (!sopAgentId) setSopAgentId(allAgents[0].id)
      }

      const loadedSkills: Skill[] = []
      for (const a of allAgents || []) {
        const procs = await api<Skill[]>(`/api/agents/${a.id}/procedures`).catch(() => [])
        for (const p of procs || []) {
          loadedSkills.push({
            ...p,
            agent_id: a.id,
            description: p.description || `Agente: ${a.name}`,
          })
        }
      }
      setSkills(loadedSkills)
    } catch (err: any) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadData()
  }, [])

  const openEditor = (skill?: Skill, defaultType: 'free_form' | 'deterministic' = 'free_form') => {
    setSelectedSkill(skill || null)
    setInitialCreateType(defaultType)
    setIsEditorOpen(true)
  }

  const handleSaveProcedure = async (payload: {
    id?: string
    agent_id: string
    name: string
    type: 'free_form' | 'deterministic'
    description: string
    trigger: string
    guardrails: string[]
    content: string
    active: boolean
  }) => {
    if (payload.id) {
      await api(`/api/agents/${payload.agent_id}/procedures/${payload.id}`, {
        method: 'PUT',
        body: JSON.stringify({
          name: payload.name,
          type: payload.type,
          trigger: payload.trigger,
          guardrails: payload.guardrails,
          content: payload.content,
          active: payload.active,
        }),
      })
    } else {
      await api(`/api/agents/${payload.agent_id}/procedures`, {
        method: 'POST',
        body: JSON.stringify({
          name: payload.name,
          type: payload.type,
          trigger: payload.trigger,
          guardrails: payload.guardrails,
          content: payload.content,
          active: payload.active,
        }),
      })
    }
    await loadData()
  }

  const deleteSkill = async (skill: Skill) => {
    if (!window.confirm(`Deseja realmente excluir o procedimento "${skill.name}"?`)) return
    try {
      if (skill.agent_id) {
        await api(`/api/agents/${skill.agent_id}/procedures/${skill.id}`, {
          method: 'DELETE',
        })
      }
      await loadData()
    } catch (err: any) {
      setError(err.message || 'Erro ao excluir procedure')
    }
  }

  const handleImportSOP = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!sopAgentId || !sopText.trim()) return
    setImportingSOP(true)
    setError('')
    try {
      await api(`/api/agents/${sopAgentId}/procedures/import-sop`, {
        method: 'POST',
        body: JSON.stringify({ document: sopText }),
      })
      setSopText('')
      setIsSOPModalOpen(false)
      await loadData()
    } catch (err: any) {
      setError(err.message || 'Falha ao importar SOP')
    } finally {
      setImportingSOP(false)
    }
  }

  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = () => {
      if (typeof reader.result === 'string') {
        setSopText(reader.result)
      }
    }
    reader.readAsText(file)
  }

  return (
    <div className="max-w-7xl mx-auto space-y-5">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 border-b border-zinc-200/80 pb-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight text-zinc-900 flex items-center gap-2">
            <Sparkles className="w-6 h-6 text-zinc-700" />
            Procedimentos (ElevenLabs Workspace)
          </h1>
          <p className="text-xs text-zinc-500 mt-1">
            Editor estilo Notion com pílulas atômicas inline, menu de comando <code className="font-mono">/</code> em dois níveis, propriedades (<code className="font-mono">Diretrizes</code>, <code className="font-mono">Usado por</code>, <code className="font-mono">Usa</code>, <code className="font-mono">Acionar</code>) e Structured Procedures.
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <div className="inline-flex p-0.5 bg-zinc-100 rounded-lg border border-zinc-200/80">
            <button
              type="button"
              onClick={() => setViewMode('workspace')}
              className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium transition-colors ${
                viewMode === 'workspace'
                  ? 'bg-white text-zinc-900 shadow-2xs'
                  : 'text-zinc-500 hover:text-zinc-800'
              }`}
            >
              <PanelLeft className="w-3.5 h-3.5" />
              Workspace ElevenLabs
            </button>
            <button
              type="button"
              onClick={() => setViewMode('cards')}
              className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium transition-colors ${
                viewMode === 'cards'
                  ? 'bg-white text-zinc-900 shadow-2xs'
                  : 'text-zinc-500 hover:text-zinc-800'
              }`}
            >
              <LayoutGrid className="w-3.5 h-3.5" />
              Cards
            </button>
          </div>

          <button
            type="button"
            onClick={() => setIsSOPModalOpen(true)}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-white hover:bg-zinc-50 border border-zinc-200 text-zinc-800 text-xs font-medium shadow-2xs transition-colors"
          >
            <FileUp className="w-3.5 h-3.5 text-zinc-600" />
            From SOP
          </button>

          <button
            type="button"
            onClick={() => openEditor(undefined, 'deterministic')}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-purple-50 hover:bg-purple-100 border border-purple-200 text-purple-900 text-xs font-semibold transition-colors"
          >
            <FileCode2 className="w-3.5 h-3.5 text-purple-700" />
            + Structured
          </button>

          <Button size="sm" onClick={() => openEditor(undefined, 'free_form')}>
            <Plus className="w-3.5 h-3.5" />
            + Free-form
          </Button>
        </div>
      </div>

      {error && (
        <div className="p-3 bg-red-50 border border-red-200 rounded-lg text-xs text-red-700">
          {error}
        </div>
      )}

      {loading ? (
        <div className="text-center py-12 text-xs text-zinc-400">Carregando procedimentos…</div>
      ) : viewMode === 'workspace' ? (
        <NotionProcedureWorkspace
          agents={agents}
          selectedAgentId={selectedAgentId}
          onSelectAgent={setSelectedAgentId}
          procedures={skills}
          tools={tools}
          knowledgeBases={knowledgeBases}
          onSaveProcedure={handleSaveProcedure}
          onDeleteProcedure={deleteSkill}
          onOpenImportSOP={() => setIsSOPModalOpen(true)}
        />
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {skills.map((skill) => (
            <div
              key={skill.id}
              onClick={() => openEditor(skill)}
              className="bg-white border border-zinc-200/80 hover:border-zinc-300 rounded-xl p-5 shadow-xs transition-all cursor-pointer space-y-2"
            >
              <div className="flex items-center justify-between">
                <h3 className="font-semibold text-sm text-zinc-900">{skill.name}</h3>
                <span className="text-[10px] px-2 py-0.5 rounded-full bg-zinc-100 text-zinc-700 font-medium">
                  {skill.type === 'deterministic' ? 'Structured' : 'Free-form'}
                </span>
              </div>
              <p className="text-xs text-zinc-500 line-clamp-2">
                {skill.trigger || 'Sub-procedimento (sem trigger direto)'}
              </p>
            </div>
          ))}
        </div>
      )}

      {/* Modal Editor de Procedure */}
      <ProcedureEditorModal
        open={isEditorOpen}
        onClose={() => setIsEditorOpen(false)}
        initialSkill={selectedSkill}
        initialType={initialCreateType}
        agents={agents}
        defaultAgentId={selectedAgentId || agents[0]?.id}
        tools={tools}
        knowledgeBases={knowledgeBases}
        siblingProcedures={skills.filter(
          (s) => s.agent_id === (selectedSkill?.agent_id || selectedAgentId || agents[0]?.id),
        )}
        onSave={handleSaveProcedure}
      />

      {/* Modal From SOP */}
      <Modal open={isSOPModalOpen} onClose={() => setIsSOPModalOpen(false)} sizeClass="max-w-2xl">
        <form onSubmit={handleImportSOP} className="p-6 space-y-4">
          <div className="flex items-center justify-between border-b border-zinc-100 pb-3">
            <div>
              <h3 className="text-sm font-semibold text-zinc-900 flex items-center gap-2">
                <FileUp className="w-4 h-4 text-zinc-700" />
                Bootstrap from SOP (Importar Procedimento Operacional Padrão)
              </h3>
              <p className="text-[11px] text-zinc-500 mt-0.5">
                Envie um arquivo (.md, .txt) ou cole seu documento de SOP. A plataforma identifica até 10 procedimentos e gera rascunhos com Name, Trigger e Content automaticamente.
              </p>
            </div>
            <button
              type="button"
              onClick={() => setIsSOPModalOpen(false)}
              className="text-zinc-400 hover:text-zinc-600"
            >
              <X className="w-4 h-4" />
            </button>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label className="block text-xs font-semibold text-zinc-700 uppercase tracking-wider mb-1">
                Agente de Destino
              </label>
              <select
                required
                value={sopAgentId}
                onChange={(e) => setSopAgentId(e.target.value)}
                className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-xs text-zinc-900"
              >
                {agents.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name}
                  </option>
                ))}
              </select>
            </div>

            <div>
              <label className="block text-xs font-semibold text-zinc-700 uppercase tracking-wider mb-1">
                Carregar Arquivo (.md, .txt)
              </label>
              <input
                type="file"
                accept=".md,.txt,.html"
                onChange={handleFileUpload}
                className="w-full text-xs text-zinc-600 file:mr-3 file:py-1.5 file:px-3 file:rounded-lg file:border-0 file:text-xs file:font-semibold file:bg-zinc-100 file:text-zinc-800 hover:file:bg-zinc-200"
              />
            </div>
          </div>

          <div>
            <label className="block text-xs font-semibold text-zinc-700 uppercase tracking-wider mb-1">
              Conteúdo do SOP (separe múltiplos procedimentos com títulos # Título)
            </label>
            <textarea
              rows={9}
              required
              value={sopText}
              onChange={(e) => setSopText(e.target.value)}
              placeholder={`# Reembolso de Pedido\nTrigger: Quando o cliente pedir reembolso, devolução ou estorno de uma compra\n1. Peça o código do pedido.\n2. Verifique o status com [tool id="..."].`}
              className="w-full p-3 bg-zinc-50 border border-zinc-200 rounded-xl text-xs font-mono text-zinc-900 leading-relaxed"
            />
          </div>

          <div className="flex justify-end gap-2 pt-3 border-t border-zinc-100">
            <Button type="button" variant="secondary" onClick={() => setIsSOPModalOpen(false)}>
              Cancelar
            </Button>
            <Button type="submit" disabled={importingSOP || !sopText.trim() || !sopAgentId}>
              <Check className="w-4 h-4" />
              {importingSOP ? 'Extraindo Procedures…' : 'Importar Procedures do SOP'}
            </Button>
          </div>
        </form>
      </Modal>
    </div>
  )
}
