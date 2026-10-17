import { useEffect, useState } from 'react'
import {
  Sparkles,
  Wrench,
  BookOpen,
  GitBranch,
  AlertTriangle,
  Plus,
  Trash2,
  X,
  Check,
  ShieldAlert,
  ArrowUp,
  ArrowDown,
  MessageSquare,
  Volume2,
  HelpCircle,
  PhoneOff,
  RotateCcw,
  Split,
  FileCode2,
} from 'lucide-react'
import {
  type Skill,
  type Tool,
  type KnowledgeBase,
  type Agent,
  type StructuredStep,
  type ProcedureValidationError,
} from '../api'
import Modal from './Modal'
import Button from './Button'

interface ProcedureEditorModalProps {
  open: boolean
  onClose: () => void
  initialSkill?: Skill | null
  initialType?: 'free_form' | 'deterministic'
  agents: Agent[]
  defaultAgentId?: string
  lockAgent?: boolean
  tools: Tool[]
  knowledgeBases: KnowledgeBase[]
  siblingProcedures: Skill[]
  declaredVariables?: string[]
  onSave: (payload: {
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
}

const STEP_TYPE_META: Record<
  StructuredStep['type'],
  { label: string; badge: string; desc: string; icon: any }
> = {
  ask: {
    label: 'Ask (Perguntar e Aguardar)',
    badge: 'bg-blue-50 text-blue-700 border-blue-200',
    desc: 'Faz uma pergunta ao Caller e pausa aguardando a resposta.',
    icon: HelpCircle,
  },
  tell: {
    label: 'Tell (Informar)',
    badge: 'bg-emerald-50 text-emerald-700 border-emerald-200',
    desc: 'Transmite a informação com as próprias palavras e avança imediatamente.',
    icon: MessageSquare,
  },
  say: {
    label: 'Say (Falar Literalmente)',
    badge: 'bg-indigo-50 text-indigo-700 border-indigo-200',
    desc: 'Fala a frase exatamente palavra por palavra e avança.',
    icon: Volume2,
  },
  tool_call: {
    label: 'Tool (Chamar Ferramenta)',
    badge: 'bg-amber-50 text-amber-800 border-amber-200',
    desc: 'Executa uma ferramenta específica com overrides e tratamento de falha.',
    icon: Wrench,
  },
  branch: {
    label: 'If / Else (Condicional)',
    badge: 'bg-purple-50 text-purple-700 border-purple-200',
    desc: 'Avalia condições na ordem e executa os passos do primeiro ramo verdadeiro.',
    icon: Split,
  },
  sub_procedure: {
    label: 'Sub-procedure',
    badge: 'bg-cyan-50 text-cyan-700 border-cyan-200',
    desc: 'Executa outro procedure deste agente e retorna ao passo seguinte.',
    icon: GitBranch,
  },
  system_tool: {
    label: 'System Tool (Ação de Sistema)',
    badge: 'bg-rose-50 text-rose-700 border-rose-200',
    desc: 'Executa ação nativa como encerrar a chamada (end_call) ou transferir.',
    icon: PhoneOff,
  },
  retry: {
    label: 'Retry (Repetir Tool)',
    badge: 'bg-orange-50 text-orange-700 border-orange-200',
    desc: 'Tenta executar a Tool novamente (1 a 3 vezes) em caso de falha.',
    icon: RotateCcw,
  },
}

export default function ProcedureEditorModal({
  open,
  onClose,
  initialSkill,
  initialType = 'free_form',
  agents,
  defaultAgentId,
  lockAgent = false,
  tools,
  knowledgeBases,
  siblingProcedures,
  declaredVariables = [],
  onSave,
}: ProcedureEditorModalProps) {
  const [name, setName] = useState('')
  const [procType, setProcType] = useState<'free_form' | 'deterministic'>('free_form')
  const [description, setDescription] = useState('')
  const [targetAgentId, setTargetAgentId] = useState('')
  const [isSubProcedure, setIsSubProcedure] = useState(false)
  const [trigger, setTrigger] = useState('')
  const [guardrails, setGuardrails] = useState<string[]>([])
  const [guardrailInput, setGuardrailInput] = useState('')
  const [content, setContent] = useState('')
  const [steps, setSteps] = useState<StructuredStep[]>([])
  const [active, setActive] = useState(true)
  const [saving, setSaving] = useState(false)
  const [errorMsg, setErrorMsg] = useState('')
  const [showSlashMenu, setShowSlashMenu] = useState(false)
  const [showErrorsDialog, setShowErrorsDialog] = useState(false)

  useEffect(() => {
    if (!open) return
    setErrorMsg('')
    setShowSlashMenu(false)
    setShowErrorsDialog(false)

    if (initialSkill) {
      setName(initialSkill.name)
      const t = initialSkill.type === 'deterministic' ? 'deterministic' : 'free_form'
      setProcType(t)
      setDescription(initialSkill.description || '')
      setTargetAgentId(initialSkill.agent_id || defaultAgentId || agents[0]?.id || '')
      const emptyTrig = !initialSkill.trigger || initialSkill.trigger.trim() === ''
      setIsSubProcedure(emptyTrig)
      setTrigger(initialSkill.trigger || '')
      setGuardrails(initialSkill.guardrails || [])
      setActive(initialSkill.active)

      if (t === 'deterministic') {
        try {
          const parsed = JSON.parse(initialSkill.content || '{"steps":[]}')
          setSteps(Array.isArray(parsed.steps) ? parsed.steps : [])
        } catch {
          setSteps([{ type: 'ask', instruction: '' }])
        }
        setContent(initialSkill.content || '')
      } else {
        setContent(initialSkill.content || '')
        setSteps([{ type: 'ask', instruction: 'Pergunte o número do pedido.' }])
      }
    } else {
      setName('')
      setProcType(initialType)
      setDescription('')
      setTargetAgentId(defaultAgentId || agents[0]?.id || '')
      setIsSubProcedure(false)
      setTrigger('')
      setGuardrails([])
      setActive(true)
      setContent(
        '1. Peça ao cliente os dados necessários para identificar a solicitação.\n2. Consulte o sistema usando a ferramenta apropriada (digite / para inserir [tool]).\n3. Confirme o resultado de forma clara e cordial.',
      )
      setSteps([
        { type: 'ask', instruction: 'Peça ao cliente o número do protocolo ou CPF.' },
        { type: 'tell', instruction: 'Informe que você está verificando os dados no sistema.' },
      ])
    }
  }, [open, initialSkill, initialType, defaultAgentId, agents])

  // Sincroniza steps -> content JSON quando o tipo é deterministic
  useEffect(() => {
    if (procType === 'deterministic') {
      setContent(JSON.stringify({ steps }, null, 2))
    }
  }, [steps, procType])

  // Extrai referências inline e verifica se existem (validação em tempo real igual à ElevenLabs)
  const computeValidationAndRefs = () => {
    const errs: ProcedureValidationError[] = []
    const refTools: { id: string; name?: string; valid: boolean }[] = []
    const refKBs: { id: string; name?: string; valid: boolean }[] = []
    const refProcs: { id: string; name?: string; valid: boolean }[] = []
    const refSysTools: string[] = []

    if (procType === 'free_form') {
      const combined = `${isSubProcedure ? '' : trigger}\n${content}`
      const regexes = [
        /@\[(tool|procedure|knowledge|system_tool):([^\]]+)\]/g,
        /\[(tool|kb|knowledge|procedure|system_tool)\s+id="([^"]+)"\]/g,
      ]
      const seen = new Set<string>()

      for (const re of regexes) {
        let m: RegExpExecArray | null
        while ((m = re.exec(combined)) !== null) {
          const kind = m[1] === 'kb' ? 'knowledge' : m[1]
          const id = m[2]
          const key = `${kind}:${id}`
          if (seen.has(key)) continue
          seen.add(key)

          if (kind === 'tool') {
            const found = tools.find((t) => t.id === id || t.name === id)
            refTools.push({ id, name: found?.name, valid: !!found })
            if (!found) {
              errs.push({
                path: `tool[${id}]`,
                message: `Referência quebrada: a Tool "${id}" não existe ou foi removida.`,
              })
            }
          } else if (kind === 'knowledge') {
            const found = knowledgeBases.find((k) => k.id === id || k.name === id)
            refKBs.push({ id, name: found?.name, valid: !!found })
            if (!found) {
              errs.push({
                path: `kb[${id}]`,
                message: `Referência quebrada: a Base de Conhecimento "${id}" não existe.`,
              })
            }
          } else if (kind === 'procedure') {
            const found = siblingProcedures.find((p) => p.id === id || p.name === id)
            const selfRef = initialSkill && id === initialSkill.id
            refProcs.push({ id, name: found?.name, valid: !!found && !selfRef })
            if (selfRef) {
              errs.push({
                path: `procedure[${id}]`,
                message: 'Auto-referência: um Procedure não pode referenciar a si mesmo.',
              })
            } else if (!found) {
              errs.push({
                path: `procedure[${id}]`,
                message: `Referência quebrada: o Procedure "${id}" não existe neste agente.`,
              })
            }
          } else if (kind === 'system_tool') {
            refSysTools.push(id)
          }
        }
      }
    } else {
      // Validação de Structured Procedure
      if (steps.length === 0) {
        errs.push({ path: 'steps', message: 'Adicione pelo menos 1 passo ao Structured Procedure.' })
      }
      steps.forEach((st, idx) => {
        if (idx > 0 && st.type === 'branch' && steps[idx - 1].type === 'branch') {
          errs.push({
            path: `steps[${idx}]`,
            message: `Passo ${idx + 1}: Dois blocos If (Condicional) não podem ficar seguidos diretamente.`,
          })
        }
        if ((st.type === 'ask' || st.type === 'tell') && !st.instruction?.trim()) {
          errs.push({
            path: `steps[${idx}].instruction`,
            message: `Passo ${idx + 1} (${st.type.toUpperCase()}): A instrução é obrigatória.`,
          })
        }
        if (st.type === 'say' && !st.message?.trim()) {
          errs.push({
            path: `steps[${idx}].message`,
            message: `Passo ${idx + 1} (SAY): A mensagem exata é obrigatória.`,
          })
        }
        if (st.type === 'tool_call') {
          if (!st.tool_id && !st.tool_name) {
            errs.push({
              path: `steps[${idx}].tool_id`,
              message: `Passo ${idx + 1} (TOOL): Selecione qual ferramenta deve ser chamada.`,
            })
          } else {
            const found = tools.find((t) => t.id === st.tool_id || t.name === st.tool_name)
            if (found) {
              refTools.push({ id: found.id, name: found.name, valid: true })
            } else if (st.tool_id) {
              refTools.push({ id: st.tool_id, name: st.tool_name, valid: false })
              errs.push({
                path: `steps[${idx}].tool_id`,
                message: `Passo ${idx + 1} (TOOL): A ferramenta selecionada não existe mais.`,
              })
            }
          }
        }
        if (
          st.type === 'system_tool' &&
          (st.system_tool_name === 'end_call' || st.system_tool_name === 'encerrar_conversa') &&
          idx !== steps.length - 1
        ) {
          errs.push({
            path: `steps[${idx}].system_tool`,
            message: `Passo ${idx + 1}: O encerramento da chamada (end_call) deve ser o último passo da lista.`,
          })
        }
        if (st.type === 'sub_procedure') {
          if (!st.procedure_id) {
            errs.push({
              path: `steps[${idx}].procedure_id`,
              message: `Passo ${idx + 1}: Selecione o Sub-procedure de destino.`,
            })
          } else if (initialSkill && st.procedure_id === initialSkill.id) {
            errs.push({
              path: `steps[${idx}].procedure_id`,
              message: `Passo ${idx + 1}: Um Procedure não pode chamar a si mesmo.`,
            })
          }
        }
      })
    }

    return { errs, refTools, refKBs, refProcs, refSysTools }
  }

