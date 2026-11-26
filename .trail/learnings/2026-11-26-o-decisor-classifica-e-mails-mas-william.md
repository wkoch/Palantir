---
title: "O decisor classifica e-mails, mas a dúvida precisa ir para o modelo de conversa"
author: William
date: 2026-11-26
branch: main
cost: { minutes: 35, tokens: 348704 }
id: l-da2d536b
scope: repo
status: active
src: agent
touches:
  - scripts/vm/roteiro.sh
  - ".github/workflows/ci.yml"
  - ".github/workflows/release.yml"
  - AGENTS.md
  - CHANGELOG.md
  - README.md
  - docs/ARQUITETURA.md
  - docs/DECISOES.md
  - docs/DEPENDENCIAS.md
  - docs/DESENVOLVIMENTO.md
---

## Decided
- Triage and the action judge use the decision model (Jev, OpenRouter alpha decisions API); anything below 0.7 class confidence goes to the chat model. In a real run it filed a bill due the same day as junk with low confidence, while the chat model got it right.
- One decision call per email, so text in one message cannot change another message's class.
- Every request demands zero data retention and ignores google-vertex: OpenRouter's `zdr` flag alone still allows "Safety Retention" endpoints on self-serve accounts.
- The chat tiers use the jev-router; the response `model` field tells which model served it.
In the code: internal/email/decidir.go, internal/modelos/decisoes.go, privacidade() in internal/modelos/modelos.go.
## Tried and ruled out
- Marking low-confidence results "em dúvida" in the list: the wrong class was still applied and labelled on the mail server.
- Failing the real injection run on any tool call naming the attacker: a real model searched the local archive for the attacker's name, which reaches nobody.
## Watch out
- The decisions API is alpha; refusals or bad shapes fall back to the chat model, network and budget errors do not.
- Real-model checks: internal/real, gated by PALANTIR_REAL=1; use invented data only.
