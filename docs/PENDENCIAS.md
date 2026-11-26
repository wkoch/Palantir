# Pendências do mantenedor

Itens que dependem de uma decisão ou de uma prova sua. Em cada um há o comportamento atual, para que nada fique parado. Uma decisão tomada vai para [DECISOES.md](DECISOES.md) e sai daqui.

## Provas que só você pode fazer

- [ ] Instalar no HP e percorrer o [roteiro de prova](INSTALACAO.md#4-roteiro-de-prova-das-funções-principais).
- [ ] `palantird diagnostico --modelos` no HP, com a sua chave. Em 26/11/2026 as funções principais e a suíte de injeção passaram contra os modelos reais no computador de desenvolvimento, com uma chave temporária e dados inventados.
- [ ] Escolher **ZDR only** nas [configurações de privacidade](https://openrouter.ai/settings/privacy) do OpenRouter.
- [ ] Triagem na sua caixa de e-mail. O IMAP foi testado contra um servidor em memória.
- [ ] Um relatório e um estudo com material seu, para julgar a qualidade.

## Decisões

### Custo da conversa
O jev-router tem mandado a conversa com ferramentas ao Claude Opus 5.5 (cerca de US$ 0,01 por resposta nos testes). Se pesar, fixe um modelo mais barato no nível `principal` ou peça um limite ao roteador.

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
