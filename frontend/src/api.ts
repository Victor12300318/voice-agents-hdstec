export class ApiError extends Error {
  status: number
  errors?: string[]
  constructor(status: number, message: string, errors?: string[]) {
    super(message)
    this.status = status
    this.errors = errors
  }
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const res = await fetch(path, {
    credentials: 'same-origin',
    ...init,
    headers: { 'Content-Type': 'application/json', ...(init.headers ?? {}) },
  })
  if (res.status === 204) return undefined as T
  const body = await res.json().catch(() => ({}))
  if (!res.ok) {
    throw new ApiError(res.status, body.error ?? `HTTP ${res.status}`, body.errors)
  }
  return body as T
}

export type Me = {
  user_id: string
  email: string
  organization: { id: string; name: string }
}

export type Option = {
  id: string
  label: string
  description?: string
  default?: boolean
}

export type ProviderCatalog = {
  provider: string
  label: string
  models: Option[]
  voices: Option[]
  languages: Option[]
  max_session_seconds: number
  input_sample_rate: number
  output_sample_rate: number
}

export type DeclaredVariable = {
  name: string
  description: string
}

export type ToolParam = {
  name: string
  type: string
  description: string
  required: boolean
}

export type ToolParamSchema = {
  name: string
  type: string
  description?: string
  required: boolean
  value_type?: 'llm_prompt' | 'constant' | 'dynamic_variable'
  constant_value?: any
  dynamic_variable?: string
}

export type ToolHeaderSchema = {
  type: 'value' | 'secret' | 'dynamic_variable'
  name: string
  value: string
}

export type ToolRequestBodySchema = {
  type: 'object'
  description?: string
  properties: ToolParamSchema[]
}

export type ToolAPISchema = {
  url: string
  method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE' | string
  path_params_schema: ToolParamSchema[]
  query_params_schema: ToolParamSchema[]
  request_body_schema: ToolRequestBodySchema | null
  request_headers: ToolHeaderSchema[]
  content_type: 'application/json' | 'application/x-www-form-urlencoded' | string
  auth_connection: any
  mtls_auth_connection: any
  response_filter: any
}

export type ToolAssignment = {
  source?: string
  dynamic_variable: string
  value_path: string
}

export type ToolResponseMock = {
  id?: string
  name?: string
  condition_type?: 'always' | 'parameter_equals'
  parameter_name?: string
  parameter_value?: string
  response: string
  enabled: boolean
}

export type ToolCodeContext = {
  config?: Record<string, string>
  secrets?: string[]
  auth_connections?: string[]
}

export type ToolConfig = {
  url: string
  method: string
  api_schema?: ToolAPISchema
  response_timeout_secs?: number
  follow_redirects?: boolean
  follow_redirects_allowed_domains?: string[]
  dynamic_variables?: {
    dynamic_variable_placeholders: Record<string, any>
  }
  assignments?: ToolAssignment[]
  interruption_mode?: string
  pre_tool_speech?: string
  tool_call_sound?: string | null
  tool_call_sound_behavior?: string
  execution_mode?: string
  tool_error_handling_mode?: string
  response_mocks?: ToolResponseMock[]
  expects_response?: boolean
  code?: string
  context_object?: ToolCodeContext
  transport?: string
  approval_policy?: string
}

export type Tool = {
  id: string
  organization_id: string
  name: string
  description: string
  kind: string
  mcp_server_id?: string
  mcp_server_name?: string
  category?: string
  input_schema: ToolParam[]
  config: ToolConfig
  secret_headers?: Record<string, string>
  has_secrets: boolean
  timeout_ms: number
  active: boolean
  created_at: string
  updated_at: string
}

export type ToolWithStats = Tool & {
  total_executions: number
  success_rate: number
}

export type ProcedureValidationError = {
  path: string
  message: string
}

export type SchemaOverride = {
  source: 'constant' | 'dynamic_variable' | 'llm' | 'omit'
  constant_value?: any
  dynamic_variable?: string
  prompt?: string
}

export type StructuredStep = {
  type: 'ask' | 'tell' | 'say' | 'branch' | 'tool_call' | 'retry' | 'sub_procedure' | 'system_tool'
  instruction?: string
  message?: string
  message_translations?: Record<string, { value: string }>
  tool_id?: string
  tool_name?: string
  schema_overrides?: Record<string, SchemaOverride>
  on_failure?: {
    branches?: any[]
    fallback: StructuredStep[]
  }
  branches?: {
    condition: {
      type: 'llm' | 'expression'
      condition?: string
      expression?: {
        type: string
        left: { type: string; name?: string }
        right: { type: string; value?: string }
      }
    }
    steps: StructuredStep[]
  }[]
  fallback?: StructuredStep[]
  procedure_id?: string
  system_tool_name?: string
  max_retries?: number
}

