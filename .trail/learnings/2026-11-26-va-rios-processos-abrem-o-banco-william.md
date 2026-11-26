---
title: Vários processos abrem o banco e os arquivos de primeira execução ao mesmo tempo
author: William
date: 2026-11-26
branch: main
cost: { minutes: 9, tokens: 26879 }
id: l-1b6a49f3
scope: repo
status: active
src: agent
touches:
  - internal/banco/banco.go
  - internal/config/acesso.go
  - deploy/pos-instalacao.sh
---

## Decided
- `banco.Abrir` holds a file lock around opening and migrating, and first-run files (config.toml, the access key) are written to a temporary file and linked into place. During installation the user's daemon starts (enabled globally) while `palantird diagnostico --instalacao` runs, and the VM failed with "table atividades already exists".
- Anything the daemon creates on first run must assume another palantird command is creating it at the same instant.
In the code: internal/banco/banco.go, internal/config/acesso.go, deploy/pos-instalacao.sh
## Tried and ruled out
- Relying on SQLite's immediate transactions alone: both processes had already read user_version 0 before either migrated.
