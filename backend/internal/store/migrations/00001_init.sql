-- +goose Up

-- Tenant. Todo dado pertence a exatamente uma Organization.
CREATE TABLE organizations (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- User: quem faz login no painel.
CREATE TABLE users (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email           text NOT NULL,
    password_hash   text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_key ON users (lower(email));

-- Agent: as colunas de configuração representam o Draft.
CREATE TABLE agents (
    id                            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id               uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name                          text NOT NULL,
    description                   text NOT NULL DEFAULT '',
    active                        boolean NOT NULL DEFAULT true,
    provider                      text NOT NULL DEFAULT '',
    model                         text NOT NULL DEFAULT '',
    voice                         text NOT NULL DEFAULT '',
    language                      text NOT NULL DEFAULT 'pt-BR',
    timezone                      text NOT NULL DEFAULT 'America/Sao_Paulo',
    system_prompt                 text NOT NULL DEFAULT '',
    first_message                 text NOT NULL DEFAULT '',
    first_message_interruptible   boolean NOT NULL DEFAULT false,
    max_duration_seconds          integer NOT NULL DEFAULT 600,
    recording_enabled             boolean NOT NULL DEFAULT true,
    -- Nomes das Dynamic Variables declaradas: [{"name": "...", "description": "..."}]
    variables                     jsonb NOT NULL DEFAULT '[]',
    initiation_webhook_url        text NOT NULL DEFAULT '',
    initiation_webhook_timeout_ms integer NOT NULL DEFAULT 3000,
    -- System Tools habilitadas: ["encerrar_conversa"]
    system_tools                  jsonb NOT NULL DEFAULT '["encerrar_conversa"]',
    created_at                    timestamptz NOT NULL DEFAULT now(),
    updated_at                    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX agents_org_idx ON agents (organization_id);

-- Procedures do Draft de um Agent.
CREATE TABLE procedures (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    agent_id        uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    name            text NOT NULL,
    trigger         text NOT NULL DEFAULT '',
    -- Guardrails: ["Nunca leia o código de barras", ...]
    guardrails      jsonb NOT NULL DEFAULT '[]',
    -- Procedure Content em markdown com References no formato @[kind:uuid]
    content         text NOT NULL DEFAULT '',
    active          boolean NOT NULL DEFAULT true,
    position        integer NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (agent_id, name)
);
CREATE INDEX procedures_agent_idx ON procedures (agent_id);

-- Agent Version: snapshot imutável do Draft (Agent + Procedures) no Publish.
-- Tools e Knowledge Bases não entram (ADR 0002).
CREATE TABLE agent_versions (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    agent_id        uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    version         integer NOT NULL,
    snapshot        jsonb NOT NULL,
    published_by    uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (agent_id, version)
);

-- Tool da Organization, compartilhável entre Agents.
CREATE TABLE tools (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            text NOT NULL,
    description     text NOT NULL DEFAULT '',
    -- 'webhook' | 'mcp'
    kind            text NOT NULL DEFAULT 'webhook',
    -- Parâmetros: [{"name","type","description","required"}]
    input_schema    jsonb NOT NULL DEFAULT '[]',
    -- webhook: {"url","method"}; mcp: {"mcp_server_id","tool_name"}
    config          jsonb NOT NULL DEFAULT '{}',
    -- Headers de autenticação; nunca devolvidos ao frontend em claro.
    secret_headers  jsonb NOT NULL DEFAULT '{}',
    timeout_ms      integer NOT NULL DEFAULT 10000,
    active          boolean NOT NULL DEFAULT true,
    mcp_server_id   uuid,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name)
);

-- MCP Server: preparado, ainda não exposto na UI.
CREATE TABLE mcp_servers (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            text NOT NULL,
    url             text NOT NULL,
    config          jsonb NOT NULL DEFAULT '{}',
    active          boolean NOT NULL DEFAULT true,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name)
);
ALTER TABLE tools ADD CONSTRAINT tools_mcp_server_fk
    FOREIGN KEY (mcp_server_id) REFERENCES mcp_servers(id) ON DELETE SET NULL;

-- Knowledge Base da Organization: documentos em texto/markdown incluídos inline.
CREATE TABLE knowledge_bases (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            text NOT NULL,
    description     text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name)
);

CREATE TABLE knowledge_documents (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id   uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    knowledge_base_id uuid NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    name              text NOT NULL,
    content           text NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now()
);

-- Conversation: uma sessão de voz Caller <-> Agent.
CREATE TABLE conversations (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id  uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    agent_id         uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    -- NULL = Test Conversation contra o Draft
    agent_version_id uuid REFERENCES agent_versions(id) ON DELETE SET NULL,
    is_test          boolean NOT NULL DEFAULT false,
    session_id       text NOT NULL DEFAULT '',
    provider         text NOT NULL,
    model            text NOT NULL DEFAULT '',
    -- starting | active | completed | failed
    status           text NOT NULL DEFAULT 'starting',
    -- caller_hangup | agent_ended | provider_error | initiation_failed | max_duration | internal_error
    ended_reason     text NOT NULL DEFAULT '',
    started_at       timestamptz NOT NULL DEFAULT now(),
    ended_at         timestamptz,
    duration_ms      bigint,
    -- Configuração resolvida no início (prompt, procedures, tools) para auditoria
    config_snapshot  jsonb NOT NULL DEFAULT '{}',
    variables        jsonb NOT NULL DEFAULT '{}',
    metadata         jsonb NOT NULL DEFAULT '{}',
    recording_key    text NOT NULL DEFAULT ''
);
CREATE INDEX conversations_org_started_idx ON conversations (organization_id, started_at DESC);
CREATE INDEX conversations_agent_idx ON conversations (agent_id);

CREATE TABLE conversation_events (
    id              bigserial PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    conversation_id uuid NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    -- caller_transcript | agent_transcript | interruption | procedure_activated |
    -- tool_call | tool_result | first_message_sent | response_latency | error | ended
    type            text NOT NULL,
    payload         jsonb NOT NULL DEFAULT '{}',
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX conversation_events_conv_idx ON conversation_events (conversation_id, id);

CREATE TABLE tool_executions (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id   uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    agent_id          uuid NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    conversation_id   uuid NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    -- NULL para System Tools
    tool_id           uuid REFERENCES tools(id) ON DELETE SET NULL,
    tool_name         text NOT NULL,
    procedure_id      uuid,
    request           jsonb NOT NULL DEFAULT '{}',
    response          jsonb NOT NULL DEFAULT '{}',
    -- success | error | rejected
    status            text NOT NULL,
    execution_time_ms integer NOT NULL DEFAULT 0,
    created_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tool_executions_conv_idx ON tool_executions (conversation_id);
CREATE INDEX tool_executions_tool_idx ON tool_executions (tool_id);

-- +goose Down
DROP TABLE tool_executions;
DROP TABLE conversation_events;
DROP TABLE conversations;
DROP TABLE knowledge_documents;
DROP TABLE knowledge_bases;
ALTER TABLE tools DROP CONSTRAINT tools_mcp_server_fk;
DROP TABLE mcp_servers;
DROP TABLE tools;
DROP TABLE agent_versions;
DROP TABLE procedures;
DROP TABLE agents;
DROP TABLE users;
DROP TABLE organizations;
