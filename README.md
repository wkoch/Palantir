# Palantír

Palantír é um sistema pessoal *AI first* para o HP ProBook x360 435 G7, com usuários comuns do Linux. Você não abre aplicativos nem gerencia pastas. Você inicia **atividades** e a IA faz o trabalho: triar e-mails, montar um relatório, preparar um estudo para concurso. Cada atividade aparece numa tela web montada a partir de uma biblioteca de componentes.

Ao entrar no computador, o Palantír abre no navegador, e é para ele que você volta. O resto é um Linux comum: outros sites, arquivos e terminal.

## Estado

As fases P0 a P4 estão implementadas e testadas com serviços simulados; a instalação completa foi conferida numa máquina virtual com Debian 13. **Falta a prova no HP, com a sua chave, a sua caixa de e-mail e os seus documentos.** O estado de cada critério está em [Marcos](docs/MARCOS.md), e o que depende de você, em [Pendências](docs/PENDENCIAS.md).

## Instalar no HP

Instale o Debian 13 sem desktop, com o disco cifrado, e execute o script de pós-instalação da release: ele instala a sessão Noctalia, o Palantír e o mecanismo de busca. O passo a passo está em [Instalação](docs/INSTALACAO.md).

## Experimentar sem custo, no computador de desenvolvimento

O simulador imita o serviço de modelos, uma caixa de e-mail e uma página web. Nada sai da máquina e nada é cobrado. Precisa de Go, Node e do `pdftotext` (pacote `poppler-utils`).

```bash
scripts/construir.sh
```

```bash
bin/palantir-simulador --dir /tmp/palantir-demo --ferramentas bin/palantir-tool
```

Em outro terminal:

```bash
bin/palantird --dir /tmp/palantir-demo
```

Abra a interface, já autorizada:

```bash
bin/palantird --dir /tmp/palantir-demo abrir
```

Importe a pasta `/tmp/palantir-demo/exemplos` na tela Acervo.

## O que há na tela

| Tela | O que faz |
| --- | --- |
| Fila | O que precisa de você, o que ficou pronto, o que está em andamento e quanto cada atividade custou |
| Conversa | Perguntas sobre o acervo, pesquisa e páginas da web, com citações e a origem de cada texto |
| Relatório | Plano, pesquisa e redação com fontes; correção seção a seção; versão aprovada volta para o acervo |
| Triagem de e-mail | Classifica e etiqueta pelo teclado; as correções viram preferências. Não envia nem apaga nada |
| Estudo | Árvore do edital, questões, cartões com repetição espaçada e desempenho; revisa sem rede |
| Acervo | Importar, procurar, organizar, conferir e copiar para um disco ou pendrive. Os originais nunca mudam |
| Painel | Custos por modelo e finalidade e o diário de auditoria |
| Ajustes | Chave do OpenRouter, IMAP, orçamento e hosts liberados |

Atalhos: `g f` fila, `g a` acervo, `g p` painel, `g j` ajustes, `n` nova atividade, `?` ajuda. No sistema, `Super` abre os programas e `Super+Shift+P`, o Palantír.

## Comandos

No HP, pelo terminal (`Super+Return`):

| Comando | Para que serve |
| --- | --- |
| `palantird abrir` | Abre o Palantír no navegador, já autorizado |
| `palantird diagnostico` | Confere ferramentas, segredos, daemon e originais |
| `palantird diagnostico --modelos` | Também faz chamadas reais (centavos) a cada nível de modelo |
| `palantird segredo definir openrouter` | Guarda a chave sem mostrá-la (`imap` para a senha do e-mail) |
| `palantird segredo listar` / `remover NOME` | Mostra o que está definido ou apaga |
| `palantird importar CAMINHO...` | Importa arquivos ou pastas |
| `palantird exportar ARQUIVO.zip` | Originais, metadados, atividades e cartões |
| `palantird restaurar ARQUIVO.zip` | Recompõe uma instalação vazia a partir de uma cópia (pare o daemon antes) |
| `palantird verificar` | Confere o hash de todos os originais |
| `palantird regenerar` | Refaz todos os derivados |
| `palantird diario -n 50` | Últimas entradas do diário |
| `palantird versao` | Versão em uso |
| `sudo palantir-versao listar` / `atualizar` / `voltar` | Versões instaladas |

No computador de desenvolvimento:

| Comando | Para que serve |
| --- | --- |
| `scripts/testar.sh` | Testes Go, cobertura, injeção de prompt e navegador |
| `scripts/testar-conteiner.sh` | Isolamento das ferramentas com o Podman de verdade |
| `scripts/testar-vm.sh` | Instalação completa numa VM com Debian 13 |
| `PALANTIR_REAL=1 PALANTIR_OPENROUTER_KEY=... go test -v ./internal/real/` | Funções principais contra os modelos reais, com dados inventados (custa centavos) |
| `scripts/construir.sh` | Compila em `bin/` |
| `scripts/empacotar.sh vX.Y.Z` | Gera o pacote de uma versão em `saida/` |
| `scripts/implantar.sh usuario@hp` | Envia a compilação atual ao HP por SSH |

## Documentação

| Documento | Para que serve |
| --- | --- |
| [AGENTS.md](AGENTS.md) | Instruções para agentes de IA que trabalham neste repositório. |
| [Visão](docs/VISAO.md) | O que é o sistema, os princípios e o que ele deliberadamente não faz. |
| [Arquitetura](docs/ARQUITETURA.md) | Base Debian, sessão Noctalia, `palantird`, acervo, telas e modelos. |
| [Segurança](docs/SEGURANCA.md) | Modelo de ameaça, injeção de prompt, permissões e a avaliação do safeagents. |
| [Atividades](docs/ATIVIDADES.md) | E-mail, relatório e estudo para concurso, em detalhe. |
| [Marcos](docs/MARCOS.md) | Fases P0 a P4, critérios e o que já foi demonstrado. |
| [Decisões](docs/DECISOES.md) | Registro das escolhas feitas e das alternativas descartadas. |
| [Instalação](docs/INSTALACAO.md) | Do instalador do Debian à primeira atividade no HP. |
| [Desenvolvimento](docs/DESENVOLVIMENTO.md) | Organização do código, testes e releases. |
| [Dependências](docs/DEPENDENCIAS.md) | O que entra no projeto, de onde vem e onde roda. |
| [Pendências](docs/PENDENCIAS.md) | Decisões e provas que dependem do mantenedor. |
| [Histórico](CHANGELOG.md) | O que mudou em cada versão. |

## Resumo da arquitetura

- **Base:** Debian 13 (trixie), instalado sem desktop, com o disco inteiro em LUKS2.
- **Sessão:** login com usuário e senha, Umbriel e Noctalia; o Palantír abre no Chromium ao entrar. Cada usuário tem o próprio daemon.
- **Núcleo:** `palantird`, um daemon em Go que gerencia atividades, o loop do agente, permissões, orçamento e o acervo.
- **Ferramentas:** servidores MCP em Podman rootless sem rede, que só saem pelo proxy do daemon.
- **Modelos:** OpenRouter, sempre com retenção zero de dados (ZDR). O jev-router escolhe o modelo de cada pedido, e o Jev decide a triagem e julga ações.
- **Dados:** originais imutáveis endereçados por hash e derivados regeneráveis em SQLite.
