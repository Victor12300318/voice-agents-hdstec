import { useState } from 'react'
import { Maximize2, Globe, X, Braces, BookOpen, Wand2, ArrowUpRight } from 'lucide-react'
import Modal from './Modal'
import Toggle from './Toggle'
import { type DeclaredVariable } from '../api'

export interface VariableCatalogItem {
  name: string
  category: 'system' | 'dynamic'
  description: string
  example: string
}

export const SYSTEM_VARIABLES_CATALOG: VariableCatalogItem[] = [
  {
    name: 'horario',
    category: 'system',
    description: 'Horário/saudação atual calculado no fuso horário configurado no agente.',
    example: 'boa tarde',
  },
  {
    name: 'primNome',
    category: 'dynamic',
    description: 'Primeiro nome do cliente retornado pelo Initiation Webhook ou contexto.',
    example: 'Carlos',
  },
  {
    name: 'data',
    category: 'system',
    description: 'Data atual por extenso no fuso horário do agente.',
    example: '15/10/2026',
  },
  {
    name: 'id_conversa',
    category: 'system',
    description: 'Identificador único (UUID) da conversa em andamento.',
    example: 'conv_9401m3s8tj69etmapa6khbgpapvz',
  },
  {
    name: 'system__agent_id',
    category: 'system',
    description: 'Identificador único do agente que iniciou a conversa.',
    example: 'agent_8401kp8bt0jdf14rzzcv5veq94yg',
  },
  {
    name: 'system__caller_id',
    category: 'system',
    description: 'Número de telefone (E.164) ou identificador de origem do Caller.',
    example: '+5511999998888',
  },
  {
    name: 'system__called_number',
    category: 'system',
    description: 'Número DID (Twilio ou SIP) discado pelo cliente para entrar em contato.',
    example: '+551140041234',
  },
  {
    name: 'system__call_duration_secs',
    category: 'system',
    description: 'Duração acumulada da chamada em segundos.',
    example: '142',
  },
  {
    name: 'system__timezone',
    category: 'system',
    description: 'Fuso horário IANA configurado para o agente.',
    example: 'America/Sao_Paulo',
  },
  {
    name: 'system__call_sid',
    category: 'system',
    description: 'CallSid da operadora telefônica (Twilio Media Streams ou Jambonz SIP).',
    example: 'CA8901k4t9z5defmb8vh3e9361y7nj',
  },
]

interface VariablesMasterDetailPopoverProps {
  open: boolean
  onClose: () => void
  declaredVariables: DeclaredVariable[]
  onSelectVariable: (varName: string) => void
}

export function VariablesMasterDetailPopover({
  open,
  onClose,
  declaredVariables,
  onSelectVariable,
}: VariablesMasterDetailPopoverProps) {
  const existingNames = new Set(SYSTEM_VARIABLES_CATALOG.map((x) => x.name))
  const customVars: VariableCatalogItem[] = declaredVariables
    .filter((d) => !existingNames.has(d.name))
    .map((d) => ({
      name: d.name,
      category: 'dynamic' as const,
      description:
        d.description || 'Variável dinâmica retornada pelo Initiation Webhook no início da chamada.',
      example: `valor_${d.name}`,
    }))

  const allVars: VariableCatalogItem[] = [...customVars, ...SYSTEM_VARIABLES_CATALOG]
  const [hovered, setHovered] = useState<VariableCatalogItem>(
    allVars[0] || SYSTEM_VARIABLES_CATALOG[0],
  )

  if (!open) return null

  return (
    <div className="absolute left-3 bottom-12 z-50 w-[460px] max-w-[92vw] bg-white border border-zinc-200/90 rounded-2xl shadow-2xl overflow-hidden grid grid-cols-12 text-xs animate-in fade-in zoom-in-95 duration-100">
      {/* Coluna Esquerda: Lista de Variáveis */}
      <div className="col-span-6 border-r border-zinc-100 max-h-72 overflow-y-auto p-1.5 space-y-0.5">
        <div className="px-2.5 py-1 flex items-center justify-between">
          <span className="text-[11px] font-medium text-zinc-400">Variáveis do sistema</span>
          <button
            type="button"
            onClick={onClose}
            className="text-zinc-400 hover:text-zinc-700"
          >
            <X className="w-3.5 h-3.5" />
          </button>
        </div>

        {allVars.map((v) => {
          const isSelected = hovered?.name === v.name
          return (
            <button
              key={v.name}
              type="button"
              onMouseEnter={() => setHovered(v)}
              onClick={() => {
                onSelectVariable(v.name)
                onClose()
              }}
              className={`w-full text-left px-2.5 py-1.5 rounded-lg font-mono text-[11px] transition-colors truncate block ${
                isSelected
                  ? 'bg-zinc-100 text-zinc-950 font-semibold'
                  : 'text-zinc-700 hover:bg-zinc-50'
              }`}
            >
              {v.name}
            </button>
          )
        })}
      </div>

      {/* Coluna Direita: Detalhes + Exemplo */}
      <div className="col-span-6 p-4 bg-zinc-50/40 flex flex-col justify-between space-y-3">
        {hovered && (
          <div className="space-y-2.5">
            <div className="font-mono font-bold text-xs text-zinc-900 break-all">
              {hovered.name}
            </div>
            <p className="text-xs text-zinc-500 leading-relaxed">{hovered.description}</p>
            <div className="pt-1 space-y-1">
              <span className="text-[11px] text-zinc-400 block">Exemplo</span>
              <code className="block px-2 py-1 bg-zinc-100 border border-zinc-200/80 rounded font-mono text-[11px] text-zinc-800 break-all">
                {hovered.example}
              </code>
            </div>
          </div>
        )}

        <div className="text-[10px] text-zinc-400 border-t border-zinc-200/60 pt-2">
          Clique para inserir <code className="font-mono">{`{{${hovered?.name}}}`}</code>
        </div>
      </div>
    </div>
  )
}

