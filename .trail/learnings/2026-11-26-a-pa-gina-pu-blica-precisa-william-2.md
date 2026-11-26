---
title: A página pública precisa aceitar navegação vinda de um arquivo local
author: William
date: 2026-11-26
branch: main
cost: { minutes: 19, tokens: 35078 }
id: l-70eef637
scope: repo
status: active
replaces: l-0e379fd7
src: agent
touches:
  - internal/servidor/servidor.go
  - internal/cli/abrir.go
  - web/src/api.ts
  - deploy/pos-instalacao.sh
  - scripts/vm/roteiro.sh
---

## Decided
- Allow cross-site requests only for a top-level navigation to the static UI (GET, not /api, Sec-Fetch-Mode navigate, Sec-Fetch-Dest document). The system shortcut redirects Chromium from a private file:// page, which the browser labels cross-site; a real VM login showed "requisição de outro site" instead of the app.
- A key from the URL replaces the stored one only if the daemon accepts it, because any website can navigate to the app with an invented key.
- The search container keeps the default AppArmor profile. Stopping it logs an AppArmor signal denial and takes about 10 s, but it stops.
- Root `podman build` needs the nftables package; with --no-install-recommends it is not pulled in.
In the code: internal/servidor/servidor.go, internal/cli/abrir.go, web/src/api.ts, deploy/busca/palantir-busca.container, deploy/pos-instalacao.sh, scripts/vm/roteiro.sh
## Tried and ruled out
- Passing the access key on the browser command line: other local users can read process arguments.
- Running the search container with AppArmor unconfined to fix a hung reboot: the hang was the virtual GPU, and the container stops without that change.
## Watch out
- In the QEMU VM the virtio GPU does not survive suspend: the display stays off and a later shutdown deadlocks in virtio_gpu (hung task in the kernel log). Check the lock through logind's LockedHint and keep suspend as the last VM step.
- Test packages blank DBUS_SESSION_BUS_ADDRESS so no test reaches the developer's keyring (a default change once wrote test values into it); Podman tests restore testes.BarramentoOriginal.
