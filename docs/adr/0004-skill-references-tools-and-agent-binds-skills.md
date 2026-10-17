# Skills referenciam Tools diretamente e o Agent vincula apenas Skills

A Organization conecta MCP Servers que catalogam Tools; cada Skill declara em seu conteúdo (via Reference) as Tools pontuais que pode executar, e o Agent vincula exclusivamente as Skills que sabe desempenhar. O runtime resolve o conjunto total de Tools autorizadas para a sessão a partir das Skills vinculadas ao Agent, dispensando a seleção manual de MCPs ou Tools na tela do Agent. Escolhemos isso para evitar dupla checagem na interface e proteger o Voice Provider contra alucinações e degradação de latência causadas pela exposição massiva de ferramentas não delimitadas.

## Considered Options

- Agent seleciona MCPs inteiros e expõe todas as tools para todas as skills — rejeitado: degrada latência no Voice Provider e aumenta risco de chamadas indevidas.
- Dupla checagem (Agent autoriza MCPs e Skill declara Tools) — rejeitado: fricção operacional e falhas silenciosas na UI quando faltar um vínculo de MCP.

## Consequences

- Uma Skill não pode ser executada por um Agent se o MCP Server correspondente for desativado na Organization.
- A tela de criação/edição do Agent foca em habilidades de negócio, enquanto o catálogo de MCPs fica centralizado no nível da Organization.
