# Decisões

Registro das escolhas feitas na concepção, em novembro de 2026. Para reabrir uma decisão, registre o motivo e a nova escolha numa entrada nova.

## Projeto separado do Melkor

O Palantír nasceu de um estudo sobre tornar o Melkor Linux um sistema *AI first*. O Melkor continua com seus objetivos originais. O Palantír é um repositório próprio, sem as restrições do Melkor: musl, ausência de runtimes dinâmicos na sessão, receitas compiladas das fontes e atualização A/B.

## Base Debian 13 com Cage e Chromium em kiosk

- **Escolhido:** Debian 13 estável, sem desktop, com Cage e Chromium em kiosk.
- **Por quê:** é a base mais simples que entrega o necessário. O produto é o `palantird` e o sistema base é apenas a moldura.
- **Descartado:** Fedora Atomic com imagem bootc. Oferece atualização atômica, desbloqueio do LUKS pelo TPM e Secure Boot, mas acrescenta complexidade que o protótipo não precisa.
- **Descartado:** NixOS. Ótima reprodutibilidade, mas o tempo iria para aprender Nix em vez de construir o Palantír.
- **Consequência:** não há retorno atômico do sistema base. O versionamento fica no `palantird` e na biblioteca de telas.

## Chromium em vez de WPE WebKit

O Chromium oferece a plataforma web mais completa para o protótipo: visualizador de PDF, reconhecimento de voz e WebGPU. O WPE com Cog continua sendo a alternativa para um sistema base mínimo no futuro.

## Sem sessão gráfica alternativa e sem modo tablet

Emergências são tratadas no console do tty2. Rotação de tela, teclado na tela e caneta ficam fora do protótipo.

## Daemon em Go

Binário único, boa concorrência para atividades em segundo plano e SDK oficial de MCP. Ferramentas MCP podem ser de qualquer linguagem, porque rodam isoladas em contêiner.

## Todos os documentos podem ir para a nuvem

O usuário autorizou o envio de todos os documentos aos modelos. Embeddings e resumos são gerados pelo OpenRouter, sem pipeline local obrigatório.

## Telas prontas em vez de HTML gerado

O modelo preenche dados no esquema de uma tela da biblioteca. HTML gerado pelo modelo é exceção, roda isolado e só entra na biblioteca depois de aprovado.

## Originais imutáveis

A IA organiza visões, não os arquivos. Isso preserva a procedência, permite regenerar derivados com modelos melhores e evita ficar preso ao sistema.

## safeagents não entra como dependência

