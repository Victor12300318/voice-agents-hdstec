# Procedures são ativados sob demanda por uma System Tool, e References a Procedures são incluídas inline

Voice Providers (Gemini Live, OpenAI Realtime) fixam System Prompt e Tools na abertura da sessão, então a sessão é aberta com o System Prompt + um índice de Triggers, e o modelo chama a System Tool `ativar_procedure` para receber o Procedure Content completo como resultado — mecanismo que depende só de function calling e é igual em qualquer provider. Todas as Tools do Agent são declaradas na abertura, mas o runtime recusa chamadas a Tools que não estejam referenciadas no Active Procedure (System Tools sempre permitidas). Uma Reference a outro Procedure dentro do Procedure Content é expandida inline na ativação (reuso de texto, não navegação); troca por iniciativa do usuário é uma nova ativação que substitui o Active Procedure — não há pilha.

## Considered Options

- Todos os Procedures completos no System Prompt — rejeitado: prompt enorme, regras misturadas, sem saber qual Procedure está ativo e sem controle de Tools.
- Roteador externo que reconecta a sessão com outro prompt — rejeitado: latência e corte de áudio.
- Pilha de Procedures (chamar/retornar) — rejeitado no MVP: exige noção explícita de "fim de Procedure" e o modelo precisa sinalizá-la de forma confiável.

## Consequences

- Inclusão inline precisa de detecção de ciclos e limite de profundidade.
- Fatos já obtidos (ex.: CPF consultado) precisam sobreviver à troca de Active Procedure.
