---
title: "A escala da tela no kiosk é aplicada pelo compositor, não pelo Chromium"
author: William
date: 2026-11-26
branch: main
cost: { minutes: 91, tokens: 1174782 }
id: l-9c5e5658
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
- Escalar com `wlr-randr --output <saída> --scale` em `deploy/palantir-quiosque`, porque o Cage não tem opção de escala.
## Tried and ruled out
- `chromium --force-device-scale-factor=1.25` com `--ozone-platform=wayland`: a janela é desenhada com metade do tamanho (um quarto da tela), visto na VM Debian 13.
## Watch out
- Na VM (virtio-gpu), o renderizador OpenGL do wlroots mostra um quadro atrasado até haver uma tecla; o DOM está correto. `WLR_RENDERER=pixman` resolve e é aplicado só pelo roteiro em `scripts/vm/roteiro.sh`, porque no hardware real o padrão é preferível.
In the code: deploy/palantir-quiosque, scripts/vm/roteiro.sh
