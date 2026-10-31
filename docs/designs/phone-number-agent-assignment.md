# Troca e Desvínculo de Agente em Números de Telefone

*   **Author:** victor.felix
*   **Status:** Draft
*   **Date:** 2026-10-31

--------------------------------------------------------------------------------

## Overview

Esta especificação define o gerenciamento do vínculo entre números de telefone cadastrados (`Twilio` e `SIP / Jambonz`) e `Agents` dentro de uma `Organization`. O objetivo é permitir que um `User` atribua, troque ou desvincule o `Agent` conectado a um número existente por meio de um painel lateral (`Sheet` de "Detalhes do número de telefone") na tela de Telefonia, sem perda de configurações existentes do número e sem interromper `Conversations` em andamento.

## The Problem

Hoje, na tela de Telefonia (`frontend/src/pages/PhoneNumbers.tsx`), cada número cadastrado exibe apenas um link estático para o `Agent` vinculado no momento da criação e um botão de exclusão do número:

1. **Impossibilidade de trocar ou desvincular o `Agent` pela interface**: Para mudar qual `Agent` atende um número, o `User` precisa excluir o número (perdendo as credenciais da Twilio salvas no registro) ou reenviar manualmente o formulário de criação com o mesmo número `E.164`.
2. **Acoplamento destrutivo entre `Agent` e número de telefone**: O banco de dados exige que todo número possua obrigatoriamente um `Agent` (`NOT NULL`) e apaga o número de telefone em cascata caso o `Agent` seja excluído (`ON DELETE CASCADE`). Uma `Organization` não consegue manter um número estacionado sem `Agent` durante manutenções ou trocas de operação.
3. **Sobrescrita silenciosa de dados na atualização**: O endpoint atual de atualização de número sobrescreve rótulo, status ativo e provedor caso receba apenas o novo identificador de `Agent`, podendo desativar o número em produção.
4. **Falta de visibilidade e proteção contra `Agents` não publicados**: O `User` consegue vincular um `Agent` inativo ou que possui apenas `Draft` (sem nenhuma `Agent Version` publicada) sem qualquer alerta na interface, e chamadas telefônicas reais caem indevidamente no `Draft` em vez de exigirem uma `Agent Version` publicada.

A solução proposta introduz um painel lateral de detalhes ao clicar em qualquer linha da tabela de Telefonia, onde o `User` pode trocar ou remover o `Agent` com salvamento imediato, visualizando alertas claros caso o `Agent` esteja inativo ou sem versão publicada, enquanto o backend preserva os demais atributos do número e garante que chamadas telefônicas sejam atendidas apenas pela última `Agent Version` publicada.

## Goals

*   Permitir que números de telefone sejam cadastrados ou mantidos sem `Agent` vinculado (`agent_id` nulo), preservando o número caso o `Agent` seja excluído.
*   Exibir na tabela de Telefonia as colunas `Nome`, `Número de telefone`, `Agente` (nome do `Agent` ou estado `"Sem agente"`), `Provedor` e menu de ações (`...` com `"Remover número"`).
*   Abrir o painel lateral (`Sheet`) "Detalhes do número de telefone" ao clicar em uma linha da tabela, permitindo atribuir, trocar ou desvincular o `Agent` com salvamento automático imediato (`onChange`).
*   Preservar todos os campos não editados do número (`label`, `direction`, `provider`, `active` e credenciais Twilio) durante a troca ou desvínculo do `Agent`.
*   Exibir aviso explícito no painel lateral (e no modal de criação) quando o `Agent` selecionado estiver inativo ou não possuir `Agent Version` publicada.
*   Garantir que chamadas telefônicas recebidas sejam atendidas somente pela última `Agent Version` publicada do `Agent` vinculado, rejeitando chamadas caso o número esteja sem `Agent`, com `Agent` inativo ou com `Agent` sem versão publicada.
*   Garantir que `Conversations` telefônicas já em andamento continuem conectadas ao `Agent` anterior até o encerramento da ligação.

## Non-Goals

*   Seleção ou fixação de Ramo (`AgentBranch` / `branch_id`) específico por número de telefone.
*   Disparo de chamadas telefônicas de saída (`outbound`) a partir do painel de detalhes do número.
*   Transferência em tempo real (mid-call) de chamadas já em andamento quando o `Agent` vinculado ao número é alterado.

## Open Questions

*   Nenhuma questão em aberto nesta etapa; todos os requisitos de escopo foram decididos durante a fase de brainstorming.

## Critical User Journeys (CUJs)

### CUJ 1: Trocar o Agent de um número cadastrado pelo painel lateral

*   **Given**: Uma `Organization` possui o número `+551140978278` ("Twilio Ricardo") vinculado ao `Agent` "Atendimento Geral" e outro `Agent` ativo e publicado chamado "Livia - Geap".
*   **When**: O `User` acessa a página de Telefonia, clica na linha do número `+551140978278` para abrir o painel "Detalhes do número de telefone" e seleciona "Livia - Geap" no seletor de `Agent`.
*   **Then**: A alteração é salva automaticamente sem exigir botão de confirmação, a coluna `Agente` na tabela passa a exibir "Livia - Geap", os demais dados do número (`label`, `provider`, `active` e credenciais Twilio) permanecem intactos, chamadas em andamento continuam no `Agent` anterior e novas chamadas para `+551140978278` passam a ser atendidas pela última `Agent Version` publicada de "Livia - Geap".

