# Áudio do navegador via WebSocket com PCM cru, e sessões de voz em memória em uma única instância

O navegador envia e recebe áudio PCM16 por WebSocket para o backend Go, que repassa ao Voice Provider; a eliminação de eco fica a cargo do `echoCancellation` do navegador e a Interruption é sinalizada por um evento que faz o navegador descartar a fila de reprodução. Escolhemos WebSocket em vez de WebRTC porque o MVP roda em localhost/rede local e WebRTC exigiria STUN/TURN e muito mais complexidade. O estado de cada Conversation ativa (sessão com o provider, Active Procedure, buffer de gravação) vive em memória no processo, o que obriga a uma única instância de backend; Redis guarda apenas sessões de login e contadores.

## Consequences

- Escalar horizontalmente exigirá roteamento sticky por Conversation ou mover o estado de sessão para fora do processo.
- Reinício do backend derruba Conversations ativas e perde suas gravações em buffer.
- Redes ruins (4G, Wi-Fi instável) degradam mais do que degradariam com WebRTC; revisitar ao adicionar canais públicos.
