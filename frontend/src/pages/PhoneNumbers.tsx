import { useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { AnimatePresence, motion } from 'motion/react'
import {
  PhoneCall,
  Plus,
  Search,
  Bot,
  Trash2,
  Radio,
  X,
  Check,
  Copy,
  RefreshCw,
  KeyRound,
  MoreHorizontal,
  AlertTriangle,
  CheckCircle2,
  ExternalLink,
} from 'lucide-react'
import {
  api,
  type PhoneNumber,
  type PhoneNumberUpdatePayload,
  type Agent,
  type TwilioAvailableNumber,
} from '../api'
import Modal from '../components/Modal'
import Sheet from '../components/Sheet'
import Button from '../components/Button'
import { springSnappy, transitionPopover, haptic } from '../components/motion'

export default function PhoneNumbersPage() {
  const [numbers, setNumbers] = useState<PhoneNumber[]>([])
  const [agents, setAgents] = useState<Agent[]>([])
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(true)
  const [errorMsg, setErrorMsg] = useState('')
  const [copiedWebhook, setCopiedWebhook] = useState<string>('')

  // Painel lateral (Sheet) de Detalhes do Número e menu de ações (...)
  const [selectedPhoneId, setSelectedPhoneId] = useState<string | null>(null)
  const [menuOpenId, setMenuOpenId] = useState<string | null>(null)
  const [sheetMenuOpen, setSheetMenuOpen] = useState(false)
  const [updatingAgent, setUpdatingAgent] = useState(false)
  const [sheetFeedback, setSheetFeedback] = useState<{
    type: 'saved' | 'error'
    message: string
  } | null>(null)

  // Modal de importação / vínculo de novo número
  const [isModalOpen, setIsModalOpen] = useState(false)
  const [provider, setProvider] = useState<'twilio' | 'jambonz'>('twilio')
  const [e164, setE164] = useState('')
  const [label, setLabel] = useState('')
  const [agentId, setAgentId] = useState('')
  const [direction, setDirection] = useState('inbound')

  // Campos específicos da Twilio
  const [twilioAccountSid, setTwilioAccountSid] = useState('')
  const [twilioAuthToken, setTwilioAuthToken] = useState('')
  const [twilioPhoneSid, setTwilioPhoneSid] = useState('')
  const [twilioAvailable, setTwilioAvailable] = useState<TwilioAvailableNumber[]>([])
  const [fetchingTwilio, setFetchingTwilio] = useState(false)
  const [twilioFetchErr, setTwilioFetchErr] = useState('')
  const [autoConfigureWebhook, setAutoConfigureWebhook] = useState(false)
  const [publicBaseUrl, setPublicBaseUrl] = useState(window.location.origin)

  const [saving, setSaving] = useState(false)

  const twilioWebhookUrl = `${publicBaseUrl.replace(/\/$/, '')}/api/telephony/twilio/webhook`
  const jambonzWebhookUrl = `${publicBaseUrl.replace(/\/$/, '')}/api/telephony/jambonz/webhook`

  const selectedPhone = numbers.find((n) => n.id === selectedPhoneId) ?? null
  const selectedPhoneAgent = selectedPhone?.agent_id
    ? agents.find((a) => a.id === selectedPhone.agent_id) ?? null
    : null
  const modalSelectedAgent = agentId ? agents.find((a) => a.id === agentId) ?? null : null

  const loadData = async () => {
    try {
      setLoading(true)
      const [nums, ags] = await Promise.all([
        api<PhoneNumber[]>('/api/phone-numbers'),
        api<Agent[]>('/api/agents'),
      ])
      setNumbers(nums || [])
      setAgents(ags || [])
    } catch (err) {
      setErrorMsg((err as Error).message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadData()
  }, [])

  // Fecha menus (...) ao clicar fora
  useEffect(() => {
    if (!menuOpenId && !sheetMenuOpen) return
    const closeMenus = () => {
      setMenuOpenId(null)
      setSheetMenuOpen(false)
    }
    window.addEventListener('click', closeMenus)
    return () => window.removeEventListener('click', closeMenus)
  }, [menuOpenId, sheetMenuOpen])

  const copyToClipboard = (text: string, tag: string) => {
    navigator.clipboard.writeText(text)
    haptic(6)
    setCopiedWebhook(tag)
    setTimeout(() => setCopiedWebhook(''), 2000)
  }

  const handleFetchTwilioNumbers = async () => {
    if (!twilioAccountSid.trim() || !twilioAuthToken.trim()) {
      setTwilioFetchErr('Informe o Account SID e o Auth Token da sua conta Twilio.')
      return
    }
    setTwilioFetchErr('')
    setFetchingTwilio(true)
    try {
      const list = await api<TwilioAvailableNumber[]>('/api/phone-numbers/twilio/available', {
        method: 'POST',
        body: JSON.stringify({
          account_sid: twilioAccountSid.trim(),
          auth_token: twilioAuthToken.trim(),
        }),
      })
      setTwilioAvailable(list || [])
      if (list && list.length > 0) {
        setE164(list[0].phone_number)
        setTwilioPhoneSid(list[0].sid)
        if (!label.trim() && list[0].friendly_name) {
          setLabel(list[0].friendly_name)
        }
      } else {
        setTwilioFetchErr('Nenhum número encontrado nesta conta da Twilio.')
      }
      haptic(8)
    } catch (err) {
      setTwilioFetchErr((err as Error).message)
    } finally {
      setFetchingTwilio(false)
    }
  }

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault()
    if (!e164.trim()) return
    setErrorMsg('')
    setSaving(true)
    try {
      await api<PhoneNumber>('/api/phone-numbers', {
        method: 'POST',
        body: JSON.stringify({
          e164,
          label,
          agent_id: agentId,
          direction,
          provider,
          twilio_account_sid: provider === 'twilio' ? twilioAccountSid.trim() : '',
          twilio_auth_token: provider === 'twilio' ? twilioAuthToken.trim() : '',
          twilio_phone_sid: provider === 'twilio' ? twilioPhoneSid.trim() : '',
          configure_webhook_url:
            provider === 'twilio' && autoConfigureWebhook && twilioPhoneSid
              ? twilioWebhookUrl
              : '',
        }),
      })
      haptic(10)
      setE164('')
      setLabel('')
      setAgentId('')
      setTwilioPhoneSid('')
      setIsModalOpen(false)
      loadData()
    } catch (err) {
      setErrorMsg((err as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const handleChangePhoneAgent = async (phoneId: string, newAgentId: string) => {
    setUpdatingAgent(true)
    setSheetFeedback(null)
    try {
      const payload: PhoneNumberUpdatePayload = { agent_id: newAgentId }
      const updated = await api<PhoneNumber>(`/api/phone-numbers/${phoneId}`, {
        method: 'PUT',
        body: JSON.stringify(payload),
      })
      haptic(8)
      setNumbers((prev) => prev.map((item) => (item.id === phoneId ? updated : item)))
      setSheetFeedback({ type: 'saved', message: 'Agente atualizado automaticamente.' })
      setTimeout(() => {
        setSheetFeedback((curr) => (curr?.type === 'saved' ? null : curr))
      }, 2500)
    } catch (err) {
      setSheetFeedback({
        type: 'error',
        message: (err as Error).message || 'Falha ao atualizar o agente deste número.',
      })
    } finally {
      setUpdatingAgent(false)
    }
  }

  const handleDelete = async (id: string, num: string) => {
    if (!window.confirm(`Remover permanentemente o número ${num}?`)) return
    try {
      await api(`/api/phone-numbers/${id}`, { method: 'DELETE' })
      haptic(8)
      if (selectedPhoneId === id) {
        setSelectedPhoneId(null)
      }
      loadData()
    } catch (err) {
      setErrorMsg((err as Error).message)
    }
  }

  const filtered = numbers.filter(
    (n) =>
      n.e164.toLowerCase().includes(search.toLowerCase()) ||
      n.label.toLowerCase().includes(search.toLowerCase()) ||
      (n.agent_name && n.agent_name.toLowerCase().includes(search.toLowerCase())),
  )

  return (
    <div className="max-w-6xl mx-auto space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 border-b border-zinc-200/80 pb-5">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight text-zinc-900 flex items-center gap-2">
            <PhoneCall className="w-6 h-6 text-zinc-700" />
            Telefonia
          </h1>
          <p className="text-sm text-zinc-500 mt-1">
            Conecte números da Twilio (Media Streams) ou Trunks SIP via Jambonz diretamente aos seus agentes de voz.
          </p>
        </div>

        <Button
          onClick={() => {
            setProvider('twilio')
            setIsModalOpen(true)
          }}
          className="shadow-sm"
        >
          <Plus className="w-4 h-4" />
          Adicionar número
        </Button>
      </div>

      {/* Cards de Webhooks Rápidos */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div className="p-4 bg-rose-50/60 border border-rose-200/80 rounded-xl flex items-start gap-3 text-xs text-rose-950">
          <div className="w-8 h-8 rounded-lg bg-rose-600 text-white font-bold flex items-center justify-center flex-shrink-0 text-[11px]">
            TW
          </div>
          <div className="flex-1 min-w-0 space-y-1.5">
            <span className="font-semibold text-rose-950 block">Twilio Voice & Media Streams</span>
            <p className="text-rose-800/90 leading-relaxed">
              Streaming bidirecional nativo em 8kHz G.711 μ-law com suporte a interrupção instantânea (clear) e transferência via TwiML.
            </p>
            <div className="flex items-center gap-1.5 pt-1">
              <code className="px-2 py-1 bg-white/90 border border-rose-200 rounded font-mono text-[11px] text-rose-900 truncate flex-1">
                POST /api/telephony/twilio/webhook
              </code>
              <button
                type="button"
                onClick={() => copyToClipboard(twilioWebhookUrl, 'twilio-card')}
                className="inline-flex items-center gap-1 px-2.5 py-1 rounded bg-rose-600 hover:bg-rose-700 text-white font-medium text-[11px] transition-colors flex-shrink-0"
              >
                <Copy className="w-3 h-3" />
                {copiedWebhook === 'twilio-card' ? 'Copiado!' : 'Copiar URL'}
              </button>
            </div>
          </div>
        </div>

        <div className="p-4 bg-blue-50/70 border border-blue-200/80 rounded-xl flex items-start gap-3 text-xs text-blue-900">
          <div className="w-8 h-8 rounded-lg bg-blue-600 text-white flex items-center justify-center flex-shrink-0">
            <Radio className="w-4 h-4" />
          </div>
          <div className="flex-1 min-w-0 space-y-1.5">
            <span className="font-semibold text-blue-950 block">Trunk SIP & Jambonz SBC</span>
            <p className="text-blue-800 leading-relaxed">
              Receba chamadas de qualquer operadora SIP (Vivo, Claro, Twilio Elastic SIP, Zenvia) através do media server Jambonz.
            </p>
            <div className="flex items-center gap-1.5 pt-1">
              <code className="px-2 py-1 bg-white/90 border border-blue-200 rounded font-mono text-[11px] text-blue-900 truncate flex-1">
                POST /api/telephony/jambonz/webhook
              </code>
              <button
                type="button"
                onClick={() => copyToClipboard(jambonzWebhookUrl, 'jambonz-card')}
                className="inline-flex items-center gap-1 px-2.5 py-1 rounded bg-blue-600 hover:bg-blue-700 text-white font-medium text-[11px] transition-colors flex-shrink-0"
              >
                <Copy className="w-3 h-3" />
                {copiedWebhook === 'jambonz-card' ? 'Copiado!' : 'Copiar URL'}
              </button>
            </div>
          </div>
        </div>
      </div>

      {errorMsg && (
        <div className="p-3 bg-red-50 border border-red-200 rounded-lg text-red-700 text-xs">{errorMsg}</div>
      )}

      {/* Search Bar */}
      <div className="flex items-center justify-between gap-3">
        <div className="relative flex-1 max-w-md">
          <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-zinc-400" />
          <input
            type="text"
            placeholder="Pesquisar por nome, número ou agente..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full pl-9 pr-4 py-2 bg-white border border-zinc-200 rounded-lg text-sm text-zinc-900 placeholder-zinc-400 focus:outline-none focus:ring-1 focus:ring-zinc-900"
          />
        </div>
        <span className="text-xs text-zinc-500">{filtered.length} número(s) configurado(s)</span>
      </div>

      {/* Tabela de Números */}
      <div className="bg-white border border-zinc-200/80 rounded-xl overflow-visible shadow-xs">
        <table className="w-full text-left text-sm">
          <thead className="border-b border-zinc-200/80 text-xs font-medium text-zinc-500">
            <tr>
              <th className="px-6 py-3.5 font-medium">Nome</th>
              <th className="px-6 py-3.5 font-medium">Número de telefone</th>
              <th className="px-6 py-3.5 font-medium">Agente</th>
              <th className="px-6 py-3.5 font-medium">Provedor</th>
              <th className="px-6 py-3.5 text-right w-14" />
            </tr>
          </thead>
          <tbody className="divide-y divide-zinc-100">
            {filtered.map((n, i) => {
              const isTwilio = n.provider === 'twilio'
              const isSelected = selectedPhoneId === n.id
              return (
                <motion.tr
                  key={n.id}
                  initial={{ opacity: 0, y: 6 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ ...springSnappy, delay: Math.min(i * 0.03, 0.2) }}
                  onClick={() => {
                    setSheetFeedback(null)
                    setSelectedPhoneId(n.id)
                  }}
                  tabIndex={0}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault()
                      setSheetFeedback(null)
                      setSelectedPhoneId(n.id)
                    }
                  }}
                  className={`cursor-pointer transition-colors ${
                    isSelected ? 'bg-zinc-50' : 'hover:bg-zinc-50/70'
                  }`}
                >
                  <td className="px-6 py-4 text-sm font-medium text-zinc-900">
                    {n.label || <span className="text-zinc-400 font-normal">Sem nome</span>}
                  </td>
                  <td className="px-6 py-4 font-mono text-sm text-zinc-800">{n.e164}</td>
                  <td className="px-6 py-4">
                    {n.agent_id ? (
                      <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-zinc-100 text-zinc-900 text-xs font-medium">
                        <Bot className="w-3.5 h-3.5 text-zinc-600" />
                        {n.agent_name || 'Agente'}
                      </span>
                    ) : (
                      <span className="inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full bg-zinc-100 text-zinc-500 text-xs font-medium">
                        Sem agente
                      </span>
                    )}
                  </td>
                  <td className="px-6 py-4">
                    <span className="inline-flex items-center px-2.5 py-0.5 rounded-full bg-zinc-100 text-zinc-700 text-xs font-medium">
                      {isTwilio ? 'Twilio' : 'SIP Trunk'}
                    </span>
                  </td>
                  <td
                    className="px-6 py-4 text-right relative"
                    onClick={(e) => e.stopPropagation()}
                  >
                    <button
                      type="button"
                      onClick={(e) => {
                        e.stopPropagation()
                        setMenuOpenId((prev) => (prev === n.id ? null : n.id))
                      }}
                      title="Opções do número"
                      className="p-1.5 text-zinc-400 hover:text-zinc-800 hover:bg-zinc-100 rounded-lg transition-colors"
                    >
                      <MoreHorizontal className="w-4 h-4" />
                    </button>

                    <AnimatePresence>
                      {menuOpenId === n.id && (
                        <motion.div
                          initial={{ opacity: 0, scale: 0.96, y: -4 }}
                          animate={{ opacity: 1, scale: 1, y: 0 }}
                          exit={{ opacity: 0, scale: 0.96, y: -4 }}
                          transition={transitionPopover}
                          className="absolute right-6 top-11 z-20 w-44 bg-white border border-zinc-200 rounded-xl shadow-lg py-1 text-left"
                        >
                          <button
                            type="button"
                            onClick={() => {
                              setMenuOpenId(null)
                              setSheetFeedback(null)
                              setSelectedPhoneId(n.id)
                            }}
                            className="w-full px-3 py-2 text-xs text-zinc-700 hover:bg-zinc-50 flex items-center gap-2"
                          >
                            <PhoneCall className="w-3.5 h-3.5 text-zinc-500" />
                            Detalhes do número
                          </button>
                          <button
                            type="button"
                            onClick={() => {
                              setMenuOpenId(null)
                              handleDelete(n.id, n.e164)
                            }}
                            className="w-full px-3 py-2 text-xs text-red-600 hover:bg-red-50 flex items-center gap-2"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                            Remover número
                          </button>
                        </motion.div>
                      )}
                    </AnimatePresence>
                  </td>
                </motion.tr>
              )
            })}
            {filtered.length === 0 && !loading && (
              <tr>
                <td colSpan={5} className="px-6 py-12 text-center text-xs text-zinc-400">
                  Nenhum número telefônico configurado. Clique em "+ Adicionar número" para vincular seu número da Twilio ou SIP.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* Sheet Lateral: Detalhes do número de telefone */}
      <Sheet
        open={Boolean(selectedPhone)}
        onClose={() => {
          setSelectedPhoneId(null)
          setSheetMenuOpen(false)
          setSheetFeedback(null)
        }}
        side="right"
        sizeClass="max-w-lg"
      >
        {selectedPhone && (
          <div className="flex flex-col h-full">
            {/* Cabeçalho do Sheet */}
            <div className="flex items-center justify-between px-6 py-4 border-b border-zinc-200/80">
              <h2 className="text-base font-semibold text-zinc-900">
                Detalhes do número de telefone
              </h2>
              <button
                type="button"
                onClick={() => setSelectedPhoneId(null)}
                className="p-1.5 text-zinc-400 hover:text-zinc-700 rounded-lg transition-colors"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            <div className="p-6 space-y-6 flex-1 overflow-y-auto">
              {/* Resumo do número */}
              <div className="flex items-start justify-between gap-4 pb-5 border-b border-zinc-200/80">
                <div className="space-y-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-lg font-semibold text-zinc-900 font-mono">
                      {selectedPhone.e164}
                    </span>
                    <button
                      type="button"
                      onClick={() => copyToClipboard(selectedPhone.e164, 'sheet-e164')}
                      title="Copiar número"
                      className="text-zinc-400 hover:text-zinc-700 transition-colors"
                    >
                      {copiedWebhook === 'sheet-e164' ? (
                        <CheckCircle2 className="w-4 h-4 text-emerald-600" />
                      ) : (
                        <Copy className="w-3.5 h-3.5" />
                      )}
                    </button>
                  </div>
                  <p className="text-sm text-zinc-500">
                    {selectedPhone.label || 'Sem nome'}
                  </p>
                  <p className="text-xs font-mono text-zinc-400 truncate">
                    {selectedPhone.id}
                  </p>
                </div>

                <div className="relative flex-shrink-0" onClick={(e) => e.stopPropagation()}>
                  <button
                    type="button"
                    onClick={() => setSheetMenuOpen((prev) => !prev)}
                    className="p-2 border border-zinc-200 hover:bg-zinc-50 rounded-lg text-zinc-600 transition-colors"
                    title="Mais opções"
                  >
                    <MoreHorizontal className="w-4 h-4" />
                  </button>

                  <AnimatePresence>
                    {sheetMenuOpen && (
                      <motion.div
                        initial={{ opacity: 0, scale: 0.96, y: -4 }}
                        animate={{ opacity: 1, scale: 1, y: 0 }}
                        exit={{ opacity: 0, scale: 0.96, y: -4 }}
                        transition={transitionPopover}
                        className="absolute right-0 top-11 z-20 w-44 bg-white border border-zinc-200 rounded-xl shadow-lg py-1"
                      >
                        <button
                          type="button"
                          onClick={() => {
                            setSheetMenuOpen(false)
                            handleDelete(selectedPhone.id, selectedPhone.e164)
                          }}
                          className="w-full px-3 py-2 text-xs text-red-600 hover:bg-red-50 flex items-center gap-2"
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                          Remover número
                        </button>
                      </motion.div>
                    )}
                  </AnimatePresence>
                </div>
              </div>

              {/* Card de Atribuição de Agente */}
              <div className="p-5 bg-zinc-50/70 border border-zinc-200/80 rounded-2xl space-y-4">
                <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
                  <div className="space-y-1">
                    <label
                      htmlFor="sheet-agent-select"
                      className="block text-sm font-semibold text-zinc-900"
                    >
                      Agente
                    </label>
                    <p className="text-xs text-zinc-500 leading-relaxed">
                      Atribua um agente para atender chamadas para este número de telefone.
                    </p>
                  </div>

                  <div className="sm:w-56 flex-shrink-0">
                    <select
                      id="sheet-agent-select"
                      value={selectedPhone.agent_id || ''}
                      disabled={updatingAgent}
                      onChange={(e) => handleChangePhoneAgent(selectedPhone.id, e.target.value)}
                      className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-xl text-sm text-zinc-900 shadow-2xs focus:outline-none focus:ring-1 focus:ring-zinc-900 disabled:opacity-50"
                    >
                      <option value="">Nenhum agente (desvinculado)</option>
                      {agents.map((a) => (
                        <option key={a.id} value={a.id}>
                          {a.name}
                        </option>
                      ))}
                    </select>
                  </div>
                </div>

                {/* Feedback de Auto-Save */}
                {updatingAgent && (
                  <div className="flex items-center gap-2 text-xs text-zinc-500">
                    <RefreshCw className="w-3.5 h-3.5 animate-spin" />
                    Salvando alteração de agente…
                  </div>
                )}

                {sheetFeedback && (
                  <div
                    className={`flex items-center gap-2 text-xs px-3 py-2 rounded-lg border ${
                      sheetFeedback.type === 'saved'
                        ? 'bg-emerald-50 border-emerald-200 text-emerald-800'
                        : 'bg-red-50 border-red-200 text-red-700'
                    }`}
                  >
                    {sheetFeedback.type === 'saved' ? (
                      <Check className="w-3.5 h-3.5 text-emerald-600 flex-shrink-0" />
                    ) : (
                      <AlertTriangle className="w-3.5 h-3.5 text-red-600 flex-shrink-0" />
                    )}
                    <span>{sheetFeedback.message}</span>
                  </div>
                )}

                {/* Avisos de Elegibilidade do Agente (Regra 3) */}
                {!selectedPhone.agent_id && (
                  <div className="p-3 bg-white border border-zinc-200 rounded-xl text-xs text-zinc-600 leading-relaxed">
                    Este número está sem agente vinculado. Chamadas recebidas neste número serão recusadas até que um agente ativo e publicado seja atribuído.
                  </div>
                )}

                {selectedPhoneAgent && !selectedPhoneAgent.active && (
                  <div className="p-3 bg-amber-50 border border-amber-200 rounded-xl flex items-start gap-2.5 text-xs text-amber-900">
                    <AlertTriangle className="w-4 h-4 text-amber-600 flex-shrink-0 mt-0.5" />
                    <div className="space-y-1">
                      <p className="font-medium">O agente selecionado está inativo.</p>
                      <p className="text-amber-800/90 leading-relaxed">
                        Chamadas telefônicas recebidas neste número serão recusadas até que o agente seja ativado.
                      </p>
                      <Link
                        to={`/agents/${selectedPhoneAgent.id}`}
                        className="inline-flex items-center gap-1 font-semibold text-amber-900 underline pt-0.5"
                      >
                        Abrir agente
                        <ExternalLink className="w-3 h-3" />
                      </Link>
                    </div>
                  </div>
                )}

                {selectedPhoneAgent &&
                  selectedPhoneAgent.active &&
                  selectedPhoneAgent.latest_version == null && (
                    <div className="p-3 bg-amber-50 border border-amber-200 rounded-xl flex items-start gap-2.5 text-xs text-amber-900">
                      <AlertTriangle className="w-4 h-4 text-amber-600 flex-shrink-0 mt-0.5" />
                      <div className="space-y-1">
                        <p className="font-medium">
                          O agente selecionado ainda não possui versão publicada.
                        </p>
                        <p className="text-amber-800/90 leading-relaxed">
                          Chamadas telefônicas atendem apenas com a última versão publicada e serão recusadas até que você publique o agente.
                        </p>
                        <Link
                          to={`/agents/${selectedPhoneAgent.id}`}
                          className="inline-flex items-center gap-1 font-semibold text-amber-900 underline pt-0.5"
                        >
                          Publicar agente
                          <ExternalLink className="w-3 h-3" />
                        </Link>
                      </div>
                    </div>
                  )}
              </div>
            </div>
          </div>
        )}
      </Sheet>

      {/* Modal Adicionar Número (Twilio ou SIP/Jambonz) */}
      <Modal open={isModalOpen} onClose={() => setIsModalOpen(false)}>
        <div className="p-6 max-h-[85vh] overflow-y-auto overscroll-contain">
          <div className="flex items-center justify-between mb-4">
            <h3 className="font-semibold text-zinc-900 text-sm">Adicionar Número Telefônico</h3>
            <button onClick={() => setIsModalOpen(false)} className="text-zinc-400 hover:text-zinc-600">
              <X className="w-4 h-4" />
            </button>
          </div>

          {/* Seletor de Provedor (Twilio vs SIP/Jambonz) */}
          <div className="grid grid-cols-2 gap-2 p-1 bg-zinc-100 rounded-xl mb-5">
            <button
              type="button"
              onClick={() => setProvider('twilio')}
              className={`py-2 px-3 rounded-lg text-xs font-semibold transition-all flex items-center justify-center gap-2 ${
                provider === 'twilio'
                  ? 'bg-white text-rose-700 shadow-xs'
                  : 'text-zinc-600 hover:text-zinc-900'
              }`}
            >
              <span className="w-2 h-2 rounded-full bg-rose-600" />
              Twilio (Nativo)
            </button>
            <button
              type="button"
              onClick={() => setProvider('jambonz')}
              className={`py-2 px-3 rounded-lg text-xs font-semibold transition-all flex items-center justify-center gap-2 ${
                provider === 'jambonz'
                  ? 'bg-white text-blue-700 shadow-xs'
                  : 'text-zinc-600 hover:text-zinc-900'
              }`}
            >
              <span className="w-2 h-2 rounded-full bg-blue-600" />
              SIP Trunk (Jambonz)
            </button>
          </div>

          <form onSubmit={handleCreate} className="space-y-4">
            {provider === 'twilio' && (
              <div className="p-4 bg-zinc-50 border border-zinc-200/80 rounded-xl space-y-3">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-semibold text-zinc-800 flex items-center gap-1.5">
                    <KeyRound className="w-3.5 h-3.5 text-rose-600" />
                    Credenciais da Conta Twilio (opcional para importar/transferir)
                  </span>
                </div>

                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  <div>
                    <label className="block text-[11px] font-medium text-zinc-600 mb-1">
                      Account SID
                    </label>
                    <input
                      type="text"
                      placeholder="ACxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
                      value={twilioAccountSid}
                      onChange={(e) => setTwilioAccountSid(e.target.value)}
                      className="w-full px-2.5 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs font-mono text-zinc-900 focus:outline-none focus:ring-1 focus:ring-zinc-900"
                    />
                  </div>
                  <div>
                    <label className="block text-[11px] font-medium text-zinc-600 mb-1">
                      Auth Token
                    </label>
                    <input
                      type="password"
                      placeholder="••••••••••••••••••••••••••••••••"
                      value={twilioAuthToken}
                      onChange={(e) => setTwilioAuthToken(e.target.value)}
                      className="w-full px-2.5 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs font-mono text-zinc-900 focus:outline-none focus:ring-1 focus:ring-zinc-900"
                    />
                  </div>
                </div>

                <div className="flex items-center justify-between gap-2 pt-1">
                  <span className="text-[11px] text-zinc-500">
                    Busque os números direto da sua conta Twilio ou digite o número manualmente abaixo.
                  </span>
                  <button
                    type="button"
                    onClick={handleFetchTwilioNumbers}
                    disabled={fetchingTwilio || !twilioAccountSid.trim() || !twilioAuthToken.trim()}
                    className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-zinc-900 hover:bg-zinc-800 disabled:opacity-40 text-white text-xs font-medium transition-colors flex-shrink-0"
                  >
                    <RefreshCw className={`w-3.5 h-3.5 ${fetchingTwilio ? 'animate-spin' : ''}`} />
                    {fetchingTwilio ? 'Buscando…' : 'Buscar Números'}
                  </button>
                </div>

                {twilioFetchErr && (
                  <p className="text-xs text-red-600 bg-red-50 border border-red-200 rounded-lg p-2">
                    {twilioFetchErr}
                  </p>
                )}

                {twilioAvailable.length > 0 && (
                  <div className="pt-2 border-t border-zinc-200/70 space-y-2">
                    <label className="block text-[11px] font-semibold text-zinc-700 uppercase tracking-wider">
                      Números encontrados na sua conta Twilio ({twilioAvailable.length})
                    </label>
                    <select
                      value={twilioPhoneSid}
                      onChange={(e) => {
                        const sid = e.target.value
                        setTwilioPhoneSid(sid)
                        const found = twilioAvailable.find((item) => item.sid === sid)
                        if (found) {
                          setE164(found.phone_number)
                          if (!label.trim() && found.friendly_name) {
                            setLabel(found.friendly_name)
                          }
                        }
                      }}
                      className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-xs font-mono text-zinc-900"
                    >
                      {twilioAvailable.map((item) => (
                        <option key={item.sid} value={item.sid}>
                          {item.phone_number} — {item.friendly_name}
                        </option>
                      ))}
                    </select>

                    <label className="flex items-center gap-2 text-xs text-zinc-700 pt-1 cursor-pointer">
                      <input
                        type="checkbox"
                        checked={autoConfigureWebhook}
                        onChange={(e) => setAutoConfigureWebhook(e.target.checked)}
                        className="rounded border-zinc-300"
                      />
                      Atualizar automaticamente o Voice Webhook URL deste número na Twilio
                    </label>

                    {autoConfigureWebhook && (
                      <div>
                        <label className="block text-[11px] text-zinc-500 mb-1">
                          Domínio público da sua plataforma (deve ser acessível pela internet):
                        </label>
                        <input
                          type="url"
                          value={publicBaseUrl}
                          onChange={(e) => setPublicBaseUrl(e.target.value)}
                          placeholder="https://voz.suaempresa.com.br"
                          className="w-full px-2.5 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs font-mono text-zinc-900"
                        />
                      </div>
                    )}
                  </div>
                )}
              </div>
            )}

            <div>
              <label className="block text-xs font-semibold text-zinc-700 uppercase tracking-wider mb-1">
                Número em formato E.164
              </label>
              <input
                type="text"
                required
                placeholder="+551140041234 ou +18005550199"
                value={e164}
                onChange={(e) => setE164(e.target.value)}
                className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-sm font-mono text-zinc-900 focus:outline-none focus:ring-1 focus:ring-zinc-900"
              />
              <span className="block text-[11px] text-zinc-400 mt-1">
                Formato internacional com código de país e DDD (ex: +5511999998888).
              </span>
            </div>

            <div>
              <label className="block text-xs font-semibold text-zinc-700 uppercase tracking-wider mb-1">
                Nome / Identificação (opcional)
              </label>
              <input
                type="text"
                placeholder="Ex: Twilio Ricardo, Central SAC"
                value={label}
                onChange={(e) => setLabel(e.target.value)}
                className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-sm text-zinc-900 focus:outline-none focus:ring-1 focus:ring-zinc-900"
              />
            </div>

            <div>
              <label className="block text-xs font-semibold text-zinc-700 uppercase tracking-wider mb-1">
                Agente de Voz Atendente (opcional)
              </label>
              <select
                value={agentId}
                onChange={(e) => setAgentId(e.target.value)}
                className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-sm text-zinc-900 focus:outline-none focus:ring-1 focus:ring-zinc-900"
              >
                <option value="">Nenhum agente (desvinculado)</option>
                {agents.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name} ({a.provider || 'gemini'} • {a.voice || 'padrão'})
                  </option>
                ))}
              </select>

              {modalSelectedAgent && !modalSelectedAgent.active && (
                <p className="mt-1.5 text-xs text-amber-700 bg-amber-50 border border-amber-200 rounded-lg p-2">
                  Aviso: este agente está inativo. Chamadas recebidas serão recusadas até que ele seja ativado.
                </p>
              )}

              {modalSelectedAgent &&
                modalSelectedAgent.active &&
                modalSelectedAgent.latest_version == null && (
                  <p className="mt-1.5 text-xs text-amber-700 bg-amber-50 border border-amber-200 rounded-lg p-2">
                    Aviso: este agente ainda não possui versão publicada. Chamadas recebidas serão recusadas até que uma versão seja publicada.
                  </p>
                )}
            </div>

            <div>
              <label className="block text-xs font-semibold text-zinc-700 uppercase tracking-wider mb-1">
                Sentido da Chamada
              </label>
              <select
                value={direction}
                onChange={(e) => setDirection(e.target.value)}
                className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-sm text-zinc-900 focus:outline-none focus:ring-1 focus:ring-zinc-900"
              >
                <option value="inbound">Entrada (Inbound)</option>
                <option value="both">Entrada e Saída (Bidirecional)</option>
              </select>
            </div>

            {provider === 'twilio' && (
              <div className="p-3 bg-rose-50/70 border border-rose-200/70 rounded-lg text-[11px] text-rose-900 space-y-1">
                <span className="font-semibold block">Configuração no Console da Twilio (A Call Comes In):</span>
                <p>
                  Aponte o <strong>Webhook (HTTP POST)</strong> do número na Twilio para:{' '}
                  <code className="font-mono bg-white px-1.5 py-0.5 rounded border border-rose-200">
                    {twilioWebhookUrl}
                  </code>
                </p>
              </div>
            )}

            <div className="flex justify-end gap-2 pt-3 border-t border-zinc-100">
              <Button type="button" variant="secondary" onClick={() => setIsModalOpen(false)}>
                Cancelar
              </Button>
              <Button type="submit" disabled={saving || !e164.trim()}>
                <Check className="w-4 h-4" />
                {saving ? 'Conectando…' : 'Conectar e Habilitar'}
              </Button>
            </div>
          </form>
        </div>
      </Modal>
    </div>
  )
}
