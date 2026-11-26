#!/bin/bash
# Compila a interface e os binários estáticos em bin/.
# Uso: scripts/construir.sh [versão]
set -euo pipefail
cd "$(dirname "$0")/.."

VERSAO="${1:-$(git describe --tags --always --dirty 2>/dev/null || echo desenvolvimento)}"

[ -d web/node_modules ] || (cd web && npm ci --no-audit --no-fund)
(cd web && npm run --silent tipos && npm run --silent construir)

export CGO_ENABLED=0 GOOS=linux GOARCH=amd64
go build -trimpath -ldflags "-s -w -X main.versao=$VERSAO" -o bin/ ./cmd/...
echo "bin/ pronto ($VERSAO)"
