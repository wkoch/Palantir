# Visão

## O problema

Hoje o computador organiza o trabalho em aplicativos e pastas, e cabe à pessoa juntar as peças: abrir o cliente de e-mail, procurar o PDF, copiar trechos para o editor, lembrar onde salvou. O Palantír inverte isso. Você diz o que quer fazer e o sistema reúne os dados, as ferramentas e a tela adequada.

## Atividades

A **atividade** é a unidade básica do sistema, assim como o processo é a unidade básica do Unix. Uma atividade tem:

- um objetivo, descrito por você;
- um estado: `rodando`, `esperando você`, `pronta`, `pausada` ou `arquivada`;
- um histórico de decisões e pedidos de correção;
- permissões próprias: quais ferramentas e quais dados pode usar;
- um orçamento de tokens;
- uma tela.

Atividades rodam em segundo plano mesmo com a tela fechada. Você troca de atividade a qualquer momento, e a que estava em andamento continua.

A tela inicial é uma **fila de atenção**, e não uma área de trabalho: o que precisa de você, o que ficou pronto, o que está em andamento e quanto cada atividade custou.

## Princípios

### A IA cria; o código executa

Chamar o modelo para tudo, a cada uso, é lento, caro, imprevisível e fácil de manipular. O Palantír separa as camadas:

| Camada | Quem decide | Exemplo |
| --- | --- | --- |
| Intenção | Você | "Triar meus e-mails" |
| Plano | Modelo, só quando a situação é nova | Quais ferramentas usar e qual tela mostrar |
| Execução | Código | Buscar no IMAP, agendar revisões, calcular uma nota |
| Julgamento | Modelo barato | "Isto é lixo, isto é urgente" |
| Exibição | Tela pronta preenchida com JSON | O modelo preenche dados, não escreve HTML |

Quando um fluxo se estabiliza, ele pode ser **compilado**: o plano que o modelo produziu vira uma rotina determinística, executada sem o modelo.

### Telas prontas, preenchidas por dados

A biblioteca de telas contém componentes e páginas pré-modelados: triagem de e-mail, documento com comentários, painel, cartões de estudo, questionário. O modelo escolhe a tela e envia os dados no esquema dela. Isso economiza tokens, mantém a interface estável e evita que o modelo gere código para o navegador.

Quando nenhuma tela serve, o modelo pode gerar uma página isolada num iframe sem rede. Se você aprovar, ela entra na biblioteca com versão e testes.

### Você não gerencia arquivos, mas nada se perde

O acervo tem duas camadas:

1. **Originais imutáveis**, endereçados por hash. Nunca são alterados nem apagados pelo sistema.
2. **Derivados regeneráveis**: texto extraído, trechos, embeddings, resumos e entidades (pessoas, datas, valores, contratos).

A IA organiza **visões**, como coleções, etiquetas e ligações, e não os bytes. Se um modelo melhor surgir, os derivados são refeitos. Toda resposta cita o original de onde veio, com página ou trecho. A qualquer momento, "exporte tudo" gera um arquivo com os originais e os metadados.

### Funciona sem a IA

Sem rede ou sem crédito, o sistema continua útil: o acervo, a busca, o material de estudo já gerado e a repetição espaçada funcionam localmente. E o computador continua sendo um Linux comum, com navegador, arquivos e terminal.

## Escopo

- Um computador: o HP ProBook x360 435 G7.
- Usuários e senhas comuns do Linux; cada pessoa tem o próprio Palantír, com acervo e chaves separados.
- Uma sessão gráfica normal (Noctalia sobre o Umbriel), em que o Palantír abre ao entrar e para onde se volta.
- Um endereço digitado sozinho abre o site, como em qualquer navegador.
- Todos os documentos do usuário podem ser enviados aos modelos.
- Modelos via OpenRouter.

## Fora do escopo do protótipo

- Modo tablet, caneta e rotação de tela.
- Atualização atômica do sistema base. O que precisa de retorno é o Palantír, não o Debian.
- Sincronização entre máquinas e distribuição pública.
- Modelos locais de linguagem.
