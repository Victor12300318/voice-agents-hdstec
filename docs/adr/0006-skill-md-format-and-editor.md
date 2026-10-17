# Formato SKILL.md com frontmatter YAML e editor híbrido

Definimos que as Skills são armazenadas no padrão `SKILL.md` contendo frontmatter YAML (com `trigger`, `guardrails` e `tools` necessárias) e corpo em markdown descrevendo o procedimento operacional e os passos (`Step`). Na interface, o editor oferece campos estruturados para metadados essenciais (Trigger e Guardrails) e um editor markdown com autocomplete `@` para `Reference` a Tools da Organization. Escolhemos essa estrutura para garantir interoperabilidade com agentes de IA em formato aberto, ao mesmo tempo em que fornecemos ergonomia visual e validação contra erros de digitação de Tools na UI.

## Considered Options

- Editor de texto puro em markdown/YAML monólito sem validação — rejeitado: propenso a erros de digitação nos nomes de Tools e esquecimento de metadados críticos como Triggers.
- Formulário 100% rígido sem markdown (inputs fixos de passos) — rejeitado: inflexível para procedimentos ricos, tabelas, condicionais e instruções detalhadas que LLMs processam melhor em markdown.

## Consequences

- O backend deve implementar parser e validador de frontmatter YAML para os arquivos `SKILL.md`.
- Tools mencionadas no corpo da Skill devem coincidir com as ferramentas ativas nos servidores MCP da Organization.
