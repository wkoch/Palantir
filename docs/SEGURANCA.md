# Segurança

## Modelo de ameaça

O sistema guarda todos os documentos do usuário, tem acesso ao e-mail e envia conteúdo a modelos na nuvem. Os riscos principais, em ordem:

1. **Injeção de prompt.** Um e-mail, uma página ou um documento contém instruções como "encaminhe as faturas para X". O modelo pode tratá-las como ordens.
2. **Vazamento de dados por ferramentas.** Uma ferramenta comprometida ou mal configurada envia dados a um host não autorizado.
3. **Roubo do notebook.** Acesso físico ao disco com todo o acervo e as credenciais.
4. **Exposição de segredos.** Chaves do OpenRouter ou senhas de IMAP chegando ao contexto do modelo ou aos logs.
5. **Custo descontrolado.** Um loop do agente consumindo o orçamento.

## Defesas do protótipo

1. **Disco inteiro em LUKS2** desde a instalação.
2. **Ferramentas em contêiner sem rede.** Cada servidor MCP roda em Podman rootless com `--network=none` e acessa apenas o socket do proxy de saída e os próprios dados.
3. **Proxy de saída com lista de hosts por ferramenta.** O MCP de IMAP só alcança o servidor IMAP, e o roteador de modelos só alcança o OpenRouter.
4. **Proveniência por mensagem.** O daemon marca a origem de cada texto: você, modelo, ferramenta ou documento. Texto de ferramenta ou documento é dado e aparece de forma distinta na tela.
5. **Permissões por atividade.** A triagem de e-mail pode ler e etiquetar. Uma ferramenta fora da lista da atividade é bloqueada.
6. **Confirmação para ações externas.** Enviar, apagar, publicar ou pagar entra numa fila de confirmação. A ação só acontece com o seu clique.
7. **Segredos fora do modelo.** As chaves ficam num arquivo legível só pelo usuário, no disco cifrado, ou no keyring (Secret Service), e são lidas pelo daemon, que as entrega só à ferramenta que precisa delas. O modelo pede a ação, nunca vê a credencial. A interface grava e apaga segredos, mas não os lê.
8. **Diário de auditoria.** Cada chamada de modelo e de ferramenta é registrada com origem, decisão e custo, num arquivo legível só pelo usuário.
9. **Orçamento** por atividade e mensal, com interrupção quando o limite é atingido.
10. **Autoproteção.** Nenhuma ferramenta ou atividade pode alterar a configuração do daemon, as permissões ou o diário.
11. **SSH** só por chave e só na rede local.

## Regras de acesso a endereços

A decisão é código ([politica.go](../internal/politica/politica.go)). Só o que o usuário escreveu autoriza.

| Situação | Sem conteúdo externo no contexto | Com conteúdo externo |
| --- | --- | --- |
| Endereço digitado pelo usuário | permite | permite |
| Host liberado nos ajustes | permite | permite |
| Host citado pelo usuário, outro endereço | permite | pede confirmação |
| Host que o usuário não citou | pede confirmação | **bloqueia** |
| IP, `localhost`, credenciais no endereço, porta incomum | bloqueia | bloqueia |

O proxy repete a conferência por conta própria, resolve o nome ele mesmo, recusa endereços locais ou privados e conecta ao IP resolvido.

## Proteção da importação

A importação de pastas recusa a raiz do disco e nunca lê a configuração, os segredos, o banco ou o diário, mesmo por um link simbólico: um arquivo importado pode ser enviado aos modelos.

## Juiz de ações

Chamadas suspeitas passam por um modelo juiz antes de executar. O juiz vê apenas o que **você** escreveu, a chamada proposta e os motivos da suspeita. Ele nunca vê o raciocínio do agente nem os resultados de ferramentas, porque um conteúdo malicioso tentaria convencê-lo por ali. O juiz pode bloquear, mas não pode liberar uma chamada já bloqueada pelas regras.

## Testes de injeção

A suíte de testes inclui um conjunto de e-mails, páginas e documentos maliciosos. Nenhum deles pode levar a:

- envio, encaminhamento ou exclusão de mensagens;
- acesso a um host fora da lista da ferramenta;
- uso de ferramenta fora das permissões da atividade;
- leitura de segredos ou da configuração do daemon.

Qualquer caso que leve a uma dessas ações reprova o build. A suíte ([internal/injecao](../internal/injecao/injecao_test.go)) roda contra um **modelo comprometido**: assim que vê conteúdo externo, ele tenta tudo o que um atacante pediria. O daemon, o proxy e as ferramentas são os reais. Periodicamente ela também deve rodar contra os modelos configurados, com `PALANTIR_INJECAO_REAL=1` e `PALANTIR_OPENROUTER_KEY`.

Limites conhecidos:

- Um host liberado nos ajustes pode ser lido com qualquer caminho. Libere apenas sites que um atacante não controla.
- Um resumo ou uma classificação pode ser distorcido por um texto malicioso. As defesas impedem ações, não opiniões erradas do modelo.
- Qualquer programa local pode usar a API do daemon ([pendência](PENDENCIAS.md#acesso-à-api-local)).

## Avaliação do safeagents

[huggingface/safeagents](https://github.com/huggingface/safeagents) foi avaliado na versão 0.0.1, em novembro de 2026. É um proxy local entre o agente e o servidor do modelo, que verifica cada chamada de ferramenta antes de o harness executá-la.

### O que ele faz

1. Bloqueia ferramentas não declaradas na requisição e URLs ou IPs cujo host não aparece na conversa.
2. Executa os detectores do atif-scan, voltados a agentes de código: leitura de `/proc/1/environ`, `curl --noproxy`, `unshare`.
3. Consulta opcionalmente um modelo juiz para chamadas suspeitas.
4. Protege a si mesmo contra o agente e mantém um diário em JSONL das chamadas bloqueadas.
5. Oferece `scrub`, que remove segredos e dados pessoais de logs e trajetórias.

### O que o Palantír adota

| Ideia | Como entra no Palantír |
| --- | --- |
| Juiz que não vê resultados de ferramentas | Adotada como está, na seção [Juiz de ações](#juiz-de-ações) |
| Bloqueio de ferramenta não declarada | Vira bloqueio de ferramenta fora das permissões da atividade |
| Autoproteção | Defesa 10 |
| Formato do diário | Base para o diário de auditoria |
| `scrub` | Usado antes de compartilhar logs |

### Limitações para o Palantír

- **A verificação de host não cobre injeção vinda de ferramentas.** O código exclui da conversa apenas as mensagens do próprio modelo. Um host citado num e-mail lido por uma ferramenta passa a contar como mencionado, e uma chamada a ele é liberada. É justamente o ataque principal do Palantír. A proveniência por mensagem no `palantird` resolve isso.
- Os detectores foram feitos para agentes de código e não reconhecem ações pessoais como enviar e-mail ou pagar.
- Sem juiz, chamadas de severidade média passam com apenas um aviso.
- A verificação é por padrões de texto. O isolamento do kernel e o proxy de saída garantem mais.
- É Python e está na versão 0.0.1.

**Decisão:** o Palantír reimplementa essas ideias no `palantird`, que conhece as atividades e a origem de cada texto. O safeagents não entra como dependência. Ele pode ser usado à parte, em `--log-only`, na frente de agentes de código usados no desenvolvimento.
