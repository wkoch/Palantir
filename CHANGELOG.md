# Histórico de versões

## v0.1.0

Primeira versão para provar o Palantír no HP. Implementa as fases P0 a P4, testadas com serviços simulados e numa máquina virtual com Debian 13.

- **Sistema:** pós-instalação para Debian 13, sessão kiosk no tty1 com Cage e Chromium que volta sozinha, console de emergência no tty2, versões instaladas lado a lado com `palantir-versao`.
- **Núcleo:** atividades persistentes em segundo plano, fila de atenção, três níveis de modelo no OpenRouter, orçamento por atividade e mensal, diário de auditoria.
- **Segurança:** ferramentas MCP em Podman sem rede, proxy de saída com lista de hosts, permissões por atividade, fila de confirmação, juiz de ações e suíte de injeção de prompt.
- **Acervo:** originais imutáveis, pasta de entrada, importação e arrastar, extração com OCR, busca textual e semântica, coleções, entidades e exportação.
- **Atividades:** conversa com citações, relatório com fontes e correção por seção, triagem de e-mail que só lê e etiqueta, estudo com questões e repetição espaçada.

Ainda não houve chamada ao OpenRouter real nem prova no HP. Veja [as pendências](docs/PENDENCIAS.md).
