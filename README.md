# Voice Agent Runtime

Plataforma caseira (MVP) para criação, configuração e execução em tempo real de agentes de voz com inteligência artificial, construída com arquitetura **provider-agnostic** (Gemini Live primeiro, OpenAI Realtime preparado).

- Vocabulário e regras do domínio: [CONTEXT.md](./CONTEXT.md)
- Decisões arquiteturais registradas: [docs/adr/](./docs/adr/)
  - [ADR 0001: Ativação de Procedures sob demanda e inclusão inline](./docs/adr/0001-procedure-activation-via-system-tool.md)
  - [ADR 0002: Versões imutáveis excluem Tools e Bases de Conhecimento](./docs/adr/0002-agent-version-excludes-tools-and-knowledge-bases.md)
  - [ADR 0003: Áudio via WebSocket PCM e sessões em memória](./docs/adr/0003-websocket-pcm-and-single-instance-runtime.md)

---

## Status da Implementação

| Etapa | Módulo / Entregável | Status |
|---|---|---|
| 1 | Docker, PostgreSQL 17, Redis 7, MinIO, Backend Go 1.26, Frontend React 19, Autenticação e Multi-tenant | ✅ |
| 2 | Agent (Draft vs Agent Version publicada), Variáveis dinâmicas/sistema, Initiation Webhook, VoiceProvider e GeminiLiveProvider | ✅ |
| 3 | Voice Runtime em tempo real, áudio bidirecional PCM16, First Message, transcrição contínua, Interrupção (barge-in) e tela de teste | ✅ |
| 4 | Webhook Tools (n8n/APIs), normalização de envelope `{success, data}`, System Tool `encerrar_conversa`, log de `tool_executions` | ✅ |
| 5 | Procedures com Triggers, Guardrails invioláveis, referências inline (`@[tool]`, `@[procedure]`, `@[knowledge]`) e Bases de Conhecimento | ✅ |
| 6 | Gravação em tempo real WAV estéreo no MinIO, histórico de conversas com áudio player/timeline de eventos e Dashboard de métricas | ✅ |
| 7 | Preparação para OpenAI Realtime (suite de testes de conformidade `VerifyCatalogCompliance` e provider WebSocket) | ✅ |

---

## Arquitetura do Sistema

```
                        ┌────────────────────────────────────────────────────────┐
                        │                  Navegador Web                         │
                        │  (React 19 + AudioWorklet PCM 16kHz + Player 24kHz)   │
                        └──────────────────────────┬─────────────────────────────┘
                                                   │ HTTP / WebSocket (/ws/conversations/{id})
                                                   ▼
                        ┌────────────────────────────────────────────────────────┐
                        │              Nginx Reverse Proxy (:8088)               │
                        └──────────────────────────┬─────────────────────────────┘
                                                   │
                                                   ▼
                        ┌────────────────────────────────────────────────────────┐
                        │            Backend Go (Binário Único :8080)            │
                        │                                                        │
                        │   ┌────────────────────────────────────────────────┐   │
                        │   │         Voice Runtime & Session Manager        │   │
                        │   │  - Resolução de Variáveis (Sistema + Webhook)  │   │
                        │   │  - Allowlist estrita de ferramentas ativas     │   │
                        │   │  - Gravação estéreo (Left: Caller, Right: Agent│   │
                        │   │  - Medição de Response Latency                 │   │
                        │   └───────────────────────┬────────────────────────┘   │
                        │                           │                            │
                        │                           ▼                            │
                        │   ┌────────────────────────────────────────────────┐   │
                        │   │         Abstração voice.Provider               │   │
                        │   │   ├── gemini.Provider (Gemini Live API)        │   │
                        │   │   └── openai.Provider (OpenAI Realtime API)    │   │
                        │   └────────────────────────────────────────────────┘   │
                        └───────┬───────────────────┬────────────────────┬───────┘
                                │                   │                    │
                                ▼                   ▼                    ▼
                    ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐
                    │  PostgreSQL 17   │  │     Redis 7      │  │      MinIO       │
                    │ Configs, Versões,│  │ Sessões de login │  │  Gravações WAV   │
                    │ Eventos e Logs   │  │   e contadores   │  │     estéreo      │
                    └──────────────────┘  └──────────────────┘  └──────────────────┘
```