### CUJ 2: Desvincular o Agent de um número mantendo o número cadastrado

*   **Given**: O número `+5511932875564` ("SIPTRUNK - GEAP") está vinculado a um `Agent` ativo.
*   **When**: O `User` abre o painel "Detalhes do número de telefone" desse número e seleciona a opção `"Nenhum agente (desvinculado)"` no seletor de `Agent`.
*   **Then**: O vínculo é removido automaticamente (`agent_id` passa a vazio/nulo), a tabela exibe `"Sem agente"` na coluna `Agente`, o número e suas configurações permanecem salvos na `Organization`, e qualquer chamada telefônica recebida nesse número é recusada com mensagem padrão de número sem atendente ativo.

### CUJ 3: Vincular um Agent inativo ou sem versão publicada

*   **Given**: A `Organization` possui um `Agent` recém-criado que está apenas em `Draft` (sem `Agent Version` publicada) ou marcado como inativo.
*   **When**: O `User` seleciona esse `Agent` para um número no painel lateral "Detalhes do número de telefone" (ou no modal de adicionar número).
*   **Then**: O sistema permite salvar o vínculo, mas exibe um aviso imediato no painel informando que o `Agent` está inativo ou não possui versão publicada (com link para abrir o editor do `Agent`) e que chamadas telefônicas para este número serão recusadas até que o `Agent` esteja ativo e publicado.

## Technical Plan (Architecture)

A solução atua em três camadas: persistência relacional (`PostgreSQL` / `store`), contratos HTTP e ingress de telefonia (`api`), e interface de gerenciamento (`PhoneNumbers.tsx` + `Sheet.tsx`).

```text
┌────────────────────────────────────────────────────────────────────────────┐
│ Frontend (React 19 — frontend/src/pages/PhoneNumbers.tsx)                  │
│                                                                            │
│  ┌───────────────────────────────────────┐   Clique na linha               │
│  │ Tabela de Telefonia                   │ ──────────────────────────────┐ │
│  │ (Nome | Número | Agente | Provedor |…)│                               ▼ │
│  └───────────────────────────────────────┘  ┌────────────────────────────┐ │
│                                             │ Sheet: Detalhes do Número  │ │
│                                             │ - Cabeçalho E.164 / Nome   │ │
│                                             │ - Select de Agent (onChange│ │
│                                             │   auto-save + opção vazio) │ │
│                                             │ - Alerta: inativo / s/ ver.│ │
│                                             └─────────────┬──────────────┘ │
└───────────────────────────────────────────────────────────┼────────────────┘
                                                            │
                           PUT /api/phone-numbers/{id}      │
                           { "agent_id": "<uuid | ''>" }    ▼
┌────────────────────────────────────────────────────────────────────────────┐
│ Backend Go (backend/internal/api & backend/internal/store)                 │
│                                                                            │
│  1. updatePhoneNumber: carrega registro atual, aplica merge parcial        │
│     apenas nos campos enviados e retorna PhoneNumberWithAgent.             │
│  2. Store: projeta COALESCE(p.agent_id::text, '') e grava                  │
│     NULLIF(agent_id, '')::uuid (FK ON DELETE SET NULL).                    │
│  3. Ingress Telefonia (Twilio / Jambonz):                                  │
│     - Rejeita chamada se agent_id vazio, Agent inativo ou sem versão       │
│       publicada (versionID == nil).                                        │
│     - Congela agent_id e phone_id nos parâmetros do WebSocket da chamada   │
│       em curso, isolando-a de trocas futuras.                              │
└────────────────────────────────────────────────────────────────────────────┘
```

### 1. Modelo de Dados e Persistência (`backend/internal/store`)

*   **Desacoplamento de `agent_id` no PostgreSQL**: Uma nova migration (`00010_phone_numbers_nullable_agent.sql`) remove a restrição `NOT NULL` da coluna `phone_numbers.agent_id` e substitui a chave estrangeira `ON DELETE CASCADE` por `REFERENCES agents(id) ON DELETE SET NULL`. Isso permite cadastrar ou manter números desvinculados (`agent_id IS NULL`) e impede que a exclusão de um `Agent` apague os números telefônicos da `Organization`.
*   **Fronteira de Nulidade no `store`**: As consultas de leitura de `phone_numbers` projetam `COALESCE(p.agent_id::text, '') AS agent_id`, mantendo o campo `AgentID` como string vazia `""` em Go e no JSON quando o valor no banco é `NULL`. As operações de escrita (`CreatePhoneNumber` e `UpdatePhoneNumber`) convertem string vazia para `NULL` via `NULLIF($4, '')::uuid`.
*   **Retorno Unificado com Nome do Agente**: `GetPhoneNumber`, `CreatePhoneNumber` e `UpdatePhoneNumber` retornam `PhoneNumberWithAgent` (incluindo `agent_name` obtido via `LEFT JOIN agents`), mantendo consistência com `ListPhoneNumbers`.
*   **Exposição de `latest_version` na Listagem de Agentes**: `Store.ListAgents` (`GET /api/agents`) passa a incluir o campo `latest_version` (`*int`, calculado via subquery em `agent_versions`), permitindo que o frontend identifique imediatamente se qualquer `Agent` listado possui `Agent Version` publicada sem consultas adicionais por `Agent`.

