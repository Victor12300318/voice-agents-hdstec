import { useEffect, useState } from 'react'
import { NavLink, Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import {
  Home,
  FolderOpen,
  Wrench,
  ListTree,
  MessageSquare,
  BarChart2,
  Settings,
  Users,
  LogOut,
  Search,
  AudioWaveform,
  PhoneCall,
  PanelLeft,
  MoreHorizontal,
} from 'lucide-react'
import { api, ApiError, type Me, type Agent } from './api'
import Login from './pages/Login'
import Dashboard from './pages/Dashboard'
import Conversations from './pages/Conversations'
import ConversationDetailPage from './pages/ConversationDetail'
import Agents from './pages/Agents'
import AgentEditor from './pages/AgentEditor'
import AgentTest from './pages/AgentTest'
import SkillsPage from './pages/Skills'
import MCPsPage from './pages/MCPs'
import KnowledgeBases from './pages/KnowledgeBases'
import AnalyticsPage from './pages/Analytics'
import SettingsPage from './pages/Settings'
import MembersPage from './pages/Members'
import PhoneNumbersPage from './pages/PhoneNumbers'

const ORB_GRADIENTS = [
  'from-indigo-500 via-purple-400 to-sky-300',
  'from-emerald-500 via-teal-400 to-cyan-300',
  'from-blue-600 via-indigo-400 to-violet-300',
  'from-amber-500 via-orange-400 to-rose-300',
]

export default function App() {
  const [me, setMe] = useState<Me | null | undefined>(undefined)
  const [sidebarAgents, setSidebarAgents] = useState<Agent[]>([])
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')
  const location = useLocation()
  const navigate = useNavigate()

  const loadMe = () =>
    api<Me>('/api/auth/me')
      .then((res) => {
        setMe(res)
        api<Agent[]>('/api/agents')
          .then((list) => setSidebarAgents(list || []))
          .catch(() => {})
      })
      .catch((e) => {
        if (e instanceof ApiError && e.status === 401) setMe(null)
        else throw e
      })

  useEffect(() => {
    loadMe()
  }, [])

  // Atualiza a lista rápida de agentes na sidebar quando navega
  useEffect(() => {
    if (me) {
      api<Agent[]>('/api/agents')
        .then((list) => setSidebarAgents(list || []))
        .catch(() => {})
    }
  }, [location.pathname, me])

  if (me === undefined) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-white text-zinc-500 text-xs">
        <AudioWaveform className="w-4 h-4 animate-pulse text-zinc-400 mr-2" />
        Carregando plataforma…
      </div>
    )
  }

  if (me === null) return <Login onLogin={loadMe} />

  const logout = async () => {
    await api('/api/auth/logout', { method: 'POST' }).catch(() => {})
    setMe(null)
  }

  // Na tela de configuração de um Agente (/agents/:id), renderiza o workspace em tela cheia estilo ElevenLabs
  if (/^\/agents\/[^/]+$/.test(location.pathname)) {
    return (
      <Routes>
        <Route path="/agents/:id" element={<AgentEditor />} />
      </Routes>
    )
  }

  const getInitials = (email: string) => {
    return (email || 'V')[0].toUpperCase()
  }

  const configureItems = [
    { label: 'Base de conhecimento', path: '/knowledge', icon: FolderOpen },
    { label: 'Ferramentas', path: '/mcps', icon: Wrench },
    { label: 'Procedimentos', path: '/skills', icon: ListTree },
  ]

  const monitorItems = [
    { label: 'Conversas', path: '/conversations', icon: MessageSquare },
    { label: 'Analytics', path: '/analytics', icon: BarChart2 },
    { label: 'Usuários', path: '/organization/members', icon: Users },
  ]

  const deployItems = [
    { label: 'Telefonia', path: '/phone-numbers', icon: PhoneCall },
    { label: 'Configurações', path: '/organization/settings', icon: Settings },
  ]

  const getPageTitle = () => {
    if (location.pathname.startsWith('/agents')) return 'Agentes'
    if (location.pathname.startsWith('/dashboard')) return 'Início'
    if (location.pathname.startsWith('/knowledge')) return 'Base de conhecimento'
    if (location.pathname.startsWith('/mcps')) return 'Ferramentas'
    if (location.pathname.startsWith('/skills')) return 'Procedimentos'
    if (location.pathname.startsWith('/conversations')) return 'Conversas'
    if (location.pathname.startsWith('/analytics')) return 'Analytics'
    if (location.pathname.startsWith('/phone-numbers')) return 'Telefonia'
    if (location.pathname.startsWith('/organization/members')) return 'Usuários'
    if (location.pathname.startsWith('/organization/settings')) return 'Configurações'
    return 'Agentes'
  }

  return (
    <div className="flex h-screen overflow-hidden bg-white text-zinc-900 font-sans antialiased">
      {/* =====================================================================
          SIDEBAR DO WORKSPACE (Semelhante à Imagem 1, apenas com o que já temos funcionando)
         ===================================================================== */}
      <aside
        className={`${
          sidebarCollapsed ? 'w-14' : 'w-60'
        } flex-shrink-0 border-r border-zinc-200/80 bg-[#fbfbfb] flex flex-col justify-between select-none transition-all duration-200 overflow-y-auto`}
      >
        <div className="p-2.5 space-y-4">
          {/* Topo: Logo IIElevenLabs */}
          <div
            className={`flex items-center ${
              sidebarCollapsed ? 'justify-center py-1.5' : 'px-2.5 py-1.5 justify-between'
            }`}
          >
            <NavLink
              to="/agents"
              className="flex items-center gap-1.5 font-bold text-sm tracking-tight text-zinc-950"
            >
              <span className="font-black text-base leading-none">II</span>
              {!sidebarCollapsed && <span>ElevenLabs</span>}
            </NavLink>
          </div>

          {/* Busca rápida "Pesquisar e criar [⌘][K]" */}
          {sidebarCollapsed ? (
            <div className="flex justify-center">
              <button
                type="button"
                onClick={() => setSidebarCollapsed(false)}
                title="Pesquisar e criar"
                className="w-9 h-9 rounded-xl bg-white border border-zinc-200/90 flex items-center justify-center text-zinc-500 hover:text-zinc-900 shadow-2xs"
              >
                <Search className="w-4 h-4" />
              </button>
            </div>
          ) : (
            <form
              onSubmit={(e) => {
                e.preventDefault()
                navigate('/agents')
              }}
              className="flex items-center justify-between px-2.5 py-1.5 bg-white border border-zinc-200/90 rounded-xl text-xs text-zinc-400 shadow-2xs"
            >
              <span className="flex items-center gap-2 truncate flex-1">
                <Search className="w-3.5 h-3.5 text-zinc-400 flex-shrink-0" />
                <input
                  type="text"
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  placeholder="Pesquisar e criar"
                  className="w-full bg-transparent text-xs text-zinc-800 placeholder-zinc-400 focus:outline-none"
                />
              </span>
              <span className="flex items-center gap-0.5 text-[10px] font-mono flex-shrink-0">
                <kbd className="px-1 py-0.2 bg-zinc-100 border border-zinc-200 rounded text-zinc-500">
                  ⌘
                </kbd>
                <kbd className="px-1 py-0.2 bg-zinc-100 border border-zinc-200 rounded text-zinc-500">
                  K
                </kbd>
              </span>
            </form>
          )}

          {/* Item: Início */}
          <div className="space-y-0.5">
            <NavLink
              to="/dashboard"
              title="Início"
              className={({ isActive }) =>
                `flex items-center ${
                  sidebarCollapsed ? 'justify-center py-2' : 'gap-2.5 px-2.5 py-1.5'
                } rounded-xl text-xs transition-colors ${
                  isActive
                    ? 'bg-zinc-200/75 text-zinc-950 font-semibold'
                    : 'text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900'
                }`
              }
            >
              <Home className="w-4 h-4 text-zinc-500 flex-shrink-0" />
              {!sidebarCollapsed && <span>Início</span>}
            </NavLink>
          </div>

          {/* Grupo: Agentes (lista rápida dos agentes reais + "••• Mais" para ir à tabela de Agentes) */}
          <div className="space-y-0.5">
            {!sidebarCollapsed && (
              <NavLink
                to="/agents"
                className="px-2.5 text-[11px] font-medium text-zinc-400 hover:text-zinc-700 block mb-1"
              >
                Agentes
              </NavLink>
            )}

            {sidebarAgents.slice(0, 4).map((ag, idx) => {
              const grad = ORB_GRADIENTS[idx % ORB_GRADIENTS.length]
              return (
                <NavLink
                  key={ag.id}
                  to={`/agents/${ag.id}`}
                  title={ag.name}
                  className={`flex items-center ${
                    sidebarCollapsed ? 'justify-center py-2' : 'gap-2.5 px-2.5 py-1.5'
                  } rounded-xl text-xs text-zinc-700 hover:bg-zinc-100 hover:text-zinc-950 transition-colors`}
                >
                  <div
                    className={`w-4 h-4 rounded-full bg-gradient-to-tr ${grad} flex-shrink-0 shadow-2xs`}
                  />
                  {!sidebarCollapsed && <span className="truncate">{ag.name}</span>}
                </NavLink>
              )
            })}

            <NavLink
              to="/agents"
              title="Todos os agentes"
              className={({ isActive }) =>
                `flex items-center ${
                  sidebarCollapsed ? 'justify-center py-2' : 'gap-2.5 px-2.5 py-1.5'
                } rounded-xl text-xs transition-colors ${
                  isActive
                    ? 'bg-zinc-200/75 text-zinc-950 font-semibold'
                    : 'text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900'
                }`
              }
            >
              <MoreHorizontal className="w-4 h-4 text-zinc-500 flex-shrink-0" />
              {!sidebarCollapsed && <span>Mais</span>}
            </NavLink>
          </div>

          {/* Grupo: Configurar */}
          <div className="space-y-0.5">
            {!sidebarCollapsed && (
              <span className="px-2.5 text-[11px] font-medium text-zinc-400 block mb-1">
                Configurar
              </span>
            )}
            {configureItems.map((item) => {
              const Icon = item.icon
              return (
                <NavLink
                  key={item.path}
                  to={item.path}
                  title={item.label}
                  className={({ isActive }) =>
                    `flex items-center ${
                      sidebarCollapsed ? 'justify-center py-2' : 'gap-2.5 px-2.5 py-1.5'
                    } rounded-xl text-xs transition-colors ${
                      isActive
                        ? 'bg-zinc-200/75 text-zinc-950 font-semibold'
                        : 'text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900'
                    }`
                  }
                >
                  <Icon className="w-4 h-4 text-zinc-500 flex-shrink-0" />
                  {!sidebarCollapsed && <span className="truncate">{item.label}</span>}
                </NavLink>
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
            {monitorItems.map((item) => {
              const Icon = item.icon
              return (
                <NavLink
                  key={item.path}
                  to={item.path}
                  title={item.label}
                  className={({ isActive }) =>
                    `flex items-center ${
                      sidebarCollapsed ? 'justify-center py-2' : 'gap-2.5 px-2.5 py-1.5'
                    } rounded-xl text-xs transition-colors ${
                      isActive
                        ? 'bg-zinc-200/75 text-zinc-950 font-semibold'
                        : 'text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900'
                    }`
                  }
                >
                  <Icon className="w-4 h-4 text-zinc-500 flex-shrink-0" />
                  {!sidebarCollapsed && <span className="truncate">{item.label}</span>}
                </NavLink>
              )
            })}
          </div>

          {/* Grupo: Implantar */}
          <div className="space-y-0.5">
            {!sidebarCollapsed && (
              <span className="px-2.5 text-[11px] font-medium text-zinc-400 block mb-1">
                Implantar
              </span>
            )}
            {deployItems.map((item) => {
              const Icon = item.icon
              return (
                <NavLink
                  key={item.path}
                  to={item.path}
                  title={item.label}
                  className={({ isActive }) =>
                    `flex items-center ${
                      sidebarCollapsed ? 'justify-center py-2' : 'gap-2.5 px-2.5 py-1.5'
                    } rounded-xl text-xs transition-colors ${
                      isActive
                        ? 'bg-zinc-200/75 text-zinc-950 font-semibold'
                        : 'text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900'
                    }`
                  }
                >
                  <Icon className="w-4 h-4 text-zinc-500 flex-shrink-0" />
                  {!sidebarCollapsed && <span className="truncate">{item.label}</span>}
                </NavLink>
              )
            })}
          </div>
        </div>

        {/* Rodapé da Sidebar: Identificador ElevenAgents */}
        <div className="p-3 border-t border-zinc-200/60 flex items-center justify-between">
          <div className="flex items-center gap-2 min-w-0">
            <div className="w-5 h-5 rounded-md bg-gradient-to-tr from-sky-500 via-emerald-400 to-amber-300 flex-shrink-0 shadow-2xs" />
            {!sidebarCollapsed && (
              <span className="text-xs font-semibold text-zinc-800 truncate">ElevenAgents</span>
            )}
          </div>
        </div>
      </aside>

      {/* =====================================================================
          ÁREA PRINCIPAL (TOP BAR + CONTEÚDO)
         ===================================================================== */}
      <div className="flex-1 flex flex-col min-w-0 bg-white overflow-hidden">
        {/* Top Bar minimalista igual à Imagem 1 */}
        <header className="h-13 px-4 sm:px-6 border-b border-zinc-200/80 flex items-center justify-between flex-shrink-0 bg-white z-20">
          <div className="flex items-center gap-2.5">
            <button
              type="button"
              onClick={() => setSidebarCollapsed(!sidebarCollapsed)}
              title="Alternar barra lateral"
              className="p-1.5 text-zinc-500 hover:text-zinc-900 hover:bg-zinc-100 rounded-lg transition-colors"
            >
              <PanelLeft className="w-4 h-4" />
            </button>
            <span className="text-xs font-medium text-zinc-800">{getPageTitle()}</span>
          </div>

          <div className="flex items-center gap-2.5">
            <div
              title={me.email}
              className="w-7 h-7 rounded-full bg-emerald-700 text-white font-semibold text-xs flex items-center justify-center ring-2 ring-emerald-700/20"
            >
              {getInitials(me.email)}
            </div>
            <button
              type="button"
              onClick={logout}
              title="Encerrar sessão"
              className="p-1.5 text-zinc-400 hover:text-zinc-800 hover:bg-zinc-100 rounded-lg transition-colors"
            >
              <LogOut className="w-3.5 h-3.5" />
            </button>
          </div>
        </header>

        {/* Conteúdo da Página */}
        <main className="flex-1 p-6 sm:p-8 overflow-y-auto bg-white">
          <Routes>
            <Route path="/dashboard" element={<Dashboard />} />
            <Route path="/conversations" element={<Conversations />} />
            <Route path="/conversations/:id" element={<ConversationDetailPage />} />
            <Route path="/agents" element={<Agents />} />
            <Route path="/agents/:id" element={<AgentEditor />} />
            <Route path="/agents/:id/test" element={<AgentTest />} />
            <Route path="/skills" element={<SkillsPage />} />
            <Route path="/mcps" element={<MCPsPage />} />
            <Route path="/knowledge" element={<KnowledgeBases />} />
            <Route path="/phone-numbers" element={<PhoneNumbersPage />} />
            <Route path="/analytics" element={<AnalyticsPage />} />
            <Route path="/organization/settings" element={<SettingsPage />} />
            <Route path="/organization/members" element={<MembersPage />} />
            <Route path="*" element={<Navigate to="/agents" replace />} />
          </Routes>
        </main>
      </div>
    </div>
  )
}
