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
| `internal/servidor`, `telas`, `ui` | API, esquemas das telas e interface embutida |
| `internal/testes`, `injecao` | Montagem para testes e suíte de injeção |
| `web` | Interface em Lit e TypeScript e os testes Playwright |
| `deploy` | Units, políticas, scripts e imagem usados no HP |

Nomes de domínio são em português, como na documentação.

## Testes

```bash
scripts/testar.sh
```

Confere a formatação, roda `go vet`, os testes com o detector de corrida, exige 80% de cobertura em Go, executa a suíte de injeção e os testes de navegador, que também exigem 80% da interface.

Os testes Go usam o daemon, o proxy e as ferramentas MCP reais, ligados em memória a um modelo simulado e a um servidor IMAP em memória. Os testes Playwright usam os binários compilados, um por teste, em pastas temporárias.

**Cuidado com testes que importam pastas.** Use somente diretórios temporários. Um teste que passe `/` ou a pasta pessoal copia o disco para o acervo de teste se a recusa falhar; a regra da raiz é testada apenas pela função, sem percorrer nada.

```bash
scripts/testar-conteiner.sh
```

Constrói a imagem e confere, com o Podman real, a extração, o OCR, o contêiner sem rede e o acesso somente pelo proxy.

```bash
scripts/testar-vm.sh
```

Instala o pacote num Debian 13 virtual (baixa a imagem de nuvem oficial para `~/.cache/palantir-vm`), reinicia e confere o kiosk, o teclado, o leitor de PDF, a queda do Chromium, o tty2, a troca de versões e a persistência. As capturas ficam em `saida/vm/`. A VM usa `WLR_RENDERER=pixman`, porque a placa virtual atrasa um quadro com OpenGL.

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