### 2. Contratos HTTP e Roteamento de Telefonia (`backend/internal/api`)

*   **Atualização Parcial Segura (`PUT /api/phone-numbers/{id}`)**: O payload de atualização passa a aceitar campos opcionais (ponteiros). O handler busca o número existente da `Organization`, aplica apenas os campos efetivamente enviados na requisição (permitindo enviar somente `{"agent_id": "..."}` para trocar ou `{"agent_id": ""}` para desvincular), preserva `label`, `direction`, `provider`, `active` e credenciais Twilio não enviados, registra log estruturado da alteração (`phone_number_updated`) e devolve o `PhoneNumberWithAgent` atualizado.
*   **Criação Opcionalmente Sem Agente (`POST /api/phone-numbers`)**: Remove a exigência de `agent_id` obrigatório no cadastro do número, validando a existência do `Agent` na `Organization` apenas quando `agent_id` for informado.
*   **Proteção no Ingress de Chamadas (`telephony_handlers.go` e `FindAgentByPhoneNumber`)**:
    *   Quando `p.AgentID == ""` (número sem `Agent`), `FindAgentByPhoneNumber` retorna imediatamente erro de número não vinculado sem consultar a tabela `agents` com UUID vazio.
    *   Nos webhooks de entrada (`handleTwilioWebhook` e `handleJambonzWebhook`), além de validar que o número e o `Agent` estão ativos, o servidor verifica se a resolução de versão retornou uma `Agent Version` publicada (`versionID != nil`). Caso o `Agent` possua apenas `Draft` (`versionID == nil`), a chamada é recusada no webhook com mensagem padrão, nunca executando o `Draft` em ligações telefônicas.
    *   Para chamadas já conectadas (`handleTwilioAudioWS` e `handleJambonzAudioWS`), o `agent_id` e o `phone_id` permanecem fixados nos parâmetros de inicialização do stream WebSocket; o fallback de leitura de credenciais Twilio passa a consultar o número diretamente sem exigir `Agent` ativo, garantindo que trocas ou desvínculos feitos durante uma ligação não afetem a `Conversation` em curso.

### 3. Interface de Usuário (`frontend/src/pages/PhoneNumbers.tsx`)

*   **Tabela de Números**: Exibe as colunas `Nome` (usando `label` ou fallback visual para número sem nome), `Número de telefone` (`e164`), `Agente` (badge com `agent_name` ou `"Sem agente"`), `Provedor` (`Twilio` ou `SIP Trunk`) e um botão de menu (`...`) na extremidade direita contendo a ação `"Remover número"` (com `e.stopPropagation()` para não abrir o painel lateral).
*   **Painel Lateral (`Sheet`) "Detalhes do número de telefone"**:
    *   Controlado por `selectedPhoneId: string | null` (derivando os dados diretamente da lista `numbers` carregada).
    *   Exibe no topo o número `E.164` (com botão de copiar), o rótulo (`label`), o identificador do número e um menu `...` para remover o número.
    *   Apresenta o card **Agente** com um `<select>` contendo a opção `"Nenhum agente (desvinculado)"` (`""`) e todos os `Agents` da `Organization`.
    *   Ao alterar o valor no `<select>` (`onChange`), dispara imediatamente `PUT /api/phone-numbers/{id}` com `{"agent_id": novoValor}`, bloqueando o seletor enquanto a requisição está em curso, atualizando o item correspondente em `numbers` ao concluir e exibindo feedback visual de salvamento (ou erro em caso de falha).
    *   Exibe um alerta contextual dentro do card sempre que o `Agent` selecionado estiver com `active === false` ou `latest_version === null`, incluindo link direto para `/agents/{agent_id}`.

## Alternatives Considered

### Alternative A: Criar um novo endpoint dedicado `PATCH /api/phone-numbers/{id}/agent`

*   **Pros**: Isola a mutação do vínculo de `Agent` em uma rota separada sem alterar a assinatura da rota `PUT /api/phone-numbers/{id}`.
*   **Cons**: Mantém o endpoint existente `PUT /api/phone-numbers/{id}` vulnerável a perda de dados caso seja chamado com payloads parciais e duplica a lógica de validação de propriedade da `Organization` e recarregamento de `agent_name`.
*   **Reason for Rejection**: Rejeitado porque corrigir `PUT /api/phone-numbers/{id}` para aplicar merge parcial sobre o registro existente resolve a falha estrutural na rota já registrada em `backend/internal/api/server.go` sem criar endpoints redundantes.

### Alternative B: Consultar `GET /api/agents/{id}` sob demanda no frontend para verificar versão publicada

