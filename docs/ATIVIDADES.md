# Atividades

As três atividades abaixo orientam o protótipo. Todas seguem o princípio "a IA cria; o código executa" descrito na [visão](VISAO.md#a-ia-cria-o-código-executa).

## E-mail: triagem rápida

**Objetivo:** ver em minutos o que importa na caixa de entrada.

- Um servidor MCP de IMAP busca as mensagens novas. Ele só alcança o servidor IMAP pelo proxy de saída.
- O modelo barato classifica cada mensagem (urgente, responder, ler depois, lixo) e escreve um resumo de uma linha. Ele recebe cabeçalho e trecho inicial, não a mensagem inteira.
- A tela é uma lista de triagem com atalhos de teclado e ações sugeridas.
- Suas correções viram preferências guardadas, usadas nas próximas classificações sem reescrever o prompt.
- **Permissões no protótipo:** ler e etiquetar. Responder, encaminhar e apagar ficam para depois da suíte de injeção e sempre passam pela fila de confirmação.

## Relatório, pesquisa e minuta

**Objetivo:** produzir um documento a partir do acervo e da web, com fontes.

- Roda em segundo plano. A atividade fica `rodando` e passa a `esperando você` quando tem perguntas, ou `pronta` quando termina.
- Cada afirmação cita a fonte: um original do acervo, com página, ou uma URL.
- A tela é um documento com comentários por seção. Pedir uma correção **regenera só aquela seção**, preservando o que já foi aprovado e economizando tokens.
- Uma versão aprovada é guardada no acervo como um novo original, com ligação às fontes.

## Estudo para concurso

**Objetivo:** estudar exatamente o que o edital pede, no ritmo certo.

1. Lê o edital e monta a árvore de assuntos.
2. Liga cada assunto a questões de provas anteriores e aos capítulos dos livros do acervo.
3. Gera explicações didáticas, exemplos e questões **uma vez só**, guardados com a fonte (livro e página).
4. Agenda as revisões por repetição espaçada (FSRS) e faz o diagnóstico dos pontos fracos com **código**, não com o modelo. Essa parte roda sem rede e sem custo.
5. Usa telas de cartões, questionário e explicação interativa.

Em conteúdo jurídico, um erro do modelo custa pontos na prova. Toda explicação tem fonte clicável e pode ser marcada como "conferida por você". Explicações não conferidas aparecem destacadas.

## Atividades de apoio

- **Conversa:** atividade livre, usada no P0 para validar o loop do agente e as telas.
- **Importar:** recebe arquivos ou pastas, grava os originais e agenda a extração.
- **Busca no acervo:** "o que eu tenho sobre X?", com resultados agrupados por visão.