### Características Centrais
- **Provider-Agnostic:** O núcleo da aplicação (`Runtime`, `Procedures`, `Tools`, `Storage`, `Frontend`) interage estritamente com a interface Go `voice.Provider`. O Gemini Live e o OpenAI Realtime são apenas plugins dessa interface.
- **Isolamento Multi-tenant:** Todas as tabelas possuem `organization_id`. Consultas são filtradas pela organização da sessão autenticada.
- **Alinhamento de Áudio em Linha do Tempo Real:** As gravações estéreo geradas no MinIO posicionam as falas em relógio de parede real (Caller no canal esquerdo, Agente no canal direito). Em caso de **interrupção**, o áudio que o agente não chegou a falar para o usuário é descartado da gravação.

---

## Requisitos

- **Docker** com **Docker Compose v2** instalado.
- **Chave de API do Gemini Live** (`GEMINI_API_KEY`).
- (Opcional) **Chave de API da OpenAI** (`OPENAI_API_KEY`) para habilitar o OpenAI Realtime.
- Navegador com permissão de microfone (Chrome, Edge, Firefox, Safari).

---

## Instalação e Configuração

1. Clone o repositório e acesse o diretório raiz:
   ```bash
   cd teste-voz-gemini
   ```

2. Crie seu arquivo de ambiente a partir do exemplo:
   ```bash
   cp .env.example .env
   ```

3. Abra o arquivo `.env` e configure suas variáveis:
   ```ini
   # Credenciais do Banco
   POSTGRES_USER=voiceagent
   POSTGRES_PASSWORD=sua-senha-segura-postgres
   POSTGRES_DB=voiceagent

   # Armazenamento de Gravações MinIO
   MINIO_ROOT_USER=voiceagent
   MINIO_ROOT_PASSWORD=sua-senha-segura-minio
   MINIO_BUCKET=recordings

   # Administrador inicial do Painel (criado na primeira subida)
   ADMIN_EMAIL=admin@exemplo.com
   ADMIN_PASSWORD=sua-senha-admin-min-8-chars
   ORG_NAME=Minha Empresa

   # Provedores de Voz
   GEMINI_API_KEY=AIzaSy...
   OPENAI_API_KEY=sk-...    # Opcional: ativa o OpenAI Realtime

   # Porta do Painel
   FRONTEND_PORT=8088
   COOKIE_SECURE=false      # true caso use HTTPS em produção
   LOG_LEVEL=info
   ```

---

## Como Iniciar com Docker

Suba toda a infraestrutura com um único comando:

```bash
docker compose up -d --build
```