*   **Pros**: Não exige nenhuma alteração na query SQL de `Store.ListAgents` (`GET /api/agents`).
*   **Cons**: O endpoint `GET /api/agents/{id}` executa múltiplas consultas adicionais (branches, snapshots de versões, procedures e cálculo de diferenças do `Draft`). Chamá-lo na tela de Telefonia introduz latência extra ao trocar de `Agent` ou gera N+1 requisições se executado para toda a lista.
*   **Reason for Rejection**: Rejeitado porque adicionar `latest_version` via subquery leve em `Store.ListAgents` entrega o dado para todos os `Agents` da `Organization` na mesma requisição inicial que a página já realiza.

### Alternative C: Alterar `PhoneNumber.AgentID` para `*string` em todas as structs Go e tipos TypeScript

*   **Pros**: Representa o valor `NULL` do banco diretamente como `null` no payload JSON.
*   **Cons**: Exige alterar a tipagem e verificações em todos os consumidores de `PhoneNumber` no backend e no frontend, sendo que o frontend (`frontend/src/pages/PhoneNumbers.tsx`) já trata string vazia `""` como ausência de vínculo (`n.agent_id ? ...`).
*   **Reason for Rejection**: Rejeitado porque projetar `COALESCE(p.agent_id::text, '')` na leitura e gravar `NULLIF($4, '')::uuid` na escrita preserva total compatibilidade com os contratos JSON existentes enquanto garante integridade referencial `uuid NULL` no PostgreSQL.

### Alternative D: Ocultar a coluna `Agente` na tabela principal e exigir botão "Salvar" manual no `Sheet`

*   **Pros**: Deixa a tabela com apenas três colunas (`Nome`, `Número de telefone` e `Provedor`) e evita requisições imediatas no `onChange` do `<select>`.
*   **Cons**: Ocultar a coluna `Agente` obriga o `User` a abrir o painel lateral linha por linha para descobrir qual `Agent` atende cada número ou quais números estão sem `Agent`. Exigir botão "Salvar" dentro de um `Sheet` que fecha ao clicar fora ou pressionar `Escape` causa perda acidental de alterações não salvas.
*   **Reason for Rejection**: Rejeitado em favor de manter a coluna `Agente` visível na tabela e aplicar salvamento automático imediato no `onChange` do seletor dentro do `Sheet`.

--------------------------------------------------------------------------------

## Non-Functional Requirements

*   **Security & Multi-Tenancy**: Todas as consultas e atualizações em `phone_numbers` e `agents` devem filtrar obrigatoriamente por `organization_id` extraído da sessão autenticada (`auth.FromContext`). O `TwilioAuthToken` nunca deve ser serializado nas respostas JSON (`json:"-"`) nem sobrescrito com vazio quando omitido no `PUT /api/phone-numbers/{id}`.
*   **Performance & Database Load**: A verificação de versão publicada dos `Agents` deve ocorrer na própria listagem `GET /api/agents` sem introduzir consultas N+1 por `Agent` ou por número de telefone.
*   **Operational Observability**: Toda alteração de vínculo em `PUT /api/phone-numbers/{id}` deve emitir log estruturado (`phone_number_updated`) contendo `org_id`, `user_id`, `phone_id`, `e164`, `previous_agent_id` e `new_agent_id`. Rejeições de chamadas no ingress (`Twilio` e `Jambonz`) devem registrar o motivo específico (número não encontrado, número sem `Agent`, `Agent` inativo ou `Agent` sem `Agent Version` publicada).
*   **Rollout & Rollback Safety**: A migration `00010_phone_numbers_nullable_agent.sql` é aditiva e não bloqueia leituras; em caso de rollback (`-- +goose Down`), registros com `agent_id IS NULL` devem ser tratados antes de restaurar a restrição `NOT NULL`.

--------------------------------------------------------------------------------

### Additional Resources

*   Vocabulário e regras do domínio: [CONTEXT.md](../../CONTEXT.md)
*   Briefing de ideação e revisão Goldfish: [concepts_brief.md](../../concepts_brief.md)

### Future Improvements

*   **Seleção de Ramo (`AgentBranch`) por número de telefone**: Permitir fixar opcionalmente um `branch_id` específico no painel "Detalhes do número de telefone" para direcionar chamadas de um número de homologação a uma branch específica do `Agent`.
*   **Disparo de chamadas de saída (`outbound`)**: Habilitar o botão de chamada de saída no cabeçalho do painel lateral para iniciar ligações de teste ou ativas a partir do número selecionado.

--------------------------------------------------------------------------------

## Detailed Implementation Plan

O trabalho está dividido em duas fases sequenciais (Fase 1: Banco de Dados, Store e API Go; Fase 2: Contrato TypeScript e Interface React), garantindo que o repositório compile e passe nos testes a cada etapa.

### Target Files to Create/Modify

#### 1. `backend/internal/store/migrations/00010_phone_numbers_nullable_agent.sql` ([Create])

