#!/bin/bash
# Testa a instalação completa num Debian 13 virtual: pós-instalação, tela de
# login, sessão Noctalia, keyring, suspensão e bloqueio, busca, cópia para um
# pendrive, segundo usuário, versões e reinício.
# Precisa de Podman e /dev/kvm. Baixa a imagem de nuvem oficial do Debian
# (cerca de 400 MB) uma vez para ~/.cache/palantir-vm; os pacotes são baixados
# a cada execução.
set -euo pipefail
cd "$(dirname "$0")/.."

[ -w /dev/kvm ] || { echo "/dev/kvm indisponível" >&2; exit 1; }
CACHE="${XDG_CACHE_HOME:-$HOME/.cache}/palantir-vm"
IMAGEM=debian-13-generic-amd64.qcow2
ORIGEM=https://cloud.debian.org/images/cloud/trixie/latest
mkdir -p "$CACHE" saida

if [ ! -f "$CACHE/$IMAGEM" ]; then
	echo "Baixando $ORIGEM/$IMAGEM"
	curl --fail --location --silent --show-error --output "$CACHE/$IMAGEM.parcial" "$ORIGEM/$IMAGEM"
	curl --fail --location --silent --show-error --output "$CACHE/SHA512SUMS" "$ORIGEM/SHA512SUMS"
	SOMA="$(awk -v n="$IMAGEM" '$2==n {print $1}' "$CACHE/SHA512SUMS")"
	echo "$SOMA  $CACHE/$IMAGEM.parcial" | sha512sum --check --quiet
	mv "$CACHE/$IMAGEM.parcial" "$CACHE/$IMAGEM"
fi

VERSAO="${1:-v0.0.1-vm}"
scripts/empacotar.sh "$VERSAO"
podman build --quiet --tag localhost/palantir-vm:1 scripts/vm
rm -rf saida/vm
podman run --rm --name palantir-vm-teste --device /dev/kvm --security-opt label=disable \
	--env PARAR_APOS_BOOT="${PARAR_APOS_BOOT:-}" --env MANTER_SE_FALHAR="${MANTER_SE_FALHAR:-}" \
	--volume "$CACHE:/cache:ro" --volume "$PWD/saida:/saida" --volume "$PWD/scripts/vm:/roteiro:ro" \
	localhost/palantir-vm:1 bash /roteiro/roteiro.sh "palantir-$VERSAO-linux-amd64.tar.gz"
