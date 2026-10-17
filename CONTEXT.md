# Voice Agent Runtime

Plataforma para criar, configurar e executar agentes de voz com IA em tempo real, independente do provedor de voz (Gemini Live primeiro, OpenAI Realtime depois).

## Language

### Agente

**Organization**:
Tenant dono de Agents, Tools e Knowledge Bases; todo dado pertence a exatamente uma Organization.
_Avoid_: Workspace, conta, cliente, tenant, projeto, sub-conta, equipe

**User**:
Pessoa que faz login no painel para configurar e testar Agents; pertence a exatamente uma Organization.
_Avoid_: Usuário (isolado), admin, operador

**Agent**:
Configuração nomeada de um atendente de voz (prompt do sistema, primeira mensagem, voz, idioma, modelo, skills e tools) pertencente a uma Organization.
_Avoid_: Bot, assistente, chatbot

**Draft**:
Estado editável e ainda não publicado de um Agent (com suas Skills); usado apenas em testes, nunca por Conversations normais.
_Avoid_: Rascunho (rótulo de UI), working copy

**Agent Version**:
Snapshot imutável e numerado de um Agent criado ao publicar o Draft; toda Conversation normal usa a Agent Version publicada mais recente.
_Avoid_: Release, revisão, publicação

**Publish**:
Ação que transforma o Draft atual em uma nova Agent Version.
_Avoid_: Deploy, salvar

**System Prompt**:
Instrução global do Agent que define persona, tom e regras válidas em toda a conversa, independente de qual Skill esteja em uso.
_Avoid_: Prompt global, instruções gerais

**First Message**:
Frase que o Agent fala ao iniciar a conversa; se vazia, o Agent aguarda o Caller falar primeiro.
_Avoid_: Saudação, greeting

**Variable**:
Par chave/valor resolvido no início da Conversation e substituído como `{{nome}}` no System Prompt, First Message e Skill Content; fixo durante toda a Conversation.
_Avoid_: Parâmetro, placeholder, estado

**System Variable**:
Variable calculada pela própria plataforma no início da Conversation (ex.: `horario`, `data`, `id_conversa`), usando o fuso horário do Agent.
_Avoid_: Variável padrão, built-in variable

**Dynamic Variable**:
Variable devolvida pelo Initiation Webhook do Agent para uma Conversation específica (ex.: nome do cliente, protocolo).
_Avoid_: Variável customizada, client data

**Initiation Webhook**:
URL configurada no Agent que a plataforma chama ao iniciar cada Conversation para obter as Dynamic Variables.
_Avoid_: Webhook de início, pre-call hook

**Voice**:
Uma voz do catálogo oferecido pelo Voice Provider escolhido para o Agent.
_Avoid_: Locutor, TTS

### Comportamento

**Skill**:
Unidade reutilizável de comportamento (instruções em formato `SKILL.md`, etapas, regras e referências) que o Agent ativa quando relevante na conversa. Ex.: "Segunda via de boleto".
_Avoid_: Procedure, procedimento, fluxo, workflow

**Trigger**:
Texto em linguagem natural de uma Skill descrevendo quando ela deve (e não deve) ser ativada; é o que o Agent enxerga para escolher a Skill.
_Avoid_: Acionar (na UI é o rótulo), condição, intent

**Guardrail**:
Instrução de alta prioridade anexada a uma Skill, entregue ao Agent separada do Skill Content enquanto a Skill está ativa; orienta o comportamento, mas não é uma garantia verificada.
_Avoid_: Diretriz, proteção (rótulos de UI), policy

**Skill Content**:
Corpo em markdown de uma Skill com as instruções passo a passo, podendo conter References.
_Avoid_: Procedure Content, conteúdo, corpo, script

**Reference**:
Menção embutida no Skill Content a outro elemento pronto (Tool, System Tool, Knowledge Base ou outra Skill); as Tools referenciadas são as únicas que a Skill pode usar.
_Avoid_: Link, tag, chip, mention

**Step**:
Etapa numerada dentro do Skill Content.
_Avoid_: Etapa (rótulo de UI), fase, passo

**Active Skill**:
A Skill que o Agent ativou e está seguindo em um dado momento da Conversation.
_Avoid_: Active Procedure, skill corrente, contexto atual

**MCP Server**:
Serviço externo conectado via Model Context Protocol que disponibiliza Tools executáveis para a Organization.
_Avoid_: Integração, plugin, conector, app

**Tool**:
Operação atômica e executável provida por um MCP Server ou nativa da Organization, com input schema, que o Agent pode chamar durante a conversa e devolve dados estruturados. Ex.: `consultar_titulos`.
_Avoid_: Ferramenta (na UI é o rótulo), função, action

**System Tool**:
Tool embutida na plataforma (não configurada por um User) que age sobre a própria Conversation, como encerrar conversa ou transferir para número.
_Avoid_: Ferramenta do sistema (rótulo de UI), built-in

**Knowledge Base**:
Conjunto de documentos de uma Organization que o Agent pode consultar para responder.
_Avoid_: Base de conhecimento (rótulo de UI), RAG, documentos

### Execução

**Conversation**:
Uma sessão de voz entre um Caller e um Agent, do início ao encerramento, com seus eventos e logs técnicos registrados; só começa após as Variables serem resolvidas com sucesso.
_Avoid_: Chamada, sessão, call, log de conversa

**Caller**:
A pessoa que conversa por voz com o Agent em uma Conversation; anônima para a plataforma além do que Initiation Webhook e Tools revelarem.
_Avoid_: Usuário (isolado), cliente, contato, customer

**Test Conversation**:
Conversation executada contra o Draft de um Agent; aparece no histórico, mas fica fora das métricas por padrão.
_Avoid_: Preview, pré-visualização (rótulo de UI), sandbox

**Interruption**:
Momento em que o Caller fala enquanto o Agent está falando, fazendo o Agent parar imediatamente; sempre permitida, exceto durante uma First Message marcada como não interrompível.
_Avoid_: Barge-in, corte

**Response Latency**:
Tempo entre o fim da fala do Caller e o primeiro áudio do Agent em resposta; turnos com chamada de Tool são medidos à parte.
_Avoid_: Latência (isolado), delay, tempo de resposta

**Voice Provider**:
Serviço externo de voz em tempo real que ouve o Caller e fala pelo Agent (ex.: Gemini Live, OpenAI Realtime).
_Avoid_: Engine, LLM, modelo