*   **Type / Scope**: Migration SQL (`goose`).
*   **Rationale**: Remover a obrigatoriedade (`NOT NULL`) e a exclusão em cascata (`ON DELETE CASCADE`) de `phone_numbers.agent_id` definidas originalmente em `backend/internal/store/migrations/00005_phone_numbers.sql`, permitindo que números fiquem sem `Agent` vinculado e que a exclusão de um `Agent` apenas limpe o vínculo (`ON DELETE SET NULL`).
*   **Proposed Changes**:
    *   Bloco `-- +goose Up`:
        *   Remover a constraint `phone_numbers_agent_id_fkey` (`ALTER TABLE phone_numbers DROP CONSTRAINT IF EXISTS phone_numbers_agent_id_fkey;`).
        *   Remover `NOT NULL` da coluna `agent_id` (`ALTER TABLE phone_numbers ALTER COLUMN agent_id DROP NOT NULL;`).
        *   Adicionar nova constraint `phone_numbers_agent_id_fkey` referenciando `agents(id) ON DELETE SET NULL`.
    *   Bloco `-- +goose Down`:
        *   Remover a constraint `phone_numbers_agent_id_fkey`.
        *   Remover registros órfãos (`DELETE FROM phone_numbers WHERE agent_id IS NULL;`) antes de restaurar `SET NOT NULL`.
        *   Restaurar `ALTER TABLE phone_numbers ALTER COLUMN agent_id SET NOT NULL` e recriar `phone_numbers_agent_id_fkey` com `REFERENCES agents(id) ON DELETE CASCADE`.

#### 2. `backend/internal/store/agents.go` ([Modify])

*   **Type / Scope**: Camada de Persistência Go (`store`).
*   **Rationale**: Expor `LatestVersion *int` (`json:"latest_version"`) diretamente em `store.Agent` e na listagem `Store.ListAgents` para que o frontend identifique em uma única consulta se o `Agent` possui `Agent Version` publicada.
*   **Proposed Changes**:
    *   Adicionar o campo `LatestVersion *int` com tag `json:"latest_version"` na struct `Agent`.
    *   Atualizar a constante `agentCols` para incluir `(SELECT MAX(v.version) FROM agent_versions v WHERE v.agent_id = agents.id) AS latest_version` ao final da lista de colunas.
    *   Atualizar `scanAgent` para incluir `&a.LatestVersion` como último argumento de `r.Scan(...)`.

#### 3. `backend/internal/api/agents_handlers.go` ([Modify])

*   **Type / Scope**: Handler HTTP Go (`api`).
*   **Rationale**: Evitar duplicidade de campo serializado entre `agentDetail` e a struct embutida `store.Agent` após a adição de `LatestVersion *int` em `store.Agent`.
*   **Proposed Changes**:
    *   Remover a declaração duplicada `LatestVersion *int` da struct `agentDetail` (mantendo o acesso `d.LatestVersion` promovido de `d.Agent.LatestVersion`).

#### 4. `backend/internal/store/phone_numbers.go` ([Modify])

*   **Type / Scope**: Camada de Persistência Go (`store`).
*   **Rationale**: Adaptar todas as leituras e escritas de `phone_numbers` para suportar `agent_id` nulo no PostgreSQL mantendo `AgentID` como `string` (`""` quando nulo), impedir consultas de `uuid` vazio em `FindAgentByPhoneNumber`, permitir atualização parcial segura e retornar `PhoneNumberWithAgent` com `agent_name` nas operações de leitura/escrita.
*   **Proposed Changes**:
    *   Definir erro sentinela `ErrPhoneNumberUnassigned = errors.New("phone_number_unassigned")`.
    *   Definir struct `UpdatePhoneNumberParams` com campos ponteiros opcionais: `Label *string`, `AgentID *string`, `Direction *string`, `Provider *string`, `TwilioAccountSid *string`, `TwilioAuthToken *string`, `TwilioPhoneSid *string`, `Active *bool`.
    *   Em `phoneCols`, substituir `p.agent_id` por `COALESCE(p.agent_id::text, '') AS agent_id`.
    *   Em `phoneColsPlain`, substituir `agent_id` por `COALESCE(agent_id::text, '') AS agent_id`.
    *   Em `GetPhoneNumber`, incluir `COALESCE((SELECT a.name FROM agents a WHERE a.id = p.agent_id), '') AS agent_name` (ou retornar `PhoneNumberWithAgent` / manter helper `GetPhoneNumberWithAgent(ctx, orgID, id string) (PhoneNumberWithAgent, error)`).
    *   Em `CreatePhoneNumber`, usar `NULLIF($4, '')::uuid` na inserção de `agent_id` e no `ON CONFLICT (organization_id, e164) DO UPDATE SET ... agent_id = EXCLUDED.agent_id`, permitindo criar número com `in.AgentID == ""`. Retornar `PhoneNumberWithAgent` (buscando via `GetPhoneNumberWithAgent` após o `INSERT ... RETURNING`).
    *   Em `UpdatePhoneNumber(ctx context.Context, orgID, id string, in UpdatePhoneNumberParams) (PhoneNumberWithAgent, error)`:
        *   Carregar o registro atual via `GetPhoneNumber(ctx, orgID, id)` (retornando `ErrNotFound` se não pertencer à `Organization`).
        *   Aplicar merge apenas dos campos não-nulos de `in` sobre os valores atuais:
            *   Se `in.Label != nil`, atualizar `label = *in.Label`.
            *   Se `in.AgentID != nil`, atualizar `agentID = strings.TrimSpace(*in.AgentID)` (onde `""` será gravado como `NULL` no banco via `NULLIF($4, '')::uuid`).
            *   Se `in.Direction != nil && *in.Direction != ""`, atualizar `direction = *in.Direction`.
            *   Se `in.Provider != nil && *in.Provider != ""`, atualizar `provider = *in.Provider`.
            *   Se `in.Active != nil`, atualizar `active = *in.Active`.
            *   Para `TwilioAccountSid`, `TwilioAuthToken` e `TwilioPhoneSid`, preservar o valor atual quando o ponteiro for `nil` ou string vazia `""`.
        *   Executar o `UPDATE` com `NULLIF($4, '')::uuid` para `agent_id` e retornar o `PhoneNumberWithAgent` atualizado.
    *   Adicionar método `GetPhoneNumberByE164(ctx context.Context, orgID, e164 string) (PhoneNumber, error)` que busca o registro em `phone_numbers` por `organization_id` e `e164` normalizado sem exigir que `agent_id` esteja preenchido, para uso no fallback de credenciais Twilio de chamadas em curso.
    *   Em `FindAgentByPhoneNumber`:
        *   Logo após `scanPhone(row)`, verificar `if p.AgentID == "" { return p, Agent{}, fmt.Errorf("phone number %s has no assigned agent: %w", e164, ErrPhoneNumberUnassigned) }` antes de chamar `s.GetAgent`.

