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

## Base Debian 13 do HP (apt)

Do Debian e do `trixie-backports`: `ca-certificates`, `curl`, `gpg`, `chromium`, `chromium-l10n`, `kitty`, `xwayland`, `xdg-desktop-portal`, `xdg-desktop-portal-gtk`, `xdg-utils`, `network-manager`, `bluez`, `pipewire`, `pipewire-pulse`, `pipewire-audio`, `wireplumber`, `libspa-0.2-bluetooth`, `rtkit`, `greetd`, `gnome-keyring`, `libpam-gnome-keyring`, `libsecret-tools`, `polkitd`, `udisks2`, `upower`, `power-profiles-daemon`, `sound-theme-freedesktop`, `wl-clipboard`, `podman`, `uidmap`, `passt`, `netavark`, `aardvark-dns`, `nftables`, `crun`, `containers-storage`, `dbus-user-session`, `libpam-systemd`, `libgl1-mesa-dri`, `mesa-vulkan-drivers`, fontes `fonts-inter` e `fonts-noto-*`, firmware `firmware-amd-graphics` e `firmware-iwlwifi` (non-free-firmware). Opcional: `openssh-server`.

Do repositório `https://pkg.noctalia.dev/deb/` (indicado pela [documentação do Noctalia](https://docs.noctalia.dev/noctalia/getting-started/installation/), mantido por Nick Hastings): `noctalia`, `noctalia-greeter`, `umbriel`, `xdg-desktop-portal-umbriel`, `luciole`, `nickh-archive-keyring` e as bibliotecas `libwlroots-0.20`, `libdrm*`, `libpixman-1-0`, `libdisplay-info*` e `libinput*`. A chave é aceita só com a impressão digital `C2A7 E07F B3D7 46BD F2A2 45CD 683B 67D7 BC99 A9B7`, e a [preferência do APT](../deploy/sessao/apt-noctalia.pref) ignora qualquer outro pacote desse repositório. A chave expira em junho de 2027.

## Mecanismo de busca (contêiner do sistema)

SearXNG, imagem oficial `docker.io/searxng/searxng` (AGPL-3.0), fixada pela soma em [palantir-busca.container](../deploy/busca/palantir-busca.container). Roda sem privilégios, só em `127.0.0.1`, e consulta buscadores públicos. Para atualizar, troque a soma e rode o teste real de pesquisa.

## Imagem das ferramentas (contêiner)

Base `docker.io/library/debian:trixie-slim` (imagem oficial), com `ca-certificates`, `poppler-utils`, `tesseract-ocr` e os idiomas `por` e `eng`, mais o binário `palantir-tool`. É construída no próprio HP.

## Host de desenvolvimento e CI

Go, Node, `poppler-utils`, Podman, `shellcheck` (CI), `govulncheck` (CI). Para a VM: imagem `debian-13-generic-amd64.qcow2` de cloud.debian.org, conferida por SHA-512, e um contêiner com `qemu-system-x86`, `qemu-utils`, `ovmf`, `xorriso`, `openssh-client` e `socat`. Na VM, `dosfstools` para o pendrive de teste. Ações do GitHub: `actions/checkout`, `actions/setup-go`, `actions/setup-node`, `actions/upload-artifact`.

## Serviço externo

OpenRouter (`https://openrouter.ai/api/v1` e `/api/alpha/decisions`): recebe o conteúdo enviado aos modelos, incluindo `typesafe/jev-router` e `typesafe/jev-1.14` (TypeSafe, hospedado nos Estados Unidos), sempre exigindo retenção zero de dados. É o único destino de rede do daemon; as ferramentas saem apenas pelo proxy, e o navegador é do usuário.
