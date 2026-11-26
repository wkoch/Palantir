# Instalação no HP

Reinstalar o HP exige o instalador do Debian e o script de pós-instalação. O processo abaixo foi conferido numa máquina virtual; no HP, anote o que sair diferente.

> **A instalação do Debian apaga o disco.** Copie antes o que quiser guardar.

## 1. Debian 13

1. Grave a imagem `netinst` do [Debian 13](https://www.debian.org/distrib/) num pendrive e inicie por ele.
2. Deixe a senha de `root` **em branco**: o instalador então instala o `sudo` e o libera para a sua conta.
3. No particionamento, escolha o disco inteiro **com cifragem** (LUKS). A senha do disco será o seu login.
4. Na seleção de programas, desmarque todos os ambientes de trabalho. Deixe apenas "utilitários de sistema padrão".
5. Aceite o firmware não livre se o instalador oferecer (vídeo AMD e Wi-Fi Intel AX200).

## 2. Palantír

Reinicie, entre no console com a sua conta, confirme que há rede e baixe a release mais recente. Troque `v0.1.0` pela versão desejada.

```bash
wget https://github.com/wkoch/Palantir/releases/download/v0.1.0/palantir-v0.1.0-linux-amd64.tar.gz
```

```bash
wget https://github.com/wkoch/Palantir/releases/download/v0.1.0/SHA256SUMS
```

```bash
sha256sum -c SHA256SUMS
```

```bash
tar -xzf palantir-v0.1.0-linux-amd64.tar.gz
```

```bash
sudo ./palantir-v0.1.0-linux-amd64/pos-instalacao.sh
```

O script instala os pacotes, constrói a imagem das ferramentas (precisa de rede e leva alguns minutos), ativa o daemon e a sessão e termina com um diagnóstico. Pode ser executado de novo sem problema.

| Opção | Efeito |
| --- | --- |
| `--usar-networkmanager` | Passa a rede para o NetworkManager. Depois de reiniciar, reconecte o Wi-Fi com `nmtui` no tty2 |
| `--ssh-chave "ssh-ed25519 AAAA..."` | SSH só por chave e só na rede local, para implantar a partir do computador de desenvolvimento |
| `--usuario NOME` | Necessário se o script for executado como `root`, sem `sudo` |

## 3. Primeiro uso

Reinicie. Depois da senha do disco, a fila de atenção aparece sozinha.

1. Em **Ajustes**, cole a chave do [OpenRouter](https://openrouter.ai/keys). Defina um limite de gasto também no site do OpenRouter, como segunda proteção.
2. Abra o console com `Ctrl+Alt+F2`, entre e confira os modelos. Volte com `Ctrl+Alt+F1`.

   ```bash
   palantird diagnostico --modelos
   ```

3. Em **Ajustes**, configure o IMAP. Use uma [senha de aplicativo](PENDENCIAS.md#provedor-de-e-mail), nunca a senha principal.

## 4. Roteiro de prova das funções principais

| Fase | Faça | Deve acontecer |
| --- | --- | --- |
| P0 | Reinicie o notebook | Depois da senha do disco, a fila aparece sem outra interação |
| P0 | Crie duas conversas seguidas | A primeira continua rodando enquanto você usa a segunda |
| P0 | No tty2, `pkill chromium` | A sessão volta sozinha em segundos |
| P1 | Acervo → importar a sua pasta de documentos | Os documentos passam de "extraindo" a "pronto"; PDFs digitalizados passam por OCR |
| P1 | Conversa: "o que eu tenho sobre X?" | Resposta citando os originais; clique na marca para abrir |
| P1 | `palantird regenerar` e `palantird verificar` | Nenhum original muda de hash |
| P2 | Peça um relatório e comente uma seção | Só aquela seção muda; toda afirmação tem fonte |
| P3 | Triagem de e-mail numa manhã real | Poucos minutos; correções valem na próxima busca; etiquetas `Palantir_*` no servidor |
| P4 | Estudo com um edital real e uma apostila | Árvore de assuntos, material, cartões e questões; revisões funcionam com o Wi-Fi desligado |

## Atualizar e voltar

```bash
sudo palantir-versao atualizar
```

```bash
sudo palantir-versao voltar
```

Se uma versão nova tiver migrado o banco, a anterior se recusa a abri-lo e informa a cópia a restaurar (`~/.local/share/palantir/palantir.db.vN.bak`).

Do computador de desenvolvimento, com o SSH configurado:

```bash
scripts/implantar.sh usuario@hp
```

## Onde ficam as coisas

| Caminho | Conteúdo |
| --- | --- |
| `~/.config/palantir/config.toml` | Configuração (modelos, hosts, IMAP) |
| `~/.config/palantir/ajustes.toml` | O que foi mudado pela tela de ajustes |
| `~/.config/palantir/segredos.json` | Chave e senha, legíveis só por você |
| `~/.local/share/palantir/` | Banco, originais e diário |
| `~/Palantir/Entrada` | O que for copiado aqui entra no acervo |
| `~/Palantir/Exportacoes` | Arquivos baixados pelo navegador do kiosk |
| `/etc/palantir/quiosque.env` | Escala da tela e ajustes gráficos |
| `/opt/palantir/` | Versões instaladas |

## Problemas

| Sintoma | O que fazer no tty2 |
| --- | --- |
| Tela preta ou a sessão reinicia sem parar | `sudo journalctl -b -u palantir-sessao` |
| "daemon desconectado" | `systemctl --user status palantird` e `journalctl --user -b -u palantird` |
| Documentos com erro de extração | `palantird diagnostico`; reconstrua a imagem com `sudo palantir-versao ativar $(cat /opt/palantir/atual/VERSAO)` |
| A tela demora a atualizar até você teclar | Descomente `WLR_RENDERER=pixman` em `/etc/palantir/quiosque.env` e `sudo systemctl restart palantir-sessao` |
| Letras grandes ou pequenas demais | `PALANTIR_ESCALA` no mesmo arquivo |
| Voltar a um login comum | `sudo systemctl disable --now palantir-sessao && sudo systemctl start getty@tty1` |
