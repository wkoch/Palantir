# Pendências do mantenedor

Itens que dependem de uma decisão ou de uma prova sua. Em cada um há o comportamento atual, para que nada fique parado. Uma decisão tomada vai para [DECISOES.md](DECISOES.md) e sai daqui.

## Provas que só você pode fazer

- [ ] Instalar no HP e percorrer o [roteiro de prova](INSTALACAO.md#4-roteiro-de-prova-das-funções-principais).
- [ ] `palantird diagnostico --modelos` com a sua chave. **Nenhuma chamada foi feita ao OpenRouter real**: o formato dos pedidos foi testado só contra o simulador.
- [ ] Triagem na sua caixa de e-mail. O IMAP foi testado contra um servidor em memória.
- [ ] Um relatório e um estudo com material real, para julgar a qualidade das instruções dadas aos modelos.
- [ ] Suíte de injeção contra os modelos reais (custa créditos):

  ```bash
  PALANTIR_INJECAO_REAL=1 PALANTIR_OPENROUTER_KEY=... go test -count=1 -v ./internal/injecao/
  ```

## Decisões

### Licença do repositório
O repositório é público e não tem licença, o que reserva todos os direitos. Escolha uma (MIT, Apache-2.0, AGPL-3.0) ou torne o repositório privado.

### Onde guardar os segredos
A arquitetura previa o keyring (Secret Service). Com login automático não há senha para destravá-lo. **Atual:** arquivo `segredos.json` com permissão 0600 no disco cifrado. A opção `armazenamento = "secret-service"` existe, mas exige um keyring destravado. Recomendo manter o arquivo.

### Suspensão com a tampa fechada
**Atual:** o padrão do Debian, que suspende e volta sem pedir senha. Um notebook roubado suspenso está com o disco aberto. Opções: desligar ao fechar a tampa (`HandleLidSwitch=poweroff`, seguro e simples, pois o estado persiste), ou uma tela de bloqueio ao voltar.

### Acesso à API local
A interface escuta só em `127.0.0.1` e recusa outros sites, mas qualquer programa em execução na máquina pode usar a API. No HP só rodam o Palantír e o que você abrir no tty2. Se isso mudar, a proteção seria um token entregue ao Chromium da sessão.

### Modelos e orçamento
**Atual:** Claude Haiku 5.5, Sonnet 5.5 e Opus 5.5 para os três níveis, `baai/bge-m3` para a busca semântica, US$ 20 por mês e US$ 1 por atividade. Ajuste em `config.toml`.

### Gastos automáticos ao importar
**Atual:** cada documento importado recebe vetores e, com `enriquecer = true`, resumo e entidades. Importar uma pasta grande consome créditos, limitado pelo teto mensal. Desligue com `enriquecer = false`.

### Etiquetas no servidor de e-mail
**Atual:** a classificação é gravada como etiqueta `Palantir_*` (`etiquetar = true`). Desmarque em Ajustes para uma triagem que não altera nada.

### Provedor de e-mail
Só há autenticação por senha (IMAP). Gmail exige senha de aplicativo; contas Microsoft costumam exigir OAuth2, que não foi implementado. Diga qual é o seu provedor.

### Pesquisa na web
Não há mecanismo de busca: a pesquisa lê endereços que você informa, hosts liberados e, com a sua confirmação, os sugeridos pelo modelo. Integrar uma API de busca (paga ou uma instância SearXNG) é uma decisão sua. A lista inicial de hosts é `pt.wikipedia.org`, `www.planalto.gov.br` e `www.gov.br`.

### Cópia de segurança
Há exportação manual, mas nenhuma cópia automática. Para o acervo real, vale definir destino (pendrive, outro computador, nuvem) e frequência.

### Aparência
O kiosk usa o tema claro (o Chromium sem desktop informa preferência clara) e escala 1,25. Há um tema escuro pronto na interface; diga se prefere que ele seja o padrão.

## Fora do que foi implementado

Estes itens estão em [Depois do P4](MARCOS.md#depois-do-p4) ou ficaram simples de propósito:

- Responder, encaminhar e apagar e-mails. A fila de confirmação já existe e hoje serve ao acesso a endereços.
- Páginas geradas pelo modelo em iframe isolado e sua promoção a telas.
- Compilação de fluxos em rotinas determinísticas.
- Extração com Docling: foi usado Poppler com OCR Tesseract, muito menor. Tabelas complexas perdem estrutura.
- Resolução de questões de provas anteriores com gabarito próprio: as questões são criadas a partir do material importado.