interface RichPromptCardProps {
  title: string
  subtitle?: string
  showBookIcon?: boolean
  showWandButton?: boolean
  showDisclosureLink?: boolean
  shadedBg?: boolean
  value: string
  onChange: (next: string) => void
  placeholder?: string
  rows?: number
  declaredVariables: DeclaredVariable[]
  timezone?: string
  onTimezoneChange?: (tz: string) => void
  interruptible?: boolean
  onInterruptibleChange?: (val: boolean) => void
}

// Renderiza o texto destacando [super fast] em pílula magenta/rosa e {{variavel}} em pílula cinza-claro mono (idêntico às Imagens 1 e 2!)
function renderHighlightedTokens(text: string) {
  const parts = text.split(/(\{\{[^}]+\}\}|\[[^\]]+\])/g)
  return parts.map((p, idx) => {
    if (p.startsWith('{{') && p.endsWith('}}')) {
      return (
        <span
          key={idx}
          className="inline-flex items-center px-1.5 py-0.5 mx-0.5 rounded-md bg-zinc-100 text-zinc-900 font-mono text-xs font-semibold border border-zinc-200/90"
        >
          {p}
        </span>
      )
    }
    if (p.startsWith('[') && p.endsWith(']')) {
      return (
        <span
          key={idx}
          className="inline-flex items-center px-1.5 py-0.5 mx-0.5 rounded-md bg-fuchsia-50 text-fuchsia-600 font-mono text-xs font-medium border border-fuchsia-200/80"
        >
          {p}
        </span>
      )
    }
    return <span key={idx}>{p}</span>
  })
}