#### 5. `backend/internal/api/phone_numbers_handlers.go` ([Modify])

*   **Type / Scope**: Handlers HTTP Go (`api`).
*   **Rationale**: Permitir cadastro de número sem `Agent`, suportar payload parcial em `PUT /api/phone-numbers/{id}` sem sobrescrever campos omitidos, validar `agent_id` contra a `Organization` quando enviado, registrar log de auditoria e devolver `PhoneNumberWithAgent`.
*   **Proposed Changes**:
    *   Em `createPhoneNumber`:
        *   Remover o bloqueio obrigatório `if req.AgentID == ""` (permitindo `req.AgentID == ""`).
        *   Executar a verificação `s.Store.GetAgent(r.Context(), orgID, req.AgentID)` somente quando `strings.TrimSpace(req.AgentID) != ""`.
    *   Alterar `updatePhoneNumberRequest` para usar ponteiros:
        *   `Label *string json:"label"`
        *   `AgentID *string json:"agent_id"`
        *   `Direction *string json:"direction"`
        *   `Provider *string json:"provider"`
        *   `TwilioAccountSid *string json:"twilio_account_sid"`
        *   `TwilioAuthToken *string json:"twilio_auth_token"`
        *   `TwilioPhoneSid *string json:"twilio_phone_sid"`
        *   `Active *bool json:"active"`
    *   Em `updatePhoneNumber`:
        *   Validar se `id` é UUID válido (`isUUID(id)`).
        *   Carregar o estado anterior via `s.Store.GetPhoneNumber(r.Context(), orgID, id)` (usando `s.notFoundOr(w, r, err)` em caso de erro).
        *   Se `req.AgentID != nil && strings.TrimSpace(*req.AgentID) != ""`, validar que o `Agent` existe na `Organization` via `s.Store.GetAgent(r.Context(), orgID, strings.TrimSpace(*req.AgentID))`, respondendo `HTTP 400` (`"agente não encontrado"`) se não existir.
        *   Chamar `s.Store.UpdatePhoneNumber(r.Context(), orgID, id, store.UpdatePhoneNumberParams{...})`.
        *   Registrar log estruturado `s.Log.Info("phone_number_updated", "org_id", orgID, "user_id", sess.UserID, "phone_id", id, "e164", updated.E164, "previous_agent_id", prev.AgentID, "new_agent_id", updated.AgentID)`.
        *   Responder `HTTP 200` com `updated` (`store.PhoneNumberWithAgent`).

#### 6. `backend/internal/api/telephony_handlers.go` ([Modify])

