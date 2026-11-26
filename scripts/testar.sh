#!/bin/bash
# Verificação completa: formatação, vet, testes Go com detector de corrida e
# cobertura mínima, e testes de ponta a ponta no navegador.
# Uso: scripts/testar.sh [--sem-navegador]
set -euo pipefail
cd "$(dirname "$0")/.."

MINIMO=80
command -v pdftotext >/dev/null || { echo "instale o poppler-utils (pdftotext)" >&2; exit 1; }

echo "== formatação e vet"
[ -z "$(gofmt -l cmd internal)" ] || { echo "arquivos sem gofmt:"; gofmt -l cmd internal; exit 1; }
go vet ./...
bash -n scripts/*.sh deploy/pos-instalacao.sh deploy/palantir-versao
sh -n deploy/palantir-imagem

echo "== testes Go (com detector de corrida)"
go test -count=1 -race ./...

echo "== cobertura Go"
mkdir -p saida
go test -count=1 -coverpkg=./... -coverprofile=saida/cobertura.out ./... >/dev/null
# O pacote de apoio aos testes e os main não entram na conta.
grep -v -E '/internal/testes/|/cmd/[^/]+/main\.go' saida/cobertura.out >saida/cobertura.filtrada.out
TOTAL="$(go tool cover -func=saida/cobertura.filtrada.out | awk '/^total:/ {sub("%","",$3); print $3}')"
echo "cobertura Go: $TOTAL% (mínimo $MINIMO%)"
awk -v t="$TOTAL" -v m="$MINIMO" 'BEGIN { exit !(t+0 >= m) }' || { echo "cobertura abaixo do mínimo" >&2; exit 1; }

echo "== suíte de injeção de prompt"
go test -count=1 ./internal/injecao/

if [ "${1:-}" != "--sem-navegador" ]; then
	echo "== interface (Playwright)"
	[ -d web/node_modules ] || (cd web && npm ci --no-audit --no-fund)
	(cd web && npm run --silent tipos && npx playwright test)
fi
echo "tudo certo"
