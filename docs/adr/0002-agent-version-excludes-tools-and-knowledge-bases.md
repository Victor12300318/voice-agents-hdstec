# Agent Version congela só o que é do Agent; Tools e Knowledge Bases são usadas ao vivo

Publish cria uma Agent Version imutável com System Prompt, First Message, Voice, idioma, modelo, Variables declaradas, Initiation Webhook e Procedures. Tools e Knowledge Bases pertencem à Organization, são compartilhadas entre Agents e referenciadas por ID — editá-las afeta imediatamente todas as Agent Versions publicadas que as referenciam. Escolhemos isso porque uma Tool representa uma integração externa (URL, auth, contrato) cuja mudança normalmente deve valer para todos, sem exigir republicar cada Agent; a reprodutibilidade do histórico fica garantida pelo registro de request/response de cada execução de Tool.

## Consequences

- Mudar o input schema de uma Tool pode quebrar Procedures publicados; a tela da Tool deve listar os Agents que a usam antes de salvar.
- Uma Agent Version não é 100% reproduzível sozinha — Tools/Knowledge Bases do momento não ficam congeladas.