*   **Type / Scope**: Handlers de Ingress de Telefonia e WebSocket (`api`).
*   **Rationale**: Garantir que chamadas telefônicas (`Twilio` e `Jambonz`) atendam estritamente com a última `Agent Version` publicada (rejeitando chamadas quando `versionID == nil` em vez de usar o `Draft`) e que chamadas em andamento não percam acesso às credenciais da Twilio caso o número seja desvinculado durante a ligação.
*   **Proposed Changes**:
    *   Em `handleJambonzWebhook`:
        *   Resolver a configuração da chamada com `cfg, versionID, err := s.resolveAgentConfigForCall(r.Context(), phone.OrganizationID, agent, req.CallSid)`.
        *   Se `err != nil || versionID == nil`, registrar `s.Log.Warn("jambonz_agent_no_published_version", "to", req.To, "agent_id", agent.ID, "error", err)`, responder `HTTP 200` com `telephony.BuildRejectApp("O número chamado não está associado a nenhum atendente de voz ativo no momento.")` e encerrar o handler antes de abrir o WebSocket.
        *   Incluir `"phone_id", phone.ID, "agent_id", agent.ID, "version_id", *versionID` no log `jambonz_routing_call`.
    *   Em `handleJambonzAudioWS`:
        *   Após `cfg, versionID, err := s.resolveAgentConfigForCall(ctx, orgID, ag, callSid)`, verificar `if err != nil || versionID == nil`, fechando o WebSocket com erro se nenhuma versão publicada existir.
    *   Em `handleTwilioWebhook`:
        *   Resolver a configuração da chamada com `cfg, versionID, err := s.resolveAgentConfigForCall(r.Context(), phone.OrganizationID, agent, callSid)`.
        *   Se `err != nil || versionID == nil`, registrar `s.Log.Warn("twilio_agent_no_published_version", "to", to, "agent_id", agent.ID, "error", err)`, responder `HTTP 200` (`text/xml`) com `telephony.BuildTwilioRejectTwiML("O número chamado não está associado a nenhum atendente de voz ativo no momento.")` e encerrar o handler.
        *   Incluir `"phone_id", phone.ID, "agent_id", agent.ID, "version_id", *versionID` no log `twilio_routing_call`.
    *   Em `handleTwilioAudioWS`:
        *   No fallback `else if did != ""` (quando `phoneID` está vazio), substituir `s.Store.FindAgentByPhoneNumber(ctx, did)` por `s.Store.GetPhoneNumberByE164(ctx, orgID, did)` para ler `TwilioAccountSid` e `TwilioAuthToken` mesmo que o número tenha sido desvinculado durante a chamada.
        *   Após `cfg, versionID, err := s.resolveAgentConfigForCall(ctx, orgID, ag, callSid)`, verificar `if err != nil || versionID == nil`, fechando o WebSocket se nenhuma versão publicada existir.

#### 7. `backend/internal/store/phone_numbers_test.go` ([Modify])

*   **Type / Scope**: Testes Automatizados Go (`store`).
*   **Rationale**: Cobrir com testes automatizados o ciclo de vida de números com e sem `Agent`, atualização parcial sem perda de campos, `ON DELETE SET NULL` ao excluir `Agent` e rejeição em `FindAgentByPhoneNumber` para números desvinculados.
*   **Proposed Changes**:
    *   Adicionar teste de integração verificando:
        1. Criação de número sem `Agent` (`AgentID: ""`) e leitura via `ListPhoneNumbers` / `GetPhoneNumber` sem erro de scan `NULL`.
        2. Atualização parcial via `UpdatePhoneNumber` alterando apenas `AgentID` (de vazio para Agente A, de Agente A para Agente B, e de Agente B para `""`) verificando que `Label`, `Provider` (`"twilio"`), `Direction` (`"both"`), `Active` (`true`) e `TwilioAccountSid` permanecem inalterados.
        3. Comportamento de `FindAgentByPhoneNumber` retornando `ErrPhoneNumberUnassigned` quando `AgentID == ""`.
        4. Exclusão do `Agent` mantendo o registro em `phone_numbers` com `AgentID == ""`.

#### 8. `frontend/src/api.ts` ([Modify])

*   **Type / Scope**: Contratos TypeScript de API (`frontend`).
*   **Rationale**: Refletir o campo `latest_version` no tipo `Agent` retornado por `GET /api/agents` e tipar o payload parcial de atualização de número telefônico.
*   **Proposed Changes**:
    *   Adicionar `latest_version?: number | null` ao tipo `Agent`.
    *   Adicionar o tipo `PhoneNumberUpdatePayload`:
        *   `label?: string`
        *   `agent_id?: string`
        *   `direction?: string`
        *   `provider?: string`
        *   `twilio_account_sid?: string`
        *   `twilio_auth_token?: string`
        *   `twilio_phone_sid?: string`
        *   `active?: boolean`

#### 9. `frontend/src/pages/PhoneNumbers.tsx` ([Modify])

