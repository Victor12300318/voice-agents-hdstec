import { useState } from 'react'
import { AnimatePresence, motion } from 'motion/react'
import {
  GitBranch,
  GitFork,
  GitMerge,
  RefreshCw,
  Archive,
  ArchiveRestore,
  Sliders,
  Plus,
  Check,
  Copy,
  AlertTriangle,
  History,
  Trash2,
  Eye,
} from 'lucide-react'
import {
  api,
  type AgentDetail,
  type AgentBranch,
  type AgentVersionSummary,
  type MergePreview,
  type RebasePreview,
} from '../api'
import Button from './Button'
import Modal from './Modal'
import { springStudio, springTabPill } from './motion'

interface AgentVersioningPanelProps {
  agent: AgentDetail
  versions: AgentVersionSummary[]
  activeBranchId: string
  viewingVersionId: string | null
  publishing: boolean
  onSelectBranch: (branchId: string) => void
  onSelectVersion: (versionId: string | null) => void
  onPublishToBranch: (branchId: string, commitMessage: string) => Promise<void>
  onRefresh: () => Promise<void>
}

export default function AgentVersioningPanel({
  agent,
  versions,
  activeBranchId,
  viewingVersionId,
  publishing,
  onSelectBranch,
  onSelectVersion,
  onPublishToBranch,
  onRefresh,
}: AgentVersioningPanelProps) {
  const [subTab, setSubTab] = useState<'branches' | 'versions' | 'traffic'>('branches')
  const [showArchived, setShowArchived] = useState(false)
  const [enabling, setEnabling] = useState(false)
  const [errorMsg, setErrorMsg] = useState('')
  const [feedbackMsg, setFeedbackMsg] = useState('')
  const [copiedId, setCopiedId] = useState<string | null>(null)

  // Modal Nova Branch
  const [createBranchOpen, setCreateBranchOpen] = useState(false)
  const [newBranchName, setNewBranchName] = useState('')
  const [newBranchDesc, setNewBranchDesc] = useState('')
  const [newBranchParentVer, setNewBranchParentVer] = useState('')
  const [creatingBranch, setCreatingBranch] = useState(false)

  // Modal Publicar Versão (Commit)
  const [commitModalOpen, setCommitModalOpen] = useState(false)
  const [commitMessage, setCommitMessage] = useState('')

  // Modal Merge + Preview
  const [mergeModalBranch, setMergeModalBranch] = useState<AgentBranch | null>(null)
  const [mergeTargetBranchId, setMergeTargetBranchId] = useState('Main')
  const [mergeArchiveSource, setMergeArchiveSource] = useState(true)
  const [mergeForce, setMergeForce] = useState(false)
  const [mergePreview, setMergePreview] = useState<MergePreview | null>(null)
  const [mergePreviewError, setMergePreviewError] = useState('')
  const [loadingMergePreview, setLoadingMergePreview] = useState(false)
  const [merging, setMerging] = useState(false)

  // Modal Rebase + Preview
  const [rebaseModalBranch, setRebaseModalBranch] = useState<AgentBranch | null>(null)
  const [rebasePreview, setRebasePreview] = useState<RebasePreview | null>(null)
  const [rebasePreviewError, setRebasePreviewError] = useState('')
  const [loadingRebasePreview, setLoadingRebasePreview] = useState(false)
  const [rebasing, setRebasing] = useState(false)

  // Estado de Traffic Deployment
  const branches = agent.branches || []
  const activeBranches = branches.filter((b) => !b.archived)
  const [trafficDraft, setTrafficDraft] = useState<Record<string, number>>(() => {
    const map: Record<string, number> = {}
    for (const b of branches) {
      if (!b.archived) map[b.id] = b.traffic_percentage
    }
    return map
  })
  const [savingTraffic, setSavingTraffic] = useState(false)

  const syncTrafficFromBranches = (list: AgentBranch[]) => {
    const next: Record<string, number> = {}
    for (const b of list) {
      if (!b.archived) next[b.id] = b.traffic_percentage
    }
    setTrafficDraft(next)
  }

  const copyCode = (code: string) => {
    navigator.clipboard.writeText(code)
    setCopiedId(code)
    setTimeout(() => setCopiedId((prev) => (prev === code ? null : prev)), 1500)
  }

  const handleEnableVersioning = async () => {
    setErrorMsg('')
    setEnabling(true)
    try {
      await api(`/api/agents/${agent.id}/versioning/enable`, { method: 'POST' })
      setFeedbackMsg('Controle de versão habilitado permanentemente com a branch Main (100% tráfego).')
      await onRefresh()
    } catch (e: any) {
      setErrorMsg(e.message || 'Falha ao habilitar controle de versão')
    } finally {
      setEnabling(false)
    }
  }

  const openCreateBranchModal = (parentVersionCode?: string) => {
    setErrorMsg('')
    setNewBranchName('')
    setNewBranchDesc('')
    setNewBranchParentVer(
      parentVersionCode ||
        agent.current_branch?.head_version_id ||
        versions[0]?.version_id ||
        '',
    )
    setCreateBranchOpen(true)
  }

  const handleCreateBranch = async () => {
    if (!newBranchName.trim()) return
    setCreatingBranch(true)
    setErrorMsg('')
    try {
      const res = await api<{ created_branch_id: string; created_version_id: string }>(
        `/api/agents/${agent.id}/branches`,
        {
          method: 'POST',
          body: JSON.stringify({
            name: newBranchName.trim(),
            description: newBranchDesc.trim(),
            parent_version_id: newBranchParentVer,
          }),
        },
      )
      setCreateBranchOpen(false)
      setFeedbackMsg(
        `Branch "${newBranchName.trim()}" (${res.created_branch_id}) criada na versão inicial ${res.created_version_id}.`,
      )
      await onRefresh()
      onSelectBranch(res.created_branch_id)
    } catch (e: any) {
      setErrorMsg(e.message || 'Erro ao criar branch')
    } finally {
      setCreatingBranch(false)
    }
  }

  const handleArchiveToggle = async (b: AgentBranch) => {
    setErrorMsg('')
    try {
      await api(`/api/agents/${agent.id}/branches/${encodeURIComponent(b.id)}`, {
        method: 'PATCH',
        body: JSON.stringify({ archived: !b.archived }),
      })
      setFeedbackMsg(
        !b.archived
          ? `Branch "${b.name}" arquivada com sucesso.`
          : `Branch "${b.name}" restaurada com sucesso.`,
      )
      await onRefresh()
    } catch (e: any) {
      setErrorMsg(e.message || 'Não foi possível alterar o arquivamento da branch')
    }
  }

  const handleDiscardDraft = async (b: AgentBranch) => {
    setErrorMsg('')
    try {
      await api(`/api/agents/${agent.id}/branches/${encodeURIComponent(b.id)}/draft`, {
        method: 'DELETE',
      })
      setFeedbackMsg(`Rascunho (Draft) descartado na branch "${b.name}".`)
      await onRefresh()
    } catch (e: any) {
      setErrorMsg(e.message || 'Erro ao descartar rascunho')
    }
  }

  const openMergeModal = async (sourceBranch: AgentBranch) => {
    const mainB = branches.find((b) => b.is_main)
    const defaultTarget =
      sourceBranch.is_main
        ? activeBranches.find((b) => b.id !== sourceBranch.id)?.id || 'Main'
        : mainB?.id || 'Main'
    setMergeModalBranch(sourceBranch)
    setMergeTargetBranchId(defaultTarget)
    setMergeArchiveSource(!sourceBranch.is_main)
    setMergeForce(false)
    await loadMergePreview(sourceBranch.id, defaultTarget, false)
  }

  const loadMergePreview = async (srcId: string, tgtId: string, force: boolean) => {
    setLoadingMergePreview(true)
    setMergePreviewError('')
    setMergePreview(null)
    try {
      const p = await api<MergePreview>(
        `/api/agents/${agent.id}/branches/${encodeURIComponent(srcId)}/preview-merge`,
        {
          method: 'POST',
          body: JSON.stringify({ target_branch_id: tgtId, force }),
        },
      )
      setMergePreview(p)
    } catch (e: any) {
      setMergePreviewError(e.message || 'Não há alterações para mesclar')
    } finally {
      setLoadingMergePreview(false)
    }
  }

  const handleConfirmMerge = async () => {
    if (!mergeModalBranch) return
    setMerging(true)
    setErrorMsg('')
    try {
      await api(
        `/api/agents/${agent.id}/branches/${encodeURIComponent(mergeModalBranch.id)}/merge`,
        {
          method: 'POST',
          body: JSON.stringify({
            target_branch_id: mergeTargetBranchId,
            archive_source_branch: mergeArchiveSource,
            force: mergeForce,
          }),
        },
      )
      const targetObj = branches.find((b) => b.id === mergeTargetBranchId || b.name === mergeTargetBranchId)
      setMergeModalBranch(null)
      setFeedbackMsg(
        `Merge concluído! Nova versão criada em "${targetObj?.name || mergeTargetBranchId}" e tráfego transferido automaticamente.`,
      )
      await onRefresh()
      if (targetObj) {
        onSelectBranch(targetObj.id)
      }
    } catch (e: any) {
      setMergePreviewError(e.message || 'Falha ao executar merge')
    } finally {
      setMerging(false)
    }
  }

  const openRebaseModal = async (branch: AgentBranch) => {
    setRebaseModalBranch(branch)
    setLoadingRebasePreview(true)
    setRebasePreviewError('')
    setRebasePreview(null)
    try {
      const p = await api<RebasePreview>(
        `/api/agents/${agent.id}/branches/${encodeURIComponent(branch.id)}/preview-rebase`,
        { method: 'POST' },
      )
      setRebasePreview(p)
    } catch (e: any) {
      setRebasePreviewError(e.message || 'Branch já está atualizada com a Main')
    } finally {
      setLoadingRebasePreview(false)
    }
  }

  const handleConfirmRebase = async () => {
    if (!rebaseModalBranch) return
    setRebasing(true)
    try {
      await api(
        `/api/agents/${agent.id}/branches/${encodeURIComponent(rebaseModalBranch.id)}/rebase`,
        { method: 'POST' },
      )
      setRebaseModalBranch(null)
      setFeedbackMsg(
        `Rebase concluído! As últimas alterações da Main foram incorporadas em "${rebaseModalBranch.name}" preservando as edições da branch.`,
      )
      await onRefresh()
    } catch (e: any) {
      setRebasePreviewError(e.message || 'Falha ao realizar rebase')
    } finally {
      setRebasing(false)
    }
  }

  const totalTraffic = activeBranches.reduce(
    (acc, b) => acc + (trafficDraft[b.id] ?? b.traffic_percentage ?? 0),
    0,
  )

  const handleSaveTraffic = async () => {
    setErrorMsg('')
    setSavingTraffic(true)
    try {
      const deployments = activeBranches.map((b) => ({
        branch_id: b.id,
        percentage: Number(trafficDraft[b.id] ?? b.traffic_percentage ?? 0),
      }))
      const res = await api<{ branches: AgentBranch[] }>(`/api/agents/${agent.id}/deployments`, {
        method: 'POST',
        body: JSON.stringify({ deployments }),
      })
      syncTrafficFromBranches(res.branches)
      setFeedbackMsg('Distribuição determinística de tráfego (100%) salva com sucesso.')
      await onRefresh()
    } catch (e: any) {
      setErrorMsg(e.message || 'A soma dos percentuais deve ser exatamente 100%')
    } finally {
      setSavingTraffic(false)
    }
  }

  if (!agent.versioning_enabled) {
    return (
      <div className="max-w-4xl mx-auto px-6 sm:px-10 py-8 space-y-6">
        <div className="bg-white border border-zinc-200 rounded-2xl p-6 sm:p-8 shadow-2xs space-y-6">
          <div className="flex items-start justify-between gap-4 flex-wrap">
            <div className="space-y-1.5">
              <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-[11px] font-semibold bg-zinc-100 text-zinc-800 border border-zinc-200">
                <GitBranch className="w-3.5 h-3.5" />
                ElevenLabs Agent Versioning
              </span>
              <h2 className="text-xl font-bold text-zinc-950">
                Experimente configurações com segurança usando branches, versões e divisão de tráfego
              </h2>
              <p className="text-xs text-zinc-500 max-w-2xl leading-relaxed">
                Crie branches isoladas, salve rascunhos por usuário/branch, faça merge ou rebase com
                resolução de conflitos e distribua o tráfego de chamadas ao vivo por percentual
                determinístico (ex: 90% Main / 10% Experimento).
              </p>
            </div>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3 text-xs">
            <div className="p-4 rounded-xl bg-zinc-50 border border-zinc-200/80 space-y-1">
              <div className="font-semibold text-zinc-900 flex items-center gap-1.5">
                <History className="w-3.5 h-3.5 text-zinc-700" />
                Snapshots imutáveis
              </div>
              <p className="text-zinc-500 text-[11px] leading-relaxed">
                Cada versão recebe um ID único (<code className="font-mono">agtvrsn_xxxx</code>)
                congelando prompt, voz, LLM, ferramentas, guardrails e procedimentos.
              </p>
            </div>

            <div className="p-4 rounded-xl bg-zinc-50 border border-zinc-200/80 space-y-1">
              <div className="font-semibold text-zinc-900 flex items-center gap-1.5">
                <GitBranch className="w-3.5 h-3.5 text-zinc-700" />
                Branches isoladas & Rebase
              </div>
              <p className="text-zinc-500 text-[11px] leading-relaxed">
                Crie branches (<code className="font-mono">agtbrch_xxxx</code>) a partir de qualquer
                versão, puxe atualizações da Main com Rebase e faça Merge com preview de conflitos.
              </p>
            </div>

            <div className="p-4 rounded-xl bg-zinc-50 border border-zinc-200/80 space-y-1">
              <div className="font-semibold text-zinc-900 flex items-center gap-1.5">
                <Sliders className="w-3.5 h-3.5 text-zinc-700" />
                Divisão de tráfego (100%)
              </div>
              <p className="text-zinc-500 text-[11px] leading-relaxed">
                Roteamento determinístico por conversa para rollouts graduais (5% → 25% → 50% → 100%)
                sem risco para a operação principal.
              </p>
            </div>
          </div>

          <div className="p-3.5 rounded-xl bg-amber-50/80 border border-amber-200/90 flex items-start gap-2.5 text-xs text-amber-900">
            <AlertTriangle className="w-4 h-4 text-amber-600 flex-shrink-0 mt-0.5" />
            <div>
              <span className="font-semibold">Atenção:</span> Uma vez habilitado em um agente, o
              controle de versão não pode ser desabilitado. Habilitar criará automaticamente a branch{' '}
              <strong className="font-semibold">Main</strong> com 100% do tráfego e a primeira versão
              imutável.
            </div>
          </div>

          {errorMsg && (
            <div className="p-3 rounded-xl bg-red-50 border border-red-200 text-xs text-red-700">
              {errorMsg}
            </div>
          )}

          <div className="flex items-center justify-between pt-2">
            <span className="text-[11px] text-zinc-400">
              {versions.length > 0
                ? `${versions.length} snapshot(s) anteriores serão vinculados à branch Main.`
                : 'O estado atual do agente será salvo como a versão inicial v1 da Main.'}
            </span>
            <Button onClick={handleEnableVersioning} disabled={enabling}>
              {enabling ? 'Habilitando…' : 'Habilitar Controle de Versão'}
            </Button>
          </div>
        </div>
      </div>
    )
  }

  const currentBranch =
    branches.find((b) => b.id === activeBranchId || b.name === activeBranchId) ||
    agent.current_branch ||
    branches[0]

  const displayedBranches = branches.filter((b) => (showArchived ? true : !b.archived))

  return (
    <div className="max-w-5xl mx-auto px-6 sm:px-10 py-8 space-y-6">
      {/* Cabeçalho com Branch Ativa e Ações Rápidas */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-2 flex-wrap">
            <h2 className="text-xl font-bold text-zinc-950">Controle de versão</h2>
            {currentBranch && (
              <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-semibold bg-zinc-900 text-white">
                <GitBranch className="w-3 h-3" />
                {currentBranch.name}
                <span className="opacity-70 font-mono text-[10px]">({currentBranch.id})</span>
              </span>
            )}
            {viewingVersionId && (
              <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-semibold bg-amber-100 text-amber-900 border border-amber-300">
                <Eye className="w-3 h-3" />
                Visualizando snapshot {viewingVersionId}
                <button
                  type="button"
                  onClick={() => onSelectVersion(null)}
                  className="underline ml-1 hover:text-amber-950 cursor-pointer"
                >
                  Voltar ao topo da branch
                </button>
              </span>
            )}
          </div>
          <p className="text-xs text-zinc-500 mt-1">
            Gerencie branches isoladas (<code className="font-mono">agtbrch_xxxx</code>), versões
            imutáveis (<code className="font-mono">agtvrsn_xxxx</code>), rascunhos por usuário e
            implantação percentual de tráfego.
          </p>
        </div>

        <div className="flex items-center gap-2 flex-wrap">
          <Button
            variant="secondary"
            size="sm"
            onClick={() => openCreateBranchModal(currentBranch?.head_version_id)}
          >
            <Plus className="w-3.5 h-3.5 mr-1" />
            Nova branch
          </Button>
          <Button
            variant="secondary"
            size="sm"
            onClick={() => {
              syncTrafficFromBranches(branches)
              setSubTab('traffic')
            }}
          >
            <Sliders className="w-3.5 h-3.5 mr-1" />
            Distribuir tráfego
          </Button>
          <Button
            size="sm"
            disabled={publishing || currentBranch?.archived}
            onClick={() => {
              setCommitMessage('')
              setCommitModalOpen(true)
            }}
          >
            {publishing
              ? 'Publicando…'
              : `Comitar versão em ${currentBranch?.name || 'Main'}`}
          </Button>
        </div>
      </div>

      {errorMsg && (
        <div className="p-3 rounded-xl bg-red-50 border border-red-200 text-xs text-red-700 flex items-center justify-between">
          <span>{errorMsg}</span>
          <button
            type="button"
            onClick={() => setErrorMsg('')}
            className="text-red-500 hover:text-red-800 font-semibold"
          >
            Fechar
          </button>
        </div>
      )}

      {feedbackMsg && (
        <div className="p-3 rounded-xl bg-emerald-50 border border-emerald-200 text-xs text-emerald-800 flex items-center justify-between">
          <span>{feedbackMsg}</span>
          <button
            type="button"
            onClick={() => setFeedbackMsg('')}
            className="text-emerald-600 hover:text-emerald-900 font-semibold"
          >
            OK
          </button>
        </div>
      )}

      {/* Aviso de Per-agent settings read-only em branches secundárias */}
      {currentBranch && !currentBranch.is_main && (
        <div className="p-3 rounded-xl bg-zinc-100 border border-zinc-200/90 text-xs text-zinc-700 flex items-center justify-between gap-3">
          <div>
            <strong className="font-semibold text-zinc-900">
              Branch secundária ({currentBranch.name}):
            </strong>{' '}
            Configurações globais por agente (como nome do agente e status ativo) pertencem à branch{' '}
            <strong className="font-semibold">Main</strong> e ficam em modo somente-leitura aqui.
          </div>
          <button
            type="button"
            onClick={() => {
              const mainB = branches.find((b) => b.is_main)
              if (mainB) onSelectBranch(mainB.id)
            }}
            className="px-2.5 py-1 rounded-lg bg-white border border-zinc-200 text-zinc-800 hover:bg-zinc-50 font-semibold whitespace-nowrap cursor-pointer"
          >
            Ir para Main
          </button>
        </div>
      )}

      {/* Sub-abas com pílula deslizante layoutId */}
      <div className="flex items-center justify-between border-b border-zinc-200 pb-2.5">
        <div className="inline-flex p-0.5 bg-zinc-100 rounded-xl border border-zinc-200/80">
          {(
            [
              { id: 'branches', label: `Branches (${activeBranches.length})` },
              { id: 'versions', label: `Versões (${versions.length})` },
              { id: 'traffic', label: 'Implantação de Tráfego (100%)' },
            ] as const
          ).map((t) => {
            const active = subTab === t.id
            return (
              <button
                key={t.id}
                type="button"
                onClick={() => {
                  if (t.id === 'traffic') syncTrafficFromBranches(branches)
                  setSubTab(t.id)
                }}
                className={`relative px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-colors cursor-pointer ${
                  active ? 'text-zinc-950' : 'text-zinc-500 hover:text-zinc-800'
                }`}
              >
                {active && (
                  <motion.span
                    layoutId="versioning-subtab-pill"
                    transition={springTabPill}
                    className="absolute inset-0 rounded-lg bg-white shadow-2xs"
                    aria-hidden="true"
                  />
                )}
                <span className="relative z-10">{t.label}</span>
              </button>
            )
          })}
        </div>

        {subTab === 'branches' && (
          <label className="inline-flex items-center gap-2 text-xs text-zinc-600 cursor-pointer select-none">
            <input
              type="checkbox"
              checked={showArchived}
              onChange={(e) => setShowArchived(e.target.checked)}
              className="rounded border-zinc-300 text-zinc-900"
            />
            Mostrar branches arquivadas
          </label>
        )}
      </div>

      {/* ABA 1: BRANCHES */}
      <AnimatePresence mode="wait">
        {subTab === 'branches' && (
          <motion.div
            key="subtab-branches"
            initial={{ opacity: 0, y: 6 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0 }}
            transition={springStudio}
            className="bg-white border border-zinc-200 rounded-2xl divide-y divide-zinc-100 text-xs shadow-2xs"
          >
            {displayedBranches.map((b) => {
              const isSelected = currentBranch?.id === b.id
              return (
                <div
                  key={b.id}
                  className={`p-4 sm:p-5 flex flex-col lg:flex-row lg:items-center justify-between gap-4 transition-colors ${
                    isSelected ? 'bg-zinc-50/90' : 'hover:bg-zinc-50/40'
                  }`}
                >
                  <div className="space-y-1.5">
                    <div className="flex items-center gap-2 flex-wrap">
                      <GitBranch
                        className={`w-4 h-4 ${
                          b.is_main ? 'text-zinc-950' : 'text-zinc-500'
                        }`}
                      />
                      <span className="font-bold text-sm text-zinc-950">{b.name}</span>

                      <button
                        type="button"
                        onClick={() => copyCode(b.id)}
                        title="Copiar ID da branch"
                        className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md bg-zinc-100 hover:bg-zinc-200/70 text-zinc-600 font-mono text-[11px] cursor-pointer"
                      >
                        <span>{b.id}</span>
                        {copiedId === b.id ? (
                          <Check className="w-3 h-3 text-emerald-600" />
                        ) : (
                          <Copy className="w-3 h-3 text-zinc-400" />
                        )}
                      </button>

                      {b.is_main && (
                        <span className="px-2 py-0.5 rounded-full bg-zinc-900 text-white font-semibold text-[10px]">
                          Main (Produção)
                        </span>
                      )}

                      {b.traffic_percentage > 0 && (
                        <span className="px-2.5 py-0.5 rounded-full bg-emerald-50 text-emerald-700 border border-emerald-200 font-semibold text-[11px]">
                          {b.traffic_percentage}% do tráfego
                        </span>
                      )}

                      {b.archived && (
                        <span className="px-2 py-0.5 rounded-full bg-zinc-200 text-zinc-700 font-medium text-[10px]">
                          Arquivada
                        </span>
                      )}

                      {b.has_draft && (
                        <span className="px-2 py-0.5 rounded-full bg-amber-50 text-amber-800 border border-amber-200 font-medium text-[10px]">
                          Rascunho não publicado
                        </span>
                      )}

                      {!b.is_main && b.behind_main && !b.archived && (
                        <span className="px-2 py-0.5 rounded-full bg-sky-50 text-sky-800 border border-sky-200 font-medium text-[10px]">
                          Atrás da Main (Rebase disponível)
                        </span>
                      )}
                    </div>

                    {b.description && <p className="text-zinc-500 text-xs">{b.description}</p>}

                    <div className="flex items-center gap-3 text-[11px] text-zinc-400 font-mono">
                      <span>
                        Topo:{' '}
                        <strong className="text-zinc-700">
                          {b.head_version_id || '—'} (v{b.head_version_number || 1})
                        </strong>
                      </span>
                      {b.parent_version_id && (
                        <span>Origem: {b.parent_version_id}</span>
                      )}
                      <span>{b.versions_count} versão(ões)</span>
                    </div>
                  </div>

                  {/* Ações da Branch */}
                  <div className="flex items-center gap-1.5 flex-wrap">
                    <button
                      type="button"
                      onClick={() => {
                        onSelectVersion(null)
                        onSelectBranch(b.id)
                      }}
                      className={`px-3 py-1.5 rounded-xl text-xs font-semibold border transition-colors cursor-pointer ${
                        isSelected
                          ? 'bg-zinc-900 text-white border-zinc-900'
                          : 'bg-white hover:bg-zinc-50 text-zinc-800 border-zinc-200'
                      }`}
                    >
                      {isSelected ? 'Branch ativa no editor' : 'Abrir no editor'}
                    </button>

                    {!b.archived && (
                      <button
                        type="button"
                        onClick={() => openCreateBranchModal(b.head_version_id)}
                        title="Ramificar a partir desta branch"
                        className="px-2.5 py-1.5 rounded-xl border border-zinc-200 bg-white hover:bg-zinc-50 text-zinc-700 font-medium inline-flex items-center gap-1 cursor-pointer"
                      >
                        <GitFork className="w-3.5 h-3.5 text-zinc-500" />
                        <span>Ramificar</span>
                      </button>
                    )}

                    {!b.archived && activeBranches.length > 1 && (
                      <button
                        type="button"
                        onClick={() => openMergeModal(b)}
                        className="px-2.5 py-1.5 rounded-xl border border-zinc-200 bg-white hover:bg-zinc-50 text-zinc-800 font-medium inline-flex items-center gap-1 cursor-pointer"
                      >
                        <GitMerge className="w-3.5 h-3.5 text-indigo-600" />
                        <span>Merge</span>
                      </button>
                    )}

                    {!b.is_main && !b.archived && (
                      <button
                        type="button"
                        onClick={() => openRebaseModal(b)}
                        className="px-2.5 py-1.5 rounded-xl border border-zinc-200 bg-white hover:bg-zinc-50 text-zinc-800 font-medium inline-flex items-center gap-1 cursor-pointer"
                      >
                        <RefreshCw className="w-3.5 h-3.5 text-sky-600" />
                        <span>Rebase</span>
                      </button>
                    )}

                    {b.has_draft && (
                      <button
                        type="button"
                        onClick={() => handleDiscardDraft(b)}
                        title="Descartar rascunho não publicado nesta branch"
                        className="px-2.5 py-1.5 rounded-xl border border-amber-200 bg-amber-50 hover:bg-amber-100 text-amber-800 font-medium inline-flex items-center gap-1 cursor-pointer"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                        <span>Descartar draft</span>
                      </button>
                    )}

                    {!b.is_main && (
                      <button
                        type="button"
                        disabled={!b.archived && b.traffic_percentage > 0}
                        onClick={() => handleArchiveToggle(b)}
                        title={
                          !b.archived && b.traffic_percentage > 0
                            ? 'Remova todo o tráfego (0%) desta branch antes de arquivá-la'
                            : b.archived
                              ? 'Desarquivar branch'
                              : 'Arquivar branch'
                        }
                        className="px-2.5 py-1.5 rounded-xl border border-zinc-200 bg-white hover:bg-zinc-50 disabled:opacity-40 text-zinc-600 font-medium inline-flex items-center gap-1 cursor-pointer"
                      >
                        {b.archived ? (
                          <>
                            <ArchiveRestore className="w-3.5 h-3.5" />
                            <span>Restaurar</span>
                          </>
                        ) : (
                          <>
                            <Archive className="w-3.5 h-3.5" />
                            <span>Arquivar</span>
                          </>
                        )}
                      </button>
                    )}
                  </div>
                </div>
              )
            })}
          </motion.div>
        )}

        {/* ABA 2: HISTÓRICO DE VERSÕES IMUTÁVEIS */}
        {subTab === 'versions' && (
          <motion.div
            key="subtab-versions"
            initial={{ opacity: 0, y: 6 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0 }}
            transition={springStudio}
            className="bg-white border border-zinc-200 rounded-2xl divide-y divide-zinc-100 text-xs shadow-2xs"
          >
            {versions.length === 0 ? (
              <div className="p-10 text-center text-zinc-400">
                Nenhuma versão publicada ainda.
              </div>
            ) : (
              versions.map((v) => {
                const isViewing = viewingVersionId === v.version_id
                return (
                  <div
                    key={v.version_id || v.version}
                    className={`p-4 flex flex-col sm:flex-row sm:items-center justify-between gap-3 ${
                      isViewing ? 'bg-amber-50/50' : 'hover:bg-zinc-50/60'
                    }`}
                  >
                    <div className="space-y-1">
                      <div className="flex items-center gap-2 flex-wrap">
                        <span className="font-bold text-zinc-950">Versão v{v.version}</span>
                        {v.version_id && (
                          <button
                            type="button"
                            onClick={() => copyCode(v.version_id)}
                            className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md bg-zinc-100 hover:bg-zinc-200/70 text-zinc-700 font-mono text-[11px] cursor-pointer"
                          >
                            <span>{v.version_id}</span>
                            {copiedId === v.version_id ? (
                              <Check className="w-3 h-3 text-emerald-600" />
                            ) : (
                              <Copy className="w-3 h-3 text-zinc-400" />
                            )}
                          </button>
                        )}
                        {v.branch_name && (
                          <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-zinc-100 text-zinc-700 font-medium text-[11px]">
                            <GitBranch className="w-3 h-3" />
                            {v.branch_name}
                          </span>
                        )}
                      </div>
                      <p className="text-zinc-600">
                        {v.commit_message || 'Snapshot imutável da configuração do agente'}
                      </p>
                      <span className="text-[11px] text-zinc-400 block">
                        Publicada em {new Date(v.created_at).toLocaleString('pt-BR')}
                      </span>
                    </div>

                    <div className="flex items-center gap-2">
                      <button
                        type="button"
                        onClick={() =>
                          onSelectVersion(isViewing ? null : v.version_id || String(v.version))
                        }
                        className={`px-3 py-1.5 rounded-xl border text-xs font-semibold transition-colors cursor-pointer ${
                          isViewing
                            ? 'bg-amber-600 text-white border-amber-600'
                            : 'bg-white hover:bg-zinc-50 text-zinc-800 border-zinc-200'
                        }`}
                      >
                        {isViewing ? 'Sair da inspeção' : 'Inspecionar snapshot'}
                      </button>

                      <button
                        type="button"
                        onClick={() => openCreateBranchModal(v.version_id || String(v.version))}
                        className="px-3 py-1.5 rounded-xl border border-zinc-200 bg-white hover:bg-zinc-50 text-zinc-700 font-medium inline-flex items-center gap-1 cursor-pointer"
                      >
                        <GitFork className="w-3.5 h-3.5 text-zinc-500" />
                        <span>Criar branch daqui</span>
                      </button>
                    </div>
                  </div>
                )
              })
            )}
          </motion.div>
        )}

        {/* ABA 3: IMPLANTAÇÃO PERCENTUAL DE TRÁFEGO (TRAFFIC SPLITTING) */}
        {subTab === 'traffic' && (
          <motion.div
            key="subtab-traffic"
            initial={{ opacity: 0, y: 6 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0 }}
            transition={springStudio}
            className="bg-white border border-zinc-200 rounded-2xl p-6 space-y-6 text-xs shadow-2xs"
          >
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
              <div>
                <h3 className="text-sm font-bold text-zinc-950">
                  Distribuição determinística de tráfego (Rollout / Teste A/B)
                </h3>
                <p className="text-zinc-500 text-xs mt-0.5">
                  O roteamento usa hash determinístico do ID da conversa/chamada. A soma de todas as
                  branches ativas deve ser exatamente <strong>100%</strong>.
                </p>
              </div>

              {activeBranches.length >= 2 && (
                <div className="flex items-center gap-1.5 flex-wrap">
                  <button
                    type="button"
                    onClick={() => {
                      const next: Record<string, number> = {}
                      activeBranches.forEach((b) => {
                        next[b.id] = b.is_main ? 100 : 0
                      })
                      setTrafficDraft(next)
                    }}
                    className="px-2.5 py-1 rounded-lg border border-zinc-200 hover:bg-zinc-50 text-zinc-700 font-medium cursor-pointer"
                  >
                    100% Main
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      const main = activeBranches.find((b) => b.is_main) || activeBranches[0]
                      const second = activeBranches.find((b) => b.id !== main.id)
                      if (!second) return
                      const next: Record<string, number> = {}
                      activeBranches.forEach((b) => {
                        next[b.id] = b.id === main.id ? 90 : b.id === second.id ? 10 : 0
                      })
                      setTrafficDraft(next)
                    }}
                    className="px-2.5 py-1 rounded-lg border border-zinc-200 hover:bg-zinc-50 text-zinc-700 font-medium cursor-pointer"
                  >
                    90% / 10% Canary
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      const main = activeBranches.find((b) => b.is_main) || activeBranches[0]
                      const second = activeBranches.find((b) => b.id !== main.id)
                      if (!second) return
                      const next: Record<string, number> = {}
                      activeBranches.forEach((b) => {
                        next[b.id] = b.id === main.id ? 50 : b.id === second.id ? 50 : 0
                      })
                      setTrafficDraft(next)
                    }}
                    className="px-2.5 py-1 rounded-lg border border-zinc-200 hover:bg-zinc-50 text-zinc-700 font-medium cursor-pointer"
                  >
                    50% / 50% A/B
                  </button>
                </div>
              )}
            </div>

            <div className="space-y-3">
              {activeBranches.map((b) => {
                const val = trafficDraft[b.id] ?? b.traffic_percentage ?? 0
                return (
                  <div
                    key={b.id}
                    className="p-4 rounded-xl border border-zinc-200/80 bg-zinc-50/50 flex flex-col sm:flex-row sm:items-center justify-between gap-4"
                  >
                    <div className="space-y-0.5">
                      <div className="flex items-center gap-2">
                        <span className="font-bold text-zinc-900">{b.name}</span>
                        <span className="font-mono text-[11px] text-zinc-400">{b.id}</span>
                        {b.is_main && (
                          <span className="px-2 py-0.5 rounded-full bg-zinc-900 text-white text-[10px] font-semibold">
                            Main
                          </span>
                        )}
                      </div>
                      <span className="text-[11px] text-zinc-500 block">
                        Versão ativa: {b.head_version_id || 'v1'}
                      </span>
                    </div>

                    <div className="flex items-center gap-3 sm:w-72">
                      <input
                        type="range"
                        min={0}
                        max={100}
                        step={5}
                        value={val}
                        onChange={(e) =>
                          setTrafficDraft((prev) => ({
                            ...prev,
                            [b.id]: parseInt(e.target.value, 10) || 0,
                          }))
                        }
                        className="flex-1 accent-zinc-900"
                      />
                      <div className="flex items-center gap-1">
                        <input
                          type="number"
                          min={0}
                          max={100}
                          value={val}
                          onChange={(e) =>
                            setTrafficDraft((prev) => ({
                              ...prev,
                              [b.id]: Math.max(0, Math.min(100, parseInt(e.target.value, 10) || 0)),
                            }))
                          }
                          className="w-16 px-2.5 py-1.5 bg-white border border-zinc-200 rounded-lg font-mono text-right font-semibold text-zinc-900"
                        />
                        <span className="text-zinc-500 font-semibold">%</span>
                      </div>
                    </div>
                  </div>
                )
              })}
            </div>

            <div className="flex items-center justify-between pt-2 border-t border-zinc-100">
              <div className="flex items-center gap-2">
                <span className="text-zinc-500">Soma total alocada:</span>
                <span
                  className={`px-2.5 py-0.5 rounded-full font-mono font-bold ${
                    totalTraffic === 100
                      ? 'bg-emerald-50 text-emerald-700 border border-emerald-200'
                      : 'bg-red-50 text-red-700 border border-red-200'
                  }`}
                >
                  {totalTraffic}%
                </span>
                {totalTraffic !== 100 && (
                  <span className="text-red-600 text-[11px]">
                    (Deve somar exatamente 100% para salvar)
                  </span>
                )}
              </div>

              <Button
                onClick={handleSaveTraffic}
                disabled={savingTraffic || totalTraffic !== 100}
              >
                {savingTraffic ? 'Salvando tráfego…' : 'Implantar divisão de tráfego'}
              </Button>
            </div>
          </motion.div>
        )}
      </AnimatePresence>

      {/* MODAL 1: CRIAR NOVA BRANCH */}
      <Modal open={createBranchOpen} onClose={() => setCreateBranchOpen(false)}>
        <div className="p-6 space-y-4 text-xs">
          <h3 className="text-sm font-bold text-zinc-950">Criar nova branch</h3>
          <div>
            <label className="block font-semibold text-zinc-800 mb-1">
              Versão base (<code className="font-mono">parent_version_id</code>)
            </label>
            <select
              value={newBranchParentVer}
              onChange={(e) => setNewBranchParentVer(e.target.value)}
              className="w-full px-3 py-2 bg-zinc-50 border border-zinc-200 rounded-xl font-mono text-zinc-900"
            >
              {versions.map((v) => (
                <option key={v.version_id || v.version} value={v.version_id || String(v.version)}>
                  {v.version_id} — v{v.version} ({v.branch_name || 'Main'})
                </option>
              ))}
            </select>
            <p className="text-[11px] text-zinc-400 mt-1">
              Branches podem ser criadas a partir de qualquer versão em qualquer branch existente.
            </p>
          </div>

          <div>
            <label className="block font-semibold text-zinc-800 mb-1">
              Nome da branch (máx. 140 caracteres)
            </label>
            <input
              type="text"
              maxLength={140}
              value={newBranchName}
              onChange={(e) => setNewBranchName(e.target.value)}
              placeholder="ex: experiment/novo-fluxo-saudacao"
              className="w-full px-3 py-2 bg-zinc-50 border border-zinc-200 rounded-xl font-mono text-zinc-900"
            />
            <p className="text-[11px] text-zinc-400 mt-1">
              Permitidos: letras, números e <code className="font-mono">() [] {'{}'} - / .</code>
            </p>
          </div>

          <div>
            <label className="block font-semibold text-zinc-800 mb-1">
              Descrição / Hipótese do experimento
            </label>
            <textarea
              rows={3}
              value={newBranchDesc}
              onChange={(e) => setNewBranchDesc(e.target.value)}
              placeholder="Descreva o objetivo desta branch, métricas de sucesso e alterações planejadas..."
              className="w-full px-3 py-2 bg-zinc-50 border border-zinc-200 rounded-xl text-zinc-900"
            />
          </div>

          <div className="flex justify-end gap-2 pt-2">
            <Button variant="secondary" onClick={() => setCreateBranchOpen(false)}>
              Cancelar
            </Button>
            <Button
              onClick={handleCreateBranch}
              disabled={creatingBranch || !newBranchName.trim()}
            >
              {creatingBranch ? 'Criando…' : 'Criar branch'}
            </Button>
          </div>
        </div>
      </Modal>

      {/* MODAL 2: COMITAR NOVA VERSÃO NA BRANCH */}
      <Modal open={commitModalOpen} onClose={() => setCommitModalOpen(false)}>
        <div className="p-6 space-y-4 text-xs">
          <h3 className="text-sm font-bold text-zinc-950">
            Comitar nova versão em &quot;{currentBranch?.name || 'Main'}&quot;
          </h3>
          <p className="text-zinc-600 leading-relaxed">
            Um novo snapshot imutável (<code className="font-mono">agtvrsn_xxxx</code>) será criado
            na branch <strong>{currentBranch?.name || 'Main'}</strong> e seu rascunho atual nesta
            branch será limpo automaticamente.
          </p>
          <div>
            <label className="block font-semibold text-zinc-800 mb-1">
              Descrição da versão (opcional)
            </label>
            <input
              type="text"
              value={commitMessage}
              onChange={(e) => setCommitMessage(e.target.value)}
              placeholder="ex: Ajuste na saudação e tempo de silêncio"
              className="w-full px-3 py-2 bg-zinc-50 border border-zinc-200 rounded-xl text-zinc-900"
            />
          </div>
          <div className="flex justify-end gap-2 pt-2">
            <Button variant="secondary" onClick={() => setCommitModalOpen(false)}>
              Cancelar
            </Button>
            <Button
              disabled={publishing}
              onClick={async () => {
                setCommitModalOpen(false)
                await onPublishToBranch(
                  currentBranch?.id || 'Main',
                  commitMessage.trim() || 'Nova versão publicada',
                )
              }}
            >
              Publicar versão
            </Button>
          </div>
        </div>
      </Modal>

      {/* MODAL 3: PREVIEW & MERGE DE BRANCHES */}
      <Modal open={Boolean(mergeModalBranch)} onClose={() => setMergeModalBranch(null)}>
        <div className="p-6 space-y-4 text-xs">
          <h3 className="text-sm font-bold text-zinc-950">
            Merge da branch &quot;{mergeModalBranch?.name || ''}&quot;
          </h3>
          <div>
            <label className="block font-semibold text-zinc-800 mb-1">
              Branch de destino (<code className="font-mono">target_branch_id</code>)
            </label>
            <select
              value={mergeTargetBranchId}
              onChange={async (e) => {
                const nextTgt = e.target.value
                setMergeTargetBranchId(nextTgt)
                if (mergeModalBranch) {
                  await loadMergePreview(mergeModalBranch.id, nextTgt, mergeForce)
                }
              }}
              className="w-full px-3 py-2 bg-zinc-50 border border-zinc-200 rounded-xl text-zinc-900 font-medium"
            >
              {activeBranches
                .filter((b) => b.id !== mergeModalBranch?.id)
                .map((b) => (
                  <option key={b.id} value={b.id}>
                    {b.name} ({b.id}) {b.is_main ? '— Principal' : ''}
                  </option>
                ))}
            </select>
          </div>

          <div className="space-y-2 pt-1">
            {!mergeModalBranch?.is_main && (
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={mergeArchiveSource}
                  onChange={(e) => setMergeArchiveSource(e.target.checked)}
                  className="rounded border-zinc-300 text-zinc-900"
                />
                <span>
                  Arquivar a branch de origem após o merge (transfere o tráfego para o destino)
                </span>
              </label>
            )}

            <label className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={mergeForce}
                onChange={async (e) => {
                  const nextForce = e.target.checked
                  setMergeForce(nextForce)
                  if (mergeModalBranch) {
                    await loadMergePreview(mergeModalBranch.id, mergeTargetBranchId, nextForce)
                  }
                }}
                className="rounded border-zinc-300 text-zinc-900"
              />
              <span>
                Forçar valores da branch de origem em caso de conflito (<code className="font-mono">force=true</code>)
              </span>
            </label>
          </div>

          {loadingMergePreview && (
            <div className="p-4 rounded-xl bg-zinc-50 text-zinc-500 text-center">
              Calculando preview de alterações e conflitos…
            </div>
          )}

          {mergePreviewError && (
            <div className="p-3 rounded-xl bg-amber-50 border border-amber-200 text-amber-900">
              {mergePreviewError}
            </div>
          )}

          {mergePreview && (
            <div className="space-y-3">
              <div className="p-3 rounded-xl bg-zinc-50 border border-zinc-200 space-y-2">
                <div className="font-semibold text-zinc-900 flex items-center justify-between">
                  <span>Campos que serão atualizados ({mergePreview.overridden_fields.length})</span>
                  {mergePreview.conflicts.length > 0 && (
                    <span className="px-2 py-0.5 rounded-full bg-amber-100 text-amber-800 text-[10px]">
                      {mergePreview.conflicts.length} conflito(s) resolvido(s)
                    </span>
                  )}
                </div>
                <div className="max-h-44 overflow-y-auto divide-y divide-zinc-200/60">
                  {mergePreview.overridden_fields.map((f) => (
                    <div key={f.field} className="py-1.5 flex items-center justify-between gap-2">
                      <span className="font-medium text-zinc-800">{f.label}</span>
                      <span className="font-mono text-[10px] px-2 py-0.5 rounded bg-zinc-200/70 text-zinc-700">
                        Vencedor: {f.winner === 'source' ? mergePreview.source_branch_name : mergePreview.target_branch_name}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          )}

          <div className="flex justify-end gap-2 pt-2">
            <Button variant="secondary" onClick={() => setMergeModalBranch(null)}>
              Cancelar
            </Button>
            <Button
              onClick={handleConfirmMerge}
              disabled={merging || !mergePreview?.can_merge}
            >
              {merging ? 'Mesclando…' : 'Confirmar Merge'}
            </Button>
          </div>
        </div>
      </Modal>

      {/* MODAL 4: PREVIEW & REBASE COM A MAIN */}
      <Modal open={Boolean(rebaseModalBranch)} onClose={() => setRebaseModalBranch(null)}>
        <div className="p-6 space-y-4 text-xs">
          <h3 className="text-sm font-bold text-zinc-950">
            Rebase de &quot;{rebaseModalBranch?.name || ''}&quot; com a Main
          </h3>
          <p className="text-zinc-600 leading-relaxed">
            O Rebase puxa as últimas alterações da branch <strong>Main</strong> para esta branch,
            preservando todas as configurações que já foram editadas diretamente nesta branch.
          </p>

          {loadingRebasePreview && (
            <div className="p-4 rounded-xl bg-zinc-50 text-zinc-500 text-center">
              Calculando preview do rebase…
            </div>
          )}

          {rebasePreviewError && (
            <div className="p-3 rounded-xl bg-amber-50 border border-amber-200 text-amber-900">
              {rebasePreviewError}
            </div>
          )}

          {rebasePreview && (
            <div className="p-3 rounded-xl bg-zinc-50 border border-zinc-200 space-y-2">
              <div className="font-semibold text-zinc-900">
                 Versão da Main: <code className="font-mono">{rebasePreview.main_version_id}</code>
              </div>
              <p className="text-zinc-600 text-[11px]">
                {rebasePreview.overridden_fields.length > 0
                  ? `${rebasePreview.overridden_fields.length} campo(s) atualizados na Main serão incorporados.`
                  : 'Sincronizará o ponteiro base com o topo atual da Main.'}
              </p>
              {rebasePreview.preserved_branch_fields.length > 0 && (
                <p className="text-emerald-700 text-[11px] font-medium">
                  Preservados da branch: {rebasePreview.preserved_branch_fields.join(', ')}
                </p>
              )}
            </div>
          )}

          <div className="flex justify-end gap-2 pt-2">
            <Button variant="secondary" onClick={() => setRebaseModalBranch(null)}>
              Cancelar
            </Button>
            <Button
              onClick={handleConfirmRebase}
              disabled={rebasing || !rebasePreview?.can_rebase}
            >
              {rebasing ? 'Executando Rebase…' : 'Confirmar Rebase'}
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  )
}
