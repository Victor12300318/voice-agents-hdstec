import { useEffect, useRef, useState } from 'react'
import { Link, useParams, useNavigate } from 'react-router-dom'
import Sheet from '../components/Sheet'
import Modal from '../components/Modal'
import Button from '../components/Button'
import Toggle from '../components/Toggle'
import VoiceOrb from '../components/VoiceOrb'
import NotionProcedureWorkspace from '../components/NotionProcedureWorkspace'
import { RichPromptCard, SYSTEM_VARIABLES_CATALOG } from '../components/VariablesPopoverModal'
import ElevenToolsPanel from '../components/ElevenToolsPanel'
import ElevenToolDrawer, { type ToolCategory } from '../components/ElevenToolDrawer'
import { haptic } from '../components/motion'
import {
  api,
  type AgentDetail,
  type AgentUpdatePayload,
  type Skill,
  type ToolWithStats,
  type KnowledgeBase,
  type ProviderCatalog,
  type Conversation,
} from '../api'
import { AudioPlayer, AudioRecorder } from '../audio'
import {
  ChevronLeft,
  ChevronRight,
  ChevronDown,
  PanelLeft,
  MoreHorizontal,
  Globe,
  PhoneCall,
  Braces,
  Send,
  Search,
  UserCircle2,
  ListTree,
  GitFork,
  GitBranch,
  FolderOpen,
  BarChart2,
  Wrench,
  Shield,
  Settings,
  MessageSquare,
  Triangle,
  Volume2,
  PlusCircle,
  Info,
  Play,
  Square,
  Mic,
  MicOff,
  Headphones,
  Sparkles,
  Check,
  Copy,
  AlertCircle,
  Plus,
  Trash2,
  X,
} from 'lucide-react'

const FALLBACK_CATALOGS: ProviderCatalog[] = [
  {
    provider: 'gemini',
    label: 'Google Gemini Live',
    models: [
      {
        id: 'gemini-2.5-flash-native-audio',
        label: 'Gemini 2.5 Flash Native Audio',
        default: true,
      },
      {
        id: 'gemini-3.8-live-extended-thinking',
        label: 'Gemini 3.8 Live (Extended Thinking)',
      },
    ],
    voices: [
      { id: 'Aoede', label: 'Aoede — Expressiva e Natural', default: true },
      { id: 'Kore', label: 'Kore — Calma e Profissional' },
      { id: 'Puck', label: 'Puck — Dinâmica e Ágil' },
      { id: 'Charon', label: 'Charon — Grave e Segura' },
      { id: 'Fenrir', label: 'Fenrir — Firme e Objetiva' },
    ],
    languages: [
      { id: 'pt-BR', label: 'Português (Brasil)', default: true },
      { id: 'en-US', label: 'English (US)' },
      { id: 'es-ES', label: 'Español' },
    ],
    max_session_seconds: 900,
    input_sample_rate: 16000,
    output_sample_rate: 24000,
  },
  {
    provider: 'openai_realtime',
    label: 'OpenAI Realtime & GPT-Live',
    models: [
      { id: 'gpt-live-1', label: 'GPT-Live 1', description: 'Modelo full-duplex de voz ultra-natural ($0.05/min)', default: true },
      { id: 'gpt-realtime-2.1', label: 'GPT Realtime 2.1', description: 'Última geração OpenAI Realtime de baixa latência' },
      { id: 'gpt-realtime-2.1-mini', label: 'GPT Realtime 2.1 Mini', description: 'Ultra-rápido e econômico de nova geração' },
      { id: 'gpt-realtime', label: 'GPT Realtime', description: 'Modelo estável de produção' },
      { id: 'gpt-realtime-mini', label: 'GPT Realtime Mini', description: 'Compacto e econômico' },
      { id: 'gpt-realtime-1.5', label: 'GPT Realtime 1.5', description: 'Versão 1.5' },
    ],
    voices: [
      { id: 'bossa', label: 'Bossa (PT-BR)', default: true },
      { id: 'tempo', label: 'Tempo (PT-BR)' },
      { id: 'marin', label: 'Marin' },
      { id: 'coral', label: 'Coral' },
      { id: 'sage', label: 'Sage' },
      { id: 'shimmer', label: 'Shimmer' },
      { id: 'cedar', label: 'Cedar' },
      { id: 'alloy', label: 'Alloy' },
      { id: 'echo', label: 'Echo' },
    ],
    languages: [
      { id: 'pt-BR', label: 'Português (Brasil)', default: true },
      { id: 'en-US', label: 'English (US)' },
      { id: 'es-ES', label: 'Español' },
    ],
    max_session_seconds: 1800,
    input_sample_rate: 16000,
    output_sample_rate: 24000,
  },
]

const LANGUAGE_FLAGS: Record<string, { flag: string; label: string }> = {
  'pt-BR': { flag: '🇧🇷', label: 'Português (Brasil)' },
  'en-US': { flag: '🇺🇸', label: 'English (US)' },
  'es-ES': { flag: '🇪🇸', label: 'Español' },
}

const VOICE_TRAITS_MAP: Record<string, { badge: string; tooltip: string }> = {
  Kore: {
    badge: 'Feminina • Firme e Profissional',
    tooltip: 'Kore — Feminina • Firme, confiante e profissional • Ideal para vendas e negociação',
  },
  Zephyr: {
    badge: 'Feminina • Brilhante e Clara',
    tooltip: 'Zephyr — Feminina • Brilhante, clara e energética • Ideal para recepção e triagem',
  },
  Puck: {
    badge: 'Masculina • Animada e Dinâmica',
    tooltip: 'Puck — Masculina • Animada, ágil e dinâmica • Ideal para atendimento rápido',
  },
  Charon: {
    badge: 'Masculina • Informativa e Segura',
    tooltip: 'Charon — Masculina • Informativa, grave e segura • Ideal para suporte técnico',
  },
  Fenrir: {
    badge: 'Masculina • Empolgada e Expressiva',
    tooltip: 'Fenrir — Masculina • Empolgada, expressiva e incisiva • Ideal para ofertas comerciais',
  },
  Leda: {
    badge: 'Feminina • Jovem e Amigável',
    tooltip: 'Leda — Feminina • Jovem, amigável e calorosa • Ideal para fidelização e vendas',
  },
  Orus: {
    badge: 'Masculina • Firme e Objetiva',
    tooltip: 'Orus — Masculina • Firme, madura e objetiva • Ideal para confirmação cadastral',
  },
  Aoede: {
    badge: 'Feminina • Leve e Natural',
    tooltip: 'Aoede — Feminina • Leve, natural e fluida • Ideal para consultoria e atendimento',
  },
  Callirrhoe: {
    badge: 'Feminina • Tranquila e Serena',
    tooltip: 'Callirrhoe — Feminina • Tranquila, serena e paciente • Ideal para SAC e retenção',
  },
  Autonoe: {
    badge: 'Feminina • Brilhante e Cordial',
    tooltip: 'Autonoe — Feminina • Brilhante, articulada e cordial • Ideal para vendas consultivas',
  },
  Enceladus: {
    badge: 'Masculina • Suave e Calma',
    tooltip: 'Enceladus — Masculina • Suave, calma e acolhedora • Ideal para concierge',
  },
  Iapetus: {
    badge: 'Masculina • Clara e Nítida',
    tooltip: 'Iapetus — Masculina • Clara, direta e dicção nítida • Ideal para leitura de dados',
  },
  Umbriel: {
    badge: 'Masculina • Tranquila e Estável',
    tooltip: 'Umbriel — Masculina • Tranquila, equilibrada e estável • Ideal para suporte N1',
  },
  Algieba: {
    badge: 'Masculina • Suave e Polida',
    tooltip: 'Algieba — Masculina • Suave, polida e cordial • Ideal para atendimento executivo',
  },
  Despina: {
    badge: 'Feminina • Suave e Empática',
    tooltip: 'Despina — Feminina • Suave, empática e gentil • Ideal para saúde e clínicas',
  },
  Erinome: {
    badge: 'Feminina • Clara e Precisa',
    tooltip: 'Erinome — Feminina • Clara, precisa e profissional • Ideal para agendamentos',
  },
  Algenib: {
    badge: 'Masculina • Encorpada e Marcante',
    tooltip: 'Algenib — Masculina • Timbre encorpado e marcante • Ideal para retenção',
  },
  Rasalgethi: {
    badge: 'Masculina • Informativa e Didática',
    tooltip: 'Rasalgethi — Masculina • Informativa, didática e pausada • Ideal para planos',
  },
  Laomedeia: {
    badge: 'Feminina • Animada e Simpática',
    tooltip: 'Laomedeia — Feminina • Animada, simpática e envolvente • Ideal para upgrades',
  },
  Achernar: {
    badge: 'Feminina • Macia e Natural',
    tooltip: 'Achernar — Feminina • Macia, natural e conversacional • Ideal para pós-venda',
  },
  Alnilam: {
    badge: 'Masculina • Firme e Assertiva',
    tooltip: 'Alnilam — Masculina • Firme, assertiva e clara • Ideal para validação de segurança',
  },
  Schedar: {
    badge: 'Masculina • Equilibrada e Neutra',
    tooltip: 'Schedar — Masculina • Equilibrada, neutra e versátil • Ideal para corporativo',
  },
  Gacrux: {
    badge: 'Feminina • Madura e Experiente',
    tooltip: 'Gacrux — Feminina • Madura, experiente e segura • Ideal para consultoria financeira',
  },
  Pulcherrima: {
    badge: 'Feminina • Direta e Objetiva',
    tooltip: 'Pulcherrima — Feminina • Direta, objetiva e clara • Ideal para chamadas rápidas',
  },
  Achird: {
    badge: 'Masculina • Amigável e Próxima',
    tooltip: 'Achird — Masculina • Amigável, próxima e descontraída • Ideal para boas-vindas',
  },
  Zubenelgenubi: {
    badge: 'Masculina • Casual e Espontânea',
    tooltip: 'Zubenelgenubi — Masculina • Casual, espontânea e natural • Ideal para público jovem',
  },
  Vindemiatrix: {
    badge: 'Feminina • Gentil e Atenciosa',
    tooltip: 'Vindemiatrix — Feminina • Gentil, calma e atenciosa • Ideal para suporte ao cliente',
  },
  Sadachbia: {
    badge: 'Masculina • Vivaz e Ágil',
    tooltip: 'Sadachbia — Masculina • Vivaz, prestativa e ágil • Ideal para televendas',
  },
  Sadaltager: {
    badge: 'Masculina • Conhecedora e Especialista',
    tooltip: 'Sadaltager — Masculina • Conhecedora, especialista e confiável • Ideal para consultoria',
  },
  Sulafat: {
    badge: 'Feminina • Calorosa e Carismática',
    tooltip: 'Sulafat — Feminina • Calorosa, acolhedora e carismática • Ideal para fidelização',
  },
  marin: {
    badge: 'Feminina • Natural e Empática',
    tooltip: 'Marin — Feminina • Natural, empática e expressiva • Ideal para atendimento geral',
  },
  alloy: {
    badge: 'Neutra • Versátil e Clara',
    tooltip: 'Alloy — Versátil, equilibrada e clara • Ideal para múltiplos fluxos',
  },
  shimmer: {
    badge: 'Feminina • Clara e Suave',
    tooltip: 'Shimmer — Feminina • Clara, suave e acolhedora • Ideal para relacionamento',
  },
  echo: {
    badge: 'Masculina • Grave e Firme',
    tooltip: 'Echo — Masculina • Grave, serena e firme • Ideal para suporte técnico',
  },
  bossa: {
    badge: 'Feminina • Brasileira Natural (GPT-Live 1)',
    tooltip: 'Bossa — Voz feminina brasileira natural nativa do GPT-Live 1',
  },
  tempo: {
    badge: 'Masculina • Brasileira Natural (GPT-Live 1)',
    tooltip: 'Tempo — Voz masculina brasileira natural nativa do GPT-Live 1',
  },
  coral: {
    badge: 'Feminina • Amigável e Calorosa',
    tooltip: 'Coral — Feminina • Amigável, clara e calorosa • Ideal para vendas e atendimento',
  },
  sage: {
    badge: 'Feminina • Calma e Confiante',
    tooltip: 'Sage — Feminina • Madura, tranquila e confiante',
  },
  cedar: {
    badge: 'Masculina • Natural e Firme',
    tooltip: 'Cedar — Masculina de nova geração • Natural e firme',
  },
  quartz: {
    badge: 'Feminina • Expressiva (GPT-Live 1)',
    tooltip: 'Quartz — Feminina expressiva do GPT-Live 1',
  },
  gleam: {
    badge: 'Feminina • Suave e Natural (GPT-Live 1)',
    tooltip: 'Gleam — Feminina suave e natural do GPT-Live 1',
  },
  willow: {
    badge: 'Feminina • Calorosa (GPT-Live 1)',
    tooltip: 'Willow — Feminina calorosa do GPT-Live 1',
  },
}

type TranscriptMsg = {
  id: string
  role: 'caller' | 'agent' | 'system'
  text: string
  time: string
}

