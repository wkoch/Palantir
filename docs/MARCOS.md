# Marcos

Cada fase só é considerada concluída depois de demonstrada no HP ProBook x360 435 G7. Um build concluído, um teste automatizado aprovado e o uso real no notebook são resultados distintos e devem ser registrados separadamente.

## P0: sessão e esqueleto

**Entrega:** o HP liga direto no Palantír.

- Script de pós-instalação para Debian 13 com Cage, Chromium, Podman e NetworkManager.
- Login automático no tty1 e console de emergência no tty2.
- `palantird` com atividades persistentes, fila de atenção, roteador OpenRouter e diário.
- Atividade de conversa com troca entre atividades e custo visível.

**Critérios:**

- Depois da senha do LUKS, a fila de atenção aparece sem outra interação.
- Uma atividade iniciada continua rodando enquanto outra está em uso.
- Reiniciar o notebook preserva o estado das atividades.
- Se o Chromium for encerrado, a sessão volta sozinha.

## P1: acervo

**Entrega:** importar documentos e encontrá-los sem pastas.

- Pasta vigiada, importação de pasta existente e arrastar para a tela.
- Originais endereçados por hash, extração em contêiner, FTS5 e busca vetorial.
- Visões por coleção e por entidade.
- Exportação completa.

**Critérios:**

- Importar a pasta de documentos atual e responder "o que eu tenho sobre X?" citando os originais.
- Nenhum original muda de hash depois de regenerar todos os derivados.

## P2: relatório

**Entrega:** a atividade de relatório descrita em [ATIVIDADES.md](ATIVIDADES.md#relatório-pesquisa-e-minuta).

**Critérios:**

- Um relatório pedido roda em segundo plano e avisa quando fica pronto.
- Corrigir uma seção não altera as outras.
- Toda afirmação tem fonte.

## P3: e-mail só leitura

**Entrega:** a triagem descrita em [ATIVIDADES.md](ATIVIDADES.md#e-mail-triagem-rápida), com proxy de saída e suíte de injeção.

**Critérios:**

- A suíte de [testes de injeção](SEGURANCA.md#testes-de-injeção) passa.
- O MCP de IMAP não alcança nenhum host além do servidor configurado.
- A triagem de uma manhã real leva poucos minutos e as correções melhoram a classificação seguinte.

## P4: estudo para concurso

**Entrega:** a atividade descrita em [ATIVIDADES.md](ATIVIDADES.md#estudo-para-concurso).

**Critérios:**

- A árvore de assuntos de um edital real aponta para questões e capítulos do acervo.
- As revisões agendadas funcionam sem rede.
- Uma sessão de estudo real é concluída do começo ao fim no Palantír.

## Depois do P4

- Ações de e-mail com confirmação: responder, encaminhar, apagar.
- Páginas geradas pelo modelo promovidas para a biblioteca de telas.
- Compilação de fluxos estáveis em rotinas determinísticas.
- Saída de comandos do sistema em JSON, para atividades de administração.
- Atividades que sobrevivem à suspensão e despertar programado para tarefas agendadas.

## Situação

Atualizada em 26 de novembro de 2026. Um critério só é marcado como cumprido quando demonstrado **no HP**.

| Fase | Critério | Implementado e testado | VM Debian 13 | HP |
| --- | --- | --- | --- | --- |
| P0 | A fila aparece depois do boot, sem outra interação | sim | sim (sem LUKS) | pendente |
| P0 | Uma atividade continua enquanto outra está em uso | sim | | pendente |
| P0 | Reiniciar preserva o estado | sim | sim | pendente |
| P0 | A sessão volta quando o Chromium é encerrado | sim | sim | pendente |
| P1 | Importar a pasta de documentos e responder citando originais | sim, modelo simulado | parcial (importação e busca) | pendente |
| P1 | Nenhum original muda de hash ao regenerar | sim | | pendente |
| P2 | Relatório em segundo plano que avisa quando fica pronto | sim, modelo simulado | | pendente |
| P2 | Corrigir uma seção não altera as outras | sim | | pendente |
| P2 | Toda afirmação tem fonte | sim: parágrafos sem fonte válida são descartados | | pendente |
| P3 | A suíte de injeção passa | sim, simulado e modelos reais | | pendente |
| P3 | O IMAP não alcança outro host | sim, com o Podman real | | pendente |
| P3 | Triagem de uma manhã real em poucos minutos | | | pendente |
| P4 | Árvore de um edital real apontando para o acervo | sim, com edital de exemplo | | pendente |
| P4 | Revisões sem rede | sim | | pendente |
| P4 | Uma sessão de estudo real concluída | | | pendente |

Na suíte comum o modelo é simulado: ela mede o funcionamento, não a qualidade das respostas. Em 26/11/2026, no computador de desenvolvimento, conversa com citações (P1), relatório (P2), triagem e suíte de injeção (P3) e estudo (P4) também passaram contra os modelos reais, com dados inventados. Isso não substitui a prova no HP.

