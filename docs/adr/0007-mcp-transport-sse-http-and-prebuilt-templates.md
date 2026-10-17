# Conexão MCP via transporte remoto SSE/HTTP e templates guiados

Conexões com MCP Servers na Organization utilizarão o transporte remoto baseado em rede (SSE/HTTP com autenticação via headers), dispensando a execução de subprocessos locais via `stdio` no servidor do backend. A interface oferecerá templates prontos de configuração rápida para serviços populares (n8n, Supabase, PostgreSQL, APIs REST) e um conector genérico para endpoints SSE customizados. Escolhemos isso para garantir isolamento de segurança, escalabilidade em contêineres e evitar que o backend Go precise executar scripts `npx` ou binários arbitrários locais do usuário.

## Considered Options

- Subprocessos locais via `stdio` (ex.: rodando `npx -y @modelcontextprotocol/...`) — rejeitado: grave risco de segurança em ambiente multi-tenant e incompatível com deploys serverless/conteinerizados efêmeros.
- Cadastro manual de ferramentas via formulário JSON Schema sem MCP — rejeitado: ignora a capacidade do protocolo MCP de autodescoberta (`tools/list`) e introspecção dinâmica de schemas.

## Consequences

- Servidores MCP externos precisam expor um endpoint HTTP/SSE acessível pelo backend da plataforma.
- Ferramentas de sistema (System Tools como desligar ou transferir) continuam embutidas nativamente no runtime Go sem depender de servidores MCP.
