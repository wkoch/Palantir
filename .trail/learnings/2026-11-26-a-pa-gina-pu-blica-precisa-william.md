---
title: A página pública precisa aceitar navegação vinda de um arquivo local
author: William
date: 2026-11-26
branch: main
cost: { minutes: 53, tokens: 615477 }
id: l-0e379fd7
scope: repo
status: active
src: agent
touches:
  - CHANGELOG.md
  - README.md
  - docs/ARQUITETURA.md
  - docs/PENDENCIAS.md
  - docs/SEGURANCA.md
  - internal/acervo/acervo_test.go
  - internal/acervo/exportar.go
  - internal/cli/cli.go
  - internal/cli/cli_test.go
  - internal/copia/copia.go
---

## Decided
- Allow cross-site requests only for a top-level navigation to the static UI (GET, not /api, Sec-Fetch-Mode navigate, Sec-Fetch-Dest document). The system shortcut redirects Chromium from a private file:// page, which the browser labels cross-site; a real VM login showed "requisição de outro site" instead of the app.
- A key from the URL replaces the stored one only if the daemon accepts it, because any website can navigate to the app with an invented key.
- The search container runs with AppArmor unconfined: on Debian 13 the default container profile denies the signal crun sends to a root container, and reboot hangs. It keeps an unprivileged user, no capabilities, seccomp and a read-only root.
- Root `podman build` needs the nftables package; with --no-install-recommends it is not pulled in.
In the code: internal/servidor/servidor.go, internal/cli/abrir.go, web/src/api.ts, deploy/busca/palantir-busca.container, deploy/pos-instalacao.sh
## Tried and ruled out
- Passing the access key on the browser command line: other local users can read process arguments.
## Watch out
- In the QEMU VM the virtio display stays off after resume from suspend; check the lock through logind's LockedHint, not screenshots.
- Test packages blank DBUS_SESSION_BUS_ADDRESS so no test reaches the developer's keyring (a default change once wrote test values into it); Podman tests restore testes.BarramentoOriginal.
