# Visualizadores de Áudio WebGL/Three.js e Componentes ElevenLabs na Test Conversation

Adotamos a biblioteca de componentes sensoriais da ElevenLabs UI (`Orb` 3D WebGL com shaders de ruído Perlin, `LiveWaveform`, `BarVisualizer` e `ShimmeringText`) no palco de teste de voz (`AgentTest`). O `Orb` é portado diretamente para a biblioteca nativa `three` sobre canvas React sem `@react-three/fiber`, garantindo total compatibilidade com React 19 e sem acoplamento ao backend proprietário da ElevenLabs. O visualizador é alimentado em tempo real por dois nós do Web Audio API: o `MediaStream` do microfone do Caller (`AudioRecorder`) e o `AnalyserNode` de decodificação PCM do Agent (`AudioPlayer`).

## Considered Options

- Manter apenas o `VoiceOrb` 2D canvas simples — rejeitado: faltava profundidade tridimensional, feedback de bandas de frequência do áudio e visualização explícita do microfone do Caller.
- Importar `@elevenlabs/react` e o componente de nível mais alto `ConversationBar` — rejeitado: exige conexão WebRTC direta aos servidores fechados da ElevenLabs, violando o princípio soberano do Voice Agent Runtime multi-provider.
- Usar `@react-three/fiber` no React 19 — rejeitado: gera conflitos de dependências de pares e risco de quebra no bundle de produção do Vite.

## Consequences

- Dependência `three` adicionada ao frontend Vite para renderização de shaders WebGL fluidos.
- `AudioRecorder` e `AudioPlayer` expõem medições e fluxos de áudio para reatividade visual instantânea sem latência adicional de processamento.
