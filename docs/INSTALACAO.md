# Instalação no HP

Reinstalar o HP exige o instalador do Debian e o script de pós-instalação. O processo abaixo foi conferido numa máquina virtual; no HP, anote o que sair diferente.

> **A instalação do Debian apaga o disco.** Copie antes o que quiser guardar.

## 1. Debian 13

1. Grave a imagem `netinst` do [Debian 13](https://www.debian.org/distrib/) num pendrive e inicie por ele.
2. Deixe a senha de `root` **em branco**: o instalador então instala o `sudo` e o libera para a sua conta.
3. Crie a sua conta com a senha que usará para entrar.
4. No particionamento, escolha o disco inteiro **com cifragem** (LUKS).
5. Na seleção de programas, desmarque todos os ambientes de trabalho. Deixe apenas "utilitários de sistema padrão".
6. Aceite o firmware não livre se o instalador oferecer (vídeo AMD e Wi-Fi Intel AX200).

## 2. Palantír

Reinicie, entre no console com a sua conta, confirme que há rede e baixe a release mais recente. Troque `v0.2.0` pela versão desejada.

```bash
wget https://github.com/wkoch/Palantir/releases/download/v0.2.0/palantir-v0.2.0-linux-amd64.tar.gz
```

```bash
wget https://github.com/wkoch/Palantir/releases/download/v0.2.0/SHA256SUMS
```

```bash
sha256sum -c SHA256SUMS
```

```bash
tar -xzf palantir-v0.2.0-linux-amd64.tar.gz
```

```bash
sudo ./palantir-v0.2.0-linux-amd64/pos-instalacao.sh
```

O script acrescenta o `trixie-backports` e o repositório do Noctalia, instala a sessão e os programas, constrói a imagem das ferramentas, liga o mecanismo de busca e termina com um diagnóstico. Baixa alguns gigabytes e leva vários minutos. Pode ser executado de novo sem problema.

| Opção | Efeito |
| --- | --- |
| `--usuario NOME` | Prepara outra conta (rode de novo para cada pessoa); obrigatória se o script for executado como `root` |
| `--manter-rede` | Não passa a rede para o NetworkManager; o Wi-Fi não aparecerá no Noctalia |
| `--sem-busca` | Não instala o SearXNG; a IA só lê endereços informados |
| `--ssh-chave "ssh-ed25519 AAAA..."` | SSH só por chave e só na rede local, para implantar a partir do computador de desenvolvimento |

## 3. Primeiro uso

Reinicie. Depois da senha do disco, aparece a tela de login. Entre com a sua senha: a barra do Noctalia aparece e o Palantír abre no Chromium.

1. Se usa Wi-Fi, conecte pelo ícone de rede na barra.
2. No Palantír, em **Ajustes**, cole a chave do [OpenRouter](https://openrouter.ai/keys). Ela fica no keyring do Linux, destravado pela sua senha. Defina um limite de gasto também no site do OpenRouter.
3. Nas [configurações de privacidade](https://openrouter.ai/settings/privacy) do OpenRouter, escolha **ZDR only**. O Palantír já exige retenção zero em cada pedido; a opção da conta fecha também a retenção para análise de abuso.
4. Abra um terminal (`Super+Return`) e confira os modelos. Custa centavos.

   ```bash
   palantird diagnostico --modelos
   ```

5. Em **Ajustes**, configure o IMAP. Há só login por senha: se o provedor oferecer senha de aplicativo, use-a em vez da principal.

| Atalho | Faz |
| --- | --- |
| `Super` | Lançador de programas |
| `Super+Shift+P` | Abre o Palantír |
| `Super+Return` | Terminal |
| `Super+S` / `Super+vírgula` | Central de controle e configurações do Noctalia, se mantidos no padrão |
| `Super+Q` | Fecha a janela |

## 4. Roteiro de prova das funções principais

| Fase | Faça | Deve acontecer |
| --- | --- | --- |
| P0 | Reinicie o notebook e entre | O Palantír abre sozinho, já com acesso |
| P0 | Feche a tampa e abra | Tela de bloqueio; a senha devolve a sessão |
| P0 | Crie duas conversas seguidas | A primeira continua rodando enquanto você usa a segunda |
| P0 | Digite só um endereço no campo da fila | O site abre em outra aba |
| P1 | Acervo → importar a sua pasta de documentos | Os documentos passam de "extraindo" a "pronto"; PDFs digitalizados passam por OCR |
| P1 | Conversa: "o que eu tenho sobre X?" | Resposta citando os originais; clique na marca para abrir |
| P1 | `palantird regenerar` e `palantird verificar` | Nenhum original muda de hash |
| P1 | Acervo → copiar para um pendrive | O arquivo aparece em `Palantir-copias/` no pendrive |
| P2 | Peça um relatório sobre um assunto público | Pesquisa na web; só a seção comentada muda; toda afirmação tem fonte |
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

O sistema e o Noctalia atualizam pelo `apt`:

```bash
sudo apt update && sudo apt upgrade
```

Do computador de desenvolvimento, com o SSH configurado:

```bash
scripts/implantar.sh usuario@hp
```

## Restaurar uma cópia

Numa instalação nova, antes de usar o Palantír:

```bash
systemctl --user stop palantird
```

```bash
palantird restaurar /caminho/palantir-AAAA-MM-DD-HHMMSS.zip
```

```bash
systemctl --user start palantird
```

Depois defina de novo a chave do OpenRouter e a senha do e-mail.

## Onde ficam as coisas

| Caminho | Conteúdo |
| --- | --- |
| `~/.config/palantir/config.toml` | Configuração (modelos, hosts, IMAP, busca) |
| `~/.config/palantir/ajustes.toml` | O que foi mudado pela tela de ajustes |
| `~/.config/palantir/acesso` | Chave que a interface apresenta ao daemon |
| `~/.local/share/palantir/` | Banco, originais e diário |
| `~/Palantir/Entrada` | O que for copiado aqui entra no acervo |
| `/etc/xdg/umbriel/config.toml` | Compositor; copie para `~/.config/umbriel/` para mudar |
| `~/.config/noctalia/config.toml` | Shell: tema, bloqueio |
| `/etc/palantir/busca-settings.yml` | Mecanismo de busca |
| `/opt/palantir/` | Versões instaladas |

As chaves ficam no keyring.

## Problemas

Use um terminal (`Super+Return`) ou, sem a sessão gráfica, `Ctrl+Alt+F2`.

| Sintoma | O que fazer |
| --- | --- |
| Tela preta depois do login | No console: `journalctl --user -b` e `sudo journalctl -b -u greetd` |
| "Esta janela não tem acesso" | `Super+Shift+P`, ou `palantird abrir` |
| "daemon desconectado" | `systemctl --user status palantird` e `journalctl --user -b -u palantird` |
| Chave "não configurada" depois de definida | O keyring está travado: saia da sessão e entre de novo com a senha |
| Documentos com erro de extração | `palantird diagnostico`; `systemctl --user restart palantird` recarrega a imagem |
| A pesquisa na web falha | `sudo systemctl status palantir-busca` |
| Letras grandes ou pequenas demais | `scale` em `[output.eDP-1]` na configuração do Umbriel |