  const { errs: validationErrors, refTools, refKBs, refProcs, refSysTools } =
    computeValidationAndRefs()

  const insertReferenceToken = (token: string) => {
    setContent((prev) => {
      const cleaned = prev.endsWith('/') ? prev.slice(0, -1) : prev
      return `${cleaned}${cleaned.endsWith(' ') || cleaned.endsWith('\n') || cleaned === '' ? '' : ' '}${token} `
    })
    setShowSlashMenu(false)
  }

  const addGuardrail = () => {
    if (!guardrailInput.trim()) return
    setGuardrails([...guardrails, guardrailInput.trim()])
    setGuardrailInput('')
  }

  const addStep = (type: StructuredStep['type']) => {
    const newStep: StructuredStep = { type }
    if (type === 'ask') newStep.instruction = 'Pergunte ao cliente...'
    if (type === 'tell') newStep.instruction = 'Informe ao cliente que...'
    if (type === 'say') newStep.message = 'Sua solicitação foi registrada com sucesso.'
    if (type === 'tool_call') {
      const firstTool = tools[0]
      newStep.tool_id = firstTool?.id || ''
      newStep.tool_name = firstTool?.name || ''
      newStep.instruction = 'Chame a ferramenta com os dados informados.'
    }
    if (type === 'branch') {
      newStep.branches = [
        {
          condition: { type: 'llm', condition: 'O cliente confirmou os dados' },
          steps: [{ type: 'tell', instruction: 'Agradeça a confirmação e prossiga.' }],
        },
      ]
      newStep.fallback = [{ type: 'ask', instruction: 'Peça para o cliente repetir os dados.' }]
    }
    if (type === 'sub_procedure') {
      const other = siblingProcedures.find((p) => p.id !== initialSkill?.id)
      newStep.procedure_id = other?.id || ''
    }
    if (type === 'system_tool') {
      newStep.system_tool_name = 'end_call'
    }
    setSteps([...steps, newStep])
  }

