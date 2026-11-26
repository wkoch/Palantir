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
