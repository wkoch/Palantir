#!/bin/bash
# Envia uma compilação do host de desenvolvimento ao HP por SSH e a ativa.
# Uso: scripts/implantar.sh usuario@hp [versão]
set -euo pipefail
cd "$(dirname "$0")/.."

DESTINO="${1:?informe o destino, como usuario@hp}"
VERSAO="${2:-v0.0.0-dev.$(date +%Y%m%d%H%M%S)}"
[[ "$DESTINO" != -* ]] || { echo "destino inválido" >&2; exit 1; }

scripts/empacotar.sh "$VERSAO"
PACOTE="palantir-$VERSAO-linux-amd64.tar.gz"
scp "saida/$PACOTE" "$DESTINO:/tmp/$PACOTE"
ssh -t "$DESTINO" "sudo palantir-versao instalar /tmp/$PACOTE && sudo palantir-versao ativar $VERSAO && rm /tmp/$PACOTE"
echo "Implantado. Para voltar: ssh -t $DESTINO sudo palantir-versao voltar"
