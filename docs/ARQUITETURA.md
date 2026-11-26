# Arquitetura

## Visão geral

```
tty1: unit da sessão ─► Cage ─► Chromium (kiosk) ─► http://127.0.0.1:7700 (interface)
                                                         │ WebSocket
┌────────────────────────────────────────────────────────┴───────────────┐
│ palantird (Go, serviço de usuário)                                      │
│  atividades · loop do agente · roteador OpenRouter                     │
│  permissões · confirmações · orçamento · diário de auditoria           │
│  proxy de saída (lista de hosts por ferramenta)                        │
├────────────────┬───────────────────┬────────────────────────────────────┤
│ Acervo         │ Biblioteca de     │ Ferramentas MCP em Podman          │
│ originais +    │ telas (esquemas   │ --network=none, só sockets para    │
│ SQLite FTS5    │ JSON + componen-  │ o proxy e para os dados da         │
│ + vetores      │ tes web)          │ ferramenta (IMAP, web, PDF)        │
└────────────────┴───────────────────┴────────────────────────────────────┘
tty2: console de emergência
```

## Sistema base

| Peça | Escolha |
| --- | --- |
| Distribuição | Debian 13 (trixie) estável, instalado pelo instalador oficial sem nenhum desktop |
| Disco | Partição única com LUKS2 e ext4 dentro. A senha do LUKS no boot faz o papel do login. |
| Sessão | A unit `palantir-sessao` abre uma sessão PAM no tty1 e executa o Cage com o Chromium em kiosk |
| Emergência | `Ctrl+Alt+F2` abre o console no tty2 |
| Rede | NetworkManager, configurado pelo `nmtui` no console |
| Contêineres | Podman rootless para ferramentas MCP e extração de documentos |
| Desenvolvimento | No host de desenvolvimento, enviando o binário ao HP por SSH, com acesso só por chave e só na rede local |

O kernel 6.12 do trixie atende o AMD Renoir do HP. O firmware da placa Wi-Fi Intel AX200 vem no pacote `firmware-iwlwifi`, incluído pelo instalador.

O repositório versiona o script de pós-instalação ([deploy/pos-instalacao.sh](../deploy/pos-instalacao.sh)) com os pacotes, a unit da sessão, a unit do `palantird` e a política do Chromium. Reinstalar o HP exige apenas o instalador do Debian e esse script. O passo a passo está em [INSTALACAO.md](INSTALACAO.md).

## Sessão

O Cage é um compositor Wayland que exibe uma única janela em tela cheia. A unit o executa com `-s`, que permite trocar de console, e ele abre o `palantir-quiosque`:

```
wlr-randr --output <saída> --scale <escala>
chromium --kiosk --ozone-platform=wayland --user-data-dir=<perfil dedicado> http://127.0.0.1:7700/
```

- Perfil dedicado com sincronização, extensões e senhas salvas desativadas.
- Política do Chromium que permite navegar só na origem local. Links externos não são navegáveis; o endereço é mostrado dentro da própria tela.
- O quiosque espera o daemon responder antes de abrir o navegador.
- Se o Chromium ou o Cage terminarem, a unit os reinicia.
- A escala é aplicada pelo compositor: `--force-device-scale-factor` desenha a janela pela metade no Wayland.
- O teclado segue o escolhido na instalação do Debian.
- O Chromium foi escolhido por oferecer a plataforma web mais completa: visualizador de PDF, reconhecimento de voz e WebGPU.

## palantird

Daemon em Go, executado como serviço de usuário. Go oferece um binário único, boa concorrência para atividades em segundo plano e um SDK oficial de MCP. A lógica não depende de systemd.

| Módulo | Responsabilidade |
| --- | --- |
| Atividades | Ciclo de vida, estado persistente, fila de atenção, retomada após reinício |
| Agente | Loop de raciocínio e uso de ferramentas, com o contexto de cada atividade |
| Roteador de modelos | Escolha do nível, chamadas ao OpenRouter, cache, contabilidade de custo |
| Permissões | Ferramentas e dados que cada atividade pode usar |
| Confirmações | Fila de ações com efeito externo aguardando seu clique |
| Proxy de saída | Único caminho de rede das ferramentas, com lista de hosts por ferramenta |
| Diário | Registro de cada chamada de modelo e ferramenta, com custo e decisão |
| Acervo | Importação, extração, índices e visões |
| Telas | Registro dos esquemas e entrega dos dados à interface |

A interface fala com o daemon apenas por HTTP e WebSocket locais. O daemon recusa requisições com outro cabeçalho `Host` ou outra origem, e exige o cabeçalho `X-Palantir` em tudo o que altera algo.

