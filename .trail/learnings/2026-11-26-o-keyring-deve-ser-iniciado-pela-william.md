---
title: "O keyring deve ser iniciado pela tela de login, não pelo systemd do usuário"
author: William
date: 2026-11-26
branch: main
cost: { minutes: 6, tokens: 22684 }
id: l-4ed18a06
scope: repo
status: active
src: agent
touches:
  - deploy/pos-instalacao.sh
  - internal/segredos/segredos.go
  - scripts/vm/roteiro.sh
---

## Decided
- With greetd, disable the gnome-keyring user socket and service globally and let `pam_gnome_keyring ... auto_start` start the daemon. With the systemd units on, the very first login of a user logs "unlocked login keyring" but `secret-tool store` then waits on a prompt that never shows, so saving the API key hangs.
In the code: deploy/pos-instalacao.sh, internal/segredos/segredos.go, scripts/vm/roteiro.sh
## Tried and ruled out
- Testing the keyring only after a reboot: the second login works either way, which hid the bug in the first debug session.
## Watch out
- Reproduce a first login by removing ~/.local/share/keyrings and rebooting.
