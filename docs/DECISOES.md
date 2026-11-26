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
