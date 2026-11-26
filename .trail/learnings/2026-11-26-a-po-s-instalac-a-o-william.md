---
title: A pós-instalação reinicia o gerenciador do usuário quando falta o barramento D-Bus
author: William
date: 2026-11-26
branch: main
cost: { minutes: 91, tokens: 1174782 }
id: l-ab1e90c0
scope: repo
status: active
src: agent
touches:
  - cmd/palantir-tool/main_test.go
  - internal/acervo/acervo.go
  - internal/acervo/acervo_test.go
  - internal/acervo/processar.go
  - internal/atividades/gerente_test.go
  - internal/banco/banco_test.go
  - internal/cli/cli_test.go
  - internal/config/config_test.go
  - internal/conversa/conversa_test.go
  - internal/daemon/daemon.go
---

## Decided
- Em `deploy/pos-instalacao.sh`, reiniciar `user@UID.service` se `/run/user/UID/bus` não existir, porque um gerenciador iniciado antes de instalar `dbus-user-session` não cria o barramento e o `podman build` rootless falha com "Interactive authentication required".
## Tried and ruled out
- Apenas `loginctl enable-linger` e `systemctl start user@UID.service`: não basta quando já havia uma sessão aberta antes da instalação dos pacotes.
In the code: deploy/pos-instalacao.sh