  const updateStep = (idx: number, patch: Partial<StructuredStep>) => {
    setSteps(steps.map((s, i) => (i === idx ? { ...s, ...patch } : s)))
  }

  const moveStep = (idx: number, dir: -1 | 1) => {
    const target = idx + dir
    if (target < 0 || target >= steps.length) return
    const copy = [...steps]
    const temp = copy[idx]
    copy[idx] = copy[target]
    copy[target] = temp
    setSteps(copy)
  }

  const removeStep = (idx: number) => {
    setSteps(steps.filter((_, i) => i !== idx))
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) {
      setErrorMsg('O nome do Procedure é obrigatório.')
      return
    }
    if (!targetAgentId) {
      setErrorMsg('Selecione um agente para vincular este Procedure.')
      return
    }
    setErrorMsg('')
    setSaving(true)
    try {
      const finalContent =
        procType === 'deterministic' ? JSON.stringify({ steps }, null, 2) : content
      await onSave({
        id: initialSkill?.id,
        agent_id: targetAgentId,
        name: name.trim(),
        type: procType,
        description: description.trim(),
        trigger: isSubProcedure ? '' : trigger.trim(),
        guardrails,
        content: finalContent,
        active,
      })
      onClose()
    } catch (err: any) {
      setErrorMsg(err.message || 'Erro ao salvar procedure')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      sizeClass="max-w-4xl"
      className="max-h-[92vh] flex flex-col overflow-hidden"
    >
      {/* Header estilo ElevenLabs Procedures */}
      <div className="px-6 py-4 border-b border-zinc-200 flex items-center justify-between bg-zinc-50/70">
        <div className="flex items-center gap-2.5">
          <div className="w-8 h-8 rounded-lg bg-zinc-900 text-white flex items-center justify-center">
            <Sparkles className="w-4 h-4" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h2 className="text-sm font-semibold text-zinc-900">
                {initialSkill ? `Editar Procedure: ${initialSkill.name}` : 'Novo Procedure (ElevenLabs Style)'}
              </h2>
              {isSubProcedure && (
                <span className="text-[10px] font-semibold px-2 py-0.5 rounded-full bg-cyan-50 text-cyan-700 border border-cyan-200">
                  Sub-procedure
                </span>
              )}
            </div>
            <p className="text-[11px] text-zinc-500">
              {procType === 'free_form'
                ? 'Free-form Procedure: instruções em linguagem natural com referências inline (/)'
                : 'Structured Procedure: sequência determinística de passos tipados (Ask, Tell, Say, Tool, If)'}
            </p>
          </div>
        </div>

        <div className="flex items-center gap-2">
          {/* Errors Badge igual à ElevenLabs */}
          {validationErrors.length > 0 && (
            <button
              type="button"
              onClick={() => setShowErrorsDialog(!showErrorsDialog)}
              className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-semibold bg-red-50 text-red-700 border border-red-200 hover:bg-red-100 transition-colors"
            >
              <AlertTriangle className="w-3.5 h-3.5 text-red-600" />
              {validationErrors.length} {validationErrors.length === 1 ? 'Erro' : 'Erros'}
            </button>
          )}
          <button onClick={onClose} className="text-zinc-400 hover:text-zinc-700 p-1 rounded-md">
            <X className="w-5 h-5" />
          </button>
        </div>
      </div>

      {/* Painel expansível de Erros / Referências Quebradas */}
      {showErrorsDialog && validationErrors.length > 0 && (
        <div className="px-6 py-3 bg-red-50/90 border-b border-red-200 space-y-1.5">
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-red-900">
              Diagnóstico de Validação e Referências Quebradas:
            </span>
            <button
              type="button"
              onClick={() => setShowErrorsDialog(false)}
              className="text-[11px] text-red-700 underline"
            >
              Fechar
            </button>
          </div>
          <ul className="space-y-1 text-xs text-red-800 list-disc list-inside">
            {validationErrors.map((err, i) => (
              <li key={i}>
                <code className="font-mono text-[11px] bg-white/80 px-1 py-0.5 rounded mr-1">
                  {err.path}
                </code>
                {err.message}
              </li>
            ))}
          </ul>
        </div>
      )}

      {/* Body */}
      <form onSubmit={handleSubmit} className="p-6 overflow-y-auto overscroll-contain flex-1 space-y-5">
        {errorMsg && (
          <div className="p-3 bg-red-50 border border-red-200 rounded-lg text-xs text-red-700">
            {errorMsg}
          </div>
        )}

        {/* Seletor de Tipo (Free-form vs Structured) — na ElevenLabs é definido na criação */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 p-1 bg-zinc-100 rounded-xl">
          <button
            type="button"
            disabled={!!initialSkill}
            onClick={() => setProcType('free_form')}
            className={`py-2 px-3 rounded-lg text-xs font-semibold transition-all flex items-center justify-center gap-2 ${
              procType === 'free_form'
                ? 'bg-white text-zinc-900 shadow-xs'
                : 'text-zinc-600 hover:text-zinc-900 disabled:opacity-50'
            }`}
          >
            <Sparkles className="w-3.5 h-3.5 text-blue-600" />
            Free-form Procedure (Markdown + /)
          </button>
          <button
            type="button"
            disabled={!!initialSkill}
            onClick={() => setProcType('deterministic')}
            className={`py-2 px-3 rounded-lg text-xs font-semibold transition-all flex items-center justify-center gap-2 ${
              procType === 'deterministic'
                ? 'bg-white text-zinc-900 shadow-xs'
                : 'text-zinc-600 hover:text-zinc-900 disabled:opacity-50'
            }`}
          >
            <FileCode2 className="w-3.5 h-3.5 text-purple-600" />
            Structured Procedure (Passos Determinísticos)
          </button>
        </div>

        {/* Nome & Agente */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div className="md:col-span-2">
            <label className="block text-xs font-semibold text-zinc-700 uppercase tracking-wider mb-1">
              Name (Identificação no Dashboard — não enviado ao LLM)
            </label>
            <input
              type="text"
              required
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Ex: Solicitação de Reembolso, Verificação de Identidade"
              className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-sm text-zinc-900 focus:outline-none focus:ring-1 focus:ring-zinc-900"
            />
          </div>

          <div>
            <label className="block text-xs font-semibold text-zinc-700 uppercase tracking-wider mb-1">
              Agente
            </label>
            <select
              value={targetAgentId}
              disabled={lockAgent}
              onChange={(e) => setTargetAgentId(e.target.value)}
              className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-sm text-zinc-900 focus:outline-none focus:ring-1 focus:ring-zinc-900 disabled:bg-zinc-100"
            >
              {agents.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </select>
          </div>
        </div>

        {/* Trigger & Sub-procedure Toggle */}
        <div className="p-4 bg-zinc-50/80 border border-zinc-200/80 rounded-xl space-y-2.5">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div>
              <label className="text-xs font-semibold text-zinc-800 uppercase tracking-wider block">
                Trigger (Quando o Agent deve usar este Procedure)
              </label>
              <span className="text-[11px] text-zinc-500">
                Escreva da perspectiva do Caller (ex: "Quando o cliente pedir reembolso, estorno ou devolução de um pedido").
              </span>
            </div>

            <label className="inline-flex items-center gap-2 text-xs font-medium text-cyan-800 bg-cyan-50 border border-cyan-200 px-2.5 py-1 rounded-lg cursor-pointer">
              <input
                type="checkbox"
                checked={isSubProcedure}
                onChange={(e) => setIsSubProcedure(e.target.checked)}
                className="rounded border-cyan-300"
              />
              Sub-procedure (Trigger vazio)
            </label>
          </div>

          {isSubProcedure ? (
            <div className="p-3 bg-cyan-50/60 border border-cyan-200/70 rounded-lg text-xs text-cyan-900">
              <strong>Modo Sub-procedure ativo:</strong> Este procedure tem trigger vazio e não disputa atenção no início da conversa. Ele só será executado quando outro Procedure chamá-lo via referência inline <code className="font-mono bg-white px-1 rounded">[procedure id="..."]</code> ou bloco Sub-procedure.
            </div>
          ) : (
            <textarea
              rows={2}
              value={trigger}
              onChange={(e) => setTrigger(e.target.value)}
              placeholder="Ex: Quando o cliente pedir 2ª via de boleto, código de barras ou informar que a fatura venceu."
              className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-xs text-zinc-900 focus:outline-none focus:ring-1 focus:ring-zinc-900"
            />
          )}
        </div>

        {/* Conteúdo: Free-form vs Structured */}
        {procType === 'free_form' ? (
          <div className="space-y-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div>
                <label className="text-xs font-semibold text-zinc-700 uppercase tracking-wider block">
                  Content (Instruções em Linguagem Natural & Inline References)
                </label>
                <span className="text-[11px] text-zinc-500">
                  Digite <kbd className="px-1.5 py-0.5 bg-zinc-100 border border-zinc-300 rounded font-mono text-[10px]">/</kbd> ou clique em <strong>Inserir Referência</strong> para embutir Tools, Knowledge Bases, Sub-procedures ou System Tools.
                </span>
              </div>

              <div className="relative">
                <button
                  type="button"
                  onClick={() => setShowSlashMenu(!showSlashMenu)}
                  className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-zinc-900 hover:bg-zinc-800 text-white text-xs font-medium transition-colors"
                >
                  <Plus className="w-3.5 h-3.5" />
                  Inserir Referência (/)
                </button>

                {/* Slash Menu Dropdown categorizado estilo ElevenLabs */}
                {showSlashMenu && (
                  <div className="absolute right-0 mt-1.5 w-80 max-h-80 overflow-y-auto bg-white border border-zinc-200 rounded-xl shadow-xl z-50 p-2 space-y-2 text-xs">
                    <div className="flex items-center justify-between px-2 py-1 border-b border-zinc-100">
                      <span className="font-semibold text-zinc-700">Menu de Recursos Inline (/)</span>
                      <button
                        type="button"
                        onClick={() => setShowSlashMenu(false)}
                        className="text-zinc-400 hover:text-zinc-600"
                      >
                        <X className="w-3.5 h-3.5" />
                      </button>
                    </div>

                    {/* Tools */}
                    <div>
                      <span className="px-2 text-[10px] font-bold uppercase text-zinc-400 tracking-wider block mb-1">
                        Tools (Auto-anexadas ao Procedure)
                      </span>
                      {tools.length === 0 ? (
                        <p className="px-2 py-1 text-[11px] text-zinc-400 italic">Nenhuma Tool cadastrada</p>
                      ) : (
                        tools.map((t) => (
                          <button
                            key={t.id}
                            type="button"
                            onClick={() => insertReferenceToken(`[tool id="${t.id}"]`)}
                            className="w-full text-left px-2.5 py-1.5 rounded-lg hover:bg-amber-50 flex items-center justify-between group"
                          >
                            <span className="flex items-center gap-1.5 font-mono text-zinc-800 group-hover:text-amber-900">
                              <Wrench className="w-3.5 h-3.5 text-amber-600" />
                              {t.name}
                            </span>
                            <span className="text-[10px] text-zinc-400">Tool</span>
                          </button>
                        ))
                      )}
                    </div>

                    {/* Knowledge Bases */}
                    <div>
                      <span className="px-2 text-[10px] font-bold uppercase text-zinc-400 tracking-wider block mb-1">
                        Knowledge Base Documents
                      </span>
                      {knowledgeBases.length === 0 ? (
                        <p className="px-2 py-1 text-[11px] text-zinc-400 italic">Nenhuma KB cadastrada</p>
                      ) : (
                        knowledgeBases.map((kb) => (
                          <button
                            key={kb.id}
                            type="button"
                            onClick={() => insertReferenceToken(`[kb id="${kb.id}"]`)}
                            className="w-full text-left px-2.5 py-1.5 rounded-lg hover:bg-blue-50 flex items-center justify-between group"
                          >
                            <span className="flex items-center gap-1.5 text-zinc-800 group-hover:text-blue-900">
                              <BookOpen className="w-3.5 h-3.5 text-blue-600" />
                              {kb.name}
                            </span>
                            <span className="text-[10px] text-zinc-400">KB</span>
                          </button>
                        ))
                      )}
                    </div>

                    {/* Sub-procedures / Procedures */}
                    <div>
                      <span className="px-2 text-[10px] font-bold uppercase text-zinc-400 tracking-wider block mb-1">
                        Procedures / Sub-procedures do Agente
                      </span>
                      {siblingProcedures.filter((p) => p.id !== initialSkill?.id).length === 0 ? (
                        <p className="px-2 py-1 text-[11px] text-zinc-400 italic">
                          Nenhum outro procedure neste agente
                        </p>
                      ) : (
                        siblingProcedures
                          .filter((p) => p.id !== initialSkill?.id)
                          .map((p) => (
                            <button
                              key={p.id}
                              type="button"
                              onClick={() => insertReferenceToken(`[procedure id="${p.id}"]`)}
                              className="w-full text-left px-2.5 py-1.5 rounded-lg hover:bg-cyan-50 flex items-center justify-between group"
                            >
                              <span className="flex items-center gap-1.5 text-zinc-800 group-hover:text-cyan-900">
                                <GitBranch className="w-3.5 h-3.5 text-cyan-600" />
                                {p.name}
                              </span>
                              <span className="text-[10px] text-cyan-600">
                                {!p.trigger ? 'Sub-proc' : 'Proc'}
                              </span>
                            </button>
                          ))
                      )}
                    </div>

                    {/* System Tools */}
                    <div>
                      <span className="px-2 text-[10px] font-bold uppercase text-zinc-400 tracking-wider block mb-1">
                        System Tools
                      </span>
                      <button
                        type="button"
                        onClick={() => insertReferenceToken(`[system_tool id="end_call"]`)}
                        className="w-full text-left px-2.5 py-1.5 rounded-lg hover:bg-rose-50 flex items-center justify-between"
                      >
                        <span className="flex items-center gap-1.5 font-mono text-rose-800">
                          <PhoneOff className="w-3.5 h-3.5 text-rose-600" />
                          end_call (encerrar_conversa)
                        </span>
                        <span className="text-[10px] text-rose-500">System</span>
                      </button>
                      <button
                        type="button"
                        onClick={() => insertReferenceToken(`[system_tool id="transferir_numero"]`)}
                        className="w-full text-left px-2.5 py-1.5 rounded-lg hover:bg-rose-50 flex items-center justify-between"
                      >
                        <span className="flex items-center gap-1.5 font-mono text-rose-800">
                          <PhoneOff className="w-3.5 h-3.5 text-rose-600" />
                          transferir_numero
                        </span>
                        <span className="text-[10px] text-rose-500">System</span>
                      </button>
                    </div>

                    {/* Dynamic Variables */}
                    {declaredVariables.length > 0 && (
                      <div>
                        <span className="px-2 text-[10px] font-bold uppercase text-zinc-400 tracking-wider block mb-1">
                          Dynamic Variables
                        </span>
                        {declaredVariables.map((v) => (
                          <button
                            key={v}
                            type="button"
                            onClick={() => insertReferenceToken(`{{${v}}}`)}
                            className="w-full text-left px-2.5 py-1.5 rounded-lg hover:bg-zinc-100 font-mono text-zinc-800"
                          >
                            {`{{${v}}}`}
                          </button>
                        ))}
                      </div>
                    )}
                  </div>
                )}
              </div>
            </div>

            <textarea
              rows={9}
              value={content}
              onChange={(e) => {
                const val = e.target.value
                setContent(val)
                if (val.endsWith('/')) {
                  setShowSlashMenu(true)
                }
              }}
              placeholder='Ex: 1. Peça o número do pedido ao cliente.\n2. Consulte os detalhes usando [tool id="..."].\n3. Caso precise escalar, siga [procedure id="..."] ou finalize com [system_tool id="end_call"].'
              className="w-full p-3.5 bg-zinc-950 text-zinc-100 rounded-xl text-xs font-mono leading-relaxed focus:outline-none focus:ring-2 focus:ring-zinc-700"
            />

            {/* Barra de Tags Clicáveis de Recursos Inline (igual às tags com chave inglesa da ElevenLabs) */}
            {(refTools.length > 0 || refKBs.length > 0 || refProcs.length > 0 || refSysTools.length > 0) && (
              <div className="p-3 bg-zinc-50 border border-zinc-200 rounded-xl space-y-2">
                <span className="text-[11px] font-semibold text-zinc-600 block">
                  Recursos Anexados Inline neste Procedure:
                </span>
                <div className="flex flex-wrap gap-1.5">
                  {refTools.map((rt) => (
                    <span
                      key={rt.id}
                      className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-mono border ${
                        rt.valid
                          ? 'bg-amber-50 text-amber-900 border-amber-200'
                          : 'bg-red-50 text-red-700 border-red-300'
                      }`}
                    >
                      <Wrench className="w-3 h-3" />
                      {rt.name || rt.id}
                      {!rt.valid && <span className="text-[10px] font-sans font-bold">(Inválida)</span>}
                    </span>
                  ))}
                  {refKBs.map((rk) => (
                    <span
                      key={rk.id}
                      className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs border ${
                        rk.valid
                          ? 'bg-blue-50 text-blue-900 border-blue-200'
                          : 'bg-red-50 text-red-700 border-red-300'
                      }`}
                    >
                      <BookOpen className="w-3 h-3" />
                      {rk.name || rk.id}
                      {!rk.valid && <span className="text-[10px] font-bold">(Inválida)</span>}
                    </span>
                  ))}
                  {refProcs.map((rp) => (
                    <span
                      key={rp.id}
                      className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs border ${
                        rp.valid
                          ? 'bg-cyan-50 text-cyan-900 border-cyan-200'
                          : 'bg-red-50 text-red-700 border-red-300'
                      }`}
                    >
                      <GitBranch className="w-3 h-3" />
                      {rp.name || rp.id}
                      {!rp.valid && <span className="text-[10px] font-bold">(Inválido)</span>}
                    </span>
                  ))}
                  {refSysTools.map((st) => (
                    <span
                      key={st}
                      className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-mono bg-rose-50 text-rose-800 border border-rose-200"
                    >
                      <PhoneOff className="w-3 h-3" />
                      {st}
                    </span>
                  ))}
                </div>
              </div>
            )}
          </div>
        ) : (
          /* Editor Visual de Structured Procedure (type: deterministic) */
          <div className="space-y-4">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div>
                <label className="text-xs font-semibold text-zinc-800 uppercase tracking-wider block">
                  Steps (Sequência Ordenada Determinística)
                </label>
                <span className="text-[11px] text-zinc-500">
                  Apenas passos <strong>Ask</strong> aguardam resposta do Caller. <strong>Tell</strong> e <strong>Say</strong> falam e avançam imediatamente.
                </span>
              </div>
            </div>

            <div className="space-y-3">
              {steps.map((st, idx) => {
                const meta = STEP_TYPE_META[st.type] || STEP_TYPE_META.tell
                const Icon = meta.icon
                return (
                  <div
                    key={idx}
                    className="p-4 bg-white border border-zinc-200 rounded-xl shadow-2xs space-y-3"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <div className="flex items-center gap-2">
                        <span className="w-6 h-6 rounded-full bg-zinc-900 text-white text-xs font-bold flex items-center justify-center">
                          {idx + 1}
                        </span>
                        <span
                          className={`inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-semibold border ${meta.badge}`}
                        >
                          <Icon className="w-3.5 h-3.5" />
                          {meta.label}
                        </span>
                      </div>

                      <div className="flex items-center gap-1">
                        <button
                          type="button"
                          onClick={() => moveStep(idx, -1)}
                          disabled={idx === 0}
                          className="p-1 text-zinc-400 hover:text-zinc-700 disabled:opacity-30"
                          title="Mover para cima"
                        >
                          <ArrowUp className="w-4 h-4" />
                        </button>
                        <button
                          type="button"
                          onClick={() => moveStep(idx, 1)}
                          disabled={idx === steps.length - 1}
                          className="p-1 text-zinc-400 hover:text-zinc-700 disabled:opacity-30"
                          title="Mover para baixo"
                        >
                          <ArrowDown className="w-4 h-4" />
                        </button>
                        <button
                          type="button"
                          onClick={() => removeStep(idx)}
                          className="p-1 text-zinc-400 hover:text-red-600"
                          title="Remover passo"
                        >
                          <Trash2 className="w-4 h-4" />
                        </button>
                      </div>
                    </div>

                    {/* Campos específicos por tipo de Step */}
                    {(st.type === 'ask' || st.type === 'tell') && (
                      <div>
                        <label className="block text-[11px] font-medium text-zinc-600 mb-1">
                          Instrução para o Agent ({st.type === 'ask' ? 'pergunta que aguarda resposta' : 'fala com suas próprias palavras'})
                        </label>
                        <input
                          type="text"
                          value={st.instruction || ''}
                          onChange={(e) => updateStep(idx, { instruction: e.target.value })}
                          placeholder={
                            st.type === 'ask'
                              ? 'Ex: Pergunte ao cliente o número do pedido (8 dígitos).'
                              : 'Ex: Explique que o reembolso leva de 5 a 10 dias úteis.'
                          }
                          className="w-full px-3 py-2 bg-zinc-50 border border-zinc-200 rounded-lg text-xs text-zinc-900"
                        />
                      </div>
                    )}

                    {st.type === 'say' && (
                      <div>
                        <label className="block text-[11px] font-medium text-zinc-600 mb-1">
                          Mensagem Exata (o Agent falará literalmente palavra por palavra)
                        </label>
                        <input
                          type="text"
                          value={st.message || ''}
                          onChange={(e) => updateStep(idx, { message: e.target.value })}
                          placeholder="Ex: Sua solicitação de reembolso foi registrada com sucesso."
                          className="w-full px-3 py-2 bg-zinc-50 border border-zinc-200 rounded-lg text-xs text-zinc-900"
                        />
                      </div>
                    )}

                    {st.type === 'tool_call' && (
                      <div className="space-y-3 pt-1">
                        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                          <div>
                            <label className="block text-[11px] font-medium text-zinc-600 mb-1">
                              Ferramenta (Tool)
                            </label>
                            <select
                              value={st.tool_id || ''}
                              onChange={(e) => {
                                const found = tools.find((t) => t.id === e.target.value)
                                updateStep(idx, {
                                  tool_id: e.target.value,
                                  tool_name: found?.name || '',
                                })
                              }}
                              className="w-full px-3 py-1.5 bg-zinc-50 border border-zinc-200 rounded-lg text-xs text-zinc-900"
                            >
                              <option value="">Selecione uma Tool...</option>
                              {tools.map((t) => (
                                <option key={t.id} value={t.id}>
                                  @{t.name} — {t.description}
                                </option>
                              ))}
                            </select>
                          </div>
                          <div>
                            <label className="block text-[11px] font-medium text-zinc-600 mb-1">
                              Instrução de Chamada
                            </label>
                            <input
                              type="text"
                              value={st.instruction || ''}
                              onChange={(e) => updateStep(idx, { instruction: e.target.value })}
                              placeholder="Ex: Consulte o pedido usando o ID informado pelo cliente."
                              className="w-full px-3 py-1.5 bg-zinc-50 border border-zinc-200 rounded-lg text-xs text-zinc-900"
                            />
                          </div>
                        </div>

                        {/* Tratamento de Falha (on_failure + Retry) */}
                        <div className="p-3 bg-amber-50/50 border border-amber-200/70 rounded-lg space-y-2">
                          <div className="flex items-center justify-between">
                            <span className="text-[11px] font-semibold text-amber-900">
                              Failure Handler (on_failure & Retry automático)
                            </span>
                            <label className="inline-flex items-center gap-1.5 text-[11px] text-amber-900 cursor-pointer">
                              <input
                                type="checkbox"
                                checked={!!st.on_failure}
                                onChange={(e) => {
                                  if (e.target.checked) {
                                    updateStep(idx, {
                                      on_failure: {
                                        fallback: [
                                          {
                                            type: 'tell',
                                            instruction:
                                              'Peça desculpas pela instabilidade e avise que tentará novamente.',
                                          },
                                          { type: 'retry', max_retries: 1 },
                                        ],
                                      },
                                    })
                                  } else {
                                    updateStep(idx, { on_failure: undefined })
                                  }
                                }}
                              />
                              Habilitar recuperação de falha
                            </label>
                          </div>

                          {st.on_failure && (
                            <div className="grid grid-cols-1 sm:grid-cols-3 gap-2 pt-1">
                              <div className="sm:col-span-2">
                                <label className="block text-[10px] text-amber-800 mb-0.5">
                                  Mensagem ao Caller em caso de falha:
                                </label>
                                <input
                                  type="text"
                                  value={st.on_failure.fallback[0]?.instruction || ''}
                                  onChange={(e) => {
                                    const currentRetries =
                                      st.on_failure?.fallback.find((f) => f.type === 'retry')
                                        ?.max_retries ?? 1
                                    updateStep(idx, {
                                      on_failure: {
                                        fallback: [
                                          { type: 'tell', instruction: e.target.value },
                                          ...(currentRetries > 0
                                            ? [{ type: 'retry' as const, max_retries: currentRetries }]
                                            : []),
                                        ],
                                      },
                                    })
                                  }}
                                  className="w-full px-2.5 py-1 bg-white border border-amber-200 rounded text-xs"
                                />
                              </div>
                              <div>
                                <label className="block text-[10px] text-amber-800 mb-0.5">
                                  Tentativas Extras (Retry 1-3):
                                </label>
                                <select
                                  value={
                                    st.on_failure.fallback.find((f) => f.type === 'retry')
                                      ?.max_retries ?? 1
                                  }
                                  onChange={(e) => {
                                    const r = parseInt(e.target.value)
                                    const tellStep = st.on_failure?.fallback[0] || {
                                      type: 'tell' as const,
                                      instruction: 'Tentando novamente.',
                                    }
                                    updateStep(idx, {
                                      on_failure: {
                                        fallback:
                                          r > 0
                                            ? [tellStep, { type: 'retry', max_retries: r }]
                                            : [tellStep],
                                      },
                                    })
                                  }}
                                  className="w-full px-2.5 py-1 bg-white border border-amber-200 rounded text-xs"
                                >
                                  <option value={0}>Sem Retry (apenas avisa)</option>
                                  <option value={1}>1 tentativa extra (Retry)</option>
                                  <option value={2}>2 tentativas extras</option>
                                  <option value={3}>3 tentativas extras</option>
                                </select>
                              </div>
                            </div>
                          )}
                        </div>
                      </div>
                    )}

                    {st.type === 'branch' && (
                      <div className="space-y-2.5 p-3 bg-purple-50/40 border border-purple-200/70 rounded-lg">
                        <div>
                          <label className="block text-[11px] font-semibold text-purple-900 mb-1">
                            IF (Condição em linguagem natural avaliada pelo modelo):
                          </label>
                          <input
                            type="text"
                            value={st.branches?.[0]?.condition?.condition || ''}
                            onChange={(e) => {
                              const currentStepText =
                                st.branches?.[0]?.steps?.[0]?.instruction ||
                                'Explique os próximos passos.'
                              updateStep(idx, {
                                branches: [
                                  {
                                    condition: { type: 'llm', condition: e.target.value },
                                    steps: [{ type: 'tell', instruction: currentStepText }],
                                  },
                                ],
                              })
                            }}
                            placeholder="Ex: O cliente informou que o pedido já foi enviado."
                            className="w-full px-2.5 py-1.5 bg-white border border-purple-200 rounded text-xs"
                          />
                        </div>
                        <div>
                          <label className="block text-[11px] font-medium text-purple-800 mb-1">
                            Então (Passo executado se verdadeiro):
                          </label>
                          <input
                            type="text"
                            value={st.branches?.[0]?.steps?.[0]?.instruction || ''}
                            onChange={(e) => {
                              const currentCond = st.branches?.[0]?.condition?.condition || ''
                              updateStep(idx, {
                                branches: [
                                  {
                                    condition: { type: 'llm', condition: currentCond },
                                    steps: [{ type: 'tell', instruction: e.target.value }],
                                  },
                                ],
                              })
                            }}
                            placeholder="Ex: Informe que pedidos enviados precisam ser devolvidos antes do estorno."
                            className="w-full px-2.5 py-1.5 bg-white border border-purple-200 rounded text-xs"
                          />
                        </div>
                        <div>
                          <label className="block text-[11px] font-medium text-purple-800 mb-1">
                            ELSE / Fallback (Executado se a condição não for atendida):
                          </label>
                          <input
                            type="text"
                            value={st.fallback?.[0]?.instruction || ''}
                            onChange={(e) =>
                              updateStep(idx, {
                                fallback: e.target.value
                                  ? [{ type: 'tell', instruction: e.target.value }]
                                  : [],
                              })
                            }
                            placeholder="Ex: Prossiga com o cancelamento imediato."
                            className="w-full px-2.5 py-1.5 bg-white border border-purple-200 rounded text-xs"
                          />
                        </div>
                      </div>
                    )}

                    {st.type === 'sub_procedure' && (
                      <div>
                        <label className="block text-[11px] font-medium text-zinc-600 mb-1">
                          Procedure / Sub-procedure de Destino
                        </label>
                        <select
                          value={st.procedure_id || ''}
                          onChange={(e) => updateStep(idx, { procedure_id: e.target.value })}
                          className="w-full px-3 py-1.5 bg-zinc-50 border border-zinc-200 rounded-lg text-xs text-zinc-900"
                        >
                          <option value="">Selecione um Sub-procedure deste agente...</option>
                          {siblingProcedures
                            .filter((p) => p.id !== initialSkill?.id)
                            .map((p) => (
                              <option key={p.id} value={p.id}>
                                {p.name} ({!p.trigger ? 'Sub-procedure' : 'Entry Procedure'})
                              </option>
                            ))}
                        </select>
                      </div>
                    )}

                    {st.type === 'system_tool' && (
                      <div>
                        <label className="block text-[11px] font-medium text-zinc-600 mb-1">
                          Ação de Sistema
                        </label>
                        <select
                          value={st.system_tool_name || 'end_call'}
                          onChange={(e) => updateStep(idx, { system_tool_name: e.target.value })}
                          className="w-full px-3 py-1.5 bg-zinc-50 border border-zinc-200 rounded-lg text-xs text-zinc-900"
                        >
                          <option value="end_call">end_call (Encerrar chamada — deve ser o último passo)</option>
                          <option value="transferir_numero">transferir_numero (Transferir para humano)</option>
                        </select>
                      </div>
                    )}
                  </div>
                )
              })}
            </div>

            {/* Barra de Adição de Novos Passos Tipados (igual ao menu de Steps da ElevenLabs) */}
            <div className="p-3 bg-zinc-50 border border-dashed border-zinc-300 rounded-xl">
              <span className="text-[11px] font-semibold text-zinc-600 block mb-2">
                + Adicionar Passo ao Structured Procedure:
              </span>
              <div className="flex flex-wrap gap-1.5">
                {(['ask', 'tell', 'say', 'tool_call', 'branch', 'sub_procedure', 'system_tool'] as const).map(
                  (t) => {
                    const m = STEP_TYPE_META[t]
                    const Icon = m.icon
                    return (
                      <button
                        key={t}
                        type="button"
                        onClick={() => addStep(t)}
                        className="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg bg-white hover:bg-zinc-100 border border-zinc-200 text-xs font-medium text-zinc-800 shadow-2xs transition-colors"
                      >
                        <Icon className="w-3.5 h-3.5 text-zinc-600" />
                        {m.label.split(' ')[0]}
                      </button>
                    )
                  },
                )}
              </div>
            </div>
          </div>
        )}

        {/* Guardrails opcionais */}
        <div>
          <label className="block text-xs font-semibold text-zinc-700 uppercase tracking-wider mb-1">
            Guardrails Adicionais (Diretrizes de Alta Prioridade enquanto ativo)
          </label>
          <div className="flex gap-2 mb-2">
            <input
              type="text"
              value={guardrailInput}
              onChange={(e) => setGuardrailInput(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && (e.preventDefault(), addGuardrail())}
              placeholder="Ex: Nunca confirme estorno sem antes validar os últimos 4 dígitos do CPF"
              className="flex-1 px-3 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs text-zinc-900"
            />
            <button
              type="button"
              onClick={addGuardrail}
              className="px-3 py-1.5 bg-zinc-100 hover:bg-zinc-200 text-zinc-800 text-xs font-medium rounded-lg"
            >
              Adicionar
            </button>
          </div>
          {guardrails.length > 0 && (
            <div className="flex flex-wrap gap-1.5">
              {guardrails.map((g, i) => (
                <span
                  key={i}
                  className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs bg-amber-50 text-amber-800 border border-amber-200"
                >
                  <ShieldAlert className="w-3 h-3 text-amber-600" />
                  {g}
                  <button
                    type="button"
                    onClick={() => setGuardrails(guardrails.filter((_, idx) => idx !== i))}
                    className="hover:text-amber-950"
                  >
                    <X className="w-3 h-3" />
                  </button>
                </span>
              ))}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between pt-4 border-t border-zinc-200">
          <label className="inline-flex items-center gap-2 text-xs text-zinc-700 cursor-pointer">
            <input
              type="checkbox"
              checked={active}
              onChange={(e) => setActive(e.target.checked)}
              className="rounded border-zinc-300"
            />
            Procedure habilitado no Draft do Agente
          </label>

          <div className="flex items-center gap-2">
            <Button type="button" variant="secondary" onClick={onClose}>
              Cancelar
            </Button>
            <Button type="submit" disabled={saving || !name.trim()}>
              <Check className="w-4 h-4" />
              {saving ? 'Salvando…' : 'Salvar Procedure'}
            </Button>
          </div>
        </div>
      </form>
    </Modal>
  )
}