Aguarde alguns segundos e acerte o acesso:
- **Painel Web:** [http://localhost:8088](http://localhost:8088)
- **Console do MinIO:** [http://localhost:9001](http://localhost:9001)

Faça login no painel com o `ADMIN_EMAIL` e `ADMIN_PASSWORD` definidos no seu `.env`.

> **Dica:** Caso troque o e-mail ou a senha no `.env` após o banco já ter sido criado, sincronize o usuário com:
> ```bash
> docker compose up -d backend
> docker compose exec backend /server reset-admin
> ```

---

## Como Usar a Plataforma

### 1. Como Criar um Agente
1. Acesse o menu **Agentes** e digite o nome do agente no topo (ex: `Livia - Geap`).
2. Clique em **+ Criar Agente**. Você será direcionado para o editor completo.
3. Configure:
   - **System Prompt:** persona e tom de atendimento (ex: *"Você é a Lívia, assistente virtual da GEAP..."*).
   - **Primeira Mensagem:** saudação inicial usando variáveis (ex: `"{{horario}}, sou a Livia. Como posso ajudar?"`).
   - **Voice Provider:** selecione `Gemini Live` (ou `OpenAI Realtime`).
   - **Modelo e Voz:** escolha a voz desejada do catálogo (ex: `Kore`, `Puck`, `Zephyr`).
4. Clique em **Salvar Draft**.
5. Quando estiver satisfeito, clique em **Publicar** para gerar uma versão numerada (v1, v2...).

### 2. Como Testar uma Conversa por Voz
1. No editor do agente ou na listagem, clique no botão azul **🎙️ Testar Agente**.
2. Escolha testar contra o **Draft (Rascunho atual)** ou uma **Versão Publicada**.
3. Clique em **🎙️ Iniciar Conversa de Voz** e permita o acesso ao microfone.
4. Fale naturalmente com o agente.
5. **Teste a Interrupção (Barge-in):** fale enquanto o agente estiver falando. O agente parará de falar imediatamente e o texto da interrupção será sinalizado na tela.

### 3. Como Adicionar uma Ferramenta (Tool / Webhook)
1. Acesse o menu **Ferramentas** na barra lateral.
2. Clique em **+ Adicionar ferramenta**.
3. Preencha:
   - **Nome:** identificador técnico para o modelo (ex: `consultar_beneficiario`).
   - **Descrição:** instruções de quando o modelo deve chamar (ex: *"Usar quando o cliente solicitar dados do plano ou boleto pelo CPF"*).
   - **URL do Webhook:** endereço HTTP POST (ex: endpoint de webhook do n8n).
   - **Headers de Autenticação:** adicione headers secretos como `Authorization: Bearer <token>`.
   - **Parâmetros de Entrada:** adicione campos esperados (ex: `cpf`, tipo `string`, obrigatório).
4. Utilize o painel **Testar Execução do Webhook** para rodar um teste imediato e verificar o envelope normalizado `{ "success": true, "data": ... }`.
5. Salve a ferramenta. No editor do Agente, marque a ferramenta na seção **Ferramentas da Organização**.

### 4. Como Adicionar um Procedimento (Procedure)
1. No editor do Agente, clique na aba **⇄ Procedimentos**.
2. Clique no botão `+` para criar um novo procedimento (ex: `Segunda Via de Boleto`).
3. Preencha os campos estruturados:
   - **Acionar (Trigger):** descreva em linguagem natural quando o procedimento deve ser acionado (ex: *"Quando o cliente pedir segunda via de boleto, fatura ou código de barras..."*).
   - **Diretrizes (Guardrails):** adicione regras invioláveis de proteção (ex: *"NUNCA leia o código de barras nem valores sem autorização prévia"*).
   - **Conteúdo (Instruções Passo a Passo):** escreva o fluxo em markdown. Use os seletores rápidos para referenciar:
     - `+ Ferramenta` -> insere `@[tool:ID]` (libera a ferramenta para execução apenas durante este procedimento).
     - `+ Procedimento` -> insere `@[procedure:ID]` (expande instruções inline com prevenção de ciclos).
     - `+ Base Conhecimento` -> insere `@[knowledge:ID]` (injeta texto de documentos cadastrados).
4. Clique em **Salvar Procedimento** e depois em **Publicar** no topo do agente.

---

## Como Adicionar Outro Voice Provider

A plataforma foi arquitetada para que a inclusão de um novo Voice Provider não exija alterações no banco de dados, nos Procedures ou no Frontend.

### Passos para adicionar um novo provedor (ex: `MeuNovoProvider`):

1. **Crie o pacote em `backend/internal/voice/meuprovedor`:**
   Implemente a interface `voice.Provider`:
   ```go
   type Provider interface {
       Catalog() voice.Catalog
       Connect(ctx context.Context, cfg voice.SessionConfig) (voice.Session, error)
   }
   ```

2. **Defina o Catálogo (`Catalog`):**
   Informe os modelos disponíveis, vozes, idiomas suportados, sample rates de entrada/saída e duração máxima da sessão.

3. **Implemente a Sessão (`voice.Session`):**
   ```go
   type Session interface {
       SendAudio(pcm []byte) error
       SendText(text string) error
       SendToolResult(call voice.ToolCall, result map[string]any) error
       Events() <-chan voice.Event
       Close() error
   }
   ```
   A sessão deve ler o fluxo de áudio/eventos do provedor e emitir os eventos unificados no canal:
   - `voice.EventAudio` (chunks PCM)
   - `voice.EventCallerTranscript` e `voice.EventAgentTranscript`
   - `voice.EventInterrupted` (interrupção detectada)
   - `voice.EventToolCall` (modelo solicitou chamada de função)
   - `voice.EventTurnComplete`

4. **Valide com a Suite de Conformidade:**
   No arquivo de teste do novo provider, execute:
   ```go
   func TestMeuProviderCatalog(t *testing.T) {
       p := New("minha-chave")
       voice.VerifyCatalogCompliance(t, p.Catalog())
   }
   ```

5. **Registre no Registry do Backend:**
   No arquivo `backend/cmd/server/main.go`, adicione o provider na inicialização:
   ```go
   if cfg.MeuProviderKey != "" {
       provList = append(provList, meuprovedor.New(cfg.MeuProviderKey))
   }
   ```

Pronto! O novo provedor aparecerá automaticamente no menu cascata de Provedor → Modelo → Voz no painel web, e qualquer agente poderá utilizá-lo sem nenhuma outra mudança.

---

## Testes Automatizados

Para rodar todos os testes de unidade e integração (com instâncias descartáveis de Postgres e Redis):

```bash
docker compose --profile test run --rm backend-test sh -c "go vet ./... && go test -count=1 ./..."
docker compose stop postgres-test
```
