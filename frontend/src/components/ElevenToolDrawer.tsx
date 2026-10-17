import { useEffect, useState } from 'react'
import {
  Webhook,
  Wrench,
  Blocks,
  Code2,
  BookOpen,
  Video,
  ChevronRight,
  ChevronDown,
  Plus,
  Trash2,
  X,
  Play,
  ExternalLink,
  Copy,
  Check,
} from 'lucide-react'
import Sheet from './Sheet'
import {
  api,
  type ToolWithStats,
  type ToolParamSchema,
  type ToolHeaderSchema,
  type ToolAssignment,
  type ToolResponseMock,
} from '../api'

export type ToolCategory = 'webhook' | 'client' | 'mcp' | 'code'

interface ElevenToolDrawerProps {
  open: boolean
  onClose: () => void
  initialKind?: ToolCategory
  editingTool?: ToolWithStats | null
  onSaved: (savedTool: ToolWithStats) => void
}

const DEFAULT_CODE_TEMPLATE = `export default async (ctx) => {
  // ctx.args.<paramName> — parâmetros fornecidos pelo agente na chamada
  const { valor } = ctx.args;

  return {
    sucesso: true,
    resultado: \`Valor processado: \${valor ?? 'nenhum'}\`,
  };
};
`

export default function ElevenToolDrawer({
  open,
  onClose,
  initialKind = 'webhook',
  editingTool = null,
  onSaved,
}: ElevenToolDrawerProps) {
  const [kind, setKind] = useState<ToolCategory>(initialKind)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')

  // Campos de Webhook / API Schema
  const [method, setMethod] = useState<'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'>('GET')
  const [url, setUrl] = useState('')
  const [authConnection, setAuthConnection] = useState<string>('')
  const [authTokenValue, setAuthTokenValue] = useState<string>('')
  const [contentType, setContentType] = useState<
    'application/json' | 'application/x-www-form-urlencoded'
  >('application/json')
  const [responseFilter, setResponseFilter] = useState<string>('')
  const [requestHeaders, setRequestHeaders] = useState<ToolHeaderSchema[]>([])
  const [pathParams, setPathParams] = useState<ToolParamSchema[]>([])
  const [queryParams, setQueryParams] = useState<ToolParamSchema[]>([])
  const [bodyDescription, setBodyDescription] = useState<string>('')
  const [bodyParams, setBodyParams] = useState<ToolParamSchema[]>([])

  // Campos Comuns ElevenLabs (Avançado, Variáveis Dinâmicas, Atribuições, Mocks)
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [responseTimeoutSecs, setResponseTimeoutSecs] = useState<number>(20)
  const [followRedirects, setFollowRedirects] = useState<boolean>(false)
  const [followRedirectsDomains, setFollowRedirectsDomains] = useState<string>('')
  const [interruptionMode, setInterruptionMode] = useState<string>('allow')
  const [preToolSpeech, setPreToolSpeech] = useState<string>('auto')
  const [toolCallSound, setToolCallSound] = useState<string>('')
  const [toolCallSoundBehavior, setToolCallSoundBehavior] = useState<string>('auto')
  const [executionMode, setExecutionMode] = useState<string>('immediate')
  const [toolErrorHandlingMode, setToolErrorHandlingMode] = useState<string>('auto')

  const [dynamicVarPlaceholders, setDynamicVarPlaceholders] = useState<Record<string, string>>({})
  const [newDynVarKey, setNewDynVarKey] = useState('')
  const [newDynVarVal, setNewDynVarVal] = useState('')

  const [assignments, setAssignments] = useState<ToolAssignment[]>([])
  const [responseMocks, setResponseMocks] = useState<ToolResponseMock[]>([])

  // Campos específicos: Client Tool
  const [expectsResponse, setExpectsResponse] = useState<boolean>(false)
  const [clientParams, setClientParams] = useState<ToolParamSchema[]>([])
  const [copiedSnippet, setCopiedSnippet] = useState(false)

  // Campos específicos: MCP Tool
  const [mcpTransport, setMcpTransport] = useState<'SSE' | 'STREAMABLE_HTTP'>('SSE')
  const [mcpApprovalPolicy, setMcpApprovalPolicy] = useState<
    'always_ask' | 'fine_grained' | 'no_approval'
  >('no_approval')
  const [mcpSecretToken, setMcpSecretToken] = useState('')
  const [mcpParams, setMcpParams] = useState<ToolParamSchema[]>([])

  // Campos específicos: Code Tool
  const [codeSource, setCodeSource] = useState<string>(DEFAULT_CODE_TEMPLATE)
  const [codeTab, setCodeTab] = useState<'code' | 'run'>('code')
  const [codeParams, setCodeParams] = useState<ToolParamSchema[]>([
    {
      name: 'valor',
      type: 'string',
      description: 'Valor de entrada a ser processado pelo código',
      required: true,
      value_type: 'llm_prompt',
    },
  ])
  const [codeContextConfig, setCodeContextConfig] = useState<Record<string, string>>({})
  const [newCtxKey, setNewCtxKey] = useState('')
  const [newCtxVal, setNewCtxVal] = useState('')
  const [codeTestArgs, setCodeTestArgs] = useState('{\n  "valor": "100"\n}')
  const [codeRunOutput, setCodeRunOutput] = useState<any>(null)
  const [codeRunLogs, setCodeRunLogs] = useState<string[]>([])
  const [codeRunning, setCodeRunning] = useState(false)

  // Modo Editor JSON ("</> Editar como JSON")
  const [isJsonMode, setIsJsonMode] = useState(false)
  const [jsonText, setJsonText] = useState('')
  const [jsonError, setJsonError] = useState('')

  const [saving, setSaving] = useState(false)
  const [errorMsg, setErrorMsg] = useState('')

  // Inicializa o estado quando abre a gaveta
  useEffect(() => {
    if (!open) return
    setErrorMsg('')
    setJsonError('')
    setIsJsonMode(false)
    setAdvancedOpen(false)

    if (editingTool) {
      const k = ((editingTool.kind as ToolCategory) || 'webhook')
      setKind(k)
      setName(editingTool.name || '')
      setDescription(editingTool.description || '')

      const cfg = editingTool.config || { url: '', method: 'GET' }
      const apiSchema = cfg.api_schema

      setMethod(
        ((apiSchema?.method || cfg.method || (k === 'webhook' ? 'POST' : 'GET')).toUpperCase() as any) ||
          'GET',
      )
      setUrl(apiSchema?.url || cfg.url || '')
      setContentType((apiSchema?.content_type as any) || 'application/json')
      setAuthConnection(apiSchema?.auth_connection ? String(apiSchema.auth_connection) : '')
      setAuthTokenValue('')
      setResponseFilter(apiSchema?.response_filter ? String(apiSchema.response_filter) : '')
      setRequestHeaders(apiSchema?.request_headers || [])
      setPathParams(apiSchema?.path_params_schema || [])
      setQueryParams(apiSchema?.query_params_schema || [])

      if (apiSchema?.request_body_schema?.properties) {
        setBodyDescription(apiSchema.request_body_schema.description || '')
        setBodyParams(apiSchema.request_body_schema.properties)
      } else if (k === 'webhook' && (editingTool.input_schema || []).length > 0) {
        // Compatibilidade com ferramentas antigas que tinham apenas input_schema
        const legacyParams: ToolParamSchema[] = editingTool.input_schema.map((p) => ({
          name: p.name,
          type: p.type || 'string',
          description: p.description || '',
          required: p.required ?? true,
          value_type: 'llm_prompt',
        }))
        if ((cfg.method || 'POST').toUpperCase() === 'GET') {
          setQueryParams(legacyParams)
          setBodyParams([])
        } else {
          setBodyParams(legacyParams)
        }
      } else {
        setBodyDescription('')
        setBodyParams([])
      }

      setResponseTimeoutSecs(
        cfg.response_timeout_secs ||
          (editingTool.timeout_ms ? Math.round(editingTool.timeout_ms / 1000) : 20),
      )
      setFollowRedirects(Boolean(cfg.follow_redirects))
      setFollowRedirectsDomains((cfg.follow_redirects_allowed_domains || []).join(', '))
      setInterruptionMode(cfg.interruption_mode || 'allow')
      setPreToolSpeech(cfg.pre_tool_speech || 'auto')
      setToolCallSound(cfg.tool_call_sound || '')
      setToolCallSoundBehavior(cfg.tool_call_sound_behavior || 'auto')
      setExecutionMode(cfg.execution_mode || 'immediate')
      setToolErrorHandlingMode(cfg.tool_error_handling_mode || 'auto')

      const placeholders: Record<string, string> = {}
      if (cfg.dynamic_variables?.dynamic_variable_placeholders) {
        for (const [dk, dv] of Object.entries(cfg.dynamic_variables.dynamic_variable_placeholders)) {
          placeholders[dk] = String(dv)
        }
      }
      setDynamicVarPlaceholders(placeholders)
      setAssignments(cfg.assignments || [])
      setResponseMocks(cfg.response_mocks || [])

      // Client tool
      setExpectsResponse(Boolean(cfg.expects_response))
      setClientParams(
        (editingTool.input_schema || []).map((p) => ({
          name: p.name,
          type: p.type || 'string',
          description: p.description || '',
          required: p.required ?? true,
          value_type: 'llm_prompt',
        })),
      )

      // MCP tool
      setMcpTransport((cfg.transport as any) || 'SSE')
      setMcpApprovalPolicy((cfg.approval_policy as any) || 'no_approval')
      setMcpSecretToken('')
      setMcpParams(
        (editingTool.input_schema || []).map((p) => ({
          name: p.name,
          type: p.type || 'string',
          description: p.description || '',
          required: p.required ?? true,
          value_type: 'llm_prompt',
        })),
      )

      // Code tool
      setCodeSource(cfg.code || DEFAULT_CODE_TEMPLATE)
      setCodeParams(
        (editingTool.input_schema || []).map((p) => ({
          name: p.name,
          type: p.type || 'string',
          description: p.description || '',
          required: p.required ?? true,
          value_type: 'llm_prompt',
        })),
      )
      setCodeContextConfig(cfg.context_object?.config || {})
    } else {
      setKind(initialKind)
      setName('')
      setDescription('')
      setMethod('GET')
      setUrl('')
      setAuthConnection('')
      setAuthTokenValue('')
      setContentType('application/json')
      setResponseFilter('')
      setRequestHeaders([])
      setPathParams([])
      setQueryParams([])
      setBodyDescription('')
      setBodyParams([])
      setResponseTimeoutSecs(20)
      setFollowRedirects(false)
      setFollowRedirectsDomains('')
      setInterruptionMode('allow')
      setPreToolSpeech('auto')
      setToolCallSound('')
      setToolCallSoundBehavior('auto')
      setExecutionMode('immediate')
      setToolErrorHandlingMode('auto')
      setDynamicVarPlaceholders({})
      setAssignments([])
      setResponseMocks([])
      setExpectsResponse(false)
      setClientParams([])
      setMcpTransport('SSE')
      setMcpApprovalPolicy('no_approval')
      setMcpSecretToken('')
      setMcpParams([])
      setCodeSource(DEFAULT_CODE_TEMPLATE)
      setCodeParams([
        {
          name: 'valor',
          type: 'string',
          description: 'Valor de entrada para o código',
          required: true,
          value_type: 'llm_prompt',
        },
      ])
      setCodeContextConfig({})
    }
  }, [open, editingTool, initialKind])

  // Detecta automaticamente {param} de caminho na URL (ignorando {{variavel_dinamica}})
  const handleUrlChange = (nextUrl: string) => {
    setUrl(nextUrl)
    // Remove blocos {{...}} temporariamente para extrair apenas {caminho}
    const withoutDouble = nextUrl.replace(/\{\{[^}]*\}\}/g, '')
    const matches = Array.from(withoutDouble.matchAll(/\{([A-Za-z0-9_]+)\}/g)).map((m) => m[1])
    if (matches.length > 0) {
      setPathParams((prev) => {
        const next = [...prev]
        for (const m of matches) {
          if (!next.some((p) => p.name === m)) {
            next.push({
              name: m,
              type: 'string',
              description: `Parâmetro de caminho {${m}}`,
              required: true,
              value_type: 'llm_prompt',
            })
          }
        }
        return next.filter((p) => matches.includes(p.name))
      })
    } else {
      setPathParams([])
    }

    // Detecta {{variavel}} para sugerir em Variáveis Dinâmicas
    const dynMatches = Array.from(nextUrl.matchAll(/\{\{([A-Za-z0-9_]+)\}\}/g)).map((m) => m[1])
    if (dynMatches.length > 0) {
      setDynamicVarPlaceholders((prev) => {
        const next = { ...prev }
        for (const dm of dynMatches) {
          if (!(dm in next)) {
            next[dm] = ''
          }
        }
        return next
      })
    }
  }

  // Constrói o JSON exato no padrão ElevenLabs a partir do formulário visual
  const buildElevenLabsJsonObject = () => {
    const domains = followRedirectsDomains
      .split(',')
      .map((d) => d.trim())
      .filter(Boolean)

    if (kind === 'webhook') {
      const hasBody =
        (method === 'POST' || method === 'PUT' || method === 'PATCH') && bodyParams.length > 0
      return {
        type: 'webhook',
        name,
        description,
        api_schema: {
          url,
          method,
          path_params_schema: pathParams,
          query_params_schema: queryParams,
          request_body_schema: hasBody
            ? {
                type: 'object' as const,
                description: bodyDescription || 'Corpo da solicitação enviado pelo agente',
                properties: bodyParams,
              }
            : null,
          request_headers: requestHeaders,
          content_type: contentType,
          auth_connection: authConnection || null,
          mtls_auth_connection: null,
          response_filter: responseFilter || null,
        },
        response_timeout_secs: responseTimeoutSecs,
        follow_redirects: followRedirects,
        follow_redirects_allowed_domains: domains,
        dynamic_variables: {
          dynamic_variable_placeholders: dynamicVarPlaceholders,
        },
        assignments,
        interruption_mode: interruptionMode,
        pre_tool_speech: preToolSpeech,
        tool_call_sound: toolCallSound ? toolCallSound : null,
        tool_call_sound_behavior: toolCallSoundBehavior,
        execution_mode: executionMode,
        tool_error_handling_mode: toolErrorHandlingMode,
        response_mocks: responseMocks,
      }
    }

    if (kind === 'client') {
      const propsObj: Record<string, any> = {}
      const reqList: string[] = []
      for (const p of clientParams) {
        if (!p.name.trim()) continue
        propsObj[p.name.trim()] = {
          type: p.type || 'string',
          description: p.description || '',
        }
        if (p.required) reqList.push(p.name.trim())
      }
      return {
        type: 'client',
        name,
        description,
        expects_response: expectsResponse,
        response_timeout_secs: responseTimeoutSecs,
        parameters: {
          type: 'object',
          properties: propsObj,
          required: reqList,
        },
        dynamic_variables: {
          dynamic_variable_placeholders: dynamicVarPlaceholders,
        },
        assignments,
        interruption_mode: interruptionMode,
        pre_tool_speech: preToolSpeech,
        tool_call_sound: toolCallSound ? toolCallSound : null,
        tool_call_sound_behavior: toolCallSoundBehavior,
        execution_mode: executionMode,
        tool_error_handling_mode: toolErrorHandlingMode,
        response_mocks: responseMocks,
      }
    }

    if (kind === 'mcp') {
      return {
        type: 'mcp',
        name,
        description,
        mcp_config: {
          url,
          transport: mcpTransport,
          approval_policy: mcpApprovalPolicy,
          request_headers: requestHeaders,
        },
        response_timeout_secs: responseTimeoutSecs,
        parameters: mcpParams,
        response_mocks: responseMocks,
      }
    }

    // kind === 'code'
    return {
      type: 'code',
      name,
      description,
      code: codeSource,
      parameters: codeParams,
      context_object: {
        config: codeContextConfig,
        secrets: [],
        auth_connections: [],
      },
      response_timeout_secs: responseTimeoutSecs,
      dynamic_variables: {
        dynamic_variable_placeholders: dynamicVarPlaceholders,
      },
      assignments,
      interruption_mode: interruptionMode,
      pre_tool_speech: preToolSpeech,
      tool_call_sound: toolCallSound ? toolCallSound : null,
      tool_call_sound_behavior: toolCallSoundBehavior,
      execution_mode: executionMode,
      tool_error_handling_mode: toolErrorHandlingMode,
      response_mocks: responseMocks,
    }
  }

  // Aplica um JSON editado pelo usuário de volta para os estados visuais
  const applyJsonToVisualState = (rawJson: string): boolean => {
    try {
      const parsed = JSON.parse(rawJson)
      setJsonError('')

      if (parsed.type && ['webhook', 'client', 'mcp', 'code'].includes(parsed.type)) {
        setKind(parsed.type)
      }
      if (typeof parsed.name === 'string') setName(parsed.name)
      if (typeof parsed.description === 'string') setDescription(parsed.description)
      if (typeof parsed.response_timeout_secs === 'number') {
        setResponseTimeoutSecs(parsed.response_timeout_secs)
      }
      if (typeof parsed.follow_redirects === 'boolean') {
        setFollowRedirects(parsed.follow_redirects)
      }
      if (Array.isArray(parsed.follow_redirects_allowed_domains)) {
        setFollowRedirectsDomains(parsed.follow_redirects_allowed_domains.join(', '))
      }
      if (parsed.dynamic_variables?.dynamic_variable_placeholders) {
        setDynamicVarPlaceholders(parsed.dynamic_variables.dynamic_variable_placeholders)
      }
      if (Array.isArray(parsed.assignments)) {
        setAssignments(parsed.assignments)
      }
      if (typeof parsed.interruption_mode === 'string') {
        setInterruptionMode(parsed.interruption_mode)
      }
      if (typeof parsed.pre_tool_speech === 'string') {
        setPreToolSpeech(parsed.pre_tool_speech)
      }
      setToolCallSound(parsed.tool_call_sound || '')
      if (typeof parsed.tool_call_sound_behavior === 'string') {
        setToolCallSoundBehavior(parsed.tool_call_sound_behavior)
      }
      if (typeof parsed.execution_mode === 'string') {
        setExecutionMode(parsed.execution_mode)
      }
      if (typeof parsed.tool_error_handling_mode === 'string') {
        setToolErrorHandlingMode(parsed.tool_error_handling_mode)
      }
      if (Array.isArray(parsed.response_mocks)) {
        setResponseMocks(parsed.response_mocks)
      }

      // Se possui api_schema (Webhook)
      if (parsed.api_schema) {
        const s = parsed.api_schema
        if (typeof s.url === 'string') setUrl(s.url)
        if (typeof s.method === 'string') setMethod(s.method.toUpperCase() as any)
        if (typeof s.content_type === 'string') setContentType(s.content_type as any)
        setAuthConnection(s.auth_connection ? String(s.auth_connection) : '')
        setResponseFilter(s.response_filter ? String(s.response_filter) : '')

        // Suporta path_params_schema tanto como Array quanto como Objeto { [name]: { type, description } } (padrão CLI da ElevenLabs)
        if (Array.isArray(s.path_params_schema)) {
          setPathParams(s.path_params_schema)
        } else if (s.path_params_schema && typeof s.path_params_schema === 'object') {
          setPathParams(
            Object.entries(s.path_params_schema).map(([k, v]: [string, any]) => ({
              name: k,
              type: v?.type || 'string',
              description: v?.description || '',
              required: true,
              value_type: v?.value_type || 'llm_prompt',
            })),
          )
        }

        if (Array.isArray(s.query_params_schema)) {
          setQueryParams(s.query_params_schema)
        } else if (s.query_params_schema && typeof s.query_params_schema === 'object') {
          const props = s.query_params_schema.properties || s.query_params_schema
          if (props && typeof props === 'object' && !Array.isArray(props)) {
            setQueryParams(
              Object.entries(props).map(([k, v]: [string, any]) => ({
                name: k,
                type: v?.type || 'string',
                description: v?.description || '',
                required: true,
                value_type: v?.value_type || 'llm_prompt',
              })),
            )
          }
        }

        if (s.request_body_schema) {
          setBodyDescription(s.request_body_schema.description || '')
          if (Array.isArray(s.request_body_schema.properties)) {
            setBodyParams(s.request_body_schema.properties)
          } else if (
            s.request_body_schema.properties &&
            typeof s.request_body_schema.properties === 'object'
          ) {
            const reqArr: string[] = Array.isArray(s.request_body_schema.required)
              ? s.request_body_schema.required
              : []
            setBodyParams(
              Object.entries(s.request_body_schema.properties).map(([k, v]: [string, any]) => ({
                name: k,
                type: v?.type || 'string',
                description: v?.description || '',
                required: reqArr.length > 0 ? reqArr.includes(k) : true,
                value_type: v?.value_type || 'llm_prompt',
              })),
            )
          }
        } else {
          setBodyParams([])
        }

        if (Array.isArray(s.request_headers)) {
          setRequestHeaders(s.request_headers)
        }
      }

      // Se for Client tool
      if (typeof parsed.expects_response === 'boolean') {
        setExpectsResponse(parsed.expects_response)
      }
      if (parsed.parameters) {
        if (Array.isArray(parsed.parameters)) {
          if (parsed.type === 'code') setCodeParams(parsed.parameters)
          else if (parsed.type === 'mcp') setMcpParams(parsed.parameters)
          else setClientParams(parsed.parameters)
        } else if (parsed.parameters.properties && typeof parsed.parameters.properties === 'object') {
          const reqArr: string[] = Array.isArray(parsed.parameters.required)
            ? parsed.parameters.required
            : []
          const list: ToolParamSchema[] = Object.entries(parsed.parameters.properties).map(
            ([k, v]: [string, any]) => ({
              name: k,
              type: v?.type || 'string',
              description: v?.description || '',
              required: reqArr.includes(k),
              value_type: 'llm_prompt',
            }),
          )
          if (parsed.type === 'code') setCodeParams(list)
          else setClientParams(list)
        }
      }

      // Se for MCP tool
      if (parsed.mcp_config) {
        if (typeof parsed.mcp_config.url === 'string') setUrl(parsed.mcp_config.url)
        if (parsed.mcp_config.transport) setMcpTransport(parsed.mcp_config.transport)
        if (parsed.mcp_config.approval_policy)
          setMcpApprovalPolicy(parsed.mcp_config.approval_policy)
        if (Array.isArray(parsed.mcp_config.request_headers)) {
          setRequestHeaders(parsed.mcp_config.request_headers)
        }
      }

      // Se for Code tool
      if (typeof parsed.code === 'string') {
        setCodeSource(parsed.code)
      }
      if (parsed.context_object?.config) {
        setCodeContextConfig(parsed.context_object.config)
      }

      return true
    } catch (err: any) {
      setJsonError(`JSON inválido: ${err.message}`)
      return false
    }
  }

  const handleToggleJsonMode = () => {
    if (!isJsonMode) {
      const obj = buildElevenLabsJsonObject()
      setJsonText(JSON.stringify(obj, null, 2))
      setJsonError('')
      setIsJsonMode(true)
    } else {
      if (applyJsonToVisualState(jsonText)) {
        setIsJsonMode(false)
      }
    }
  }

  // Executa a Code Tool localmente no navegador (aba "Testar (Run)")
  const handleRunCodeInBrowser = async () => {
    setCodeRunning(true)
    setCodeRunOutput(null)
    setCodeRunLogs([])
    const logs: string[] = []
    const startMs = performance.now()
    try {
      const parsedArgs = codeTestArgs.trim() ? JSON.parse(codeTestArgs) : {}
      const ctx = {
        args: parsedArgs,
        config: codeContextConfig,
        secrets: {},
        auth_connections: {},
      }
      // Transforma "export default async (ctx) =>" em uma função executável
      const executableBody = codeSource.replace(/export\s+default\s+/, 'return ')
      const customConsole = {
        log: (...a: any[]) =>
          logs.push(a.map((x) => (typeof x === 'object' ? JSON.stringify(x) : String(x))).join(' ')),
        warn: (...a: any[]) =>
          logs.push(
            '[WARN] ' +
              a.map((x) => (typeof x === 'object' ? JSON.stringify(x) : String(x))).join(' '),
          ),
        error: (...a: any[]) =>
          logs.push(
            '[ERROR] ' +
              a.map((x) => (typeof x === 'object' ? JSON.stringify(x) : String(x))).join(' '),
          ),
      }
      const factory = new Function('console', executableBody)
      const fn = factory(customConsole)
      const res = await fn(ctx)
      const elapsed = Math.round(performance.now() - startMs)
      logs.push(`✓ Executado em ${elapsed} ms`)
      setCodeRunLogs(logs)
      setCodeRunOutput({ ok: true, data: res })
    } catch (err: any) {
      setCodeRunLogs(logs)
      setCodeRunOutput({ ok: false, error: err.message || String(err) })
    } finally {
      setCodeRunning(false)
    }
  }

  const handleSave = async () => {
    setErrorMsg('')
    let currentObj = buildElevenLabsJsonObject()

    if (isJsonMode) {
      try {
        const parsed = JSON.parse(jsonText)
        if (!applyJsonToVisualState(jsonText)) return
        currentObj = parsed
      } catch (e: any) {
        setJsonError(`JSON inválido: ${e.message}`)
        return
      }
    }

    const finalName = (currentObj.name || name).trim()
    if (!finalName) {
      setErrorMsg('Informe o nome da ferramenta.')
      return
    }
    if ((kind === 'webhook' || kind === 'mcp') && !url.trim() && !(currentObj as any).api_schema?.url) {
      setErrorMsg('Informe a URL da ferramenta.')
      return
    }

    setSaving(true)
    try {
      // Monta o input_schema consolidado para o LLM (Gemini Live / OpenAI Realtime)
      const llmInputSchema: { name: string; type: string; description: string; required: boolean }[] =
        []
      const seenNames = new Set<string>()
      const pushParam = (p: ToolParamSchema) => {
        const n = p.name.trim()
        if (!n || seenNames.has(n)) return
        if (p.value_type && p.value_type !== 'llm_prompt') return
        seenNames.add(n)
        llmInputSchema.push({
          name: n,
          type: p.type || 'string',
          description: p.description || `Parâmetro ${n}`,
          required: p.required ?? true,
        })
      }

      if (kind === 'webhook') {
        pathParams.forEach((p) => pushParam({ ...p, required: true }))
        queryParams.forEach(pushParam)
        if (method === 'POST' || method === 'PUT' || method === 'PATCH') {
          bodyParams.forEach(pushParam)
        }
      } else if (kind === 'client') {
        clientParams.forEach(pushParam)
      } else if (kind === 'mcp') {
        mcpParams.forEach(pushParam)
      } else if (kind === 'code') {
        codeParams.forEach(pushParam)
      }

      // Coleta cabeçalhos secretos para secret_headers do backend
      const secHeaders: Record<string, string> = {}
      for (const h of requestHeaders) {
        if (h.name.trim() && h.value.trim()) {
          secHeaders[h.name.trim()] = h.value.trim()
        }
      }
      if (authConnection === 'bearer' && authTokenValue.trim()) {
        secHeaders['Authorization'] = authTokenValue.trim().startsWith('Bearer ')
          ? authTokenValue.trim()
          : `Bearer ${authTokenValue.trim()}`
      } else if (authConnection === 'api_key' && authTokenValue.trim()) {
        secHeaders['X-API-Key'] = authTokenValue.trim()
      }
      if (kind === 'mcp' && mcpSecretToken.trim()) {
        secHeaders['Authorization'] = mcpSecretToken.trim().startsWith('Bearer ')
          ? mcpSecretToken.trim()
          : `Bearer ${mcpSecretToken.trim()}`
      }

      const domains = followRedirectsDomains
        .split(',')
        .map((d) => d.trim())
        .filter(Boolean)

      const payload = {
        name: finalName,
        description: description.trim(),
        kind,
        type: kind,
        input_schema: llmInputSchema,
        timeout_ms: Math.max(1000, Math.min(60000, responseTimeoutSecs * 1000)),
        active: true,
        secret_headers: Object.keys(secHeaders).length > 0 ? secHeaders : undefined,
        config: {
          url: url.trim(),
          method: kind === 'webhook' ? method : 'POST',
          api_schema: {
            url: url.trim(),
            method: kind === 'webhook' ? method : 'POST',
            path_params_schema: pathParams,
            query_params_schema: queryParams,
            request_body_schema:
              (method === 'POST' || method === 'PUT' || method === 'PATCH') && bodyParams.length > 0
                ? {
                    type: 'object' as const,
                    description: bodyDescription || 'Corpo da requisição',
                    properties: bodyParams,
                  }
                : null,
            request_headers: requestHeaders,
            content_type: contentType,
            auth_connection: authConnection || null,
            mtls_auth_connection: null,
            response_filter: responseFilter || null,
          },
          response_timeout_secs: responseTimeoutSecs,
          follow_redirects: followRedirects,
          follow_redirects_allowed_domains: domains,
          dynamic_variables: {
            dynamic_variable_placeholders: dynamicVarPlaceholders,
          },
          assignments,
          interruption_mode: interruptionMode,
          pre_tool_speech: preToolSpeech,
          tool_call_sound: toolCallSound ? toolCallSound : null,
          tool_call_sound_behavior: toolCallSoundBehavior,
          execution_mode: executionMode,
          tool_error_handling_mode: toolErrorHandlingMode,
          response_mocks: responseMocks,
          expects_response: expectsResponse,
          code: codeSource,
          context_object: {
            config: codeContextConfig,
            secrets: [],
            auth_connections: [],
          },
          transport: mcpTransport,
          approval_policy: mcpApprovalPolicy,
        },
      }

      let saved: ToolWithStats
      if (editingTool) {
        saved = await api<ToolWithStats>(`/api/tools/${editingTool.id}`, {
          method: 'PUT',
          body: JSON.stringify(payload),
        })
      } else {
        saved = await api<ToolWithStats>('/api/tools', {
          method: 'POST',
          body: JSON.stringify(payload),
        })
      }
      onSaved(saved)
      onClose()
    } catch (err: any) {
      setErrorMsg(err.errors ? err.errors.join(' • ') : err.message || 'Erro ao salvar ferramenta')
    } finally {
      setSaving(false)
    }
  }

  const getHeaderMeta = () => {
    switch (kind) {
      case 'client':
        return {
          icon: Wrench,
          title: editingTool ? 'Editar ferramenta cliente' : 'Adicionar ferramenta cliente',
          docUrl: 'https://elevenlabs.io/docs/eleven-agents/customization/tools/client-tools',
        }
      case 'mcp':
        return {
          icon: Blocks,
          title: editingTool
            ? 'Editar ferramenta de integração'
            : 'Adicionar ferramenta de integração',
          docUrl: 'https://elevenlabs.io/docs/eleven-agents/customization/tools/mcp',
        }
      case 'code':
        return {
          icon: Code2,
          title: editingTool ? 'Editar ferramenta código' : 'Adicionar ferramenta código',
          docUrl: 'https://elevenlabs.io/docs/eleven-agents/customization/tools/code-tools',
        }
      default:
        return {
          icon: Webhook,
          title: editingTool ? 'Editar ferramenta webhook' : 'Adicionar ferramenta webhook',
          docUrl: 'https://elevenlabs.io/docs/eleven-agents/customization/tools/webhook-tools',
        }
    }
  }

  const headerMeta = getHeaderMeta()
  const HeaderIcon = headerMeta.icon

  // Helper para renderizar uma lista de parâmetros (Query, Body, Client, Code, MCP)
  const renderParamsEditor = (
    list: ToolParamSchema[],
    setList: (next: ToolParamSchema[]) => void,
    allowValueType = true,
  ) => {
    if (list.length === 0) return null
    return (
      <div className="space-y-2.5 pt-2">
        {list.map((p, idx) => (
          <div
            key={idx}
            className="bg-white border border-zinc-200/90 rounded-xl p-3.5 space-y-3 shadow-2xs"
          >
            <div className="grid grid-cols-1 sm:grid-cols-12 gap-2.5 items-center">
              <div className="sm:col-span-4">
                <label className="block text-[11px] font-medium text-zinc-500 mb-1">
                  Identificador
                </label>
                <input
                  type="text"
                  value={p.name}
                  placeholder="ex: latitude"
                  onChange={(e) => {
                    const next = [...list]
                    next[idx] = { ...p, name: e.target.value.replace(/\s+/g, '_') }
                    setList(next)
                  }}
                  className="w-full px-2.5 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs font-mono text-zinc-900 focus:outline-none focus:border-zinc-400"
                />
              </div>

              <div className="sm:col-span-3">
                <label className="block text-[11px] font-medium text-zinc-500 mb-1">
                  Tipo de dados
                </label>
                <select
                  value={p.type || 'string'}
                  onChange={(e) => {
                    const next = [...list]
                    next[idx] = { ...p, type: e.target.value }
                    setList(next)
                  }}
                  className="w-full px-2.5 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs text-zinc-900 focus:outline-none focus:border-zinc-400"
                >
                  <option value="string">string</option>
                  <option value="number">number</option>
                  <option value="integer">integer</option>
                  <option value="boolean">boolean</option>
                  <option value="array">array</option>
                  <option value="object">object</option>
                </select>
              </div>

              {allowValueType && (
                <div className="sm:col-span-3">
                  <label className="block text-[11px] font-medium text-zinc-500 mb-1">
                    Tipo de valor
                  </label>
                  <select
                    value={p.value_type || 'llm_prompt'}
                    onChange={(e) => {
                      const next = [...list]
                      next[idx] = { ...p, value_type: e.target.value as any }
                      setList(next)
                    }}
                    className="w-full px-2.5 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs text-zinc-900 focus:outline-none focus:border-zinc-400"
                  >
                    <option value="llm_prompt">Prompt do LLM</option>
                    <option value="constant">Constante</option>
                    <option value="dynamic_variable">Variável dinâmica</option>
                  </select>
                </div>
              )}

              <div
                className={`${
                  allowValueType ? 'sm:col-span-2' : 'sm:col-span-5'
                } flex items-center justify-end gap-2 pt-4`}
              >
                <label className="inline-flex items-center gap-1 text-[11px] text-zinc-600 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={p.required ?? true}
                    onChange={(e) => {
                      const next = [...list]
                      next[idx] = { ...p, required: e.target.checked }
                      setList(next)
                    }}
                    className="rounded border-zinc-300 text-zinc-900"
                  />
                  Obrig.
                </label>
                <button
                  type="button"
                  onClick={() => setList(list.filter((_, i) => i !== idx))}
                  className="p-1 text-zinc-400 hover:text-red-600 rounded-lg"
                  title="Remover parâmetro"
                >
                  <Trash2 className="w-3.5 h-3.5" />
                </button>
              </div>
            </div>

            {(!p.value_type || p.value_type === 'llm_prompt') && (
              <div>
                <label className="block text-[11px] font-medium text-zinc-500 mb-1">
                  Descrição (para o LLM saber o que coletar)
                </label>
                <input
                  type="text"
                  value={p.description || ''}
                  placeholder="Descreva o formato esperado (ex: DDD de 2 dígitos do cliente)"
                  onChange={(e) => {
                    const next = [...list]
                    next[idx] = { ...p, description: e.target.value }
                    setList(next)
                  }}
                  className="w-full px-2.5 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs text-zinc-900 focus:outline-none focus:border-zinc-400"
                />
              </div>
            )}

            {p.value_type === 'constant' && (
              <div>
                <label className="block text-[11px] font-medium text-zinc-500 mb-1">
                  Valor constante
                </label>
                <input
                  type="text"
                  value={p.constant_value ?? ''}
                  placeholder="Valor fixo enviado na requisição"
                  onChange={(e) => {
                    const next = [...list]
                    next[idx] = { ...p, constant_value: e.target.value }
                    setList(next)
                  }}
                  className="w-full px-2.5 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs font-mono text-zinc-900 focus:outline-none focus:border-zinc-400"
                />
              </div>
            )}

            {p.value_type === 'dynamic_variable' && (
              <div>
                <label className="block text-[11px] font-medium text-zinc-500 mb-1">
                  Nome da variável dinâmica
                </label>
                <input
                  type="text"
                  value={p.dynamic_variable ?? ''}
                  placeholder="ex: telefone ou CPF_cliente_3_pri"
                  onChange={(e) => {
                    const next = [...list]
                    next[idx] = { ...p, dynamic_variable: e.target.value }
                    setList(next)
                  }}
                  className="w-full px-2.5 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs font-mono text-zinc-900 focus:outline-none focus:border-zinc-400"
                />
              </div>
            )}
          </div>
        ))}
      </div>
    )
  }

  return (
    <Sheet open={open} onClose={onClose} side="right" sizeClass="max-w-2xl">
      <div className="h-full flex flex-col overflow-hidden bg-white text-zinc-900 font-sans">
        {/* =====================================================================
            CABEÇALHO DA GAVETA (Idêntico à Imagem 1: [Ícone] Título 📖 ... ×)
           ===================================================================== */}
        <div className="px-6 py-4 flex items-center justify-between border-b border-zinc-100 flex-shrink-0">
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 rounded-lg border border-zinc-200/90 bg-white flex items-center justify-center shadow-2xs text-zinc-800">
              <HeaderIcon className="w-4 h-4" />
            </div>
            <div className="flex items-center gap-2">
              <h2 className="text-[15px] font-semibold text-zinc-950 tracking-tight">
                {headerMeta.title}
              </h2>
              <a
                href={headerMeta.docUrl}
                target="_blank"
                rel="noreferrer"
                title="Abrir documentação oficial da ElevenLabs"
                className="text-zinc-400 hover:text-zinc-700 transition-colors"
              >
                <BookOpen className="w-4 h-4" />
              </a>
            </div>
          </div>

          <button
            type="button"
            onClick={onClose}
            className="p-1.5 rounded-lg text-zinc-400 hover:text-zinc-800 hover:bg-zinc-100 transition-colors cursor-pointer"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* =====================================================================
            CORPO ROLÁVEL (Formulário Visual em Cards Cinzas OU Editor JSON)
           ===================================================================== */}
        <div className="flex-1 overflow-y-auto overscroll-contain px-6 py-5 space-y-4">
          {errorMsg && (
            <div className="p-3 rounded-xl bg-red-50 border border-red-200 text-xs text-red-700 flex items-center justify-between">
              <span>{errorMsg}</span>
              <button onClick={() => setErrorMsg('')} className="text-red-400 hover:text-red-700">
                <X className="w-3.5 h-3.5" />
              </button>
            </div>
          )}

          {isJsonMode ? (
            /* MODO EDITOR JSON (Estrutura exata ElevenLabs) */
            <div className="space-y-3 h-full flex flex-col">
              <div className="flex items-center justify-between">
                <div>
                  <h3 className="text-xs font-semibold text-zinc-900">
                    Configuração JSON da Ferramenta ({kind})
                  </h3>
                  <p className="text-[11px] text-zinc-500">
                    Edite ou cole diretamente o JSON no padrão oficial da ElevenLabs.
                  </p>
                </div>
                <button
                  type="button"
                  onClick={() => {
                    navigator.clipboard.writeText(jsonText)
                  }}
                  className="inline-flex items-center gap-1 px-2.5 py-1 rounded-lg border border-zinc-200 text-[11px] font-medium text-zinc-700 hover:bg-zinc-50"
                >
                  <Copy className="w-3 h-3" />
                  Copiar JSON
                </button>
              </div>

              {jsonError && (
                <div className="p-2.5 rounded-lg bg-red-50 border border-red-200 text-xs text-red-700">
                  {jsonError}
                </div>
              )}

              <textarea
                value={jsonText}
                onChange={(e) => {
                  setJsonText(e.target.value)
                  setJsonError('')
                }}
                rows={26}
                spellCheck={false}
                className="w-full flex-1 p-4 rounded-2xl bg-zinc-950 text-zinc-100 font-mono text-xs leading-relaxed focus:outline-none border border-zinc-800"
              />
            </div>
          ) : (
            /* MODO FORMULÁRIO VISUAL POR CATEGORIA DE FERRAMENTA */
            <>
              {/* ===============================================================
                  1. CARD PRINCIPAL: CONFIGURAÇÃO (Imagem 1)
                 =============================================================== */}
              <div className="bg-[#f7f7f8] border border-zinc-200/70 rounded-2xl p-4 space-y-3">
                <div>
                  <div className="flex items-center gap-1.5">
                    <h3 className="text-[13px] font-semibold text-zinc-900">Configuração</h3>
                    <Video className="w-3.5 h-3.5 text-zinc-400" />
                  </div>
                  <p className="text-xs text-zinc-500 mt-0.5">
                    {kind === 'webhook' && 'Descreva ao LLM como e quando usar a ferramenta.'}
                    {kind === 'client' &&
                      'Configure a função cliente que o agente acionará diretamente no navegador ou aplicativo.'}
                    {kind === 'mcp' &&
                      'Conecte um servidor Model Context Protocol (MCP) externo via SSE ou HTTP Streamable.'}
                    {kind === 'code' &&
                      'Execute código JavaScript customizado em ambiente isolado sem precisar hospedar um webhook.'}
                  </p>
                </div>

                {/* Caixa branca interna (Idêntica à Imagem 1) */}
                <div className="bg-white border border-zinc-200/80 rounded-xl p-4 space-y-4 shadow-2xs">
                  {/* Nome */}
                  <div>
                    <label className="block text-xs font-semibold text-zinc-900 mb-1.5">Nome</label>
                    <input
                      type="text"
                      value={name}
                      onChange={(e) =>
                        setName(
                          kind === 'client'
                            ? e.target.value.replace(/\s+/g, '')
                            : e.target.value.replace(/\s+/g, '_'),
                        )
                      }
                      placeholder={
                        kind === 'client'
                          ? 'ex: logMessage ou mostrarModalOferta'
                          : kind === 'mcp'
                            ? 'ex: zapier_mcp_server'
                            : kind === 'code'
                              ? 'ex: convert_temperature'
                              : 'ex: get_weather ou consultar_planos_tim'
                      }
                      className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-xs text-zinc-900 focus:outline-none focus:border-zinc-400 transition-colors"
                    />
                  </div>

                  {/* Descrição */}
                  <div>
                    <label className="block text-xs font-semibold text-zinc-900 mb-1.5">
                      Descrição
                    </label>
                    <textarea
                      rows={3}
                      value={description}
                      onChange={(e) => setDescription(e.target.value)}
                      placeholder="Descreva detalhadamente quando o agente deve chamar esta ferramenta e como tratar o retorno..."
                      className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-xs text-zinc-900 focus:outline-none focus:border-zinc-400 transition-colors resize-y"
                    />
                  </div>

                  {/* Campos específicos de WEBHOOK (Método, URL, Autenticação) */}
                  {kind === 'webhook' && (
                    <>
                      <div>
                        <label className="block text-xs font-semibold text-zinc-900 mb-1.5">
                          Método
                        </label>
                        <select
                          value={method}
                          onChange={(e) => setMethod(e.target.value as any)}
                          className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-xs text-zinc-900 focus:outline-none focus:border-zinc-400 transition-colors"
                        >
                          <option value="GET">GET</option>
                          <option value="POST">POST</option>
                          <option value="PUT">PUT</option>
                          <option value="PATCH">PATCH</option>
                          <option value="DELETE">DELETE</option>
                        </select>
                      </div>

                      <div>
                        <label className="block text-xs font-semibold text-zinc-900 mb-1.5">
                          URL
                        </label>
                        <input
                          type="text"
                          value={url}
                          onChange={(e) => handleUrlChange(e.target.value)}
                          placeholder="https://api.example.com/v1"
                          className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-xs text-zinc-900 placeholder-zinc-400 focus:outline-none focus:border-zinc-400 transition-colors"
                        />
                        <div className="mt-1.5 flex items-center gap-1 text-[11px] text-zinc-500 flex-wrap">
                          <span>Digite</span>
                          <kbd className="px-1 py-0.5 bg-zinc-100 border border-zinc-200 rounded text-[10px] font-mono text-zinc-700">
                            {'{{'}
                          </kbd>
                          <span>para usar uma</span>
                          <span className="underline decoration-zinc-400">
                            variável de ambiente
                          </span>
                          <span>·</span>
                          <a
                            href="https://elevenlabs.io/docs/eleven-agents/customization/tools/webhook-tools"
                            target="_blank"
                            rel="noreferrer"
                            className="inline-flex items-center gap-0.5 hover:text-zinc-800"
                          >
                            Documentos <ExternalLink className="w-2.5 h-2.5" />
                          </a>
                        </div>
                      </div>

                      <div>
                        <label className="block text-xs font-semibold text-zinc-900 mb-1.5">
                          Autenticação
                        </label>
                        <select
                          value={authConnection}
                          onChange={(e) => setAuthConnection(e.target.value)}
                          className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-xs text-zinc-600 focus:outline-none focus:border-zinc-400 transition-colors"
                        >
                          <option value="">
                            O espaço de trabalho não tem conexões de autenticação
                          </option>
                          <option value="bearer">Bearer Token (Authorization: Bearer ...)</option>
                          <option value="api_key">API Key (Header X-API-Key)</option>
                          <option value="basic">Basic Auth (Authorization: Basic ...)</option>
                        </select>
                        {authConnection !== '' && (
                          <input
                            type="password"
                            value={authTokenValue}
                            onChange={(e) => setAuthTokenValue(e.target.value)}
                            placeholder={
                              authConnection === 'bearer'
                                ? 'Cole o Bearer Token...'
                                : 'Cole a credencial / chave secreta...'
                            }
                            className="mt-2 w-full px-3 py-2 bg-zinc-50 border border-zinc-200 rounded-lg text-xs font-mono text-zinc-900 focus:outline-none focus:border-zinc-400"
                          />
                        )}
                        <div className="mt-1.5 flex items-center gap-1.5 text-[11px] text-zinc-500">
                          <button
                            type="button"
                            onClick={() => setAuthConnection('bearer')}
                            className="hover:text-zinc-800"
                          >
                            Criar nova conexão de autenticação
                          </button>
                          <span>·</span>
                          <span>Variáveis de ambiente</span>
                        </div>
                      </div>
                    </>
                  )}

                  {/* Campos específicos de CLIENT TOOL (Wait for response) */}
                  {kind === 'client' && (
                    <div className="pt-1">
                      <label className="flex items-start gap-2.5 cursor-pointer select-none">
                        <input
                          type="checkbox"
                          checked={expectsResponse}
                          onChange={(e) => setExpectsResponse(e.target.checked)}
                          className="mt-0.5 rounded border-zinc-300 text-zinc-900"
                        />
                        <div>
                          <span className="text-xs font-semibold text-zinc-900 block">
                            Aguardar resposta (Wait for response)
                          </span>
                          <span className="text-[11px] text-zinc-500">
                            Quando ativado, o agente aguardará o retorno da função executada no
                            cliente e adicionará a resposta ao contexto da conversa.
                          </span>
                        </div>
                      </label>
                    </div>
                  )}

                  {/* Campos específicos de MCP TOOL (Transporte, Server URL, Secret Token) */}
                  {kind === 'mcp' && (
                    <>
                      <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                        <div>
                          <label className="block text-xs font-semibold text-zinc-900 mb-1.5">
                            Transporte
                          </label>
                          <select
                            value={mcpTransport}
                            onChange={(e) => setMcpTransport(e.target.value as any)}
                            className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-xs text-zinc-900"
                          >
                            <option value="SSE">SSE (Server-Sent Events)</option>
                            <option value="STREAMABLE_HTTP">HTTP Streamable (JSON-RPC)</option>
                          </select>
                        </div>

                        <div className="sm:col-span-2">
                          <label className="block text-xs font-semibold text-zinc-900 mb-1.5">
                            URL do Servidor MCP
                          </label>
                          <input
                            type="text"
                            value={url}
                            onChange={(e) => handleUrlChange(e.target.value)}
                            placeholder="https://mcp.zapier.com/api/mcp/..."
                            className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-xs font-mono text-zinc-900"
                          />
                        </div>
                      </div>

                      <div>
                        <label className="block text-xs font-semibold text-zinc-900 mb-1.5">
                          Token Secreto (Opcional — Header Authorization)
                        </label>
                        <input
                          type="password"
                          value={mcpSecretToken}
                          onChange={(e) => setMcpSecretToken(e.target.value)}
                          placeholder="Bearer ..."
                          className="w-full px-3 py-2 bg-white border border-zinc-200 rounded-lg text-xs font-mono text-zinc-900"
                        />
                      </div>
                    </>
                  )}

                  {/* Seção Colapsável "> Avançado" (Idêntica à Imagem 1) */}
                  <div className="pt-1 border-t border-zinc-100">
                    <button
                      type="button"
                      onClick={() => setAdvancedOpen(!advancedOpen)}
                      className="flex items-center gap-1.5 text-xs font-medium text-zinc-500 hover:text-zinc-900 transition-colors cursor-pointer py-1"
                    >
                      {advancedOpen ? (
                        <ChevronDown className="w-3.5 h-3.5" />
                      ) : (
                        <ChevronRight className="w-3.5 h-3.5" />
                      )}
                      <span>Avançado</span>
                    </button>

                    {!advancedOpen && kind === 'webhook' && (
                      <div className="mt-2 text-xs text-zinc-300 select-none">
                        Filtragem de resposta
                      </div>
                    )}

                    {advancedOpen && (
                      <div className="mt-3 space-y-3.5 pt-2 border-t border-zinc-100 text-xs">
                        {kind === 'webhook' && (
                          <>
                            <div>
                              <label className="block font-semibold text-zinc-800 mb-1">
                                Filtragem de resposta (response_filter)
                              </label>
                              <input
                                type="text"
                                value={responseFilter}
                                onChange={(e) => setResponseFilter(e.target.value)}
                                placeholder="Opcional: caminho JSON (ex: data.planos)"
                                className="w-full px-3 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs font-mono"
                              />
                            </div>

                            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                              <div>
                                <label className="block font-semibold text-zinc-800 mb-1">
                                  Tipo de conteúdo (Content-Type)
                                </label>
                                <select
                                  value={contentType}
                                  onChange={(e) => setContentType(e.target.value as any)}
                                  className="w-full px-3 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs"
                                >
                                  <option value="application/json">
                                    application/json (Padrão)
                                  </option>
                                  <option value="application/x-www-form-urlencoded">
                                    application/x-www-form-urlencoded
                                  </option>
                                </select>
                              </div>

                              <div>
                                <label className="block font-semibold text-zinc-800 mb-1">
                                  Timeout de resposta (segundos)
                                </label>
                                <input
                                  type="number"
                                  min={1}
                                  max={60}
                                  value={responseTimeoutSecs}
                                  onChange={(e) =>
                                    setResponseTimeoutSecs(parseInt(e.target.value) || 20)
                                  }
                                  className="w-full px-3 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs font-mono"
                                />
                              </div>
                            </div>

                            <div className="space-y-2">
                              <label className="inline-flex items-center gap-2 cursor-pointer">
                                <input
                                  type="checkbox"
                                  checked={followRedirects}
                                  onChange={(e) => setFollowRedirects(e.target.checked)}
                                  className="rounded border-zinc-300 text-zinc-900"
                                />
                                <span className="font-medium text-zinc-800">
                                  Seguir redirecionamentos HTTP (follow_redirects)
                                </span>
                              </label>
                              {followRedirects && (
                                <input
                                  type="text"
                                  value={followRedirectsDomains}
                                  onChange={(e) => setFollowRedirectsDomains(e.target.value)}
                                  placeholder="Domínios permitidos separados por vírgula (ex: api.example.com)"
                                  className="w-full px-3 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs font-mono"
                                />
                              )}
                            </div>
                          </>
                        )}

                        {kind !== 'webhook' && (
                          <div>
                            <label className="block font-semibold text-zinc-800 mb-1">
                              Timeout de resposta (segundos)
                            </label>
                            <input
                              type="number"
                              min={1}
                              max={60}
                              value={responseTimeoutSecs}
                              onChange={(e) =>
                                setResponseTimeoutSecs(parseInt(e.target.value) || 20)
                              }
                              className="w-full px-3 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs font-mono"
                            />
                          </div>
                        )}

                        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                          <div>
                            <label className="block font-semibold text-zinc-800 mb-1">
                              Modo de interrupção (interruption_mode)
                            </label>
                            <select
                              value={interruptionMode}
                              onChange={(e) => setInterruptionMode(e.target.value)}
                              className="w-full px-3 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs"
                            >
                              <option value="allow">allow (Permitir interrupções)</option>
                              <option value="block">block (Bloquear durante execução)</option>
                            </select>
                          </div>

                          <div>
                            <label className="block font-semibold text-zinc-800 mb-1">
                              Fala pré-ferramenta (pre_tool_speech)
                            </label>
                            <select
                              value={preToolSpeech}
                              onChange={(e) => setPreToolSpeech(e.target.value)}
                              className="w-full px-3 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs"
                            >
                              <option value="auto">auto (Automático)</option>
                              <option value="force">force (Forçar fala antes)</option>
                              <option value="none">none (Sem fala prévia)</option>
                            </select>
                          </div>
                        </div>

                        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                          <div>
                            <label className="block font-semibold text-zinc-800 mb-1">
                              Som de chamada (tool_call_sound)
                            </label>
                            <select
                              value={toolCallSound}
                              onChange={(e) => setToolCallSound(e.target.value)}
                              className="w-full px-3 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs"
                            >
                              <option value="">Nenhum (null)</option>
                              <option value="typing">Digitação (typing)</option>
                              <option value="processing">Processamento (processing)</option>
                            </select>
                          </div>

                          <div>
                            <label className="block font-semibold text-zinc-800 mb-1">
                              Modo de execução (execution_mode)
                            </label>
                            <select
                              value={executionMode}
                              onChange={(e) => setExecutionMode(e.target.value)}
                              className="w-full px-3 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs"
                            >
                              <option value="immediate">immediate (Imediato)</option>
                              <option value="post_tool_speech">post_tool_speech</option>
                              <option value="async">async (Assíncrono)</option>
                            </select>
                          </div>
                        </div>

                        <div>
                          <label className="block font-semibold text-zinc-800 mb-1">
                            Tratamento de erros (tool_error_handling_mode)
                          </label>
                          <select
                            value={toolErrorHandlingMode}
                            onChange={(e) => setToolErrorHandlingMode(e.target.value)}
                            className="w-full px-3 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs"
                          >
                            <option value="auto">auto (Automático)</option>
                            <option value="summarized">summarized (Resumido)</option>
                            <option value="passthrough">passthrough (Passar erro bruto)</option>
                            <option value="hide">hide (Ocultar erro)</option>
                          </select>
                        </div>
                      </div>
                    )}
                  </div>
                </div>
              </div>

              {/* ===============================================================
                  SEÇÕES ESPECÍFICAS DE WEBHOOK (Imagem 2: Cabeçalhos, Caminho, Consulta, Corpo)
                 =============================================================== */}
              {kind === 'webhook' && (
                <>
                  {/* CARD 2: CABEÇALHOS (Imagem 2) */}
                  <div className="bg-[#f7f7f8] border border-zinc-200/70 rounded-2xl p-4 space-y-3">
                    <div className="flex items-center justify-between gap-4">
                      <div>
                        <h3 className="text-[13px] font-semibold text-zinc-900">Cabeçalhos</h3>
                        <p className="text-xs text-zinc-500 mt-0.5">
                          Defina cabeçalhos que serão enviados com a solicitação
                        </p>
                      </div>
                      <button
                        type="button"
                        onClick={() =>
                          setRequestHeaders([
                            ...requestHeaders,
                            { type: 'value', name: '', value: '' },
                          ])
                        }
                        className="px-3 py-1.5 rounded-lg bg-white hover:bg-zinc-50 border border-zinc-200/90 text-xs font-medium text-zinc-900 shadow-2xs transition-colors flex-shrink-0 cursor-pointer"
                      >
                        Adicionar cabeçalho
                      </button>
                    </div>

                    {requestHeaders.length > 0 && (
                      <div className="space-y-2 pt-1">
                        {requestHeaders.map((h, idx) => (
                          <div
                            key={idx}
                            className="bg-white border border-zinc-200/90 rounded-xl p-3 grid grid-cols-1 sm:grid-cols-12 gap-2 items-center shadow-2xs"
                          >
                            <select
                              value={h.type || 'value'}
                              onChange={(e) => {
                                const next = [...requestHeaders]
                                next[idx] = { ...h, type: e.target.value as any }
                                setRequestHeaders(next)
                              }}
                              className="sm:col-span-3 px-2.5 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs text-zinc-800"
                            >
                              <option value="value">Valor</option>
                              <option value="secret">Segredo</option>
                              <option value="dynamic_variable">Variável dinâmica</option>
                            </select>

                            <input
                              type="text"
                              value={h.name}
                              placeholder="Nome (ex: Authorization)"
                              onChange={(e) => {
                                const next = [...requestHeaders]
                                next[idx] = { ...h, name: e.target.value }
                                setRequestHeaders(next)
                              }}
                              className="sm:col-span-4 px-2.5 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs font-mono text-zinc-900"
                            />

                            <input
                              type={h.type === 'secret' ? 'password' : 'text'}
                              value={h.value}
                              placeholder={
                                h.type === 'dynamic_variable'
                                  ? 'Nome da variável'
                                  : 'Valor do cabeçalho'
                              }
                              onChange={(e) => {
                                const next = [...requestHeaders]
                                next[idx] = { ...h, value: e.target.value }
                                setRequestHeaders(next)
                              }}
                              className="sm:col-span-4 px-2.5 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs font-mono text-zinc-900"
                            />

                            <div className="sm:col-span-1 flex justify-end">
                              <button
                                type="button"
                                onClick={() =>
                                  setRequestHeaders(requestHeaders.filter((_, i) => i !== idx))
                                }
                                className="p-1 text-zinc-400 hover:text-red-600 rounded-lg"
                              >
                                <Trash2 className="w-3.5 h-3.5" />
                              </button>
                            </div>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>

                  {/* CARD 3: PARÂMETROS DE CAMINHO (Imagem 2) */}
                  <div className="bg-[#f7f7f8] border border-zinc-200/70 rounded-2xl p-4 space-y-3">
                    <div>
                      <h3 className="text-[13px] font-semibold text-zinc-900">
                        Parâmetros de caminho
                      </h3>
                      <p className="text-xs text-zinc-500 mt-0.5">
                        Adicione o caminho entre chaves à URL para configurá-los aqui.
                      </p>
                    </div>

                    {renderParamsEditor(pathParams, setPathParams, true)}
                  </div>

                  {/* CARD 4: PARÂMETROS DE CONSULTA (Imagem 2) */}
                  <div className="bg-[#f7f7f8] border border-zinc-200/70 rounded-2xl p-4 space-y-3">
                    <div className="flex items-center justify-between gap-4">
                      <div>
                        <h3 className="text-[13px] font-semibold text-zinc-900">
                          Parâmetros de consulta
                        </h3>
                        <p className="text-xs text-zinc-500 mt-0.5">
                          Defina parâmetros que serão coletados pelo LLM e enviados como consulta da
                          solicitação.
                        </p>
                      </div>
                      <button
                        type="button"
                        onClick={() =>
                          setQueryParams([
                            ...queryParams,
                            {
                              name: '',
                              type: 'string',
                              description: '',
                              required: true,
                              value_type: 'llm_prompt',
                            },
                          ])
                        }
                        className="px-3 py-1.5 rounded-lg bg-white hover:bg-zinc-50 border border-zinc-200/90 text-xs font-medium text-zinc-900 shadow-2xs transition-colors flex-shrink-0 cursor-pointer"
                      >
                        Adicionar parâmetro
                      </button>
                    </div>

                    {renderParamsEditor(queryParams, setQueryParams, true)}
                  </div>

                  {/* CARD 4B: PARÂMETROS DO CORPO (Exibido quando Método é POST, PUT ou PATCH) */}
                  {(method === 'POST' || method === 'PUT' || method === 'PATCH') && (
                    <div className="bg-[#f7f7f8] border border-zinc-200/70 rounded-2xl p-4 space-y-3">
                      <div className="flex items-center justify-between gap-4">
                        <div>
                          <h3 className="text-[13px] font-semibold text-zinc-900">
                            Parâmetros do corpo
                          </h3>
                          <p className="text-xs text-zinc-500 mt-0.5">
                            Defina propriedades que serão coletadas pelo LLM e enviadas no corpo (
                            {contentType}) da solicitação.
                          </p>
                        </div>
                        <button
                          type="button"
                          onClick={() =>
                            setBodyParams([
                              ...bodyParams,
                              {
                                name: '',
                                type: 'string',
                                description: '',
                                required: true,
                                value_type: 'llm_prompt',
                              },
                            ])
                          }
                          className="px-3 py-1.5 rounded-lg bg-white hover:bg-zinc-50 border border-zinc-200/90 text-xs font-medium text-zinc-900 shadow-2xs transition-colors flex-shrink-0 cursor-pointer"
                        >
                          Adicionar parâmetro
                        </button>
                      </div>

                      {renderParamsEditor(bodyParams, setBodyParams, true)}
                    </div>
                  )}
                </>
              )}

              {/* ===============================================================
                  SEÇÕES ESPECÍFICAS DE CLIENT TOOL (Parâmetros + Snippet SDK)
                 =============================================================== */}
              {kind === 'client' && (
                <>
                  <div className="bg-[#f7f7f8] border border-zinc-200/70 rounded-2xl p-4 space-y-3">
                    <div className="flex items-center justify-between gap-4">
                      <div>
                        <h3 className="text-[13px] font-semibold text-zinc-900">Parâmetros</h3>
                        <p className="text-xs text-zinc-500 mt-0.5">
                          Defina os parâmetros que o LLM deve coletar e passar para a função no
                          cliente.
                        </p>
                      </div>
                      <button
                        type="button"
                        onClick={() =>
                          setClientParams([
                            ...clientParams,
                            {
                              name: '',
                              type: 'string',
                              description: '',
                              required: true,
                              value_type: 'llm_prompt',
                            },
                          ])
                        }
                        className="px-3 py-1.5 rounded-lg bg-white hover:bg-zinc-50 border border-zinc-200/90 text-xs font-medium text-zinc-900 shadow-2xs transition-colors flex-shrink-0 cursor-pointer"
                      >
                        Adicionar parâmetro
                      </button>
                    </div>

                    {renderParamsEditor(clientParams, setClientParams, false)}
                  </div>

                  {/* Snippet de registro no cliente conforme documentação ElevenLabs */}
                  <div className="bg-[#f7f7f8] border border-zinc-200/70 rounded-2xl p-4 space-y-2.5">
                    <div className="flex items-center justify-between">
                      <div>
                        <h3 className="text-[13px] font-semibold text-zinc-900">
                          Registro no Código Cliente (JavaScript SDK)
                        </h3>
                        <p className="text-xs text-zinc-500">
                          Os nomes de ferramentas e parâmetros diferenciam maiúsculas e minúsculas.
                        </p>
                      </div>
                      <button
                        type="button"
                        onClick={() => {
                          const paramNames = clientParams
                            .map((p) => p.name)
                            .filter(Boolean)
                            .join(', ')
                          const snippet = `const conversation = await Conversation.startSession({\n  clientTools: {\n    ${name || 'minhaFuncaoCliente'}: async ({ ${paramNames} }) => {\n      console.log('Client tool acionada:', { ${paramNames} });\n      ${expectsResponse ? 'return { status: "ok" };' : ''}\n    }\n  }\n});`
                          navigator.clipboard.writeText(snippet)
                          setCopiedSnippet(true)
                          setTimeout(() => setCopiedSnippet(false), 2000)
                        }}
                        className="inline-flex items-center gap-1 px-2.5 py-1 rounded-lg bg-white border border-zinc-200 text-xs font-medium text-zinc-700 hover:bg-zinc-50"
                      >
                        {copiedSnippet ? (
                          <>
                            <Check className="w-3 h-3 text-emerald-600" /> Copiado
                          </>
                        ) : (
                          <>
                            <Copy className="w-3 h-3" /> Copiar
                          </>
                        )}
                      </button>
                    </div>
                    <pre className="p-3 rounded-xl bg-zinc-950 text-zinc-100 font-mono text-[11px] overflow-x-auto leading-relaxed">
                      {`const conversation = await Conversation.startSession({
  clientTools: {
    ${name || 'logMessage'}: async ({ ${clientParams.map((p) => p.name).filter(Boolean).join(', ') || 'message'} }) => {
      console.log(${clientParams[0]?.name || 'message'});${expectsResponse ? '\n      return { received: true };' : ''}
    }
  }
});`}
                    </pre>
                  </div>
                </>
              )}

              {/* ===============================================================
                  SEÇÕES ESPECÍFICAS DE MCP TOOL (Approval Mode + Headers + Params)
                 =============================================================== */}
              {kind === 'mcp' && (
                <>
                  <div className="bg-[#f7f7f8] border border-zinc-200/70 rounded-2xl p-4 space-y-3">
                    <div>
                      <h3 className="text-[13px] font-semibold text-zinc-900">
                        Modo de aprovação de ferramentas (Tool Approval Mode)
                      </h3>
                      <p className="text-xs text-zinc-500 mt-0.5">
                        Controle como o agente solicita permissão para executar ferramentas deste
                        servidor MCP.
                      </p>
                    </div>

                    <div className="grid grid-cols-1 gap-2">
                      {[
                        {
                          id: 'always_ask',
                          title: 'Sempre perguntar (Recomendado)',
                          desc: 'Segurança máxima. O agente solicita permissão antes de cada uso da ferramenta.',
                        },
                        {
                          id: 'fine_grained',
                          title: 'Aprovação granular por ferramenta',
                          desc: 'Pré-selecione quais ferramentas podem rodar automaticamente e quais exigem aprovação.',
                        },
                        {
                          id: 'no_approval',
                          title: 'Sem aprovação',
                          desc: 'O assistente pode utilizar qualquer ferramenta exposta pelo servidor sem pedir confirmação.',
                        },
                      ].map((opt) => (
                        <label
                          key={opt.id}
                          onClick={() => setMcpApprovalPolicy(opt.id as any)}
                          className={`p-3 rounded-xl border text-left cursor-pointer transition-colors flex items-start gap-2.5 ${
                            mcpApprovalPolicy === opt.id
                              ? 'bg-white border-zinc-900 shadow-2xs'
                              : 'bg-white/70 border-zinc-200/80 hover:bg-white'
                          }`}
                        >
                          <input
                            type="radio"
                            name="mcp_approval"
                            checked={mcpApprovalPolicy === opt.id}
                            onChange={() => setMcpApprovalPolicy(opt.id as any)}
                            className="mt-0.5"
                          />
                          <div>
                            <div className="text-xs font-semibold text-zinc-900">{opt.title}</div>
                            <div className="text-[11px] text-zinc-500">{opt.desc}</div>
                          </div>
                        </label>
                      ))}
                    </div>
                  </div>

                  {/* Cabeçalhos HTTP do Servidor MCP */}
                  <div className="bg-[#f7f7f8] border border-zinc-200/70 rounded-2xl p-4 space-y-3">
                    <div className="flex items-center justify-between gap-4">
                      <div>
                        <h3 className="text-[13px] font-semibold text-zinc-900">Cabeçalhos HTTP</h3>
                        <p className="text-xs text-zinc-500 mt-0.5">
                          Cabeçalhos adicionais exigidos pelo servidor MCP.
                        </p>
                      </div>
                      <button
                        type="button"
                        onClick={() =>
                          setRequestHeaders([
                            ...requestHeaders,
                            { type: 'value', name: '', value: '' },
                          ])
                        }
                        className="px-3 py-1.5 rounded-lg bg-white hover:bg-zinc-50 border border-zinc-200/90 text-xs font-medium text-zinc-900 shadow-2xs"
                      >
                        Adicionar cabeçalho
                      </button>
                    </div>

                    {requestHeaders.length > 0 && (
                      <div className="space-y-2">
                        {requestHeaders.map((h, idx) => (
                          <div
                            key={idx}
                            className="bg-white border border-zinc-200 rounded-xl p-3 flex items-center gap-2"
                          >
                            <input
                              type="text"
                              value={h.name}
                              placeholder="Header"
                              onChange={(e) => {
                                const next = [...requestHeaders]
                                next[idx] = { ...h, name: e.target.value }
                                setRequestHeaders(next)
                              }}
                              className="w-1/3 px-2.5 py-1.5 border border-zinc-200 rounded-lg text-xs font-mono"
                            />
                            <input
                              type="text"
                              value={h.value}
                              placeholder="Valor"
                              onChange={(e) => {
                                const next = [...requestHeaders]
                                next[idx] = { ...h, value: e.target.value }
                                setRequestHeaders(next)
                              }}
                              className="flex-1 px-2.5 py-1.5 border border-zinc-200 rounded-lg text-xs font-mono"
                            />
                            <button
                              type="button"
                              onClick={() =>
                                setRequestHeaders(requestHeaders.filter((_, i) => i !== idx))
                              }
                              className="text-zinc-400 hover:text-red-600"
                            >
                              <Trash2 className="w-3.5 h-3.5" />
                            </button>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>

                  {/* Parâmetros da ferramenta MCP */}
                  <div className="bg-[#f7f7f8] border border-zinc-200/70 rounded-2xl p-4 space-y-3">
                    <div className="flex items-center justify-between gap-4">
                      <div>
                        <h3 className="text-[13px] font-semibold text-zinc-900">
                          Parâmetros da ferramenta MCP
                        </h3>
                        <p className="text-xs text-zinc-500 mt-0.5">
                          Argumentos que o LLM enviará na chamada JSON-RPC tools/call.
                        </p>
                      </div>
                      <button
                        type="button"
                        onClick={() =>
                          setMcpParams([
                            ...mcpParams,
                            {
                              name: '',
                              type: 'string',
                              description: '',
                              required: true,
                              value_type: 'llm_prompt',
                            },
                          ])
                        }
                        className="px-3 py-1.5 rounded-lg bg-white hover:bg-zinc-50 border border-zinc-200/90 text-xs font-medium text-zinc-900 shadow-2xs"
                      >
                        Adicionar parâmetro
                      </button>
                    </div>
                    {renderParamsEditor(mcpParams, setMcpParams, false)}
                  </div>
                </>
              )}

              {/* ===============================================================
                  SEÇÕES ESPECÍFICAS DE CODE TOOL (Editor JS + Run + Context Object)
                 =============================================================== */}
              {kind === 'code' && (
                <>
                  {/* Editor de Código JavaScript + Executor de Teste (Run) */}
                  <div className="bg-[#f7f7f8] border border-zinc-200/70 rounded-2xl p-4 space-y-3">
                    <div className="flex items-center justify-between">
                      <div>
                        <h3 className="text-[13px] font-semibold text-zinc-900">
                          Função JavaScript (Sandbox)
                        </h3>
                        <p className="text-xs text-zinc-500 mt-0.5">
                          Exporte uma função assíncrona padrão que recebe <code>ctx</code> (
                          <code>ctx.args</code>, <code>ctx.config</code>, <code>ctx.secrets</code>).
                        </p>
                      </div>
                      <div className="inline-flex rounded-lg bg-zinc-200/70 p-0.5 text-xs">
                        <button
                          type="button"
                          onClick={() => setCodeTab('code')}
                          className={`px-2.5 py-1 rounded-md font-medium transition-colors ${
                            codeTab === 'code'
                              ? 'bg-white text-zinc-950 shadow-2xs'
                              : 'text-zinc-600 hover:text-zinc-900'
                          }`}
                        >
                          Código
                        </button>
                        <button
                          type="button"
                          onClick={() => setCodeTab('run')}
                          className={`px-2.5 py-1 rounded-md font-medium transition-colors ${
                            codeTab === 'run'
                              ? 'bg-white text-zinc-950 shadow-2xs'
                              : 'text-zinc-600 hover:text-zinc-900'
                          }`}
                        >
                          Testar (Run)
                        </button>
                      </div>
                    </div>

                    {codeTab === 'code' ? (
                      <textarea
                        rows={11}
                        spellCheck={false}
                        value={codeSource}
                        onChange={(e) => setCodeSource(e.target.value)}
                        className="w-full p-3.5 rounded-xl bg-zinc-950 text-zinc-100 font-mono text-xs leading-relaxed focus:outline-none border border-zinc-800"
                      />
                    ) : (
                      <div className="bg-white border border-zinc-200 rounded-xl p-3.5 space-y-3">
                        <div className="flex items-center justify-between">
                          <span className="text-xs font-semibold text-zinc-800">
                            Parâmetros de teste (ctx.args)
                          </span>
                          <button
                            type="button"
                            onClick={handleRunCodeInBrowser}
                            disabled={codeRunning}
                            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-zinc-950 hover:bg-zinc-800 text-white text-xs font-semibold cursor-pointer"
                          >
                            <Play className="w-3 h-3" />
                            {codeRunning ? 'Executando…' : 'Run'}
                          </button>
                        </div>

                        <textarea
                          rows={4}
                          spellCheck={false}
                          value={codeTestArgs}
                          onChange={(e) => setCodeTestArgs(e.target.value)}
                          className="w-full p-2.5 rounded-lg bg-zinc-950 text-zinc-100 font-mono text-xs"
                        />

                        {codeRunOutput && (
                          <div className="space-y-2">
                            <div
                              className={`p-3 rounded-lg border font-mono text-[11px] whitespace-pre-wrap ${
                                codeRunOutput.ok
                                  ? 'bg-emerald-50 border-emerald-200 text-emerald-950'
                                  : 'bg-red-50 border-red-200 text-red-900'
                              }`}
                            >
                              <div className="font-sans font-semibold mb-1">Output:</div>
                              {codeRunOutput.ok
                                ? JSON.stringify(codeRunOutput.data, null, 2)
                                : codeRunOutput.error}
                            </div>

                            {codeRunLogs.length > 0 && (
                              <div className="p-2.5 rounded-lg bg-zinc-900 text-zinc-300 font-mono text-[11px] space-y-0.5">
                                <div className="text-[10px] uppercase tracking-wider text-zinc-400">
                                  Logs:
                                </div>
                                {codeRunLogs.map((l, i) => (
                                  <div key={i}>{l}</div>
                                ))}
                              </div>
                            )}
                          </div>
                        )}
                      </div>
                    )}
                  </div>

                  {/* Parâmetros (ctx.args) */}
                  <div className="bg-[#f7f7f8] border border-zinc-200/70 rounded-2xl p-4 space-y-3">
                    <div className="flex items-center justify-between gap-4">
                      <div>
                        <h3 className="text-[13px] font-semibold text-zinc-900">
                          Parâmetros (ctx.args)
                        </h3>
                        <p className="text-xs text-zinc-500 mt-0.5">
                          Valores que o agente extrairá da conversa e passará em{' '}
                          <code>ctx.args</code>.
                        </p>
                      </div>
                      <button
                        type="button"
                        onClick={() =>
                          setCodeParams([
                            ...codeParams,
                            {
                              name: '',
                              type: 'string',
                              description: '',
                              required: true,
                              value_type: 'llm_prompt',
                            },
                          ])
                        }
                        className="px-3 py-1.5 rounded-lg bg-white hover:bg-zinc-50 border border-zinc-200/90 text-xs font-medium text-zinc-900 shadow-2xs"
                      >
                        Adicionar parâmetro
                      </button>
                    </div>
                    {renderParamsEditor(codeParams, setCodeParams, false)}
                  </div>

                  {/* Context Object (ctx.config) */}
                  <div className="bg-[#f7f7f8] border border-zinc-200/70 rounded-2xl p-4 space-y-3">
                    <div>
                      <h3 className="text-[13px] font-semibold text-zinc-900">
                        Objeto de Contexto (Context object)
                      </h3>
                      <p className="text-xs text-zinc-500 mt-0.5">
                        Mapeie variáveis de configuração acessíveis via{' '}
                        <code>ctx.config.&lt;NOME&gt;</code>.
                      </p>
                    </div>

                    <div className="flex gap-2">
                      <input
                        type="text"
                        placeholder="Chave (ex: API_BASE_URL)"
                        value={newCtxKey}
                        onChange={(e) => setNewCtxKey(e.target.value)}
                        className="w-1/3 px-2.5 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs font-mono"
                      />
                      <input
                        type="text"
                        placeholder="Valor"
                        value={newCtxVal}
                        onChange={(e) => setNewCtxVal(e.target.value)}
                        className="flex-1 px-2.5 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs font-mono"
                      />
                      <button
                        type="button"
                        onClick={() => {
                          if (!newCtxKey.trim()) return
                          setCodeContextConfig({
                            ...codeContextConfig,
                            [newCtxKey.trim()]: newCtxVal,
                          })
                          setNewCtxKey('')
                          setNewCtxVal('')
                        }}
                        className="px-3 py-1.5 rounded-lg bg-white border border-zinc-200 text-xs font-medium text-zinc-900 hover:bg-zinc-50"
                      >
                        Adicionar
                      </button>
                    </div>

                    {Object.entries(codeContextConfig).length > 0 && (
                      <div className="space-y-1.5">
                        {Object.entries(codeContextConfig).map(([k, v]) => (
                          <div
                            key={k}
                            className="px-3 py-2 bg-white border border-zinc-200 rounded-xl flex items-center justify-between text-xs font-mono"
                          >
                            <span>
                              ctx.config.{k} = "{v}"
                            </span>
                            <button
                              type="button"
                              onClick={() => {
                                const next = { ...codeContextConfig }
                                delete next[k]
                                setCodeContextConfig(next)
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
                </>
              )}

              {/* ===============================================================
                  CARD 5: VARIÁVEIS DINÂMICAS (Imagem 2)
                 =============================================================== */}
              <div className="bg-[#f7f7f8] border border-zinc-200/70 rounded-2xl p-4 space-y-3">
                <div>
                  <h3 className="text-[13px] font-semibold text-zinc-900">Variáveis Dinâmicas</h3>
                  <p className="text-xs text-zinc-500 mt-0.5 leading-relaxed">
                    As variáveis nos parâmetros da ferramenta serão substituídas por valores reais
                    quando a conversa começar.{' '}
                    <a
                      href="https://elevenlabs.io/docs/eleven-agents/customization/personalization/dynamic-variables"
                      target="_blank"
                      rel="noreferrer"
                      className="text-zinc-900 font-medium hover:underline"
                    >
                      Saiba mais
                    </a>
                  </p>
                </div>

                <div className="flex gap-2">
                  <input
                    type="text"
                    placeholder="Variável (ex: telefone)"
                    value={newDynVarKey}
                    onChange={(e) => setNewDynVarKey(e.target.value)}
                    className="w-5/12 px-2.5 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs font-mono"
                  />
                  <input
                    type="text"
                    placeholder="Placeholder / valor padrão"
                    value={newDynVarVal}
                    onChange={(e) => setNewDynVarVal(e.target.value)}
                    className="flex-1 px-2.5 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs"
                  />
                  <button
                    type="button"
                    onClick={() => {
                      if (!newDynVarKey.trim()) return
                      setDynamicVarPlaceholders({
                        ...dynamicVarPlaceholders,
                        [newDynVarKey.trim()]: newDynVarVal,
                      })
                      setNewDynVarKey('')
                      setNewDynVarVal('')
                    }}
                    className="px-3 py-1.5 rounded-lg bg-white hover:bg-zinc-50 border border-zinc-200/90 text-xs font-medium text-zinc-900 shadow-2xs cursor-pointer"
                  >
                    Adicionar
                  </button>
                </div>

                {Object.entries(dynamicVarPlaceholders).length > 0 && (
                  <div className="space-y-1.5">
                    {Object.entries(dynamicVarPlaceholders).map(([k, v]) => (
                      <div
                        key={k}
                        className="px-3 py-2 bg-white border border-zinc-200/90 rounded-xl flex items-center justify-between gap-2 text-xs"
                      >
                        <div className="flex items-center gap-2 min-w-0 flex-1">
                          <code className="px-1.5 py-0.5 rounded bg-zinc-100 border border-zinc-200 font-mono text-[11px] text-zinc-800">
                            {`{{${k}}}`}
                          </code>
                          <input
                            type="text"
                            value={v}
                            placeholder="Valor padrão (placeholder)"
                            onChange={(e) =>
                              setDynamicVarPlaceholders({
                                ...dynamicVarPlaceholders,
                                [k]: e.target.value,
                              })
                            }
                            className="flex-1 px-2 py-1 bg-zinc-50 border border-zinc-200 rounded text-xs"
                          />
                        </div>
                        <button
                          type="button"
                          onClick={() => {
                            const next = { ...dynamicVarPlaceholders }
                            delete next[k]
                            setDynamicVarPlaceholders(next)
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

              {/* ===============================================================
                  CARD 6: ATRIBUIÇÕES DE VARIÁVEIS DINÂMICAS (Imagem 2)
                 =============================================================== */}
              <div className="bg-[#f7f7f8] border border-zinc-200/70 rounded-2xl p-4 space-y-3">
                <div className="flex items-center justify-between gap-4">
                  <div>
                    <h3 className="text-[13px] font-semibold text-zinc-900">
                      Atribuições de Variáveis Dinâmicas
                    </h3>
                    <p className="text-xs text-zinc-500 mt-0.5 leading-relaxed">
                      Configure quais variáveis dinâmicas podem ser atualizadas quando esta
                      ferramenta retorna uma resposta.
                    </p>
                    <a
                      href="https://elevenlabs.io/docs/eleven-agents/customization/tools/webhook-tools"
                      target="_blank"
                      rel="noreferrer"
                      className="text-xs text-zinc-900 font-medium hover:underline inline-block mt-0.5"
                    >
                      Saiba mais
                    </a>
                  </div>
                  <button
                    type="button"
                    onClick={() =>
                      setAssignments([
                        ...assignments,
                        { source: 'response', dynamic_variable: '', value_path: '' },
                      ])
                    }
                    className="px-3 py-1.5 rounded-lg bg-white hover:bg-zinc-50 border border-zinc-200/90 text-xs font-medium text-zinc-900 shadow-2xs transition-colors flex-shrink-0 cursor-pointer"
                  >
                    Adicionar atribuição
                  </button>
                </div>

                {assignments.length > 0 && (
                  <div className="space-y-2 pt-1">
                    {assignments.map((a, idx) => (
                      <div
                        key={idx}
                        className="bg-white border border-zinc-200/90 rounded-xl p-3 grid grid-cols-1 sm:grid-cols-11 gap-2 items-center shadow-2xs"
                      >
                        <div className="sm:col-span-5">
                          <label className="block text-[10px] font-medium text-zinc-500 mb-0.5">
                            Variável dinâmica de destino
                          </label>
                          <input
                            type="text"
                            value={a.dynamic_variable}
                            placeholder="ex: plano_escolhido"
                            onChange={(e) => {
                              const next = [...assignments]
                              next[idx] = { ...a, dynamic_variable: e.target.value }
                              setAssignments(next)
                            }}
                            className="w-full px-2.5 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs font-mono"
                          />
                        </div>

                        <div className="sm:col-span-5">
                          <label className="block text-[10px] font-medium text-zinc-500 mb-0.5">
                            Caminho JSON na resposta (value_path)
                          </label>
                          <input
                            type="text"
                            value={a.value_path}
                            placeholder="ex: data.plano ou statusPlano"
                            onChange={(e) => {
                              const next = [...assignments]
                              next[idx] = { ...a, value_path: e.target.value }
                              setAssignments(next)
                            }}
                            className="w-full px-2.5 py-1.5 bg-white border border-zinc-200 rounded-lg text-xs font-mono"
                          />
                        </div>

                        <div className="sm:col-span-1 flex justify-end pt-3.5">
                          <button
                            type="button"
                            onClick={() => setAssignments(assignments.filter((_, i) => i !== idx))}
                            className="p-1 text-zinc-400 hover:text-red-600 rounded-lg"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </button>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              {/* ===============================================================
                  CARD 7: MOCKS DE RESPOSTA (Imagem 3)
                 =============================================================== */}
              <div className="bg-[#f7f7f8] border border-zinc-200/70 rounded-2xl p-4 space-y-3">
                <div>
                  <h3 className="text-[13px] font-semibold text-zinc-900">Mocks de resposta</h3>
                  <p className="text-xs text-zinc-500 mt-0.5 leading-relaxed">
                    Use respostas mock para avaliar o comportamento do agente em simulações de teste
                    sem se conectar aos sistemas de produção. As condições são avaliadas de cima
                    para baixo e a primeira correspondência é retornada.
                  </p>
                </div>

                {responseMocks.length > 0 && (
                  <div className="space-y-2.5">
                    {responseMocks.map((mock, idx) => (
                      <div
                        key={idx}
                        className="bg-white border border-zinc-200/90 rounded-xl p-3.5 space-y-2.5 shadow-2xs"
                      >
                        <div className="flex items-center justify-between gap-2">
                          <div className="flex items-center gap-2 flex-1">
                            <select
                              value={mock.condition_type || 'always'}
                              onChange={(e) => {
                                const next = [...responseMocks]
                                next[idx] = { ...mock, condition_type: e.target.value as any }
                                setResponseMocks(next)
                              }}
                              className="px-2.5 py-1 bg-white border border-zinc-200 rounded-lg text-xs"
                            >
                              <option value="always">Sempre corresponder</option>
                              <option value="parameter_equals">Quando parâmetro for igual a</option>
                            </select>

                            {mock.condition_type === 'parameter_equals' && (
                              <>
                                <input
                                  type="text"
                                  value={mock.parameter_name || ''}
                                  placeholder="parâmetro"
                                  onChange={(e) => {
                                    const next = [...responseMocks]
                                    next[idx] = { ...mock, parameter_name: e.target.value }
                                    setResponseMocks(next)
                                  }}
                                  className="w-28 px-2 py-1 border border-zinc-200 rounded-lg text-xs font-mono"
                                />
                                <span className="text-xs text-zinc-400">=</span>
                                <input
                                  type="text"
                                  value={mock.parameter_value || ''}
                                  placeholder="valor"
                                  onChange={(e) => {
                                    const next = [...responseMocks]
                                    next[idx] = { ...mock, parameter_value: e.target.value }
                                    setResponseMocks(next)
                                  }}
                                  className="w-28 px-2 py-1 border border-zinc-200 rounded-lg text-xs font-mono"
                                />
                              </>
                            )}
                          </div>

                          <div className="flex items-center gap-2">
                            <label className="inline-flex items-center gap-1 text-[11px] text-zinc-600 cursor-pointer">
                              <input
                                type="checkbox"
                                checked={mock.enabled}
                                onChange={(e) => {
                                  const next = [...responseMocks]
                                  next[idx] = { ...mock, enabled: e.target.checked }
                                  setResponseMocks(next)
                                }}
                                className="rounded border-zinc-300"
                              />
                              Ativo
                            </label>
                            <button
                              type="button"
                              onClick={() =>
                                setResponseMocks(responseMocks.filter((_, i) => i !== idx))
                              }
                              className="p-1 text-zinc-400 hover:text-red-600"
                            >
                              <Trash2 className="w-3.5 h-3.5" />
                            </button>
                          </div>
                        </div>

                        <div>
                          <label className="block text-[11px] font-medium text-zinc-500 mb-1">
                            Resposta JSON simulada
                          </label>
                          <textarea
                            rows={3}
                            spellCheck={false}
                            value={mock.response}
                            onChange={(e) => {
                              const next = [...responseMocks]
                              next[idx] = { ...mock, response: e.target.value }
                              setResponseMocks(next)
                            }}
                            className="w-full p-2.5 bg-zinc-50 border border-zinc-200 rounded-lg font-mono text-xs text-zinc-900"
                          />
                        </div>
                      </div>
                    ))}
                  </div>
                )}

                <button
                  type="button"
                  onClick={() =>
                    setResponseMocks([
                      ...responseMocks,
                      {
                        condition_type: 'always',
                        response: '{\n  "status": "ok"\n}',
                        enabled: true,
                      },
                    ])
                  }
                  className="w-full py-2 px-3 rounded-xl bg-white hover:bg-zinc-50 border border-zinc-200/90 text-xs font-semibold text-zinc-900 inline-flex items-center justify-center gap-1.5 shadow-2xs transition-colors cursor-pointer"
                >
                  <Plus className="w-3.5 h-3.5" />
                  Adicionar mock
                </button>
              </div>
            </>
          )}
        </div>

        {/* =====================================================================
            RODAPÉ FIXO DA GAVETA (Idêntico à Imagem 1: "</> Editar como JSON 📖" | "Cancelar" | "Adicionar ferramenta")
           ===================================================================== */}
        <div className="px-6 py-3.5 border-t border-zinc-200/80 bg-white flex items-center justify-between flex-shrink-0">
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={handleToggleJsonMode}
              className="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg hover:bg-zinc-100 text-xs font-semibold text-zinc-800 transition-colors cursor-pointer"
            >
              <Code2 className="w-3.5 h-3.5 text-zinc-600" />
              <span>{isJsonMode ? 'Formulário visual' : 'Editar como JSON'}</span>
            </button>
            <a
              href={headerMeta.docUrl}
              target="_blank"
              rel="noreferrer"
              title="Ver esquema JSON na documentação"
              className="p-1.5 text-zinc-400 hover:text-zinc-700 rounded-lg transition-colors"
            >
              <BookOpen className="w-4 h-4" />
            </a>
          </div>

          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 rounded-xl border border-zinc-200/90 bg-white hover:bg-zinc-50 text-xs font-semibold text-zinc-800 transition-colors cursor-pointer"
            >
              Cancelar
            </button>

            <button
              type="button"
              onClick={handleSave}
              disabled={saving}
              className="px-4 py-2 rounded-xl bg-zinc-950 hover:bg-zinc-800 disabled:opacity-50 text-white text-xs font-semibold transition-colors cursor-pointer"
            >
              {saving
                ? 'Salvando…'
                : editingTool
                  ? 'Salvar ferramenta'
                  : 'Adicionar ferramenta'}
            </button>
          </div>
        </div>
      </div>
    </Sheet>
  )
}
