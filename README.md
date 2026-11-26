# Palantír

Palantír é um sistema pessoal *AI first* para um único usuário e um único computador, o HP ProBook x360 435 G7. Você não abre aplicativos nem gerencia pastas. Você inicia **atividades** e a IA faz o trabalho: triar e-mails, montar um relatório, preparar um estudo para concurso. Cada atividade aparece numa tela web montada a partir de uma biblioteca de componentes.

Ao entrar no computador, você já está no Palantír. A web é a interface nativa do sistema, como o console de texto é a interface nativa do Linux.

## Estado

Fase de concepção. Ainda não há código. A documentação abaixo registra a visão, a arquitetura proposta e o caminho até o primeiro protótipo.

| Documento | Para que serve |
| --- | --- |
| [AGENTS.md](AGENTS.md) | Instruções para agentes de IA que trabalham neste repositório. |
| [Visão](docs/VISAO.md) | O que é o sistema, os princípios e o que ele deliberadamente não faz. |
| [Arquitetura](docs/ARQUITETURA.md) | Base Debian, sessão kiosk, `palantird`, acervo, telas e modelos. |
| [Segurança](docs/SEGURANCA.md) | Modelo de ameaça, injeção de prompt, permissões e a avaliação do safeagents. |
| [Atividades](docs/ATIVIDADES.md) | E-mail, relatório e estudo para concurso, em detalhe. |
| [Marcos](docs/MARCOS.md) | Fases P0 a P4 e os critérios de cada uma. |
| [Decisões](docs/DECISOES.md) | Registro das escolhas feitas e das alternativas descartadas. |

## Resumo do protótipo

- **Base:** Debian 13 (trixie), instalado sem desktop, com o disco inteiro em LUKS2.
- **Sessão:** login automático no tty1 abre o Cage com o Chromium em kiosk apontado para o daemon local. O tty2 fica como console de emergência.
- **Núcleo:** `palantird`, um daemon em Go que gerencia atividades, o loop do agente, permissões, orçamento e o acervo.
- **Modelos:** OpenRouter, em três níveis (barato, principal e forte).
- **Dados:** originais imutáveis endereçados por hash e derivados regeneráveis em SQLite.
