import { useState } from 'react'
import { Settings, Key, Shield, Check } from 'lucide-react'

export default function SettingsPage() {
  const [geminiKey, setGeminiKey] = useState('AIzaSy*******************************')
  const [openAIKey, setOpenAIKey] = useState('sk-proj-********************************')
  const [saved, setSaved] = useState(false)

  const handleSave = (e: React.FormEvent) => {
    e.preventDefault()
    setSaved(true)
    setTimeout(() => setSaved(false), 2500)
  }

  return (
    <div className="max-w-4xl mx-auto space-y-6">
      <div className="border-b border-zinc-200/80 pb-5">
        <h1 className="text-2xl font-semibold tracking-tight text-zinc-900 flex items-center gap-2">
          <Settings className="w-6 h-6 text-zinc-700" />
          Configurações da Organization
        </h1>
        <p className="text-sm text-zinc-500 mt-1">
          Gerencie chaves de API dos provedores de voz e parâmetros globais de execução.
        </p>
      </div>

      <form onSubmit={handleSave} className="space-y-6">
        {/* Voice Provider API Keys */}
        <div className="bg-white border border-zinc-200/80 rounded-xl p-6 shadow-xs space-y-4">
          <h2 className="text-base font-semibold text-zinc-900 flex items-center gap-2">
            <Key className="w-4 h-4 text-zinc-500" />
            Chaves de API dos Voice Providers
          </h2>
          <p className="text-xs text-zinc-500">
            As chaves são armazenadas criptografadas e usadas pelo backend Go para abrir os WebSockets de voz em tempo real.
          </p>

          <div className="space-y-4 pt-2">
            <div>
              <label className="block text-xs font-semibold text-zinc-700 uppercase tracking-wider mb-1.5">
                Google Gemini API Key (Gemini Live Bidi)
              </label>
              <input
                type="password"
                value={geminiKey}
                onChange={(e) => setGeminiKey(e.target.value)}
                className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-sm font-mono text-zinc-900 focus:outline-none focus:ring-1 focus:ring-zinc-900"
              />
            </div>

            <div>
              <label className="block text-xs font-semibold text-zinc-700 uppercase tracking-wider mb-1.5">
                OpenAI API Key (Realtime WebSocket)
              </label>
              <input
                type="password"
                value={openAIKey}
                onChange={(e) => setOpenAIKey(e.target.value)}
                className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-sm font-mono text-zinc-900 focus:outline-none focus:ring-1 focus:ring-zinc-900"
              />
            </div>
          </div>
        </div>

        {/* Global Runtime Options */}
        <div className="bg-white border border-zinc-200/80 rounded-xl p-6 shadow-xs space-y-4">
          <h2 className="text-base font-semibold text-zinc-900 flex items-center gap-2">
            <Shield className="w-4 h-4 text-zinc-500" />
            Parâmetros Globais do Runtime
          </h2>

          <div className="space-y-3">
            <label className="flex items-center gap-3 text-sm text-zinc-800 cursor-pointer">
              <input
                type="checkbox"
                defaultChecked
                className="rounded border-zinc-300 text-zinc-900 focus:ring-zinc-900"
              />
              Habilitar cancelamento de eco nativo no navegador (echoCancellation)
            </label>
            <label className="flex items-center gap-3 text-sm text-zinc-800 cursor-pointer">
              <input
                type="checkbox"
                defaultChecked
                className="rounded border-zinc-300 text-zinc-900 focus:ring-zinc-900"
              />
              Gravar áudio PCM16 de todas as conversas normais de produção
            </label>
          </div>
        </div>

        <div className="flex items-center justify-between pt-2">
          {saved && (
            <span className="text-sm text-emerald-600 font-medium flex items-center gap-1.5">
              <Check className="w-4 h-4" />
              Configurações salvas com sucesso!
            </span>
          )}
          <div className="ml-auto">
            <button
              type="submit"
              className="px-5 py-2 bg-zinc-900 hover:bg-zinc-800 text-white text-sm font-medium rounded-lg shadow-sm transition-colors"
            >
              Salvar Alterações
            </button>
          </div>
        </div>
      </form>
    </div>
  )
}