type ActiveSection =
  | 'geral'
  | 'skills'
  | 'tools'
  | 'versoes'
  | 'kb'
  | 'configuracoes'
  | 'conversas'
  | 'analytics'

export default function AgentEditor() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()

  // Dados do Agente
  const [detail, setDetail] = useState<AgentDetail | null>(null)
  const [catalogs, setCatalogs] = useState<ProviderCatalog[]>(FALLBACK_CATALOGS)
  const [form, setForm] = useState<AgentUpdatePayload | null>(null)
  const [skills, setSkills] = useState<Skill[]>([])
  const [availableTools, setAvailableTools] = useState<ToolWithStats[]>([])
  const [knowledgeBases, setKnowledgeBases] = useState<KnowledgeBase[]>([])
  const [agentConversations, setAgentConversations] = useState<Conversation[]>([])
  const [versionsList, setVersionsList] = useState<{ version: number; created_at: string }[]>([])

  // Navegação & Sidebar (Expandida igual à Imagem 2 ou Recolhida em Ícones igual à Imagem 1)
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
  const [currentTab, setCurrentTab] = useState<ActiveSection>('geral')
  const [tags, setTags] = useState<string[]>([])
  const [addingTag, setAddingTag] = useState(false)
  const [newTagText, setNewTagText] = useState('')

  // Drawers laterais (Sheets) da coluna direita (Vozes, Ambiente de Áudio, Config de Voz, Idioma, LLM, Comportamento, Variáveis)
  const [activeRightDrawer, setActiveRightDrawer] = useState<
    | null
    | 'voices'
    | 'voice_settings'
    | 'audio_env'
    | 'language'
    | 'llm'
    | 'behavior'
    | 'variables'
  >(null)
  const [ttsFormat, setTtsFormat] = useState('ulaw_8000')
  const [audioFilter, setAudioFilter] = useState('telefone_claro')
  const [bgSoundEnabled, setBgSoundEnabled] = useState(false)

  // Modal de renomear agente (•••)
  const [isRenameModalOpen, setIsRenameModalOpen] = useState(false)

  // Gaveta de criar / editar ferramenta do agente
  const [isToolModalOpen, setIsToolModalOpen] = useState(false)
  const [toolDrawerKind, setToolDrawerKind] = useState<ToolCategory>('webhook')
  const [editingAgentTool, setEditingAgentTool] = useState<ToolWithStats | null>(null)

  // Nova variável dinâmica no drawer { } Variáveis
  const [newVarName, setNewVarName] = useState('')
  const [newVarDesc, setNewVarDesc] = useState('')

  // Teste interativo do Initiation Webhook (Twilio Personalization)
  const [testCallerID, setTestCallerID] = useState('+5511999998888')
  const [testCalledNumber, setTestCalledNumber] = useState('+551140041234')
  const [testingWebhook, setTestingWebhook] = useState(false)
  const [webhookTestResult, setWebhookTestResult] = useState<any>(null)

  // Status de salvamento / publicação
  const [saving, setSaving] = useState(false)
  const [publishing, setPublishing] = useState(false)
  const [hasUnsavedDraft, setHasUnsavedDraft] = useState(false)
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)
  const [isPlayingPreview, setIsPlayingPreview] = useState(false)
  const [previewingVoiceId, setPreviewingVoiceId] = useState<string | null>(null)
  const [loadingVoiceId, setLoadingVoiceId] = useState<string | null>(null)
  const [voiceSearch, setVoiceSearch] = useState('')
  const [cursorTooltip, setCursorTooltip] = useState<{
    text: string
    x: number
    y: number
  } | null>(null)
  const previewAudioRef = useRef<HTMLAudioElement | null>(null)

  // Gaveta de Pré-visualização (Teste de Voz ao Vivo)
  const [isTestDrawerOpen, setIsTestDrawerOpen] = useState(false)
  const [callStatus, setCallStatus] = useState<'idle' | 'connecting' | 'active' | 'ended'>('idle')
  const [transcripts, setTranscripts] = useState<TranscriptMsg[]>([])
  const [callDuration, setCallDuration] = useState(0)
  const [activeSkillName, setActiveSkillName] = useState<string | null>(null)
  const [executedTools, setExecutedTools] = useState<
    { name: string; latencyMs: number; status: string }[]
  >([])
  const [antiEcho, setAntiEcho] = useState(true)
  const [isMicMuted, setIsMicMuted] = useState(false)
  const [isAgentSpeaking, setIsAgentSpeaking] = useState(false)

  const wsRef = useRef<WebSocket | null>(null)
  const recorderRef = useRef<AudioRecorder | null>(null)
  const playerRef = useRef<AudioPlayer | null>(null)
  const timerRef = useRef<any>(null)
  const transcriptsEndRef = useRef<HTMLDivElement | null>(null)

  const loadData = async () => {
    try {
      const [cats, ag, tls, procs, kbs, vers] = await Promise.all([
        api<ProviderCatalog[]>('/api/providers').catch(() => FALLBACK_CATALOGS),
        api<AgentDetail>(`/api/agents/${id}`),
        api<ToolWithStats[]>('/api/tools').catch(() => []),
        api<Skill[]>(`/api/agents/${id}/procedures`).catch(() => []),
        api<KnowledgeBase[]>('/api/knowledge-bases').catch(() => []),
        api<{ version: number; created_at: string }[]>(`/api/agents/${id}/versions`).catch(
          () => [],
        ),
      ])

      setCatalogs(cats.length ? cats : FALLBACK_CATALOGS)
      setDetail(ag)
      setAvailableTools(tls || [])
      setSkills(procs || [])
      setKnowledgeBases(kbs || [])
      setVersionsList(vers || [])
      setHasUnsavedDraft(Boolean(ag.has_unpublished_changes))

      setForm({
        name: ag.name,
        description: ag.description || '',
        active: ag.active,
        provider: ag.provider || 'gemini',
        model: ag.model || 'gemini-2.5-flash-native-audio',
        voice: ag.voice || 'Aoede',
        language: ag.language || 'pt-BR',
        timezone: ag.timezone || 'America/Sao_Paulo',
        system_prompt: ag.system_prompt || '',
        first_message: ag.first_message || '',
        first_message_interruptible: ag.first_message_interruptible ?? true,
        max_duration_seconds: ag.max_duration_seconds || 600,
        recording_enabled: ag.recording_enabled ?? true,
        variables: ag.variables || [],
        initiation_webhook_url: ag.initiation_webhook_url || '',
        initiation_webhook_timeout_ms: ag.initiation_webhook_timeout_ms || 3000,
        system_tools: ag.system_tools || ['encerrar_conversa'],
        system_tools_config: ag.system_tools_config || {},
        tools: ag.tools || [],
      })

      const convs = await api<Conversation[]>('/api/conversations').catch(() => [])
      setAgentConversations((convs || []).filter((c) => c.agent_id === id))
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message || 'Erro ao carregar agente' })
    }
  }

  useEffect(() => {
    loadData()
    return () => {
      stopAudioResources()
    }
  }, [id])

  useEffect(() => {
    transcriptsEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [transcripts])

  const updateFormField = (patch: Partial<AgentUpdatePayload>) => {
    if (!form) return
    setForm({ ...form, ...patch })
    setHasUnsavedDraft(true)
  }

  // Auto-sincroniza todas as {{variaveis}} usadas em System Prompt, First Message e Procedimentos
  const syncReferencedVariables = (baseForm: AgentUpdatePayload) => {
    const sysSet = new Set([
      'horario',
      'data',
      'hora',
      'dia_semana',
      'id_conversa',
      ...SYSTEM_VARIABLES_CATALOG.map((x) => x.name),
    ])
    const existing = [...(baseForm.variables || [])]
    const seen = new Set(existing.map((v) => v.name))
    const allTexts = [
      baseForm.system_prompt || '',
      baseForm.first_message || '',
      ...skills.map((s) => `${s.content || ''} ${s.trigger || ''} ${(s.guardrails || []).join(' ')}`),
    ].join('\n')

    const matches = allTexts.matchAll(/\{\{\s*([A-Za-z0-9_]+)\s*\}\}/g)
    for (const m of matches) {
      const varName = m[1]
      if (varName && !sysSet.has(varName) && !varName.startsWith('system__') && !seen.has(varName)) {
        seen.add(varName)
        existing.push({ name: varName, description: 'Extraída automaticamente do prompt/procedimento' })
      }
    }
    return existing
  }

  const saveDraft = async (overrideForm?: Partial<AgentUpdatePayload>) => {
    if (!form || !id) return
    setSaving(true)
    setMessage(null)
    try {
      const merged = { ...form, ...overrideForm }
      const syncedVars = syncReferencedVariables(merged)
      const payload = { ...merged, variables: syncedVars }
      setForm(payload)
      const updated = await api<AgentDetail>(`/api/agents/${id}`, {
        method: 'PUT',
        body: JSON.stringify(payload),
      })
      setDetail(updated)
      setMessage({ type: 'success', text: 'Alterações salvas no rascunho.' })
    } catch (err: any) {
      setMessage({
        type: 'error',
        text: err.errors ? err.errors.join(', ') : err.message,
      })
    } finally {
      setSaving(false)
    }
  }

  const handlePublish = async () => {
    if (!id || !form) return
    setPublishing(true)
    setMessage(null)
    try {
      const syncedVars = syncReferencedVariables(form)
      const payload = { ...form, variables: syncedVars }
      setForm(payload)
      await api(`/api/agents/${id}`, {
        method: 'PUT',
        body: JSON.stringify(payload),
      })
      await api(`/api/agents/${id}/publish`, { method: 'POST' })
      haptic(12)
      setHasUnsavedDraft(false)
      setMessage({
        type: 'success',
        text: 'Nova versão publicada com sucesso!',
      })
      await loadData()
    } catch (err: any) {
      setMessage({
        type: 'error',
        text: err.errors ? err.errors.join(' • ') : err.message || 'Falha ao publicar',
      })
    } finally {
      setPublishing(false)
    }
  }

  const handleProviderChange = (newProv: string) => {
    if (!form) return
    const cat =
      catalogs.find((c) => c.provider.includes(newProv)) ||
      FALLBACK_CATALOGS.find((c) => c.provider === newProv) ||
      FALLBACK_CATALOGS[0]
    const defaultModel = cat.models.find((m) => m.default)?.id || cat.models[0]?.id || ''
    const defaultVoice = cat.voices.find((v) => v.default)?.id || cat.voices[0]?.id || ''
    const updated = {
      ...form,
      provider: cat.provider,
      model: defaultModel,
      voice: defaultVoice,
    }
    setForm(updated)
    setHasUnsavedDraft(true)
    saveDraft(updated)
  }

  const toggleTool = (toolId: string) => {
    if (!form) return
    const current = form.tools || []
    const updated = current.includes(toolId)
      ? current.filter((tid) => tid !== toolId)
      : [...current, toolId]
    setForm({ ...form, tools: updated })
    setHasUnsavedDraft(true)
    saveDraft({ tools: updated })
  }

  const removeSkill = async (skillId: string) => {
    if (!id) return
    try {
      await api(`/api/agents/${id}/procedures/${skillId}`, { method: 'DELETE' })
      setSkills(skills.filter((s) => s.id !== skillId))
      setHasUnsavedDraft(true)
    } catch (err: any) {
      setMessage({ type: 'error', text: err.message })
    }
  }

  const stopVoiceSample = () => {
    if (previewAudioRef.current) {
      previewAudioRef.current.pause()
      previewAudioRef.current.currentTime = 0
      previewAudioRef.current = null
    }
    window.speechSynthesis?.cancel()
    setIsPlayingPreview(false)
    setPreviewingVoiceId(null)
    setLoadingVoiceId(null)
  }

  const playVoiceSample = async (voiceId: string) => {
    if (previewingVoiceId === voiceId || loadingVoiceId === voiceId) {
      stopVoiceSample()
      return
    }
    stopVoiceSample()
    setLoadingVoiceId(voiceId)

    try {
      const resp = await fetch('/api/voices/preview', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          provider: form?.provider || 'gemini_live',
          voice: voiceId,
          language: form?.language || 'pt-BR',
        }),
      })
      if (!resp.ok) {
        throw new Error('Falha ao sintetizar amostra')
      }
      const blob = await resp.blob()
      const url = URL.createObjectURL(blob)
      const audio = new Audio(url)
      previewAudioRef.current = audio
      setLoadingVoiceId(null)
      setPreviewingVoiceId(voiceId)
      setIsPlayingPreview(true)

      audio.onended = () => {
        URL.revokeObjectURL(url)
        setPreviewingVoiceId(null)
        setIsPlayingPreview(false)
      }
      audio.onerror = () => {
        URL.revokeObjectURL(url)
        setPreviewingVoiceId(null)
        setIsPlayingPreview(false)
      }
      await audio.play()
    } catch {
      // Fallback para síntese local se o navegador bloquear ou falhar
      setLoadingVoiceId(null)
      setPreviewingVoiceId(voiceId)
      setIsPlayingPreview(true)
      if ('speechSynthesis' in window) {
        const utter = new SpeechSynthesisUtterance(
          `Olá! Eu sou a voz ${voiceId}, pronta para atender seus clientes com clareza e naturalidade.`,
        )
        utter.lang = form?.language || 'pt-BR'
        utter.onend = () => {
          setPreviewingVoiceId(null)
          setIsPlayingPreview(false)
        }
        utter.onerror = () => {
          setPreviewingVoiceId(null)
          setIsPlayingPreview(false)
        }
        window.speechSynthesis.speak(utter)
      } else {
        setTimeout(() => {
          setPreviewingVoiceId(null)
          setIsPlayingPreview(false)
        }, 2500)
      }
    }
  }

  const toggleVoicePreview = () => {
    if (isPlayingPreview) {
      stopVoiceSample()
      return
    }
    playVoiceSample(form?.voice || 'Kore')
  }

  // Teste de Voz ao Vivo (Drawer Pré-visualização)
  const stopAudioResources = () => {
    if (timerRef.current) {
      clearInterval(timerRef.current)
      timerRef.current = null
    }
    if (recorderRef.current) {
      recorderRef.current.stop()
      recorderRef.current = null
    }
    if (playerRef.current) {
      playerRef.current.close()
      playerRef.current = null
    }
    if (wsRef.current) {
      try {
        wsRef.current.close()
      } catch {}
      wsRef.current = null
    }
    setIsAgentSpeaking(false)
  }

  const startLiveTest = async () => {
    if (!id || !form) return
    await saveDraft()
    setIsTestDrawerOpen(true)
    stopAudioResources()
    setTranscripts([])
    setExecutedTools([])
    setActiveSkillName(null)
    setCallDuration(0)
    setCallStatus('connecting')

    try {
      const player = new AudioPlayer()
      player.init(24000)
      await player.resume()
      player.onSpeakingChange = (speaking) => {
        setIsAgentSpeaking(speaking)
        if (recorderRef.current && antiEcho) {
          recorderRef.current.setMuted(speaking ? true : isMicMuted)
        }
      }
      playerRef.current = player

      const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const wsUrl = `${proto}//${window.location.host}/ws/conversations/${id}?version=draft&caller=preview_panel`
      const ws = new WebSocket(wsUrl)
      ws.binaryType = 'arraybuffer'
      wsRef.current = ws

      ws.onmessage = async (event) => {
        if (typeof event.data === 'string') {
          try {
            const data = JSON.parse(event.data)
            if (data.type === 'ready') {
              setCallStatus('active')
              timerRef.current = setInterval(() => {
                setCallDuration((d) => d + 1)
              }, 1000)
              const rec = new AudioRecorder((pcm) => {
                if (ws.readyState === WebSocket.OPEN) ws.send(pcm)
              })
              await rec.start(data.input_sample_rate || 16000)
              recorderRef.current = rec
            } else if (data.type === 'caller_transcript' && data.text) {
              const frag = String(data.text).trim()
              if (frag) {
                setTranscripts((prev) => {
                  const last = prev[prev.length - 1]
                  if (last && last.role === 'caller') {
                    const sep = /^[.,!?;:]/.test(frag) ? '' : ' '
                    return [
                      ...prev.slice(0, -1),
                      { ...last, text: `${last.text.trimEnd()}${sep}${frag}` },
                    ]
                  }
                  return [
                    ...prev,
                    {
                      id: Math.random().toString(36).slice(2),
                      role: 'caller',
                      text: frag,
                      time: new Date().toLocaleTimeString('pt-BR'),
                    },
                  ]
                })
              }
            } else if (data.type === 'agent_transcript' && data.text) {
              const frag = String(data.text).trim()
              if (frag) {
                setTranscripts((prev) => {
                  const last = prev[prev.length - 1]
                  if (last && last.role === 'agent') {
                    const sep = /^[.,!?;:]/.test(frag) ? '' : ' '
                    return [
                      ...prev.slice(0, -1),
                      { ...last, text: `${last.text.trimEnd()}${sep}${frag}` },
                    ]
                  }
                  return [
                    ...prev,
                    {
                      id: Math.random().toString(36).slice(2),
                      role: 'agent',
                      text: frag,
                      time: new Date().toLocaleTimeString('pt-BR'),
                    },
                  ]
                })
              }
            } else if (data.type === 'interrupted') {
              player.flush()
            } else if (data.type === 'procedure_activated') {
              setActiveSkillName(data.text)
            } else if (data.type === 'tool_call') {
              setExecutedTools((prev) => [
                ...prev,
                { name: data.text, latencyMs: 180, status: 'success' },
              ])
            } else if (data.type === 'ended' || data.type === 'error') {
              endLiveTest()
            }
          } catch {}
        } else if (event.data instanceof ArrayBuffer) {
          player.playChunk(event.data)
        }
      }

      ws.onclose = () => {
        setCallStatus('ended')
        stopAudioResources()
      }
    } catch {
      setCallStatus('ended')
      stopAudioResources()
    }
  }

  const endLiveTest = () => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      try {
        wsRef.current.send(JSON.stringify({ type: 'hangup' }))
      } catch {}
    }
    setCallStatus('ended')
    stopAudioResources()
  }

  const formatTimer = (sec: number) => {
    const m = Math.floor(sec / 60)
      .toString()
      .padStart(2, '0')
    const s = (sec % 60).toString().padStart(2, '0')
    return `${m}:${s}`
  }

  if (!form || !detail) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-white text-xs text-zinc-400">
        Carregando configuração do agente…
      </div>
    )
  }

  const currentCatalog =
    catalogs.find((c) => form.provider.includes(c.provider) || c.provider.includes(form.provider)) ||
    FALLBACK_CATALOGS[0]

  const currentVoiceObj =
    currentCatalog.voices.find((v) => v.id === form.voice) || {
      id: form.voice,
      label: `${form.voice} - Youthful and friendly`,
    }

  const currentModelObj =
    currentCatalog.models.find((m) => m.id === form.model) || {
      id: form.model,
      label: form.model,
    }

  const currentLangMeta = LANGUAGE_FLAGS[form.language] || {
    flag: '🌐',
    label: form.language,
  }

  const selectSection = (sec: ActiveSection) => {
    setCurrentTab(sec)
    // Na ElevenLabs, ao clicar em Procedimentos, a sidebar principal recolhe em modo de ícones para dar espaço ao Workspace!
    if (sec === 'skills') {
      setSidebarCollapsed(true)
    }
  }

  return (
    <div className="flex h-screen overflow-hidden bg-white text-zinc-900 font-sans antialiased">
      {/* =====================================================================
          BARRA LATERAL DO AGENTE (Expandida igual à Imagem 2 ou Ícones igual à Imagem 1)
         ===================================================================== */}
      <aside
        className={`${
          sidebarCollapsed ? 'w-14' : 'w-60'
        } flex-shrink-0 border-r border-zinc-200/80 bg-[#fbfbfb] flex flex-col justify-between transition-all duration-200 select-none overflow-y-auto`}
      >
        <div className="p-2.5 space-y-3">
          {/* Topo: Logo IIElevenLabs / VoiceAgents */}
          <div
            className={`flex items-center ${
              sidebarCollapsed ? 'justify-center py-1.5' : 'px-2 py-1.5 justify-between'
            }`}
          >
            <Link to="/agents" className="flex items-center gap-2 font-bold text-sm tracking-tight text-zinc-950">
              <span className="font-black text-base leading-none">II</span>
              {!sidebarCollapsed && <span>ElevenAgents</span>}
            </Link>
          </div>

          {/* Barra de Busca "Pesquisar e criar [⌘][K]" */}
          {sidebarCollapsed ? (
            <div className="flex justify-center">
              <button
                type="button"
                onClick={() => setSidebarCollapsed(false)}
                title="Pesquisar e criar (⌘K)"
                className="w-9 h-9 rounded-xl bg-white border border-zinc-200/90 flex items-center justify-center text-zinc-500 hover:text-zinc-900 shadow-2xs"
              >
                <Search className="w-4 h-4" />
              </button>
            </div>
          ) : (
            <div className="flex items-center justify-between px-2.5 py-1.5 bg-white border border-zinc-200/90 rounded-xl text-xs text-zinc-400 shadow-2xs">
              <span className="flex items-center gap-2 truncate">
                <Search className="w-3.5 h-3.5 text-zinc-400 flex-shrink-0" />
                <span className="truncate">Pesquisar e criar</span>
              </span>
              <span className="flex items-center gap-0.5 text-[10px] font-mono">
                <kbd className="px-1 py-0.2 bg-zinc-100 border border-zinc-200 rounded text-zinc-500">
                  ⌘
                </kbd>
                <kbd className="px-1 py-0.2 bg-zinc-100 border border-zinc-200 rounded text-zinc-500">
                  K
                </kbd>
              </span>
            </div>
          )}

          {/* Card do Agente e Branch (Voltar para o espaço de trabalho + Orbe Jessica + Main) */}
          {sidebarCollapsed ? (
            <div className="flex flex-col items-center gap-2 pt-1">
              <Link
                to="/agents"
                title="Voltar para o espaço de trabalho"
                className="text-zinc-400 hover:text-zinc-800 p-1"
              >
                <ChevronLeft className="w-3.5 h-3.5" />
              </Link>
              <div className="p-1.5 bg-white border border-zinc-200/90 rounded-2xl shadow-2xs flex flex-col items-center gap-2.5">
                <div className="w-6 h-6 rounded-full bg-gradient-to-tr from-indigo-500 via-purple-400 to-sky-300 shadow-inner" />
                <span className="w-3 h-3 rounded-full border-2 border-emerald-600 bg-emerald-500/20" />
              </div>
            </div>
          ) : (
            <div className="bg-white border border-zinc-200/90 rounded-2xl shadow-2xs overflow-hidden text-xs">
              <Link
                to="/agents"
                className="px-3 py-2 text-[11px] text-zinc-500 hover:text-zinc-900 flex items-center gap-1 border-b border-zinc-100 bg-zinc-50/50 transition-colors"
              >
                <ChevronLeft className="w-3 h-3" />
                <span className="truncate">Voltar para o espaço de trabalho</span>
              </Link>

              <div className="px-3 py-2.5 flex items-center gap-2.5 border-b border-zinc-100">
                <div className="w-5 h-5 rounded-full bg-gradient-to-tr from-indigo-500 via-purple-400 to-sky-300 flex-shrink-0 shadow-xs" />
                <span className="font-semibold text-zinc-900 truncate">{form.name}</span>
              </div>

              <div className="px-3 py-2 flex items-center justify-between text-zinc-700 hover:bg-zinc-50 cursor-pointer">
                <span className="flex items-center gap-2 font-medium">
                  <span className="w-3 h-3 rounded-full border-2 border-emerald-600 inline-block" />
                  Main
                </span>
                <ChevronDown className="w-3.5 h-3.5 text-zinc-400" />
              </div>
            </div>
          )}

          {/* Grupos de Navegação (Visão geral / Configurar / Monitorar) */}
          <div className="space-y-4 pt-1">
            {/* Grupo: Visão geral */}
            <div className="space-y-0.5">
              {!sidebarCollapsed && (
                <span className="px-2.5 text-[11px] font-medium text-zinc-400 block mb-1">
                  Visão geral
                </span>
              )}
              {[
                { key: 'geral' as const, label: 'Destaque', icon: Triangle },
                { key: 'analytics' as const, label: 'Painéis', icon: BarChart2 },
              ].map((item) => {
                const Icon = item.icon
                const isActive = currentTab === item.key && item.label !== 'Destaque'
                return (
                  <button
                    key={item.label}
                    type="button"
                    onClick={() => selectSection(item.key)}
                    title={item.label}
                    className={`w-full flex items-center ${
                      sidebarCollapsed ? 'justify-center py-2' : 'gap-2.5 px-2.5 py-1.5'
                    } rounded-xl text-xs transition-colors ${
                      isActive
                        ? 'bg-zinc-200/75 text-zinc-950 font-semibold'
                        : 'text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900'
                    }`}
                  >
                    <Icon className="w-4 h-4 text-zinc-500 flex-shrink-0" />
                    {!sidebarCollapsed && <span className="truncate">{item.label}</span>}
                  </button>
                )
              })}
            </div>

            {/* Grupo: Configurar (Igual às Imagens 1 e 2) */}
            <div className="space-y-0.5">
              {!sidebarCollapsed && (
                <span className="px-2.5 text-[11px] font-medium text-zinc-400 block mb-1">
                  Configurar
                </span>
              )}
              {[
                { key: 'geral' as const, label: 'Agente', icon: UserCircle2 },
                { key: 'skills' as const, label: 'Procedimentos', icon: ListTree },
                { key: 'versoes' as const, label: 'Controle de versão', icon: GitBranch },
                { key: 'kb' as const, label: 'Base de conhecimento', icon: FolderOpen },
                { key: 'tools' as const, label: 'Ferramentas', icon: Wrench },
                { key: 'configuracoes' as const, label: 'Configurações', icon: Settings },
              ].map((item) => {
                const Icon = item.icon
                const isActive = currentTab === item.key
                return (
                  <button
                    key={item.label}
                    type="button"
                    onClick={() => selectSection(item.key)}
                    title={item.label}
                    className={`w-full flex items-center ${
                      sidebarCollapsed ? 'justify-center py-2' : 'gap-2.5 px-2.5 py-1.5'
                    } rounded-xl text-xs transition-colors ${
                      isActive
                        ? 'bg-zinc-200/75 text-zinc-950 font-semibold'
                        : 'text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900'
                    }`}
                  >
                    <Icon
                      className={`w-4 h-4 flex-shrink-0 ${
                        isActive ? 'text-zinc-900' : 'text-zinc-500'
                      }`}
                    />
                    {!sidebarCollapsed && <span className="truncate">{item.label}</span>}
                  </button>
                )
              })}
            </div>

            {/* Grupo: Monitorar */}
            <div className="space-y-0.5">
              {!sidebarCollapsed && (
                <span className="px-2.5 text-[11px] font-medium text-zinc-400 block mb-1">
                  Monitorar
                </span>
              )}
              <button
                type="button"
                onClick={() => selectSection('conversas')}
                title="Conversas"
                className={`w-full flex items-center ${
                  sidebarCollapsed ? 'justify-center py-2' : 'gap-2.5 px-2.5 py-1.5'
                } rounded-xl text-xs transition-colors ${
                  currentTab === 'conversas'
                    ? 'bg-zinc-200/75 text-zinc-950 font-semibold'
                    : 'text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900'
                }`}
              >
                <MessageSquare className="w-4 h-4 text-zinc-500 flex-shrink-0" />
                {!sidebarCollapsed && <span className="truncate">Conversas</span>}
              </button>
            </div>
          </div>
        </div>
      </aside>

      {/* =====================================================================
          ÁREA PRINCIPAL (TOP HEADER BAR + CANVAS DE CONFIGURAÇÃO DO AGENTE)
         ===================================================================== */}
      <div className="flex-1 flex flex-col min-w-0 bg-white overflow-hidden">
        {/* TOP HEADER BAR (Idêntica ao Topo das Imagens 1 e 2) */}
        <header className="h-13 px-4 sm:px-6 border-b border-zinc-200/80 flex items-center justify-between flex-shrink-0 bg-white z-20">
          {/* Esquerda: Toggle Sidebar + Nome do Agente (Jessica) + ••• */}
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setSidebarCollapsed(!sidebarCollapsed)}
              title="Alternar barra lateral"
              className="p-1.5 text-zinc-500 hover:text-zinc-900 hover:bg-zinc-100 rounded-lg transition-colors"
            >
              <PanelLeft className="w-4 h-4" />
            </button>

            <span className="text-xs font-semibold text-zinc-900">{form.name}</span>

            <button
              type="button"
              onClick={() => setIsRenameModalOpen(true)}
              title="Editar nome e descrição do agente"
              className="p-1 text-zinc-400 hover:text-zinc-800 hover:bg-zinc-100 rounded-md transition-colors"
            >
              <MoreHorizontal className="w-4 h-4" />
            </button>
          </div>

          {/* Direita: 🌐 | 📞 | { } Variáveis | Pré-visualização | Construa com o Architect | Publicar 🟠 */}
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setActiveRightDrawer('language')}
              title={`Idioma: ${currentLangMeta.label}`}
              className="p-1.5 text-zinc-500 hover:text-zinc-900 hover:bg-zinc-100 rounded-lg transition-colors"
            >
              <Globe className="w-4 h-4" />
            </button>

            <button
              type="button"
              onClick={() => navigate('/phone-numbers')}
              title="Conectar número de telefone (Twilio / SIP)"
              className="p-1.5 text-zinc-500 hover:text-zinc-900 hover:bg-zinc-100 rounded-lg transition-colors"
            >
              <PhoneCall className="w-4 h-4" />
            </button>

            <button
              type="button"
              onClick={() => setActiveRightDrawer('variables')}
              className="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-xl text-xs font-medium text-zinc-700 hover:bg-zinc-100 transition-colors"
            >
              <Braces className="w-3.5 h-3.5 text-zinc-500" />
              Variáveis
            </button>

            <button
              type="button"
              onClick={startLiveTest}
              className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-white hover:bg-zinc-50 border border-zinc-200/90 text-xs font-medium text-zinc-800 shadow-2xs transition-colors"
            >
              Pré-visualização
            </button>

            <button
              type="button"
              onClick={() => navigate(`/agents/${id}/test`)}
              className="hidden sm:inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-white hover:bg-zinc-50 border border-zinc-200/90 text-xs font-medium text-zinc-800 shadow-2xs transition-colors"
            >
              <Send className="w-3.5 h-3.5 text-sky-500 fill-sky-500" />
              Construa com o Architect
            </button>

            {/* Botão Publicar com bolinha laranja quando há mudanças no Draft (Igual às Imagens 1 e 2) */}
            <div className="relative inline-flex items-center">
              <button
                type="button"
                onClick={handlePublish}
                disabled={publishing || saving}
                className="inline-flex items-center gap-2 px-3.5 py-1.5 rounded-xl bg-zinc-950 hover:bg-zinc-800 disabled:opacity-50 text-white text-xs font-semibold shadow-xs transition-colors"
              >
                <span>{publishing ? 'Publicando…' : 'Publicar'}</span>
                <span className="h-3.5 w-px bg-zinc-700" />
                <ChevronDown className="w-3.5 h-3.5 text-zinc-300" />
              </button>
              {hasUnsavedDraft && (
                <span
                  className="absolute -top-1 -right-1 w-2.5 h-2.5 rounded-full bg-amber-500 ring-2 ring-white"
                  title="Há alterações não publicadas neste rascunho"
                />
              )}
            </div>
          </div>
        </header>

        {/* Banner de feedback rápido se houver mensagem */}
        {message && (
          <div
            className={`mx-6 mt-4 px-4 py-2.5 rounded-xl border text-xs flex items-center justify-between ${
              message.type === 'success'
                ? 'bg-emerald-50 border-emerald-200 text-emerald-800'
                : 'bg-red-50 border-red-200 text-red-800'
            }`}
          >
            <span className="flex items-center gap-2">
              {message.type === 'success' ? (
                <Check className="w-4 h-4 text-emerald-600" />
              ) : (
                <AlertCircle className="w-4 h-4 text-red-600" />
              )}
              {message.text}
            </span>
            <button onClick={() => setMessage(null)} className="text-zinc-400 hover:text-zinc-700">
              <X className="w-3.5 h-3.5" />
            </button>
          </div>
        )}

        {/* CONTEÚDO ROLÁVEL DA SEÇÃO ATIVA */}
        <main className="flex-1 overflow-y-auto">
          {currentTab === 'geral' && (
            /* ===============================================================
               ORGANIZAÇÃO IDÊNTICA ÀS IMAGENS 1 E 2 (TELA "AGENTE")
               =============================================================== */
            <div className="max-w-6xl mx-auto px-6 sm:px-10 py-8 space-y-6">
              {/* Cabeçalho "Agente [Novo Ver novas funcionalidades >]" + "+ Adicionar tag" */}
              <div className="space-y-2.5">
                <div className="flex flex-wrap items-center gap-3">
                  <h1 className="text-2xl font-bold tracking-tight text-zinc-950">Agente</h1>
                  <button
                    type="button"
                    onClick={() => selectSection('skills')}
                    className="inline-flex items-center gap-2 px-2.5 py-1 rounded-full bg-white hover:bg-zinc-50 border border-zinc-200/90 text-xs text-zinc-700 shadow-2xs transition-colors"
                  >
                    <span className="px-1.5 py-0.2 rounded-full bg-zinc-950 text-white text-[10px] font-semibold">
                      Novo
                    </span>
                    <span className="font-medium">Ver novas funcionalidades</span>
                    <ChevronRight className="w-3.5 h-3.5 text-zinc-400" />
                  </button>
                </div>

                {/* Linha de Tags (+ Adicionar tag) */}
                <div className="flex flex-wrap items-center gap-1.5">
                  {tags.map((t, i) => (
                    <span
                      key={i}
                      className="inline-flex items-center gap-1 px-2.5 py-0.5 rounded-lg bg-zinc-100 text-zinc-700 border border-zinc-200 text-xs font-medium"
                    >
                      {t}
                      <button
                        type="button"
                        onClick={() => setTags(tags.filter((_, idx) => idx !== i))}
                        className="text-zinc-400 hover:text-zinc-700"
                      >
                        <X className="w-3 h-3" />
                      </button>
                    </span>
                  ))}

                  {addingTag ? (
                    <input
                      type="text"
                      autoFocus
                      value={newTagText}
                      onChange={(e) => setNewTagText(e.target.value)}
                      onBlur={() => setAddingTag(false)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter' && newTagText.trim()) {
                          e.preventDefault()
                          setTags([...tags, newTagText.trim()])
                          setNewTagText('')
                          setAddingTag(false)
                        } else if (e.key === 'Escape') {
                          setAddingTag(false)
                        }
                      }}
                      placeholder="Nome da tag..."
                      className="px-2.5 py-0.5 bg-white border border-zinc-300 rounded-lg text-xs text-zinc-900 focus:outline-none"
                    />
                  ) : (
                    <button
                      type="button"
                      onClick={() => setAddingTag(true)}
                      className="inline-flex items-center gap-1 px-2.5 py-1 rounded-lg bg-white hover:bg-zinc-50 border border-zinc-200/90 text-xs font-medium text-zinc-700 shadow-2xs transition-colors"
                    >
                      <Plus className="w-3 h-3" />
                      Adicionar tag
                    </button>
                  )}
                </div>
              </div>

              {/* GRID DE 2 COLUNAS ASSIMÉTRICAS (Esquerda: Prompts | Direita: Vozes, Idioma, LLM, Comportamento) */}
              <div className="grid grid-cols-1 lg:grid-cols-12 gap-8 items-start">
                {/* COLUNA ESQUERDA (8 colunas): Prompt do sistema + Primeira mensagem */}
                <div className="lg:col-span-7 xl:col-span-8 space-y-6">
                  <RichPromptCard
                    title="Prompt do sistema"
                    showBookIcon={true}
                    showWandButton={true}
                    shadedBg={true}
                    value={form.system_prompt}
                    onChange={(next) => updateFormField({ system_prompt: next })}
                    placeholder="# Role\nVocê é uma consultora especialista em atendimento. Sua missão é resolver a solicitação do cliente de forma ágil, empática e segura..."
                    rows={10}
                    declaredVariables={form.variables || []}
                    timezone={form.timezone}
                    onTimezoneChange={(tz) => {
                      updateFormField({ timezone: tz })
                      saveDraft({ timezone: tz })
                    }}
                  />

                  <RichPromptCard
                    title="Primeira mensagem"
                    subtitle="A primeira mensagem que o agente dirá. Se estiver vazia, o agente aguardará o usuário iniciar a conversa."
                    showDisclosureLink={true}
                    shadedBg={false}
                    value={form.first_message}
                    onChange={(next) => updateFormField({ first_message: next })}
                    placeholder="[super fast] Alô, {{horario}}! [super fast] Por gentileza, eu falo com {{primNome}}?"
                    rows={3}
                    declaredVariables={form.variables || []}
                    interruptible={form.first_message_interruptible}
                    onInterruptibleChange={(val) => {
                      updateFormField({ first_message_interruptible: val })
                      saveDraft({ first_message_interruptible: val })
                    }}
                  />
                </div>

                {/* COLUNA DIREITA (4 colunas): Os 4 Cards Compactos (Vozes, Idioma, LLM, Comportamento do agente) */}
                <div className="lg:col-span-5 xl:col-span-4 space-y-6">
                  {/* 1. Bloco VOZES */}
                  <div className="space-y-2">
                    <div className="flex items-start justify-between gap-2">
                      <div>
                        <h3 className="text-[13px] font-semibold text-zinc-900">Vozes</h3>
                        <p className="text-xs text-zinc-500 mt-0.5 leading-relaxed">
                          Selecione as vozes da ElevenLabs que você deseja usar para o agente.
                        </p>
                      </div>
                      <div className="flex items-center gap-1 flex-shrink-0">
                        <button
                          type="button"
                          onClick={() => setActiveRightDrawer('audio_env')}
                          onMouseEnter={(e) =>
                            setCursorTooltip({
                              text: 'Ambiente de áudio',
                              x: e.clientX,
                              y: e.clientY,
                            })
                          }
                          onMouseMove={(e) =>
                            setCursorTooltip({
                              text: 'Ambiente de áudio',
                              x: e.clientX,
                              y: e.clientY,
                            })
                          }
                          onMouseLeave={() => setCursorTooltip(null)}
                          className="w-7 h-7 rounded-lg border border-zinc-200/90 bg-white hover:bg-zinc-50 flex items-center justify-center text-zinc-600 shadow-2xs transition-colors"
                        >
                          <Volume2 className="w-3.5 h-3.5" />
                        </button>
                        <button
                          type="button"
                          onClick={() => setActiveRightDrawer('voice_settings')}
                          onMouseEnter={(e) =>
                            setCursorTooltip({
                              text: 'Configurações comuns de voz',
                              x: e.clientX,
                              y: e.clientY,
                            })
                          }
                          onMouseMove={(e) =>
                            setCursorTooltip({
                              text: 'Configurações comuns de voz',
                              x: e.clientX,
                              y: e.clientY,
                            })
                          }
                          onMouseLeave={() => setCursorTooltip(null)}
                          className="w-7 h-7 rounded-lg border border-zinc-200/90 bg-white hover:bg-zinc-50 flex items-center justify-center text-zinc-600 shadow-2xs transition-colors"
                        >
                          <Settings className="w-3.5 h-3.5" />
                        </button>
                      </div>
                    </div>

                    <div className="bg-white border border-zinc-200/90 rounded-2xl overflow-hidden shadow-2xs divide-y divide-zinc-100 text-xs">
                      <button
                        type="button"
                        onClick={() => setActiveRightDrawer('voices')}
                        className="w-full px-3.5 py-3 flex items-center justify-between hover:bg-zinc-50 transition-colors text-left"
                      >
                        <div className="flex items-center gap-2.5 min-w-0">
                          <div className="w-5 h-5 rounded-full bg-gradient-to-tr from-blue-600 via-indigo-400 to-purple-300 flex-shrink-0 shadow-xs" />
                          <span className="font-semibold text-zinc-900 truncate">
                            {currentVoiceObj.label}
                          </span>
                        </div>
                        <div className="flex items-center gap-2 flex-shrink-0 ml-2">
                          <span className="px-2 py-0.5 rounded-md bg-zinc-100 text-zinc-700 font-medium text-[11px]">
                            Primário
                          </span>
                          <ChevronRight className="w-3.5 h-3.5 text-zinc-400" />
                        </div>
                      </button>

                      <button
                        type="button"
                        onClick={() => setActiveRightDrawer('voices')}
                        className="w-full px-3.5 py-2.5 flex items-center gap-2 text-zinc-500 hover:text-zinc-800 hover:bg-zinc-50/70 transition-colors text-left"
                      >
                        <PlusCircle className="w-3.5 h-3.5 text-zinc-400" />
                        <span>Adicionar voz adicional</span>
                      </button>
                    </div>
                  </div>

                  {/* 2. Bloco IDIOMA */}
                  <div className="space-y-2">
                    <div>
                      <h3 className="text-[13px] font-semibold text-zinc-900">Idioma</h3>
                      <p className="text-xs text-zinc-500 mt-0.5 leading-relaxed">
                        Escolha os idiomas padrão e adicionais em que o agente se comunicará.
                      </p>
                    </div>

                    <div className="bg-white border border-zinc-200/90 rounded-2xl overflow-hidden shadow-2xs divide-y divide-zinc-100 text-xs">
                      <button
                        type="button"
                        onClick={() => setActiveRightDrawer('language')}
                        className="w-full px-3.5 py-3 flex items-center justify-between hover:bg-zinc-50 transition-colors text-left"
                      >
                        <div className="flex items-center gap-2.5 min-w-0">
                          <span className="text-base leading-none">{currentLangMeta.flag}</span>
                          <span className="font-semibold text-zinc-900 truncate">
                            {currentLangMeta.label}
                          </span>
                        </div>
                        <div className="flex items-center gap-2 flex-shrink-0 ml-2">
                          <span className="px-2 py-0.5 rounded-md bg-zinc-100 text-zinc-700 font-medium text-[11px]">
                            Padrão
                          </span>
                          <ChevronRight className="w-3.5 h-3.5 text-zinc-400" />
                        </div>
                      </button>

                      <button
                        type="button"
                        onClick={() => setActiveRightDrawer('language')}
                        className="w-full px-3.5 py-2.5 flex items-center gap-2 text-zinc-500 hover:text-zinc-800 hover:bg-zinc-50/70 transition-colors text-left"
                      >
                        <PlusCircle className="w-3.5 h-3.5 text-zinc-400" />
                        <span>Adicionar idiomas adicionais</span>
                      </button>
                    </div>
                  </div>

                  {/* 3. Bloco LLM */}
                  <div className="space-y-2">
                    <div>
                      <h3 className="text-[13px] font-semibold text-zinc-900">LLM</h3>
                      <p className="text-xs text-zinc-500 mt-0.5 leading-relaxed">
                        Selecione qual provedor e modelo usar para o LLM.
                      </p>
                    </div>

                    <button
                      type="button"
                      onClick={() => setActiveRightDrawer('llm')}
                      className="w-full px-3.5 py-3 bg-white hover:bg-zinc-50 border border-zinc-200/90 rounded-2xl shadow-2xs flex items-center justify-between text-xs transition-colors text-left"
                    >
                      <span className="font-semibold text-zinc-900 flex items-center gap-1.5 truncate">
                        {currentModelObj.label}
                        <Info className="w-3.5 h-3.5 text-zinc-400 flex-shrink-0" />
                      </span>
                      <ChevronRight className="w-3.5 h-3.5 text-zinc-400 flex-shrink-0" />
                    </button>
                  </div>

                  {/* 4. Bloco COMPORTAMENTO DO AGENTE */}
                  <div className="space-y-2">
                    <div>
                      <h3 className="text-[13px] font-semibold text-zinc-900">
                        Comportamento do agente
                      </h3>
                      <p className="text-xs text-zinc-500 mt-0.5 leading-relaxed">
                        Comportamento de resposta específico de personalidade e canal.
                      </p>
                    </div>

                    <button
                      type="button"
                      onClick={() => setActiveRightDrawer('behavior')}
                      className="w-full px-3.5 py-3 bg-white hover:bg-zinc-50 border border-zinc-200/90 rounded-2xl shadow-2xs flex items-center justify-between text-xs transition-colors text-left"
                    >
                      <span className="font-semibold text-zinc-900 truncate">
                        {form.model.includes('extended-thinking')
                          ? 'Extended Thinking (Nível Médio)'
                          : 'Personalidade padrão desativada'}
                      </span>
                      <ChevronRight className="w-3.5 h-3.5 text-zinc-400 flex-shrink-0" />
                    </button>
                  </div>
                </div>
              </div>
            </div>
          )}

          {/* SEÇÃO: PROCEDIMENTOS (Workspace Estilo Notion) */}
          {currentTab === 'skills' && (
            <div className="p-4 sm:p-6">
              <NotionProcedureWorkspace
                agents={[detail]}
                selectedAgentId={detail.id}
                lockAgent={true}
                procedures={skills.map((s) => ({ ...s, agent_id: detail.id }))}
                tools={availableTools}
                knowledgeBases={knowledgeBases}
                onSaveProcedure={async (payload) => {
                  if (payload.id) {
                    await api(`/api/agents/${detail.id}/procedures/${payload.id}`, {
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
                    await api(`/api/agents/${detail.id}/procedures`, {
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
                  const updatedProcs = await api<Skill[]>(
                    `/api/agents/${detail.id}/procedures`,
                  ).catch(() => [])
                  setSkills(updatedProcs)
                  setHasUnsavedDraft(true)
                  haptic(8)
                }}
                onDeleteProcedure={async (proc) => {
                  await removeSkill(proc.id)
                }}
              />
            </div>
          )}

          {/* SEÇÃO: FERRAMENTAS (Tabela + Ferramentas do Sistema) */}
          {currentTab === 'tools' && (
            <div className="max-w-6xl mx-auto px-6 sm:px-10 py-8">
              <ElevenToolsPanel
                currentAgentId={detail.id}
                tools={availableTools}
                enabledToolIds={form.tools || []}
                systemTools={form.system_tools || ['encerrar_conversa']}
                systemToolsConfig={form.system_tools_config}
                referencedByProceduresToolIds={skills.flatMap((s) => s.referenced_tool_ids || [])}
                onToggleTool={(tid) => toggleTool(tid)}
                onToggleSystemTool={(sysName) => {
                  const current = form.system_tools || []
                  const updated = current.includes(sysName)
                    ? current.filter((x) => x !== sysName)
                    : [...current, sysName]
                  updateFormField({ system_tools: updated })
                  saveDraft({ system_tools: updated })
                }}
                onSaveSystemToolsConfig={(nextConfig, enableToolName) => {
                  const currentSys = form.system_tools || []
                  const updatedSys =
                    enableToolName && !currentSys.includes(enableToolName)
                      ? [...currentSys, enableToolName]
                      : currentSys
                  updateFormField({
                    system_tools: updatedSys,
                    system_tools_config: nextConfig,
                  })
                  saveDraft({
                    system_tools: updatedSys,
                    system_tools_config: nextConfig,
                  })
                }}
                onOpenNewToolModal={(kind) => {
                  setEditingAgentTool(null)
                  setToolDrawerKind(kind || 'webhook')
                  setIsToolModalOpen(true)
                }}
                onEditTool={(t) => {
                  setEditingAgentTool(t)
                  setToolDrawerKind((t.kind as ToolCategory) || 'webhook')
                  setIsToolModalOpen(true)
                }}
              />
            </div>
          )}

          {/* SEÇÃO: CONTROLE DE VERSÃO */}
          {currentTab === 'versoes' && (
            <div className="max-w-4xl mx-auto px-6 sm:px-10 py-8 space-y-4">
              <div className="flex items-center justify-between">
                <div>
                  <h2 className="text-xl font-bold text-zinc-900">Controle de versão</h2>
                  <p className="text-xs text-zinc-500 mt-0.5">
                    Histórico imutável de snapshots publicados deste agente e seus procedimentos.
                  </p>
                </div>
                <Button size="sm" onClick={handlePublish} disabled={publishing}>
                  Publicar versão agora
                </Button>
              </div>

              <div className="bg-white border border-zinc-200 rounded-2xl divide-y divide-zinc-100 text-xs">
                <div className="p-4 flex items-center justify-between bg-amber-50/40">
                  <div className="flex items-center gap-2.5">
                    <span className="w-2.5 h-2.5 rounded-full bg-amber-500" />
                    <div>
                      <span className="font-semibold text-zinc-900">Draft atual (Main)</span>
                      <p className="text-[11px] text-zinc-500">
                        Versão editável usada na Pré-visualização
                      </p>
                    </div>
                  </div>
                  <span className="px-2.5 py-0.5 rounded-full bg-amber-100 text-amber-800 font-medium text-[11px]">
                    Em edição
                  </span>
                </div>

                {versionsList.map((v) => (
                  <div key={v.version} className="p-4 flex items-center justify-between">
                    <div className="flex items-center gap-2.5">
                      <GitFork className="w-4 h-4 text-zinc-400" />
                      <div>
                        <span className="font-semibold text-zinc-900">Versão v{v.version}</span>
                        <p className="text-[11px] text-zinc-500">
                          Publicada em {new Date(v.created_at).toLocaleString('pt-BR')}
                        </p>
                      </div>
                    </div>
                    <span className="px-2.5 py-0.5 rounded-full bg-emerald-50 text-emerald-700 border border-emerald-200 font-medium text-[11px]">
                      Snapshot imutável
                    </span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* SEÇÃO: BASE DE CONHECIMENTO */}
          {currentTab === 'kb' && (
            <div className="max-w-4xl mx-auto px-6 sm:px-10 py-8 space-y-4">
              <div className="flex items-center justify-between">
                <div>
                  <h2 className="text-xl font-bold text-zinc-900">Base de conhecimento</h2>
                  <p className="text-xs text-zinc-500 mt-0.5">
                    Documentos que podem ser referenciados nos procedimentos via comando <code className="font-mono">/</code>.
                  </p>
                </div>
                <Button size="sm" onClick={() => navigate('/knowledge')}>
                  Gerenciar Biblioteca
                </Button>
              </div>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                {knowledgeBases.map((kb) => (
                  <div
                    key={kb.id}
                    className="p-4 bg-white border border-zinc-200 rounded-2xl space-y-1 text-xs"
                  >
                    <div className="font-semibold text-zinc-900 flex items-center gap-2">
                      <FolderOpen className="w-4 h-4 text-blue-600" />
                      {kb.name}
                    </div>
                    <p className="text-zinc-500">{kb.description || 'Sem descrição'}</p>
                    <span className="text-[11px] text-zinc-400 block pt-1">
                      {kb.documents_count} documento(s)
                    </span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* SEÇÃO: CONFIGURAÇÕES AVANÇADAS & TWILIO PERSONALIZATION WEBHOOK */}
          {currentTab === 'configuracoes' && (
            <div className="max-w-4xl mx-auto px-6 sm:px-10 py-8 space-y-6">
              <div>
                <h2 className="text-xl font-bold text-zinc-900">
                  Configurações & Personalização Twilio (Initiation Webhook)
                </h2>
                <p className="text-xs text-zinc-500 mt-0.5">
                  Busque dados de início de conversa (`dynamic_variables` e `conversation_config_override`) em paralelo enquanto a chamada Twilio está conectando.
                </p>
              </div>

              <div className="bg-white border border-zinc-200 rounded-2xl p-6 space-y-5 text-xs">
                <div className="grid grid-cols-1 sm:grid-cols-4 gap-4">
                  <div className="sm:col-span-3">
                    <label className="block font-semibold text-zinc-800 mb-1">
                      Conversation Initiation Webhook URL (POST)
                    </label>
                    <input
                      type="url"
                      value={form.initiation_webhook_url}
                      onChange={(e) => updateFormField({ initiation_webhook_url: e.target.value })}
                      placeholder="https://seu-n8n-ou-api.com/webhook/twilio-personalization"
                      className="w-full px-3 py-2 bg-zinc-50 border border-zinc-200 rounded-xl font-mono text-zinc-900"
                    />
                  </div>

                  <div>
                    <label className="block font-semibold text-zinc-800 mb-1">
                      Timeout (ms)
                    </label>
                    <input
                      type="number"
                      min={500}
                      max={10000}
                      value={form.initiation_webhook_timeout_ms}
                      onChange={(e) =>
                        updateFormField({
                          initiation_webhook_timeout_ms: parseInt(e.target.value) || 3000,
                        })
                      }
                      className="w-full px-3 py-2 bg-zinc-50 border border-zinc-200 rounded-xl font-mono text-zinc-900"
                    />
                  </div>
                </div>

                {/* Testador Interativo do Webhook de Início de Chamada */}
                <div className="p-4 bg-zinc-50 border border-zinc-200/80 rounded-2xl space-y-3">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div>
                      <span className="font-semibold text-zinc-900 block">
                        Testar Webhook de Início (Twilio Personalization)
                      </span>
                      <span className="text-[11px] text-zinc-500">
                        Envia um POST com <code className="font-mono">caller_id</code>, <code className="font-mono">agent_id</code>, <code className="font-mono">called_number</code>, <code className="font-mono">call_sid</code> e <code className="font-mono">conversation_id</code>.
                      </span>
                    </div>
                    <Button
                      size="sm"
                      type="button"
                      disabled={testingWebhook || !form.initiation_webhook_url}
                      onClick={async () => {
                        setTestingWebhook(true)
                        setWebhookTestResult(null)
                        try {
                          await saveDraft()
                          const res = await api<any>(`/api/agents/${id}/initiation-webhook/test`, {
                            method: 'POST',
                            body: JSON.stringify({
                              caller_id: testCallerID,
                              called_number: testCalledNumber,
                              call_sid: 'CA_test_' + Date.now(),
                            }),
                          })
                          setWebhookTestResult({ ok: true, data: res })
                        } catch (err: any) {
                          setWebhookTestResult({ ok: false, error: err.message || 'Erro no webhook' })
                        } finally {
                          setTestingWebhook(false)
                        }
                      }}
                    >
                      {testingWebhook ? 'Testando…' : 'Disparar Teste de Webhook'}
                    </Button>
                  </div>

                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                    <div>
                      <label className="block text-[11px] font-medium text-zinc-500 mb-1">
                        caller_id (Número do Cliente)
                      </label>
                      <input
                        type="text"
                        value={testCallerID}
                        onChange={(e) => setTestCallerID(e.target.value)}
                        className="w-full px-3 py-1.5 bg-white border border-zinc-200 rounded-lg font-mono text-xs"
                      />
                    </div>
                    <div>
                      <label className="block text-[11px] font-medium text-zinc-500 mb-1">
                        called_number (Número Twilio Discado)
                      </label>
                      <input
                        type="text"
                        value={testCalledNumber}
                        onChange={(e) => setTestCalledNumber(e.target.value)}
                        className="w-full px-3 py-1.5 bg-white border border-zinc-200 rounded-lg font-mono text-xs"
                      />
                    </div>
                  </div>

                  {webhookTestResult && (
                    <div
                      className={`p-3 rounded-xl border font-mono text-[11px] whitespace-pre-wrap overflow-x-auto ${
                        webhookTestResult.ok
                          ? 'bg-emerald-50/70 border-emerald-200 text-emerald-950'
                          : 'bg-red-50/70 border-red-200 text-red-900'
                      }`}
                    >
                      {webhookTestResult.ok
                        ? JSON.stringify(webhookTestResult.data, null, 2)
                        : `Falha: ${webhookTestResult.error}`}
                    </div>
                  )}
                </div>

                {/* Exemplo de Payload esperado no padrão ElevenLabs */}
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4 pt-1">
                  <div className="p-3.5 bg-zinc-950 text-zinc-200 rounded-xl space-y-1.5">
                    <span className="text-[10px] uppercase tracking-wider text-zinc-400 font-semibold block">
                      POST enviado ao seu Webhook
                    </span>
                    <pre className="font-mono text-[11px] leading-relaxed overflow-x-auto">{`{
  "caller_id": "+5511999998888",
  "agent_id": "${id}",
  "called_number": "+551140041234",
  "call_sid": "CA1234567890abcdef",
  "conversation_id": "conv_uuid"
}`}</pre>
                  </div>

                  <div className="p-3.5 bg-zinc-950 text-zinc-200 rounded-xl space-y-1.5">
                    <span className="text-[10px] uppercase tracking-wider text-zinc-400 font-semibold block">
                      Resposta esperada do seu Webhook
                    </span>
                    <pre className="font-mono text-[11px] leading-relaxed overflow-x-auto">{`{
  "type": "conversation_initiation_client_data",
  "dynamic_variables": {
    "primNome": "Carlos"
  },
  "conversation_config_override": {
    "agent": {
      "first_message": "Olá Carlos!"
    }
  }
}`}</pre>
                  </div>
                </div>

                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 pt-2 border-t border-zinc-100">
                  <div>
                    <label className="block font-semibold text-zinc-800 mb-1">
                      Duração Máxima da Chamada (segundos)
                    </label>
                    <input
                      type="number"
                      value={form.max_duration_seconds}
                      onChange={(e) =>
                        updateFormField({
                          max_duration_seconds: parseInt(e.target.value) || 600,
                        })
                      }
                      className="w-full px-3 py-2 bg-zinc-50 border border-zinc-200 rounded-xl text-zinc-900"
                    />
                  </div>

                  <div className="flex items-center justify-between pt-5">
                    <span className="font-semibold text-zinc-800">Gravar áudio das chamadas</span>
                    <Toggle
                      checked={form.recording_enabled}
                      onChange={(val) => updateFormField({ recording_enabled: val })}
                      label="Gravar chamadas"
                    />
                  </div>
                </div>

                <div className="pt-3 border-t border-zinc-100 flex justify-end">
                  <Button size="sm" onClick={() => saveDraft()}>
                    Salvar configurações
                  </Button>
                </div>
              </div>
            </div>
          )}

          {/* SEÇÃO: CONVERSAS */}
          {currentTab === 'conversas' && (
            <div className="max-w-5xl mx-auto px-6 sm:px-10 py-8 space-y-4">
              <div className="flex items-center justify-between">
                <h2 className="text-xl font-bold text-zinc-900">Conversas de {form.name}</h2>
                <Link to="/conversations" className="text-xs text-zinc-600 hover:text-zinc-900">
                  Ver todas da Organização →
                </Link>
              </div>

              <div className="bg-white border border-zinc-200 rounded-2xl divide-y divide-zinc-100 text-xs">
                {agentConversations.length === 0 ? (
                  <div className="p-10 text-center text-zinc-400">
                    Nenhuma conversa registrada ainda para este agente.
                  </div>
                ) : (
                  agentConversations.map((c) => (
                    <div key={c.id} className="p-4 flex items-center justify-between">
                      <div>
                        <span className="font-semibold text-zinc-900">
                          {new Date(c.started_at).toLocaleString('pt-BR')}
                        </span>
                        <span className="ml-3 text-zinc-400 font-mono">
                          {c.duration_ms ? `${Math.round(c.duration_ms / 1000)}s` : '0s'}
                        </span>
                      </div>
                      <Link
                        to={`/conversations/${c.id}`}
                        className="px-3 py-1 rounded-lg bg-zinc-100 hover:bg-zinc-200 text-zinc-800 font-medium"
                      >
                        Inspecionar →
                      </Link>
                    </div>
                  ))
                )}
              </div>
            </div>
          )}

          {/* SEÇÃO: PAINÉIS / ANALYTICS */}
          {currentTab === 'analytics' && (
            <div className="max-w-5xl mx-auto px-6 sm:px-10 py-8 space-y-6">
              <h2 className="text-xl font-bold text-zinc-900">Painéis e Métricas de {form.name}</h2>
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
                <div className="p-5 bg-white border border-zinc-200 rounded-2xl">
                  <span className="text-xs text-zinc-400">Total de Conversas</span>
                  <div className="text-2xl font-bold text-zinc-900 mt-1">
                    {agentConversations.length}
                  </div>
                </div>
                <div className="p-5 bg-white border border-zinc-200 rounded-2xl">
                  <span className="text-xs text-zinc-400">Procedimentos Ativos</span>
                  <div className="text-2xl font-bold text-zinc-900 mt-1">{skills.length}</div>
                </div>
                <div className="p-5 bg-white border border-zinc-200 rounded-2xl">
                  <span className="text-xs text-zinc-400">Taxa de Conclusão</span>
                  <div className="text-2xl font-bold text-emerald-600 mt-1">100%</div>
                </div>
              </div>
            </div>
          )}
        </main>
      </div>

      {/* =====================================================================
          GAVETAS DESLIZANTES DA DIREITA (SHEETS — IGUAIS AO VÍDEO!)
         ===================================================================== */}

      {/* Balão flutuante escuro acima do cursor do mouse (Igual à Imagem 2!) */}
      {cursorTooltip && (
        <div
          style={{
            left: cursorTooltip.x,
            top: cursorTooltip.y - 12,
          }}
          className="fixed z-[100] -translate-x-1/2 -translate-y-full pointer-events-none px-3.5 py-1.5 rounded-full bg-zinc-950 text-white text-xs font-semibold shadow-xl whitespace-nowrap transition-transform duration-75"
        >
          {cursorTooltip.text}
        </div>
      )}

      {/* 1. Gaveta de Seleção de Vozes (Com Scroll completo, Preview em cada voz e Balão de características acima do cursor!) */}
      <Sheet
        open={activeRightDrawer === 'voices'}
        onClose={() => {
          setCursorTooltip(null)
          stopVoiceSample()
          setActiveRightDrawer(null)
        }}
        side="right"
        sizeClass="max-w-md"
      >
        <div className="h-full flex flex-col overflow-hidden text-xs">
          {/* Cabeçalho fixo + Busca de Vozes */}
          <div className="px-6 pt-6 pb-4 border-b border-zinc-100 space-y-3 flex-shrink-0 bg-white">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2 font-semibold text-sm text-zinc-900">
                <Volume2 className="w-4 h-4 text-zinc-700" />
                Selecionar Voz do Agente
              </div>
              <button
                onClick={() => {
                  setCursorTooltip(null)
                  stopVoiceSample()
                  setActiveRightDrawer(null)
                }}
                className="text-zinc-400 hover:text-zinc-700 p-1 rounded-lg hover:bg-zinc-100"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            <div className="relative">
              <Search className="w-3.5 h-3.5 text-zinc-400 absolute left-3 top-1/2 -translate-y-1/2" />
              <input
                type="text"
                value={voiceSearch}
                onChange={(e) => setVoiceSearch(e.target.value)}
                placeholder="Buscar voz por nome ou característica (ex: Feminina, Jovem, Firme)..."
                className="w-full pl-8 pr-3 py-2 bg-zinc-50 border border-zinc-200 rounded-xl text-xs text-zinc-900 focus:outline-none focus:bg-white focus:border-zinc-400"
              />
            </div>
          </div>

          {/* Lista de Vozes com Scroll Vertical Fluido */}
          <div className="flex-1 overflow-y-auto overscroll-contain px-6 py-4 space-y-2.5">
            {currentCatalog.voices
              .filter((v) => {
                if (!voiceSearch.trim()) return true
                const q = voiceSearch.toLowerCase()
                const traits = VOICE_TRAITS_MAP[v.id]
                return (
                  v.id.toLowerCase().includes(q) ||
                  v.label.toLowerCase().includes(q) ||
                  (v.description || '').toLowerCase().includes(q) ||
                  (traits?.badge || '').toLowerCase().includes(q) ||
                  (traits?.tooltip || '').toLowerCase().includes(q)
                )
              })
              .map((v) => {
                const isSelected = form.voice === v.id
                const isPlayingThis = previewingVoiceId === v.id
                const isLoadingThis = loadingVoiceId === v.id
                const traits = VOICE_TRAITS_MAP[v.id] || {
                  badge: v.description || 'Natural e Expressiva',
                  tooltip: `${v.label} — ${v.description || 'Voz neural natural de baixa latência'}`,
                }

                return (
                  <div
                    key={v.id}
                    onMouseEnter={(e) =>
                      setCursorTooltip({
                        text: traits.tooltip,
                        x: e.clientX,
                        y: e.clientY,
                      })
                    }
                    onMouseMove={(e) =>
                      setCursorTooltip({
                        text: traits.tooltip,
                        x: e.clientX,
                        y: e.clientY,
                      })
                    }
                    onMouseLeave={() => setCursorTooltip(null)}
                    onClick={() => {
                      updateFormField({ voice: v.id })
                      saveDraft({ voice: v.id })
                    }}
                    className={`p-3.5 rounded-2xl border cursor-pointer flex items-center justify-between gap-3 transition-all ${
                      isSelected
                        ? 'border-zinc-900 bg-zinc-50 ring-1 ring-zinc-900'
                        : 'border-zinc-200 hover:border-zinc-300 bg-white hover:bg-zinc-50/50'
                    }`}
                  >
                    <div className="flex items-center gap-3 min-w-0">
                      <div className="w-8 h-8 rounded-full bg-gradient-to-tr from-blue-600 via-indigo-400 to-purple-300 flex-shrink-0 flex items-center justify-center shadow-xs">
                        {isPlayingThis && (
                          <span className="w-2.5 h-2.5 rounded-full bg-white animate-ping" />
                        )}
                      </div>
                      <div className="min-w-0">
                        <div className="font-semibold text-zinc-900 flex items-center gap-2 truncate">
                          <span>{v.label}</span>
                          <span className="text-[11px] font-normal text-zinc-500 truncate">
                            {traits.badge}
                          </span>
                        </div>
                        <div className="text-[11px] text-zinc-400 font-mono truncate">
                          voice_{v.id.toLowerCase()}
                        </div>
                      </div>
                    </div>

                    <div className="flex items-center gap-2 flex-shrink-0">
                      {isSelected && (
                        <span className="px-2 py-0.5 rounded-md bg-zinc-900 text-white text-[10px] font-semibold">
                          Primário
                        </span>
                      )}

                      {/* Botão de Ouvir Preview Individual da Voz */}
                      <button
                        type="button"
                        onClick={(e) => {
                          e.stopPropagation()
                          playVoiceSample(v.id)
                        }}
                        title={isPlayingThis ? 'Parar amostra' : `Ouvir amostra da voz ${v.label}`}
                        className={`w-8 h-8 rounded-xl border flex items-center justify-center transition-all ${
                          isPlayingThis || isLoadingThis
                            ? 'bg-zinc-900 border-zinc-900 text-white'
                            : 'bg-white border-zinc-200 text-zinc-700 hover:bg-zinc-100 hover:border-zinc-300'
                        }`}
                      >
                        {isLoadingThis ? (
                          <span className="w-3.5 h-3.5 rounded-full border-2 border-white border-t-transparent animate-spin" />
                        ) : isPlayingThis ? (
                          <Square className="w-3 h-3 fill-current" />
                        ) : (
                          <Play className="w-3.5 h-3.5 fill-current ml-0.5" />
                        )}
                      </button>
                    </div>
                  </div>
                )
              })}
          </div>

          {/* Rodapé fixo da Gaveta de Vozes */}
          <div className="p-4 border-t border-zinc-100 bg-white flex-shrink-0">
            <button
              type="button"
              onClick={toggleVoicePreview}
              className="w-full py-2.5 bg-zinc-900 hover:bg-zinc-800 text-white rounded-xl font-semibold flex items-center justify-center gap-2 transition-colors"
            >
              {loadingVoiceId === form.voice ? (
                <>
                  <span className="w-3.5 h-3.5 rounded-full border-2 border-white border-t-transparent animate-spin" />
                  Gerando amostra da voz {form.voice}…
                </>
              ) : isPlayingPreview ? (
                <>
                  <Square className="w-3.5 h-3.5 fill-current" /> Parar Amostra ({previewingVoiceId || form.voice})
                </>
              ) : (
                <>
                  <Play className="w-3.5 h-3.5 fill-current" /> Ouvir Amostra da Voz Selecionada ({form.voice})
                </>
              )}
            </button>
          </div>
        </div>
      </Sheet>

      {/* 2. Gaveta: Configurações comuns de voz (Igual ao Frame 4 do vídeo!) */}
      <Sheet
        open={activeRightDrawer === 'voice_settings'}
        onClose={() => setActiveRightDrawer(null)}
        side="right"
        sizeClass="max-w-md"
      >
        <div className="p-6 space-y-6 text-xs">
          <div className="flex items-center justify-between border-b border-zinc-100 pb-4">
            <div className="flex items-center gap-2 font-semibold text-sm text-zinc-900">
              <Settings className="w-4 h-4 text-zinc-700" />
              Configurações comuns de voz
            </div>
            <button onClick={() => setActiveRightDrawer(null)} className="text-zinc-400 hover:text-zinc-700">
              <X className="w-4 h-4" />
            </button>
          </div>

          <div className="space-y-2">
            <label className="block font-semibold text-zinc-900">Formato de saída TTS</label>
            <p className="text-zinc-500 leading-relaxed">
              Selecione o formato de saída de áudio para chamadas telefônicas (Twilio / SIP) e Web.
            </p>
            <select
              value={ttsFormat}
              onChange={(e) => setTtsFormat(e.target.value)}
              className="w-full px-3.5 py-2.5 bg-white border border-zinc-200 rounded-xl font-medium text-zinc-900"
            >
              <option value="ulaw_8000">μ-law 8000 Hz (Telefonia SIP / Twilio)</option>
              <option value="pcm_16000">PCM 16000 Hz</option>
              <option value="pcm_24000">PCM 24000 Hz (Alta Definição Web)</option>
            </select>
          </div>

          <div className="space-y-2">
            <label className="block font-semibold text-zinc-900">
              Tipo de normalização de texto (apenas voz)
            </label>
            <p className="text-zinc-500 leading-relaxed">
              Converte números, valores monetários e datas em palavras por extenso para pronúncia natural.
            </p>
            <div className="grid grid-cols-2 gap-2 p-1 bg-zinc-100 rounded-xl">
              <button
                type="button"
                className="py-2 rounded-lg bg-white text-zinc-900 font-semibold shadow-2xs"
              >
                Prompt do sistema
              </button>
              <button type="button" className="py-2 rounded-lg text-zinc-600 font-medium">
                Automático
              </button>
            </div>
          </div>
        </div>
      </Sheet>

      {/* 3. Gaveta: Ambiente de áudio (Igual ao Frame 5 do vídeo!) */}
      <Sheet
        open={activeRightDrawer === 'audio_env'}
        onClose={() => setActiveRightDrawer(null)}
        side="right"
        sizeClass="max-w-md"
      >
        <div className="p-6 space-y-6 text-xs">
          <div className="flex items-center justify-between border-b border-zinc-100 pb-4">
            <div className="flex items-center gap-2 font-semibold text-sm text-zinc-900">
              <Volume2 className="w-4 h-4 text-zinc-700" />
              Ambiente de áudio
            </div>
            <button onClick={() => setActiveRightDrawer(null)} className="text-zinc-400 hover:text-zinc-700">
              <X className="w-4 h-4" />
            </button>
          </div>

          <div className="space-y-2">
            <label className="block font-semibold text-zinc-900">Filtro de voz</label>
            <p className="text-zinc-500 leading-relaxed">
              Aplique um filtro de áudio à voz do agente para mudar como a voz soa na linha.
            </p>
            <select
              value={audioFilter}
              onChange={(e) => setAudioFilter(e.target.value)}
              className="w-full px-3.5 py-2.5 bg-white border border-zinc-200 rounded-xl font-medium text-zinc-900"
            >
              <option value="telefone_claro">Telefone com som claro</option>
              <option value="estudio_limpo">Estúdio sem filtro (HD)</option>
            </select>
          </div>

          <div className="flex items-center justify-between pt-2">
            <span className="font-medium text-zinc-800">Ativar som de fundo (Call Center suave)</span>
            <Toggle
              checked={bgSoundEnabled}
              onChange={setBgSoundEnabled}
              label="Ativar som de fundo"
            />
          </div>
        </div>
      </Sheet>

      {/* 4. Gaveta: Seleção de Idioma */}
      <Sheet
        open={activeRightDrawer === 'language'}
        onClose={() => setActiveRightDrawer(null)}
        side="right"
        sizeClass="max-w-md"
      >
        <div className="p-6 space-y-5 text-xs">
          <div className="flex items-center justify-between border-b border-zinc-100 pb-4">
            <div className="flex items-center gap-2 font-semibold text-sm text-zinc-900">
              <Globe className="w-4 h-4 text-zinc-700" />
              Idioma do Agente
            </div>
            <button onClick={() => setActiveRightDrawer(null)} className="text-zinc-400 hover:text-zinc-700">
              <X className="w-4 h-4" />
            </button>
          </div>

          <div className="space-y-2">
            {Object.entries(LANGUAGE_FLAGS).map(([code, meta]) => {
              const isSelected = form.language === code
              return (
                <button
                  key={code}
                  type="button"
                  onClick={() => {
                    updateFormField({ language: code })
                    saveDraft({ language: code })
                    setActiveRightDrawer(null)
                  }}
                  className={`w-full p-3.5 rounded-2xl border flex items-center justify-between text-left transition-all ${
                    isSelected
                      ? 'border-zinc-900 bg-zinc-50 ring-1 ring-zinc-900'
                      : 'border-zinc-200 hover:border-zinc-300 bg-white'
                  }`}
                >
                  <span className="flex items-center gap-2.5 font-semibold text-zinc-900">
                    <span className="text-base">{meta.flag}</span>
                    {meta.label}
                  </span>
                  {isSelected && (
                    <span className="px-2 py-0.5 rounded-md bg-zinc-900 text-white text-[10px] font-semibold">
                      Padrão
                    </span>
                  )}
                </button>
              )
            })}
          </div>
        </div>
      </Sheet>

      {/* 5. Gaveta: Seleção de Provedor e Modelo LLM */}
      <Sheet
        open={activeRightDrawer === 'llm'}
        onClose={() => setActiveRightDrawer(null)}
        side="right"
        sizeClass="max-w-md"
      >
        <div className="h-full flex flex-col overflow-hidden text-xs">
          <div className="px-6 py-4 flex items-center justify-between border-b border-zinc-100 flex-shrink-0 bg-white">
            <div className="flex items-center gap-2 font-semibold text-sm text-zinc-900">
              <Sparkles className="w-4 h-4 text-zinc-700" />
              Provedor de Voz & Modelo LLM
            </div>
            <button
              onClick={() => setActiveRightDrawer(null)}
              className="text-zinc-400 hover:text-zinc-700 cursor-pointer"
            >
              <X className="w-4 h-4" />
            </button>
          </div>

          <div className="flex-1 overflow-y-auto overscroll-contain p-6 space-y-5">
            <div className="space-y-2">
              <label className="block font-semibold text-zinc-700">Voice Provider</label>
              <div className="grid grid-cols-2 gap-2.5">
                <button
                  type="button"
                  onClick={() => handleProviderChange('gemini')}
                  className={`p-3 rounded-xl border text-left transition-all cursor-pointer ${
                    form.provider.includes('gemini')
                      ? 'border-zinc-900 bg-zinc-50 ring-1 ring-zinc-900 font-semibold'
                      : 'border-zinc-200 hover:border-zinc-300'
                  }`}
                >
                  <div className="text-zinc-900">Google Gemini Live</div>
                  <div className="text-[10px] text-zinc-400 mt-0.5">Áudio nativo baixa latência</div>
                </button>

                <button
                  type="button"
                  onClick={() => handleProviderChange('openai')}
                  className={`p-3 rounded-xl border text-left transition-all cursor-pointer ${
                    form.provider.includes('openai')
                      ? 'border-zinc-900 bg-zinc-50 ring-1 ring-zinc-900 font-semibold'
                      : 'border-zinc-200 hover:border-zinc-300'
                  }`}
                >
                  <div className="text-zinc-900">OpenAI Realtime & Live</div>
                  <div className="text-[10px] text-zinc-400 mt-0.5">GPT-Live 1 & Realtime 2.1</div>
                </button>
              </div>
            </div>

            <div className="space-y-2 pb-4">
              <label className="block font-semibold text-zinc-700">Modelo LLM</label>
              <div className="space-y-2">
                {currentCatalog.models.map((m) => {
                  const isSelected = form.model === m.id
                  return (
                    <button
                      key={m.id}
                      type="button"
                      onClick={() => {
                        updateFormField({ model: m.id })
                        saveDraft({ model: m.id })
                      }}
                      className={`w-full p-3.5 rounded-2xl border text-left flex items-center justify-between gap-3 transition-all cursor-pointer ${
                        isSelected
                          ? 'border-zinc-900 bg-zinc-50 ring-1 ring-zinc-900'
                          : 'border-zinc-200 hover:border-zinc-300'
                      }`}
                    >
                      <div className="min-w-0">
                        <div className="font-semibold text-zinc-900">{m.label}</div>
                        <div className="text-[11px] font-mono text-zinc-400">{m.id}</div>
                        {m.description && (
                          <div className="text-[11px] text-zinc-500 mt-0.5">{m.description}</div>
                        )}
                      </div>
                      {isSelected && <Check className="w-4 h-4 text-zinc-900 flex-shrink-0" />}
                    </button>
                  )
                })}
              </div>
            </div>
          </div>
        </div>
      </Sheet>

      {/* 6. Gaveta: Comportamento do Agente */}
      <Sheet
        open={activeRightDrawer === 'behavior'}
        onClose={() => setActiveRightDrawer(null)}
        side="right"
        sizeClass="max-w-md"
      >
        <div className="p-6 space-y-5 text-xs">
          <div className="flex items-center justify-between border-b border-zinc-100 pb-4">
            <div className="flex items-center gap-2 font-semibold text-sm text-zinc-900">
              <Shield className="w-4 h-4 text-zinc-700" />
              Comportamento do agente
            </div>
            <button onClick={() => setActiveRightDrawer(null)} className="text-zinc-400 hover:text-zinc-700">
              <X className="w-4 h-4" />
            </button>
          </div>

          <div className="space-y-4">
            <div>
              <label className="block font-semibold text-zinc-800 mb-1">
                Initiation Webhook URL (Contexto inicial do Caller)
              </label>
              <input
                type="url"
                value={form.initiation_webhook_url}
                onChange={(e) => updateFormField({ initiation_webhook_url: e.target.value })}
                placeholder="https://api.empresa.com/webhook/contexto"
                className="w-full px-3 py-2 bg-zinc-50 border border-zinc-200 rounded-xl font-mono text-zinc-900"
              />
            </div>

            <div>
              <label className="block font-semibold text-zinc-800 mb-1">
                Duração máxima da chamada (segundos)
              </label>
              <input
                type="number"
                value={form.max_duration_seconds}
                onChange={(e) =>
                  updateFormField({ max_duration_seconds: parseInt(e.target.value) || 600 })
                }
                className="w-full px-3 py-2 bg-zinc-50 border border-zinc-200 rounded-xl text-zinc-900"
              />
            </div>

            <div className="flex items-center justify-between pt-2">
              <span className="font-medium text-zinc-800">Gravação de áudio ativa</span>
              <Toggle
                checked={form.recording_enabled}
                onChange={(val) => updateFormField({ recording_enabled: val })}
                label="Gravação ativa"
              />
            </div>

            <Button
              size="sm"
              onClick={() => {
                saveDraft()
                setActiveRightDrawer(null)
              }}
              className="w-full justify-center"
            >
              Salvar comportamento
            </Button>
          </div>
        </div>
      </Sheet>

      {/* 7. Gaveta: { } Variáveis (Dynamic Variables + System Variables) */}
      <Sheet
        open={activeRightDrawer === 'variables'}
        onClose={() => setActiveRightDrawer(null)}
        side="right"
        sizeClass="max-w-md"
      >
        <div className="p-6 space-y-5 text-xs">
          <div className="flex items-center justify-between border-b border-zinc-100 pb-4">
            <div className="flex items-center gap-2 font-semibold text-sm text-zinc-900">
              <Braces className="w-4 h-4 text-zinc-700" />
              Variáveis Dinâmicas e de Sistema
            </div>
            <button onClick={() => setActiveRightDrawer(null)} className="text-zinc-400 hover:text-zinc-700">
              <X className="w-4 h-4" />
            </button>
          </div>

          <div className="space-y-3">
            <span className="font-semibold text-zinc-800 block">
              Declarar Nova Dynamic Variable
            </span>
            <div className="flex gap-2">
              <input
                type="text"
                placeholder="ex: primNome, cpf_cliente"
                value={newVarName}
                onChange={(e) => setNewVarName(e.target.value.replace(/[^a-zA-Z0-9_]/g, ''))}
                className="flex-1 px-3 py-1.5 bg-zinc-50 border border-zinc-200 rounded-lg font-mono text-xs"
              />
              <Button
                size="sm"
                type="button"
                onClick={() => {
                  if (!newVarName.trim()) return
                  const updatedVars = [
                    ...(form.variables || []),
                    { name: newVarName.trim(), description: newVarDesc.trim() },
                  ]
                  updateFormField({ variables: updatedVars })
                  saveDraft({ variables: updatedVars })
                  setNewVarName('')
                  setNewVarDesc('')
                }}
              >
                Adicionar
              </Button>
            </div>

            {(form.variables || []).length > 0 && (
              <div className="space-y-1.5 pt-2">
                {form.variables.map((v, idx) => (
                  <div
                    key={idx}
                    className="px-3 py-2 bg-zinc-50 border border-zinc-200 rounded-xl flex items-center justify-between"
                  >
                    <code className="font-mono font-semibold text-zinc-900">{`{{${v.name}}}`}</code>
                    <button
                      type="button"
                      onClick={() => {
                        const filtered = form.variables.filter((_, i) => i !== idx)
                        updateFormField({ variables: filtered })
                        saveDraft({ variables: filtered })
                      }}
                      className="text-zinc-400 hover:text-red-600"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>

          <div className="space-y-2 pt-3 border-t border-zinc-100">
            <span className="font-semibold text-zinc-500 uppercase tracking-wider text-[10px] block">
              Variáveis Nativas do Sistema
            </span>
            <div className="space-y-1.5 max-h-64 overflow-y-auto">
              {SYSTEM_VARIABLES_CATALOG.map((sv) => (
                <div key={sv.name} className="p-2.5 bg-zinc-50/70 border border-zinc-200/70 rounded-xl">
                  <div className="font-mono font-semibold text-zinc-900">{`{{${sv.name}}}`}</div>
                  <p className="text-[11px] text-zinc-500 mt-0.5">{sv.description}</p>
                </div>
              ))}
            </div>
          </div>
        </div>
      </Sheet>

      {/* Modal Renomear Agente (•••) */}
      <Modal open={isRenameModalOpen} onClose={() => setIsRenameModalOpen(false)}>
        <div className="p-6 space-y-4 text-xs">
          <div className="flex items-center justify-between">
            <h3 className="font-semibold text-sm text-zinc-900">Editar Identificação do Agente</h3>
            <button onClick={() => setIsRenameModalOpen(false)} className="text-zinc-400">
              <X className="w-4 h-4" />
            </button>
          </div>
          <div>
            <label className="block font-semibold text-zinc-700 mb-1">Nome do Agente</label>
            <input
              type="text"
              value={form.name}
              onChange={(e) => updateFormField({ name: e.target.value })}
              className="w-full px-3 py-2 border border-zinc-200 rounded-xl text-sm text-zinc-900"
            />
          </div>
          <div>
            <label className="block font-semibold text-zinc-700 mb-1">Descrição</label>
            <input
              type="text"
              value={form.description}
              onChange={(e) => updateFormField({ description: e.target.value })}
              className="w-full px-3 py-2 border border-zinc-200 rounded-xl text-xs text-zinc-900"
            />
          </div>
          <div className="flex justify-end gap-2 pt-2">
            <Button
              size="sm"
              onClick={() => {
                saveDraft()
                setIsRenameModalOpen(false)
              }}
            >
              Salvar
            </Button>
          </div>
        </div>
      </Modal>

      {/* Gaveta ElevenLabs de Criar / Editar Ferramenta */}
      <ElevenToolDrawer
        open={isToolModalOpen}
        onClose={() => setIsToolModalOpen(false)}
        initialKind={toolDrawerKind}
        editingTool={editingAgentTool}
        onSaved={(saved) => {
          if (editingAgentTool) {
            setAvailableTools(availableTools.map((t) => (t.id === saved.id ? saved : t)))
          } else {
            setAvailableTools([saved, ...availableTools])
            if (form) {
              const updatedTools = [...(form.tools || []), saved.id]
              setForm({ ...form, tools: updatedTools })
              saveDraft({ tools: updatedTools })
            }
          }
          setIsToolModalOpen(false)
        }}
      />

      {/* Gaveta de Pré-visualização (Live Voice Agent Test Drawer) */}
      <Sheet
        open={isTestDrawerOpen}
        onClose={() => {
          endLiveTest()
          setIsTestDrawerOpen(false)
        }}
        side="right"
        sizeClass="max-w-lg"
        draggable={callStatus !== 'active'}
      >
        <>
          <div className="px-6 py-4 border-b border-zinc-200 flex items-center justify-between bg-zinc-50/50">
            <div className="flex items-center gap-2">
              <div className="w-5 h-5 rounded-full bg-gradient-to-tr from-indigo-500 via-purple-400 to-sky-300" />
              <div>
                <h3 className="font-semibold text-sm text-zinc-900">Pré-visualização: {form.name}</h3>
                <span className="text-[10px] text-zinc-500 font-mono">
                  {form.provider.toUpperCase()} • {form.voice}
                </span>
              </div>
            </div>
            <div className="flex items-center gap-2">
              {transcripts.length > 0 && (
                <button
                  type="button"
                  onClick={() => {
                    const txt = transcripts
                      .map((t) => `[${t.time}] ${t.role === 'caller' ? 'Cliente' : form.name}: ${t.text}`)
                      .join('\n')
                    navigator.clipboard.writeText(txt)
                    setMessage({ type: 'success', text: 'Transcrição copiada!' })
                  }}
                  className="inline-flex items-center gap-1 px-2.5 py-1 rounded-lg border border-zinc-200 bg-white hover:bg-zinc-50 text-xs font-medium text-zinc-700 cursor-pointer"
                  title="Copiar transcrição da pré-visualização"
                >
                  <Copy className="w-3.5 h-3.5" />
                  <span>Copiar</span>
                </button>
              )}
              <button
                onClick={() => {
                  endLiveTest()
                  setIsTestDrawerOpen(false)
                }}
                className="text-zinc-400 hover:text-zinc-700 p-1"
              >
                <X className="w-5 h-5" />
              </button>
            </div>
          </div>

          <div className="px-6 py-4 bg-zinc-900 text-white flex flex-col items-center justify-center space-y-3">
            <VoiceOrb
              speaking={isAgentSpeaking}
              active={callStatus === 'active'}
              muted={isMicMuted}
            />

            <div className="text-center space-y-0.5">
              <span className="font-mono text-sm font-semibold tracking-wider block">
                {callStatus === 'active' ? formatTimer(callDuration) : '00:00'}
              </span>
              <span className="text-[11px] text-zinc-400 block">
                {callStatus === 'active'
                  ? isAgentSpeaking
                    ? `${form.name} falando...`
                    : isMicMuted
                      ? 'Microfone silenciado'
                      : 'Ouvindo você... (fale agora)'
                  : callStatus === 'connecting'
                    ? 'Conectando...'
                    : 'Sessão encerrada'}
              </span>
            </div>

            {callStatus === 'active' && (
              <div className="flex items-center gap-2 pt-1">
                <button
                  type="button"
                  onClick={() => {
                    const next = !antiEcho
                    setAntiEcho(next)
                    if (!next && recorderRef.current) {
                      recorderRef.current.setMuted(isMicMuted)
                    }
                  }}
                  className={`px-3 py-1 rounded-full text-[11px] font-medium transition-colors flex items-center gap-1.5 ${
                    antiEcho
                      ? 'bg-emerald-500/20 text-emerald-300 border border-emerald-500/30'
                      : 'bg-zinc-800 text-zinc-400 border border-zinc-700'
                  }`}
                >
                  <Headphones className="w-3 h-3" />
                  Anti-Eco: {antiEcho ? 'ON' : 'OFF'}
                </button>

                <button
                  type="button"
                  onClick={() => {
                    const next = !isMicMuted
                    setIsMicMuted(next)
                    if (recorderRef.current) {
                      recorderRef.current.setMuted(next)
                    }
                  }}
                  className={`px-3 py-1 rounded-full text-[11px] font-medium transition-colors flex items-center gap-1.5 ${
                    isMicMuted
                      ? 'bg-red-500/20 text-red-300 border border-red-500/30'
                      : 'bg-zinc-800 text-zinc-300 border border-zinc-700'
                  }`}
                >
                  {isMicMuted ? <MicOff className="w-3 h-3" /> : <Mic className="w-3 h-3" />}
                  {isMicMuted ? 'Mudo' : 'Mic Ativo'}
                </button>
              </div>
            )}

            {(activeSkillName || executedTools.length > 0) && (
              <div className="w-full pt-2 border-t border-zinc-800 flex flex-wrap gap-1.5 justify-center">
                {activeSkillName && (
                  <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-medium bg-emerald-950 text-emerald-300 border border-emerald-800">
                    <Sparkles className="w-2.5 h-2.5" />
                    Procedimento: {activeSkillName}
                  </span>
                )}
              </div>
            )}
          </div>

          <div className="flex-1 p-5 overflow-y-auto overscroll-contain space-y-3 bg-zinc-50">
            {transcripts.map((t) => (
              <div
                key={t.id}
                className={`flex flex-col ${
                  t.role === 'caller' ? 'items-end' : 'items-start'
                }`}
              >
                <div
                  className={`max-w-[85%] rounded-2xl px-3.5 py-2 text-xs ${
                    t.role === 'caller'
                      ? 'bg-zinc-900 text-white'
                      : 'bg-white border border-zinc-200 text-zinc-800'
                  }`}
                >
                  {t.text}
                </div>
              </div>
            ))}
            <div ref={transcriptsEndRef} />
          </div>

          <div className="p-4 border-t border-zinc-200 bg-white flex justify-between items-center">
            {callStatus === 'active' ? (
              <Button variant="danger" size="sm" onClick={endLiveTest} className="w-full justify-center">
                Encerrar Pré-visualização
              </Button>
            ) : (
              <Button size="sm" onClick={startLiveTest} className="w-full justify-center">
                Iniciar Nova Conversa
              </Button>
            )}
          </div>
        </>
      </Sheet>
    </div>
  )
}
