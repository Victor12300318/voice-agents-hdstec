import { useState } from 'react'
import { Users, Plus, UserCheck, X } from 'lucide-react'
import Modal from '../components/Modal'
import Button from '../components/Button'

export default function MembersPage() {
  const [members, setMembers] = useState([
    {
      id: 'usr-1',
      name: 'Victor Felix',
      email: 'victor.felix@affix.com.br',
      role: 'Proprietário',
      joined: '01/10/2026',
    },
    {
      id: 'usr-2',
      name: 'Equipe de Operações Voz',
      email: 'ops.voz@affix.com.br',
      role: 'Operador',
      joined: '15/09/2026',
    },
  ])

  const [isInviteOpen, setIsInviteOpen] = useState(false)
  const [inviteEmail, setInviteEmail] = useState('')

  const handleInvite = (e: React.FormEvent) => {
    e.preventDefault()
    if (!inviteEmail) return
    setMembers([
      ...members,
      {
        id: `usr-${Date.now()}`,
        name: inviteEmail.split('@')[0],
        email: inviteEmail,
        role: 'Membro',
        joined: 'Hoje',
      },
    ])
    setInviteEmail('')
    setIsInviteOpen(false)
  }

  return (
    <div className="max-w-4xl mx-auto space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 border-b border-zinc-200/80 pb-5">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight text-zinc-900 flex items-center gap-2">
            <Users className="w-6 h-6 text-zinc-700" />
            Membros da Organization
          </h1>
          <p className="text-sm text-zinc-500 mt-1">
            Pessoas autorizadas a configurar, testar e publicar agentes nesta Organization.
          </p>
        </div>
        <Button onClick={() => setIsInviteOpen(true)} className="shadow-sm">
          <Plus className="w-4 h-4" />
          Convidar Membro
        </Button>
      </div>

      <div className="bg-white border border-zinc-200/80 rounded-xl overflow-hidden shadow-xs">
        <table className="w-full text-left text-sm">
          <thead className="bg-zinc-50/50 border-b border-zinc-200 text-xs font-semibold text-zinc-600 uppercase tracking-wider">
            <tr>
              <th className="px-6 py-3.5">Nome / Usuário</th>
              <th className="px-6 py-3.5">E-mail</th>
              <th className="px-6 py-3.5">Função</th>
              <th className="px-6 py-3.5">Adicionado em</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-zinc-200">
            {members.map((m) => (
              <tr key={m.id} className="hover:bg-zinc-50/50 transition-colors">
                <td className="px-6 py-4 font-medium text-zinc-900 flex items-center gap-2">
                  <div className="w-7 h-7 rounded-full bg-zinc-100 border border-zinc-200 flex items-center justify-center font-bold text-xs text-zinc-700">
                    {m.name.charAt(0)}
                  </div>
                  {m.name}
                </td>
                <td className="px-6 py-4 text-zinc-600 font-mono text-xs">{m.email}</td>
                <td className="px-6 py-4">
                  <span className="inline-flex items-center gap-1 text-xs font-medium px-2 py-0.5 rounded-full bg-zinc-100 text-zinc-700">
                    <UserCheck className="w-3 h-3 text-zinc-500" />
                    {m.role}
                  </span>
                </td>
                <td className="px-6 py-4 text-zinc-500 text-xs">{m.joined}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <Modal open={isInviteOpen} onClose={() => setIsInviteOpen(false)}>
        <div className="p-6">
          <div className="flex items-center justify-between mb-4">
            <h3 className="font-semibold text-zinc-900">Convidar Novo Usuário</h3>
            <button onClick={() => setIsInviteOpen(false)} className="text-zinc-400 hover:text-zinc-700">
              <X className="w-5 h-5" />
            </button>
          </div>
          <form onSubmit={handleInvite} className="space-y-4">
            <div>
              <label className="block text-xs font-semibold text-zinc-700 uppercase tracking-wider mb-1">
                E-mail institucional
              </label>
              <input
                type="email"
                required
                value={inviteEmail}
                onChange={(e) => setInviteEmail(e.target.value)}
                placeholder="usuario@empresa.com"
                className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-sm text-zinc-900 focus:outline-none focus:ring-1 focus:ring-zinc-900"
              />
            </div>
            <div className="flex justify-end gap-2 pt-2">
              <Button type="button" variant="secondary" onClick={() => setIsInviteOpen(false)}>
                Cancelar
              </Button>
              <Button type="submit">Enviar Convite</Button>
            </div>
          </form>
        </div>
      </Modal>
    </div>
  )
}