export function RichPromptCard({
  title,
  subtitle,
  showBookIcon = false,
  showWandButton = false,
  showDisclosureLink = false,
  shadedBg = false,
  value,
  onChange,
  placeholder,
  rows = 8,
  declaredVariables,
  timezone,
  onTimezoneChange,
  interruptible,
  onInterruptibleChange,
}: RichPromptCardProps) {
  const [varPopoverOpen, setVarPopoverOpen] = useState(false)
  const [expandedOpen, setExpandedOpen] = useState(false)
  const [editorMode, setEditorMode] = useState<'visual' | 'simples'>('visual')
  const [editingInline, setEditingInline] = useState(false)

  // Combina as variáveis declaradas com quaisquer {{variaveis}} já escritas no texto
  const mergedDeclaredVars: DeclaredVariable[] = (() => {
    const out = [...(declaredVariables || [])]
    const seen = new Set(out.map((v) => v.name))
    const matches = (value || '').matchAll(/\{\{\s*([A-Za-z0-9_]+)\s*\}\}/g)
    for (const m of matches) {
      const name = m[1]
      if (name && !seen.has(name)) {
        seen.add(name)
        out.push({ name, description: 'Variável dinâmica usada neste prompt' })
      }
    }
    return out
  })()

  const handleTextChange = (nextVal: string) => {
    onChange(nextVal)
    if (nextVal.endsWith('{{')) {
      setVarPopoverOpen(true)
    }
  }

  const insertVariable = (varName: string) => {
    if (value.endsWith('{{')) {
      onChange(`${value}${varName}}} `)
    } else {
      onChange(`${value}${value.endsWith(' ') || !value ? '' : ' '}{{${varName}}} `)
    }
  }

  const hasTokens = /\{\{[^}]+\}\}|\[[^\]]+\]/.test(value)

  return (
    <div className="space-y-2 relative">
      {/* Header do Bloco (Igual às Imagens 1 e 2) */}
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-[13px] font-semibold text-zinc-900 flex items-center gap-1.5">
            <span className={showBookIcon ? 'underline decoration-dotted underline-offset-4' : ''}>
              {title}
            </span>
            {showBookIcon && <BookOpen className="w-3.5 h-3.5 text-zinc-400" />}
          </h3>
          {subtitle && (
            <p className="text-xs text-zinc-500 mt-0.5">
              {subtitle}{' '}
              {showDisclosureLink && (
                <a
                  href="https://elevenlabs.io/docs/eleven-agents/best-practices/prompting-guide"
                  target="_blank"
                  rel="noreferrer"
                  className="text-zinc-800 font-medium underline underline-offset-2 inline-flex items-center gap-0.5"
                >
                  Requisitos de Divulgação
                  <ArrowUpRight className="w-3 h-3" />
                </a>
              )}
            </p>
          )}
        </div>

        {showWandButton && (
          <button
            type="button"
            onClick={() => setExpandedOpen(true)}
            title="Refinar e formatar prompt"
            className="w-8 h-8 rounded-lg border border-zinc-200 bg-white hover:bg-zinc-50 flex items-center justify-center text-zinc-700 shadow-2xs transition-colors"
          >
            <Wand2 className="w-3.5 h-3.5" />
          </button>
        )}
      </div>

      {/* Caixa do Editor (Igual às Imagens 1 e 2) */}
      <div
        className={`border border-zinc-200/90 rounded-2xl overflow-hidden transition-all relative ${
          shadedBg ? 'bg-zinc-50/70' : 'bg-white'
        } focus-within:bg-white focus-within:border-zinc-400`}
      >
        {/* Botão de expandir ⤢ no canto superior direito interno */}
        <button
          type="button"
          onClick={() => setExpandedOpen(true)}
          title="Expandir editor"
          className="absolute right-3 top-3 z-10 p-1 text-zinc-400 hover:text-zinc-700 rounded-md hover:bg-zinc-200/50 transition-colors"
        >
          <Maximize2 className="w-3.5 h-3.5" />
        </button>

        {/* Renderiza as pílulas inline ({{variavel}} e [super fast]) quando não está editando o texto bruto */}
        {hasTokens && !editingInline ? (
          <div
            onClick={() => setEditingInline(true)}
            style={{ maxHeight: rows > 4 ? '320px' : undefined }}
            className="min-h-[84px] overflow-y-auto px-4 py-3.5 pr-10 text-[13px] leading-relaxed text-zinc-900 whitespace-pre-wrap cursor-text"
          >
            {renderHighlightedTokens(value)}
          </div>
        ) : (
          <textarea
            rows={rows}
            autoFocus={editingInline}
            value={value}
            onBlur={() => setEditingInline(false)}
            onChange={(e) => handleTextChange(e.target.value)}
            placeholder={placeholder}
            className="w-full px-4 py-3.5 pr-10 bg-transparent text-[13px] leading-relaxed text-zinc-800 focus:outline-none resize-y"
          />
        )}

        {/* Rodapé Integrado ("Digite {{ para adicionar variáveis" + Globo America/Sao_Paulo ou Switch Interrompível) */}
        <div className="px-4 py-2.5 border-t border-zinc-200/60 flex flex-wrap items-center justify-between gap-2 text-xs text-zinc-400">
          <button
            type="button"
            onClick={() => setVarPopoverOpen(!varPopoverOpen)}
            className="inline-flex items-center gap-1 hover:text-zinc-700 transition-colors cursor-pointer"
          >
            Digite{' '}
            <kbd className="px-1.5 py-0.5 bg-zinc-200/70 text-zinc-700 rounded font-mono text-[11px] font-semibold">
              {`{{`}
            </kbd>{' '}
            para adicionar variáveis
          </button>

          {timezone !== undefined && onTimezoneChange && (
            <div className="inline-flex items-center gap-1.5 bg-white border border-zinc-200/90 rounded-xl px-3 py-1 text-zinc-800 font-medium shadow-2xs">
              <Globe className="w-3.5 h-3.5 text-zinc-600" />
              <select
                value={timezone}
                onChange={(e) => onTimezoneChange(e.target.value)}
                className="bg-transparent text-xs font-medium text-zinc-800 focus:outline-none cursor-pointer"
              >
                <option value="America/Sao_Paulo">America/Sao_Paulo</option>
                <option value="America/Manaus">America/Manaus</option>
                <option value="America/Fortaleza">America/Fortaleza</option>
                <option value="UTC">UTC</option>
                <option value="America/New_York">America/New_York</option>
                <option value="Europe/Lisbon">Europe/Lisbon</option>
              </select>
            </div>
          )}

          {interruptible !== undefined && onInterruptibleChange && (
            <div className="inline-flex items-center gap-2 text-zinc-500">
              <Toggle
                checked={interruptible}
                onChange={onInterruptibleChange}
                label="Interrompível"
              />
              <span className="underline decoration-dotted underline-offset-4 text-xs">
                Interrompível
              </span>
            </div>
          )}
        </div>
      </div>

      {/* Popover Master-Detail de Variáveis */}
      <VariablesMasterDetailPopover
        open={varPopoverOpen}
        onClose={() => setVarPopoverOpen(false)}
        declaredVariables={mergedDeclaredVars}
        onSelectVariable={insertVariable}
      />

      {/* Modal Expandido (Visual | Simples) */}
      <Modal
        open={expandedOpen}
        onClose={() => setExpandedOpen(false)}
        sizeClass="max-w-3xl"
        className="max-h-[88vh] flex flex-col overflow-hidden"
      >
        <div className="px-5 py-3.5 border-b border-zinc-200 flex items-center justify-between bg-zinc-50/50">
          <div className="inline-flex p-0.5 bg-zinc-200/70 rounded-lg">
            <button
              type="button"
              onClick={() => setEditorMode('visual')}
              className={`px-3 py-1 rounded-md text-xs font-medium transition-all ${
                editorMode === 'visual'
                  ? 'bg-white text-zinc-900 shadow-2xs'
                  : 'text-zinc-600 hover:text-zinc-900'
              }`}
            >
              Visual
            </button>
            <button
              type="button"
              onClick={() => setEditorMode('simples')}
              className={`px-3 py-1 rounded-md text-xs font-medium transition-all ${
                editorMode === 'simples'
                  ? 'bg-white text-zinc-900 shadow-2xs'
                  : 'text-zinc-600 hover:text-zinc-900'
              }`}
            >
              Simples
            </button>
          </div>

          <button
            type="button"
            onClick={() => setExpandedOpen(false)}
            className="p-1 text-zinc-400 hover:text-zinc-700 rounded-lg"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <div className="p-6 flex-1 overflow-y-auto overscroll-contain">
          {editorMode === 'visual' ? (
            <div
              onDoubleClick={() => setEditorMode('simples')}
              title="Dê duplo clique ou mude para 'Simples' para editar o texto bruto"
              className="min-h-[380px] whitespace-pre-wrap text-[13px] leading-relaxed text-zinc-800 cursor-text"
            >
              {renderHighlightedTokens(value)}
            </div>
          ) : (
            <textarea
              rows={18}
              autoFocus
              value={value}
              onChange={(e) => handleTextChange(e.target.value)}
              className="w-full text-[13px] leading-relaxed text-zinc-900 bg-transparent border-0 focus:outline-none resize-none"
            />
          )}
        </div>

        <div className="px-5 py-3 bg-zinc-50 border-t border-zinc-200 flex items-center justify-between text-xs text-zinc-500">
          <button
            type="button"
            onClick={() => setVarPopoverOpen(!varPopoverOpen)}
            className="inline-flex items-center gap-1 hover:text-zinc-800"
          >
            <Braces className="w-3.5 h-3.5" />
            Digite{' '}
            <kbd className="px-1.5 py-0.5 bg-zinc-200 text-zinc-700 rounded font-mono text-[10px]">
              {`{{`}
            </kbd>{' '}
            para adicionar variáveis
          </button>

          {timezone && (
            <span className="inline-flex items-center gap-1.5 px-2.5 py-1 bg-white border border-zinc-200 rounded-lg text-zinc-700 font-medium">
              <Globe className="w-3.5 h-3.5 text-zinc-500" />
              {timezone}
            </span>
          )}
        </div>
      </Modal>
    </div>
  )
}