export type Skill = {
  id: string
  organization_id: string
  agent_id?: string
  name: string
  type?: 'free_form' | 'deterministic'
  description?: string
  trigger: string
  guardrails: string[]
  content: string
  active: boolean
  position?: number
  tools?: string[]
  is_sub_procedure?: boolean
  referenced_tool_ids?: string[]
  referenced_kb_ids?: string[]
  referenced_procedure_ids?: string[]
  referenced_dynamic_variables?: string[]
  validation_errors?: ProcedureValidationError[]
  created_at: string
  updated_at: string
}

// Backwards compatibility alias:
export type Procedure = Skill

export type MCPServer = {
  id: string
  organization_id: string
  name: string
  slug: string
  description: string
  icon?: string
  transport: 'sse' | 'http'
  url: string
  status: 'connected' | 'error' | 'disconnected'
  tools_count: number
  created_at: string
  updated_at: string
}

export type KnowledgeBase = {
  id: string
  organization_id: string
  name: string
  description: string
  documents_count: number
  total_bytes: number
  created_at: string
  updated_at: string
}

export type KnowledgeDocument = {
  id: string
  organization_id: string
  knowledge_base_id: string
  name: string
  content: string
  created_at: string
}

export type AgentTransferRule = {
  agent_id: string
  agent_name?: string
  condition: string
  delay_ms: number
  transfer_message?: string | null
  enable_transferred_agent_first_message: boolean
}

export type TransferToAgentToolConfig = {
  type: 'system'
  name: 'transfer_to_agent'
  description: string
  transfers: AgentTransferRule[]
}

export type GuardianJevConfig = {
  enabled: boolean
  provider?: string
  model?: string
  api_key?: string
  check_voicemail: boolean
  check_talking_to_self: boolean
  check_hallucination: boolean
  voicemail_threshold?: number
  talking_to_self_threshold?: number
  hallucination_threshold?: number
}

export type SystemToolsConfig = {
  transfer_to_agent?: TransferToAgentToolConfig
  guardian_jev?: GuardianJevConfig
}

export type Agent = {
  id: string
  organization_id: string
  name: string
  description: string
  active: boolean
  provider: string
  model: string
  voice: string
  language: string
  timezone: string
  system_prompt: string
  first_message: string
  first_message_interruptible: boolean
  max_duration_seconds: number
  recording_enabled: boolean
  variables: DeclaredVariable[]
  initiation_webhook_url: string
  initiation_webhook_timeout_ms: number
  system_tools: string[]
  system_tools_config?: SystemToolsConfig
  tools: string[]
  created_at: string
  updated_at: string
}

export type AgentDetail = Agent & {
  latest_version: number | null
  has_unpublished_changes: boolean
  publish_errors: string[]
}

export type AgentUpdatePayload = {
  name: string
  description: string
  active: boolean
  provider: string
  model: string
  voice: string
  language: string
  timezone: string
  system_prompt: string
  first_message: string
  first_message_interruptible: boolean
  max_duration_seconds: number
  recording_enabled: boolean
  variables: DeclaredVariable[]
  initiation_webhook_url: string
  initiation_webhook_timeout_ms: number
  system_tools: string[]
  system_tools_config?: SystemToolsConfig
  tools: string[]
}

export type Conversation = {
  id: string
  organization_id: string
  agent_id: string
  agent_version_id: string | null
  is_test: boolean
  session_id: string
  provider: string
  model: string
  status: string // starting | active | completed | failed
  ended_reason: string
  started_at: string
  ended_at: string | null
  duration_ms: number | null
  config_snapshot: any
  variables: Record<string, string>
  metadata: any
  recording_key: string
  cost_usd?: number
  cost_brl?: number
  tokens_usage?: any
  direction?: 'web' | 'inbound' | 'outbound'
  from_number?: string
  to_number?: string
  agent_name?: string
}

export type PhoneNumber = {
  id: string
  organization_id: string
  e164: string
  label: string
  agent_id: string
  agent_name?: string
  direction: string
  provider?: 'jambonz' | 'twilio' | string
  twilio_account_sid?: string
  twilio_phone_sid?: string
  has_twilio_auth?: boolean
  active: boolean
  created_at: string
  updated_at: string
}

export type TwilioAvailableNumber = {
  sid: string
  phone_number: string
  friendly_name: string
  voice_url: string
}

export type ConversationEvent = {
  id: number
  conversation_id: string
  type: string
  payload: any
  created_at: string
}

export type ToolExecution = {
  id: string
  organization_id: string
  agent_id: string
  conversation_id: string
  tool_id: string | null
  tool_name: string
  procedure_id: string | null
  request: any
  response: any
  status: string
  execution_time_ms: number
  created_at: string
}

export type ConversationDetail = {
  conversation: Conversation
  agent: Agent
  events: ConversationEvent[]
  tool_executions: ToolExecution[]
}

export type DashboardMetrics = {
  total_agents: number
  active_conversations?: number
  total_conversations: number
  failed_conversations: number
  avg_duration_ms: number
  avg_latency_ms: number
  total_cost_usd: number
  total_cost_brl: number
  recent_conversations: Conversation[]
}
