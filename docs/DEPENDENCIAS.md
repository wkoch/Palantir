# Dependências

Toda dependência nova entra aqui, com a procedência e onde roda. As versões exatas e as somas estão em `go.mod`, `go.sum` e `web/package-lock.json`.

## Compiladas no daemon e nas ferramentas (módulos Go, via proxy.golang.org)

| Módulo | Para quê | Onde roda | Licença |
| --- | --- | --- | --- |
| `modernc.org/sqlite` | SQLite com FTS5, sem CGO | daemon | BSD-3-Clause |
| `github.com/coder/websocket` | Avisos em tempo real | daemon | ISC |
| `github.com/BurntSushi/toml` | Configuração | daemon | MIT |
| `github.com/santhosh-tekuri/jsonschema/v6` | Validação das telas e das respostas dos modelos | daemon | Apache-2.0 |
| `github.com/modelcontextprotocol/go-sdk` | MCP (cliente e servidores) | daemon e contêiner | Apache-2.0 / MIT |
| `golang.org/x/term` | Ler segredos sem eco | daemon | BSD-3-Clause |
| `golang.org/x/net` | HTML e conjuntos de caracteres | contêiner | BSD-3-Clause |
| `github.com/emersion/go-imap/v2` (beta) | IMAP; o servidor em memória serve ao simulador | contêiner, simulador | MIT |
| `github.com/emersion/go-message` | Mensagens MIME | contêiner | MIT |

O `go-imap` v2 ainda é beta: ao atualizar, rode os testes de `internal/mcpimap`.

## Interface (npm; compiladas no build, sem Node na máquina)

| Pacote | Para quê | Onde roda | Licença |
| --- | --- | --- | --- |
| `lit` | Componentes web | navegador | BSD-3-Clause |
| `esbuild` | Empacotamento | desenvolvimento e CI | MIT |
| `typescript` | Conferência de tipos | desenvolvimento e CI | Apache-2.0 |
| `@playwright/test` (baixa o Chromium de teste) | Testes de navegador | desenvolvimento e CI | Apache-2.0 |
| `monocart-coverage-reports` | Cobertura da interface | desenvolvimento e CI | MIT |
| `@types/node` | Tipos dos testes | desenvolvimento e CI | MIT |

## Base Debian 13 do HP (apt, repositórios oficiais)

`cage`, `wlr-randr`, `chromium`, `chromium-l10n`, `curl`, `ca-certificates`, `podman`, `uidmap`, `passt`, `netavark`, `aardvark-dns`, `crun`, `containers-storage`, `dbus-user-session`, `libpam-systemd`, `kbd`, `network-manager`, `libgl1-mesa-dri`, fontes `fonts-inter` e `fonts-noto-*`, firmware `firmware-amd-graphics` e `firmware-iwlwifi` (componente non-free-firmware). Opcional: `openssh-server`.

## Imagem das ferramentas (contêiner)

Base `docker.io/library/debian:trixie-slim` (imagem oficial), com `ca-certificates`, `poppler-utils`, `tesseract-ocr` e os idiomas `por` e `eng`, mais o binário `palantir-tool`. É construída no próprio HP.

## Host de desenvolvimento e CI

Go, Node, `poppler-utils`, Podman, `shellcheck` (CI), `govulncheck` (CI). Para a VM: imagem `debian-13-generic-amd64.qcow2` de cloud.debian.org, conferida por SHA-512, e um contêiner com `qemu-system-x86`, `qemu-utils`, `ovmf`, `xorriso`, `openssh-client` e `socat`. Ações do GitHub: `actions/checkout`, `actions/setup-go`, `actions/setup-node`, `actions/upload-artifact`.

## Serviço externo

OpenRouter (`https://openrouter.ai/api/v1`): recebe o conteúdo enviado aos modelos. É o único destino de rede do daemon; as ferramentas saem apenas pelo proxy.
