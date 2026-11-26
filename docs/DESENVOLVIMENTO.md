# Desenvolvimento

## Requisitos no computador de desenvolvimento

Go (a versão do `go.mod`), Node 24 ou mais novo, `poppler-utils`, Podman para os testes de contêiner e de VM, e `/dev/kvm` para a VM.

## Organização

| Pasta | Conteúdo |
| --- | --- |
| `cmd/palantird` | O daemon e sua linha de comando |
| `cmd/palantir-tool` | Os servidores MCP (`web`, `imap`, `extrair`) |
| `cmd/palantir-simulador` | Modelo, IMAP e página web falsos |
| `internal/daemon` | Montagem dos módulos |
| `internal/atividades` | Ciclo de vida, agendamento e retomada |
| `internal/agente`, `capacidades` | Loop com ferramentas e o que cada ferramenta faz |
| `internal/politica`, `confirmacoes`, `proveniencia` | Permissões, fila de confirmação e origem dos textos |
| `internal/proxy`, `ferramentas` | Proxy de saída e execução isolada |
| `internal/modelos`, `orcamento`, `diario`, `segredos` | Roteador, custo, auditoria e chaves |
| `internal/acervo`, `extrator` | Originais, derivados, busca e extração |
| `internal/conversa`, `relatorio`, `email`, `estudo` | As atividades |
| `internal/discos`, `copia` | Destinos, cópia de segurança e restauração |
| `internal/servidor`, `telas`, `ui` | API, esquemas das telas e interface embutida |
| `internal/testes`, `injecao`, `real` | Montagem para testes, suíte de injeção e prova com modelos reais |
| `web` | Interface em Lit e TypeScript e os testes Playwright |
| `deploy` | Pós-instalação, sessão, busca, units e imagem usados no HP |

Nomes de domínio são em português, como na documentação.

## Testes

```bash
scripts/testar.sh
```

Confere a formatação, roda `go vet`, os testes com o detector de corrida, exige 80% de cobertura em Go, executa a suíte de injeção e os testes de navegador, que também exigem 80% da interface.

Os testes Go usam o daemon, o proxy e as ferramentas MCP reais, ligados em memória a um modelo simulado e a um servidor IMAP em memória. Os testes Playwright usam os binários compilados, um por teste, em pastas temporárias.

**Cuidado com testes que importam pastas.** Use somente diretórios temporários. Um teste que passe `/` ou a pasta pessoal copia o disco para o acervo de teste se a recusa falhar; a regra da raiz é testada apenas pela função, sem percorrer nada.

```bash
PALANTIR_REAL=1 PALANTIR_OPENROUTER_KEY=... go test -count=1 -v ./internal/real/
```

Roda triagem, juiz, conversa com acervo e página, relatório e estudo contra os modelos reais, com dados inventados, e mostra quem atendeu cada chamada e quanto custou (menos de US$ 0,10). Falha o que o produto garante; a qualidade é para ler na saída. Nunca use documentos ou e-mails verdadeiros aqui. Com `PALANTIR_BUSCA_URL` apontando para um SearXNG configurado como o de `deploy/busca`, a pesquisa na web também é provada.

```bash
scripts/testar-conteiner.sh
```

Constrói a imagem e confere, com o Podman real, a extração, o OCR, o contêiner sem rede e o acesso somente pelo proxy.

```bash
scripts/testar-vm.sh
```

Instala o pacote num Debian 13 virtual (baixa a imagem de nuvem oficial para `~/.cache/palantir-vm` e, dentro da máquina, os pacotes do Debian, do Noctalia e a imagem do SearXNG), entra pela tela de login digitando a senha e confere a sessão, o keyring, o bloqueio, a busca, a cópia para um pendrive virtual, um segundo usuário, a troca de versões e a persistência. As capturas ficam em `saida/vm/`. A conta e a senha da máquina existem só nela. `PARAR_APOS_BOOT=1` deixa a máquina ligada na tela de login para depuração, e `MANTER_SE_FALHAR=1`, depois de uma falha. A placa de vídeo virtual não se recupera da suspensão, então ela é o último passo e só confere que a sessão volta bloqueada; o desbloqueio com senha é testado num bloqueio comum.

Os testes não tocam o keyring nem os discos de quem os roda: o barramento D-Bus é apagado do ambiente e o `lsblk` é substituído por um de mentira.

## Ao mudar partes sensíveis

- **Ferramentas, permissões ou loop do agente:** rode a [suíte de injeção](SEGURANCA.md#testes-de-injeção). Uma ferramenta nova entra em `politica.permissoes` de propósito, nunca por padrão.
- **Esquema do banco:** acrescente uma migração no fim de `internal/banco/migracoes.go`; nunca edite uma publicada.
- **Tela nova:** esquema em `internal/telas/esquemas`, componente em `web/src/telas`.
- **Dependência nova:** registre em [DEPENDENCIAS.md](DEPENDENCIAS.md).

## Releases

1. Descreva a versão numa seção `## vX.Y.Z` do [CHANGELOG](../CHANGELOG.md).
2. Faça o commit, crie a tag e envie:

```bash
git tag -a v0.1.0 -m "v0.1.0" && git push origin main v0.1.0
```

O fluxo `release` do GitHub roda todos os testes, empacota e publica. Se algo falhar, nada é publicado.
