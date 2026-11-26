---
title: Testes de importação nunca usam caminhos reais do disco
author: William
date: 2026-11-26
branch: main
cost: { minutes: 91, tokens: 1174782 }
id: l-7890fcf5
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
- A recusa da raiz é testada só pela regra (`Contem` e contexto cancelado em `internal/acervo/acervo_test.go`), e a conferência vive em `ImportarCaminho`, valendo para interface, CLI e pasta de entrada, porque a pasta importada pode ser enviada aos modelos.
## Tried and ruled out
- Comparar prefixos de caminho no servidor HTTP: `/` virava `//`, a recusa não valia e um teste que esperava erro copiou 13 GB do disco para uma pasta temporária em `/tmp`.
In the code: internal/acervo/acervo.go, internal/acervo/acervo_test.go, internal/servidor/acervo.go
