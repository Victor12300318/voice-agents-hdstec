// frontend/src/pages/KnowledgeBases.tsx
import { useEffect, useState, type FormEvent } from 'react'
import { motion } from 'motion/react'
import { BookOpen, Plus, FileText, Trash2, HardDrive, X } from 'lucide-react'
import { api, type KnowledgeBase, type KnowledgeDocument } from '../api'
import Modal from '../components/Modal'
import Button from '../components/Button'
import { springSnappy } from '../components/motion'

export default function KnowledgeBases() {
  const [bases, setBases] = useState<KnowledgeBase[]>([])
  const [selectedKb, setSelectedKb] = useState<KnowledgeBase | null>(null)
  const [documents, setDocuments] = useState<KnowledgeDocument[]>([])
  const [errorMsg, setErrorMsg] = useState('')

  // Modal nova base
  const [modalOpen, setModalOpen] = useState(false)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')

  // Modal novo documento
  const [docModalOpen, setDocModalOpen] = useState(false)
  const [docName, setDocName] = useState('')
  const [docContent, setDocContent] = useState('')

  const loadBases = () => {
    api<KnowledgeBase[]>('/api/knowledge-bases')
      .then((res) => {
        setBases(res)
        if (selectedKb) {
          const updated = res.find((b) => b.id === selectedKb.id)
          if (updated) setSelectedKb(updated)
        }
      })
      .catch((e) => setErrorMsg(e.message))
  }

  const loadDocuments = (kbId: string) => {
    api<{ knowledge_base: KnowledgeBase; documents: KnowledgeDocument[] }>(`/api/knowledge-bases/${kbId}`)
      .then((res) => {
        setSelectedKb(res.knowledge_base)
        setDocuments(res.documents)
      })
      .catch((e) => setErrorMsg(e.message))
  }

  useEffect(() => {
    loadBases()
  }, [])

  const selectKb = (kb: KnowledgeBase) => {
    setSelectedKb(kb)
    loadDocuments(kb.id)
  }

  const createBase = async (e: FormEvent) => {
    e.preventDefault()
    setErrorMsg('')
    try {
      const created = await api<KnowledgeBase>('/api/knowledge-bases', {
        method: 'POST',
        body: JSON.stringify({ name, description }),
      })
      setName('')
      setDescription('')
      setModalOpen(false)
      loadBases()
      selectKb(created)
    } catch (err) {
      setErrorMsg((err as Error).message)
    }
  }

  const deleteBase = async (id: string, kbName: string) => {
    if (!window.confirm(`Excluir a base de conhecimento "${kbName}"?`)) return
    try {
      await api(`/api/knowledge-bases/${id}`, { method: 'DELETE' })
      if (selectedKb?.id === id) {
        setSelectedKb(null)
        setDocuments([])
      }
      loadBases()
    } catch (err) {
      setErrorMsg((err as Error).message)
    }
  }

  const addDocument = async (e: FormEvent) => {
    e.preventDefault()
    if (!selectedKb) return
    setErrorMsg('')
    try {
      await api(`/api/knowledge-bases/${selectedKb.id}/documents`, {
        method: 'POST',
        body: JSON.stringify({ name: docName, content: docContent }),
      })
      setDocName('')
      setDocContent('')
      setDocModalOpen(false)
      loadDocuments(selectedKb.id)
      loadBases()
    } catch (err) {
      setErrorMsg((err as Error).message)
    }
  }

  const deleteDocument = async (docId: string, dName: string) => {
    if (!selectedKb || !window.confirm(`Excluir o documento "${dName}"?`)) return
    try {
      await api(`/api/knowledge-bases/${selectedKb.id}/documents/${docId}`, { method: 'DELETE' })
      loadDocuments(selectedKb.id)
      loadBases()
    } catch (err) {
      setErrorMsg((err as Error).message)
    }
  }

  return (
    <div className="max-w-6xl mx-auto space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 border-b border-zinc-200/80 pb-5">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight text-zinc-900 flex items-center gap-2">
            <BookOpen className="w-6 h-6 text-zinc-700" />
            Bases de Conhecimento
          </h1>
          <p className="text-sm text-zinc-500 mt-1">
            Documentos em texto ou markdown incluídos inline quando uma Skill referencia a base.
          </p>
        </div>
        <Button onClick={() => setModalOpen(true)} className="shadow-sm">
          <Plus className="w-4 h-4" />
          Nova Base
        </Button>
      </div>

      {errorMsg && (
        <div className="p-3 bg-red-50 border border-red-200 rounded-lg text-red-700 text-xs">{errorMsg}</div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-12 gap-6 items-start">
        {/* Lista de bases */}
        <div className="lg:col-span-5 space-y-3">
          <div className="flex items-center justify-between px-1">
            <h3 className="text-xs font-semibold text-zinc-500 uppercase tracking-wider">Bases da Organização</h3>
            <span className="text-xs text-zinc-400">{bases.length} bases</span>
          </div>

          <div className="space-y-2.5">
            {bases.map((b, i) => {
              const active = selectedKb?.id === b.id
              return (
                <motion.div
                  key={b.id}
                  initial={{ opacity: 0, y: 10 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ ...springSnappy, delay: Math.min(i * 0.03, 0.15) }}
                  whileHover={{ y: -2 }}
                  onClick={() => selectKb(b)}
                  className={`cursor-pointer rounded-xl p-4 border shadow-xs hover:shadow-[var(--shadow-float)] ${
                    active ? 'bg-zinc-900 text-white border-zinc-900' : 'bg-white border-zinc-200/80 hover:border-zinc-300'
                  }`}
                >
                  <div className="flex items-start justify-between gap-2">
                    <strong className={`text-sm ${active ? 'text-white' : 'text-zinc-900'}`}>{b.name}</strong>
                    <button
                      type="button"
                      onClick={(e) => {
                        e.stopPropagation()
                        deleteBase(b.id, b.name)
                      }}
                      title="Excluir base"
                      className={`p-1 rounded-md transition-colors ${
                        active ? 'text-zinc-400 hover:text-red-400' : 'text-zinc-300 hover:text-red-600'
                      }`}
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  </div>
                  {b.description && (
                    <p className={`text-xs mt-1 line-clamp-2 ${active ? 'text-zinc-300' : 'text-zinc-500'}`}>
                      {b.description}
                    </p>
                  )}
                  <div
                    className={`flex items-center gap-3 mt-3 text-[11px] font-mono ${
                      active ? 'text-zinc-400' : 'text-zinc-400'
                    }`}
                  >
                    <span className="inline-flex items-center gap-1">
                      <FileText className="w-3 h-3" /> {b.documents_count || 0} doc(s)
                    </span>
                    <span className="inline-flex items-center gap-1">
                      <HardDrive className="w-3 h-3" /> {Math.round((b.total_bytes || 0) / 1024)} KB
                    </span>
                  </div>
                </motion.div>
              )
            })}
            {bases.length === 0 && (
              <p className="text-sm text-zinc-400 italic px-1">
                Nenhuma base cadastrada. Clique em "Nova Base" para criar a primeira.
              </p>
            )}
          </div>
        </div>

        {/* Detalhe da base selecionada */}
        <div className="lg:col-span-7 bg-white border border-zinc-200/80 rounded-xl shadow-xs p-5 min-h-[320px]">
          {selectedKb ? (
            <>
              <div className="flex items-start justify-between gap-3 pb-4 border-b border-zinc-100">
                <div>
                  <h2 className="text-lg font-semibold text-zinc-900">{selectedKb.name}</h2>
                  {selectedKb.description && <p className="text-sm text-zinc-500 mt-0.5">{selectedKb.description}</p>}
                </div>
                <Button size="sm" onClick={() => setDocModalOpen(true)}>
                  <Plus className="w-3.5 h-3.5" />
                  Adicionar Documento
                </Button>
              </div>

              <div className="pt-4 space-y-3">
                <div className="flex items-center justify-between">
                  <h4 className="text-xs font-semibold text-zinc-700 uppercase tracking-wider">
                    Documentos de Texto ({documents.length})
                  </h4>
                  <span className="text-[11px] text-zinc-400">Máx. 50 KB recomendado por base</span>
                </div>

                {documents.map((d) => (
                  <div
                    key={d.id}
                    className="flex items-start justify-between gap-3 p-3 bg-zinc-50 border border-zinc-200 rounded-lg"
                  >
                    <div className="min-w-0 flex-1">
                      <strong className="text-sm text-zinc-900 block">{d.name}</strong>
                      <small className="text-[11px] text-zinc-400">
                        {d.content.length} caracteres ({Math.round(d.content.length / 1024)} KB)
                      </small>
                      <pre className="mt-1.5 text-[11px] font-mono text-zinc-500 whitespace-pre-wrap line-clamp-3 bg-white border border-zinc-100 rounded p-2">
                        {d.content.slice(0, 200)}…
                      </pre>
                    </div>
                    <button
                      type="button"
                      onClick={() => deleteDocument(d.id, d.name)}
                      title="Excluir documento"
                      className="text-zinc-300 hover:text-red-600 p-1 rounded-md flex-shrink-0"
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </div>
                ))}

                {documents.length === 0 && (
                  <p className="text-sm text-zinc-400 italic">
                    Esta base ainda não possui documentos. Clique em "Adicionar Documento" para colar texto ou markdown.
                  </p>
                )}
              </div>
            </>
          ) : (
            <div className="h-full flex items-center justify-center text-center text-sm text-zinc-400 py-16">
              Selecione uma base de conhecimento à esquerda para gerenciar seus documentos.
            </div>
          )}
        </div>
      </div>

      {/* Modal Criar Base */}
      <Modal open={modalOpen} onClose={() => setModalOpen(false)}>
        <div className="p-6">
          <div className="flex items-center justify-between mb-4">
            <h2 className="font-semibold text-zinc-900">Nova Base de Conhecimento</h2>
            <button type="button" onClick={() => setModalOpen(false)} className="text-zinc-400 hover:text-zinc-700">
              <X className="w-5 h-5" />
            </button>
          </div>
          <form onSubmit={createBase} className="space-y-4">
            <div>
              <label className="block text-xs font-semibold text-zinc-700 uppercase tracking-wider mb-1">
                Nome da Base
              </label>
              <input
                type="text"
                placeholder="Ex: Regras IRPF e Documentos"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
                className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-sm text-zinc-900 focus:outline-none focus:ring-1 focus:ring-zinc-900"
              />
            </div>
            <div>
              <label className="block text-xs font-semibold text-zinc-700 uppercase tracking-wider mb-1">
                Descrição
              </label>
              <textarea
                rows={2}
                placeholder="Normas, perguntas frequentes e tabelas de consulta..."
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-sm text-zinc-900 focus:outline-none focus:ring-1 focus:ring-zinc-900"
              />
            </div>
            <div className="flex justify-end gap-2 pt-2">
              <Button type="button" variant="secondary" onClick={() => setModalOpen(false)}>
                Cancelar
              </Button>
              <Button type="submit">Criar Base</Button>
            </div>
          </form>
        </div>
      </Modal>

      {/* Modal Adicionar Documento */}
      <Modal open={docModalOpen} onClose={() => setDocModalOpen(false)} sizeClass="max-w-lg">
        <div className="p-6">
          <div className="flex items-center justify-between mb-4">
            <h2 className="font-semibold text-zinc-900">Adicionar Documento à Base</h2>
            <button type="button" onClick={() => setDocModalOpen(false)} className="text-zinc-400 hover:text-zinc-700">
              <X className="w-5 h-5" />
            </button>
          </div>
          <form onSubmit={addDocument} className="space-y-4">
            <div>
              <label className="block text-xs font-semibold text-zinc-700 uppercase tracking-wider mb-1">
                Nome do Documento
              </label>
              <input
                type="text"
                placeholder="Ex: regras_renegociacao.md"
                value={docName}
                onChange={(e) => setDocName(e.target.value)}
                required
                className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-sm font-mono text-zinc-900 focus:outline-none focus:ring-1 focus:ring-zinc-900"
              />
            </div>
            <div>
              <label className="block text-xs font-semibold text-zinc-700 uppercase tracking-wider mb-1">
                Conteúdo em Texto / Markdown (máx. 200 KB)
              </label>
              <textarea
                rows={10}
                placeholder="Cole aqui o texto, regras ou tabela que o agente deve usar para responder..."
                value={docContent}
                onChange={(e) => setDocContent(e.target.value)}
                required
                className="w-full px-3 py-2.5 bg-zinc-900 text-zinc-100 border border-zinc-800 rounded-lg text-xs font-mono focus:outline-none focus:ring-1 focus:ring-zinc-600 leading-relaxed"
              />
            </div>
            <div className="flex justify-end gap-2 pt-2">
              <Button type="button" variant="secondary" onClick={() => setDocModalOpen(false)}>
                Cancelar
              </Button>
              <Button type="submit">Salvar Documento</Button>
            </div>
          </form>
        </div>
      </Modal>
    </div>
  )
}
