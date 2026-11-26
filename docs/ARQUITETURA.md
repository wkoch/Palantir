# Arquitetura

## Visão geral

```
tty1: login automático ─► Cage ─► Chromium (kiosk) ─► http://localhost (interface)
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
| Sessão | Login automático no tty1, que executa o Cage com o Chromium em kiosk |
| Emergência | `Ctrl+Alt+F2` abre o console no tty2 |
| Rede | NetworkManager, configurado pelo `nmtui` no console |
| Contêineres | Podman rootless para ferramentas MCP e extração de documentos |
| Desenvolvimento | No host de desenvolvimento, enviando o binário ao HP por SSH, com acesso só por chave e só na rede local |

O kernel 6.12 do trixie atende o AMD Renoir do HP. O firmware da placa Wi-Fi Intel AX200 vem no pacote `firmware-iwlwifi`, incluído pelo instalador.

O repositório versiona um script de pós-instalação com os pacotes, a unit de login automático, a unit do `palantird` e a configuração do Chromium. Reinstalar o HP deve exigir apenas o instalador do Debian e esse script.

## Sessão

O Cage é um compositor Wayland que exibe uma única janela em tela cheia. Ele executa:

```
chromium --kiosk --ozone-platform=wayland --user-data-dir=<perfil dedicado> http://localhost:<porta>
```

- Perfil dedicado com sincronização, extensões e senhas salvas desativadas.
- Política do Chromium que permite navegar só na origem local. Links externos são abertos pela atividade, dentro de uma tela própria.
- Se o Chromium ou o Cage terminarem, a unit da sessão os reinicia.
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

A interface fala com o daemon apenas por HTTP e WebSocket locais.

## Acervo

- **Entrada:** uma pasta vigiada recebe downloads, anexos e cópias de pendrive. Também é possível arrastar para a tela ou pedir a importação de uma pasta existente.
- **Originais:** `~/.local/share/palantir/originais/<sha256>`, somente leitura depois de gravados.
- **Extração:** texto, tabelas e estrutura de PDF, DOCX e afins, num contêiner com Docling ou ferramenta equivalente. O Python fica isolado no contêiner.
- **Índices:** SQLite com FTS5 para busca textual e uma extensão vetorial para busca semântica. Embeddings e resumos são gerados pelo OpenRouter.
- **Visões:** coleções, entidades e ligações entre documentos, montadas sob demanda.
- **Exportação:** um arquivo compactado com originais e metadados em JSON.

## Biblioteca de telas

Cada tela tem um esquema JSON e um componente web (Lit ou Svelte, compilados no build). O modelo escolhe a tela e envia os dados no esquema, e o daemon valida os dados antes de entregá-los.

Telas iniciais: fila de atenção, conversa, documento com comentários por seção, lista de triagem, painel com indicadores, cartões de estudo e questionário.

Páginas geradas pelo modelo rodam num iframe isolado, sem rede e sem acesso ao daemon. Uma página aprovada entra na biblioteca com versão e testes.

## Modelos

Três níveis, configurados num arquivo, sem nomes de modelos no código:

| Nível | Uso |
| --- | --- |
| Barato | Classificar e-mails, extrair metadados, escolher a tela |
| Principal | Conversa, redação, relatórios |
| Forte | Pesquisa profunda e revisão crítica, quando pedido ou exigido pela atividade |

Cada atividade tem um orçamento, e há também um teto mensal. A fila de atenção mostra o custo de cada atividade.

## Versionamento do Palantír

O sistema base muda pouco: `apt upgrade` e o Chromium. O `palantird` e a biblioteca de telas mudam o tempo todo. Eles são instalados em diretórios versionados, e um link aponta para a versão ativa. Voltar significa trocar o link. Migrações do banco do acervo precisam ser compatíveis para leitura ou partir de uma cópia recuperável.
