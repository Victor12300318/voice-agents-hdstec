# Manter runtime em Go e frontend em React SPA com Tailwind/shadcn

Mantemos o backend em Go para o Voice Agent Runtime e o frontend em React/TypeScript (Vite SPA) com Tailwind CSS e shadcn/ui. Rejeitamos a migração para Python (FastAPI / OpenAI Agents SDK / Google ADK) e Next.js porque o backend Go já implementa com sucesso a normalização bidirecional de áudio PCM16 de baixa latência sobre WebSockets para Gemini Live e OpenAI Realtime, onde a concorrência nativa de goroutines do Go oferece performance superior e menor latência. A UI no estilo ElevenLabs será implementada modernizando a base Vite existente com componentes Tailwind/shadcn.

## Considered Options

- Reescrever o backend em Python com FastAPI e OpenAI Agents SDK / Google ADK — rejeitado: adicionaria latência e overhead de Garbage Collection no streaming de PCM em tempo real, além de descartar uma base Go robusta e testada.
- Migrar o frontend para Next.js — rejeitado: a aplicação é um painel operacional/SPA que depende de WebSockets persistentes de áudio no navegador, sem benefício de Server-Side Rendering.

## Consequences

- SDKs de agentes Python não serão utilizados; integrações com MCPs e Voice Providers continuam sendo tratadas de forma desacoplada no Go via protocolos padrão (MCP e WebSockets).
