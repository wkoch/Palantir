#!/bin/bash
# Constrói a imagem das ferramentas e testa o isolamento com o Podman de
# verdade: extração, OCR, rede somente pelo proxy.
set -euo pipefail
cd "$(dirname "$0")/.."

IMAGEM="localhost/palantir-ferramentas:teste"
CONTEXTO="$(mktemp -d)"
trap 'rm -rf "$CONTEXTO"' EXIT
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o "$CONTEXTO/palantir-tool" ./cmd/palantir-tool
cp deploy/Containerfile "$CONTEXTO/"
podman build --quiet --tag "$IMAGEM" "$CONTEXTO"
PALANTIR_IMAGEM_TESTE="$IMAGEM" go test -count=1 -tags podman -run Podman -v ./internal/ferramentas/