As boas ideias do projeto são reimplementadas no `palantird`. A avaliação completa está em [SEGURANCA.md](SEGURANCA.md#avaliação-do-safeagents).

## Implementação da v0.1.0

Escolhas feitas durante a primeira implementação, em novembro de 2026.

### Unit de sessão em vez de login automático pelo getty

- **Escolhido:** unit de sistema com sessão PAM própria no tty1, que executa o Cage e é reiniciada pelo systemd.
- **Por quê:** o reinício automático fica explícito, e o login automático do getty exigiria um script no perfil do shell.

### Interface em Lit, sem Svelte

Componentes web nativos, uma dependência de execução e empacotamento com esbuild. Os componentes desenham no DOM comum e usam uma folha de estilo única.

### SQLite sem CGO e vetores comparados em Go

`modernc.org/sqlite` mantém o binário estático. Uma extensão vetorial exigiria CGO; para o acervo de uma pessoa, comparar todos os vetores é rápido.

### Poppler e Tesseract em vez de Docling

A imagem fica com centenas de MB a menos e sem Python. Revisar se a estrutura de tabelas fizer falta.

### Segredos em arquivo por padrão

O keyring precisa de senha para destravar, e o login é automático. O arquivo 0600 fica no disco cifrado. O Secret Service continua disponível pela configuração.

### Permissões fixas no código

As ferramentas de cada tipo de atividade são uma tabela em Go. Nem a configuração, nem o modelo, nem um conteúdo externo a alteram.

### Endereço desconhecido: confirmar sem conteúdo externo, bloquear com ele

Depois que um documento ou uma página entrou no contexto, um host que o usuário não citou é tratado como possível ordem injetada e nem chega à fila de confirmação.

### O modelo não vira ação na triagem

A classificação é uma chamada sem ferramentas, com resposta restrita a quatro classes. Quem etiqueta é o código, e a ferramenta de e-mail não sabe enviar, mover nem apagar.

### Escala pelo compositor

`wlr-randr` aplica a escala. A opção de escala do Chromium desenhava a janela com metade do tamanho no Wayland, como mostrou o teste na VM.

### Nomes de domínio em português

Pacotes, tipos e a API usam os termos da documentação. Convenções do Go e das bibliotecas permanecem como são.

## Modelos, privacidade e escolhas do mantenedor

Escolhas de 26 de novembro de 2026, depois da v0.1.0.

### jev-router nos três níveis

- **Escolhido:** `typesafe/jev-router`, a pedido do mantenedor, para que cada pedido vá ao modelo adequado e gaste menos. Substitui Claude Haiku, Sonnet e Opus fixos.
- **Observado nos testes reais:** extração, triagem e redação foram para modelos baratos; conversa com ferramentas foi para o Claude Opus 5.5, que concentra o custo. O roteador não documenta como restringir os candidatos. Para fixar um modelo, informe o nome em `config.toml`.

### Retenção zero de dados obrigatória

- **Escolhido:** `zdr` e `data_collection = deny` em todo pedido, sem opção de desligar, e Vertex excluído por causa da "Safety Retention".

### Jev para decisões fechadas

- **Escolhido:** triagem e juiz de ações pelo `typesafe/jev-1.14`, cerca de US$ 0,00002 por decisão, com probabilidades que o código compara a limites. Resumos só do que é urgente ou pede resposta.
- **Por quê:** é uma resposta de formato fechado, que um texto malicioso não consegue transformar em texto livre. Nos testes reais o juiz acertou os 5 casos (até 34% nos legítimos, 93% ou mais nos maliciosos).
- **Ajuste vindo do teste:** o Jev classificou como lixo, com baixa confiança, uma fatura que vencia no dia. Por isso, abaixo de 70% de confiança decide o modelo de conversa.
- **Descartado:** chamar a API da TypeSafe diretamente. Exigiria outra chave e não há SDK em Go; o OpenRouter oferece o mesmo modelo com ZDR e a mesma contabilidade.

### O que continua como estava

- **Sem licença:** o repositório é público, com todos os direitos reservados ao autor, por enquanto.
- **E-mail:** só IMAP com senha; o provedor do mantenedor funciona assim.
- **Importação:** cada documento recebe vetores, resumo e entidades.
- **Triagem:** a classificação é gravada como etiqueta no servidor.
- **Orçamento:** US$ 20 por mês e US$ 1 por atividade.


## Sessão comum com login no lugar do kiosk

Escolhas do mantenedor em 26 de novembro de 2026. Substituem **Base Debian 13 com Cage e Chromium em kiosk**, **Sem sessão gráfica alternativa e sem modo tablet** (no que toca à sessão), **Unit de sessão em vez de login automático pelo getty** e **Segredos em arquivo por padrão**.

### Debian 13 com o conjunto Noctalia

- **Escolhido:** compositor Umbriel, shell Noctalia, tela de login Noctalia Greeter (greetd), gerenciador de arquivos Luciole e o que faltar para um notebook (NetworkManager, BlueZ, PipeWire, keyring).
- **Procedência:** o repositório APT `pkg.noctalia.dev`, indicado pela documentação do Noctalia e mantido por Nick Hastings. A pós-instalação confere a soma do pacote da chave e a impressão digital antes de confiar nele.
- **Motivo:** o mantenedor quer um computador que usa e ao qual volta, não um terminal travado numa página. O kiosk tinha sido apenas a sugestão inicial.

### Usuários e senhas comuns do Linux

- Cada usuário entra com a própria senha e tem o próprio Palantír: daemon, acervo, chaves e porta. Não há mais a conta especial `palantir`.
- Ao fechar a tampa, o notebook suspende e volta na tela de bloqueio, que pede a senha do usuário.

### A API local exige um segredo por usuário

Com vários usuários e um navegador que abre qualquer site, a recusa por `Host` e `Origin` não basta. Cada daemon cria um segredo legível só pelo dono, entregue ao navegador pelo comando `palantird abrir` através de um arquivo privado, nunca pela linha de comando.

### Segredos no keyring

A senha de login destrava o keyring (Secret Service), que passa a ser o padrão. O arquivo 0600 continua disponível pela configuração.

### Um endereço sozinho abre o site

Digitar só um endereço navega até ele, como em qualquer navegador. Um pedido que cita um endereço continua indo à IA.

### SearXNG próprio para a pesquisa da IA

- Um contêiner SearXNG na própria máquina, acessível só por ela, com a imagem fixada pela soma.
- Uma consulta é um dado que sai da máquina: depois que conteúdo externo entrou na atividade, uma consulta com palavras que você não escreveu pede confirmação.

### Cópia para disco ou pendrive

Em vez de cópia automática, uma ação na tela: escolher o disco, confirmar, e copiar originais, configurações e históricos. Os segredos não entram.

### Tema escuro

É o padrão.

### Como a sessão é instalada

- O repositório do Noctalia entra com a chave conferida pela impressão digital e restrito, por preferência do APT, ao próprio conjunto e às bibliotecas gráficas mais novas que ele exige.
- A imagem das ferramentas é construída uma vez por versão, como administrador, e cada usuário a carrega no próprio Podman. Construir por usuário repetiria minutos de download.
- O SearXNG é um contêiner do sistema, sem privilégios e só em `127.0.0.1`, e não um por usuário.
- O daemon de cada usuário roda enquanto a sessão existe, inclusive bloqueada. Sem *linger*: nada trabalha em nome de quem não entrou.
