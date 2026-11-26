#!/bin/bash
# Gera o pacote de uma versão em saida/, com a soma SHA-256.
# Uso: scripts/empacotar.sh vX.Y.Z
set -euo pipefail
cd "$(dirname "$0")/.."

VERSAO="${1:?informe a versão, como v0.1.0}"
[[ "$VERSAO" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]] || { echo "versão inválida: $VERSAO" >&2; exit 1; }

scripts/construir.sh "$VERSAO"

NOME="palantir-$VERSAO-linux-amd64"
TEMPORARIO="$(mktemp -d)"
trap 'rm -rf "$TEMPORARIO"' EXIT
PASTA="$TEMPORARIO/$NOME"
mkdir -p "$PASTA" saida
cp bin/palantird bin/palantir-tool bin/palantir-simulador "$PASTA/"
cp deploy/Containerfile deploy/palantird.service deploy/palantir-sessao.service deploy/pam-palantir-sessao \
	deploy/chromium-politica.json "$PASTA/"
install -m 755 deploy/pos-instalacao.sh deploy/palantir-versao deploy/palantir-sessao deploy/palantir-quiosque "$PASTA/"
echo "$VERSAO" >"$PASTA/VERSAO"
tar --create --gzip --owner=0 --group=0 --sort=name --file "saida/$NOME.tar.gz" --directory "$TEMPORARIO" "$NOME"
(cd saida && sha256sum "$NOME.tar.gz" >SHA256SUMS)
echo "saida/$NOME.tar.gz"