*   **Type / Scope**: Página React de Gerenciamento de Telefonia (`frontend`).
*   **Rationale**: Implementar a listagem com coluna `Agente` e menu `...`, o painel lateral (`Sheet`) "Detalhes do número de telefone" com troca e desvínculo automático de `Agent` (`onChange`), e os avisos para `Agent` inativo ou sem versão publicada.
*   **Proposed Changes**:
    *   **Estados Adicionais**:
        *   `selectedPhoneId: string | null` (identificador do número aberto no `Sheet`; o objeto `selectedPhone` é derivado de `numbers.find((n) => n.id === selectedPhoneId)`).
        *   `menuOpenId: string | null` (controle do dropdown `...` na linha da tabela ou no cabeçalho do `Sheet`).
        *   `updatingAgent: boolean` (bloqueia o `<select>` do `Sheet` enquanto `PUT /api/phone-numbers/{id}` está em curso).
        *   `sheetFeedback: { type: 'saved' | 'error'; message: string } | null` (feedback de auto-save no `Sheet`).
    *   **Tabela Principal**:
        *   Colunas: `Nome`, `Número de telefone`, `Agente`, `Provedor` e coluna de ações (`...`).
        *   Clicar na linha (`<tr>`) define `setSelectedPhoneId(n.id)` e abre o `Sheet`.
        *   Na coluna `Nome`: exibir `n.label` ou fallback `"Sem nome"`.
        *   Na coluna `Número de telefone`: exibir `n.e164` em fonte monoespaçada.
        *   Na coluna `Agente`: exibir o nome do `Agent` (`n.agent_name`) com ícone `Bot` quando `n.agent_id` estiver preenchido, ou badge `"Sem agente"` quando `n.agent_id` for vazio.
        *   Na coluna `Provedor`: exibir badge `Twilio` ou `SIP Trunk`.
        *   Na coluna direita (`...`): botão `MoreHorizontal` com `e.stopPropagation()` abrindo menu com as opções `"Detalhes do número"` (abre o `Sheet`) e `"Remover número"` (chama `handleDelete(n.id, n.e164)` com confirmação `"Remover permanentemente o número ...?"`).
    *   **Painel Lateral (`Sheet`) "Detalhes do número de telefone"**:
        *   Utilizar o componente `frontend/src/components/Sheet.tsx` (`open={Boolean(selectedPhone)}`, `onClose={() => setSelectedPhoneId(null)}`, `side="right"`, `sizeClass="max-w-lg"`).
        *   Cabeçalho com título `"Detalhes do número de telefone"` e botão `X`.
        *   Bloco de resumo do número: `selectedPhone.e164` (com botão de copiar E.164), `selectedPhone.label || 'Sem nome'`, `selectedPhone.id` em fonte monoespaçada secundária e botão `...` para remover o número.
        *   Card **Agente**:
            *   Título `"Agente"` e descrição `"Atribua um agente para atender chamadas para este número de telefone."`.
            *   Elemento `<select>` com `value={selectedPhone.agent_id || ''}`, `disabled={updatingAgent}`, contendo `<option value="">Nenhum agente (desvinculado)</option>` seguido por todos os itens de `agents`.
            *   Handler `handleChangePhoneAgent(phoneId: string, newAgentId: string)` acionado no `onChange`: envia `PUT /api/phone-numbers/${phoneId}` com `JSON.stringify({ agent_id: newAgentId })`, atualiza o item em `numbers` com a resposta (`PhoneNumberWithAgent`) e exibe indicador `"Salvo"` por 2 segundos (ou mensagem de erro em caso de falha).
            *   Bloco de aviso condicional dentro do card:
                *   Se `selectedPhone.agent_id === ''`: nota informativa indicando que o número está sem `Agent` e chamadas recebidas serão recusadas.
                *   Se o `Agent` selecionado em `agents` tiver `active === false`: alerta âmbar informando que o `Agent` está inativo e chamadas serão recusadas até sua ativação, com link para `/agents/${selectedPhone.agent_id}`.
                *   Se o `Agent` selecionado em `agents` tiver `latest_version == null`: alerta âmbar informando que o `Agent` ainda não possui nenhuma `Agent Version` publicada e chamadas telefônicas serão recusadas até que uma versão seja publicada, com link para `/agents/${selectedPhone.agent_id}`.
    *   **Modal "Conectar Número" (`handleCreate`)**:
        *   Incluir `<option value="">Nenhum agente (desvinculado)</option>` no `<select>` de `Agent` do modal de criação, remover a obrigatoriedade de `agentId` no botão de submit (`disabled={saving || !e164.trim()}`) e exibir o mesmo alerta caso o `Agent` selecionado no modal esteja inativo ou sem versão publicada.

### Verification Plan

*   **Compilation/Build (Backend & Frontend)**:
    *   Backend Go: `cd backend && go vet ./... && go build ./...`
    *   Frontend TypeScript/Vite: `cd frontend && npm run build`
*   **Unit & Integration Tests**:
    *   Suite completa do backend (com Postgres e Redis de teste via Docker Compose):
        `docker compose --profile test run --rm backend-test sh -c "go vet ./... && go test -count=1 ./..."`
*   **Manual / E2E Verification Criteria**:
    1. Abrir a tela de Telefonia (`/phone-numbers`) e verificar as colunas `Nome`, `Número de telefone`, `Agente`, `Provedor` e menu `...`.
    2. Clicar em um número cadastrado, verificar a abertura do `Sheet` "Detalhes do número de telefone", trocar o `Agent` no seletor e confirmar que o `PUT /api/phone-numbers/{id}` retorna `200 OK` atualizando imediatamente a coluna `Agente` e preservando `label`, `provider` e `active`.
    3. Selecionar `"Nenhum agente (desvinculado)"` no `Sheet` e confirmar que o número permanece listado como `"Sem agente"`.
    4. Selecionar um `Agent` sem versão publicada (`latest_version === null`) ou inativo e confirmar a exibição do aviso com link para o editor do `Agent`.

--------------------------------------------------------------------------------

<!-- Provenance: Indicates initial structural drafting via EGM. -->

*Drafted with EGM*