As ferramentas MCP estão no binário `palantir-tool`, com três servidores:

| Servidor | Ferramentas | Rede |
| --- | --- | --- |
| `web` | `buscar_url`, `pesquisar` | Proxy, nos hosts liberados e no host da chamada autorizada; o SearXNG só durante a pesquisa |
| `imap` | `buscar`, `ler`, `etiquetar` | Proxy, somente o servidor e a porta configurados |
| `extrair` | `extrair` | Nenhuma; recebe um único original, só para leitura |

Cada sessão é um contêiner novo (`--network=none --read-only --cap-drop=all`), e os segredos chegam pelo ambiente, fora da linha de comando.

## Acervo

- **Entrada:** uma pasta vigiada recebe downloads, anexos e cópias de pendrive. Também é possível arrastar para a tela ou pedir a importação de uma pasta existente.
- **Originais:** `~/.local/share/palantir/originais/<sha256>`, somente leitura depois de gravados.
- **Extração:** texto por página, num contêiner com Poppler, Tesseract (português e inglês, para páginas digitalizadas) e leitores próprios em Go para DOCX, ODF, XLSX, PPTX, HTML e e-mail.
- **Índices:** SQLite com FTS5 para busca textual e vetores guardados no mesmo banco, comparados em Go; as duas listas são combinadas por posição. Para o acervo de uma pessoa, a varredura completa dispensa uma extensão vetorial. Embeddings, resumos e entidades são gerados pelo OpenRouter, e a busca textual funciona sem rede.
- **Visões:** coleções, entidades e ligações entre documentos, montadas sob demanda.
- **Exportação e cópia:** um arquivo compactado com originais, metadados em JSON, o banco, a configuração e o diário, sem segredos nem a chave de acesso. A tela lista os discos que não são o do sistema, pede confirmação, grava em `Palantir-copias/`, relê o arquivo inteiro antes de dar o nome final e desmonta o que montou. `palantird restaurar` recompõe uma instalação vazia.

## Biblioteca de telas

Cada tela tem um esquema JSON em `internal/telas/esquemas` e um componente web em Lit, compilado no build. O modelo escolhe a tela e envia os dados no esquema, e o daemon valida os dados antes de entregá-los.

Telas: fila de atenção, conversa, documento com comentários por seção, lista de triagem, estudo (assuntos, cartões, questionário e desempenho), acervo, painel e ajustes. Numa conversa, o modelo pode escolher `indicadores` ou `tabela`.

Páginas geradas pelo modelo, num iframe isolado, sem rede e sem acesso ao daemon, ficam para depois do P4. Uma página aprovada entrará na biblioteca com versão e testes.

## Modelos

Configurados num arquivo, sem nomes de modelos no código. Todo pedido exige [retenção zero de dados](SEGURANCA.md#retenção-zero-de-dados).

| Nível | Uso | Padrão |
| --- | --- | --- |
| Barato | Resumos de e-mail, extração de metadados, dúvidas da triagem | `typesafe/jev-router` |
| Principal | Conversa, redação, relatórios, estudo | `typesafe/jev-router` |
| Forte | Pesquisa profunda e revisão crítica, quando pedido | `typesafe/jev-router` |
| Decisões | Classificar cada e-mail e julgar ações, sem gerar texto | `typesafe/jev-1.14` |
| Vetores | Busca semântica | `baai/bge-m3` |

O [jev-router](https://openrouter.ai/typesafe/jev-router) escolhe o modelo e o esforço de cada pedido e cobra o preço do modelo escolhido. O [Jev](https://typesafe.ai/) responde perguntas fechadas com probabilidades, pela API de decisões do OpenRouter, que ainda é experimental: se ela faltar ou mudar, o nível barato assume. Na triagem, as classificações com confiança abaixo de 70% também seguem para o nível barato.

Cada atividade tem um orçamento, e há também um teto mensal. A fila de atenção mostra o custo de cada atividade.

## Versionamento do Palantír

O sistema base muda pouco: `apt upgrade` e o Chromium. O `palantird`, as ferramentas e a interface mudam o tempo todo. Cada versão é instalada em `/opt/palantir/versoes/<versão>`, e o link `/opt/palantir/atual` aponta para a ativa; a imagem das ferramentas acompanha a versão. Voltar significa trocar o link (`palantir-versao voltar`). Antes de migrar o banco, o daemon grava uma cópia recuperável, e um programa antigo se recusa a abrir um banco mais novo.
