#!/bin/bash
# Pós-instalação do Palantír sobre um Debian 13 (trixie) sem desktop.
# Pode ser executado de novo: cada passo confere o que já está feito.
set -euo pipefail

AQUI="$(cd "$(dirname "$(readlink -f "$0")")" && pwd)"

uso() {
	cat <<'FIM'
uso: sudo ./pos-instalacao.sh [opções]

  --usuario NOME       conta que usará o Palantír (padrão: quem chamou o sudo)
  --pacote ARQUIVO     instala este pacote .tar.gz em vez de baixar a release
  --versao vX.Y.Z      baixa esta release (padrão: a mais recente)
  --usar-networkmanager
                       passa a rede para o NetworkManager (depois use o nmtui)
  --ssh-chave CHAVE    instala o SSH só por chave, na rede local, com esta chave pública
  --sem-reiniciar      não pergunta se deve reiniciar no fim
FIM
}

erro() {
	echo "pos-instalacao: $*" >&2
	exit 1
}

passo() { printf '\n== %s\n' "$*"; }

USUARIO="${SUDO_USER:-}"
PACOTE=""
VERSAO=""
NM=0
CHAVE_SSH=""
REINICIAR=1
while [ $# -gt 0 ]; do
	case "$1" in
	--usuario) USUARIO="${2:-}"; shift 2 ;;
	--pacote) PACOTE="${2:-}"; shift 2 ;;
	--versao) VERSAO="${2:-}"; shift 2 ;;
	--usar-networkmanager) NM=1; shift ;;
	--ssh-chave) CHAVE_SSH="${2:-}"; shift 2 ;;
	--sem-reiniciar) REINICIAR=0; shift ;;
	-h | --help) uso; exit 0 ;;
	*) uso >&2; erro "opção desconhecida: $1" ;;
	esac
done

[ "$(id -u)" -eq 0 ] || erro "execute com sudo"
. /etc/os-release
[ "${ID:-}" = debian ] && [ "${VERSION_CODENAME:-}" = trixie ] ||
	erro "este script é para o Debian 13 (trixie); encontrado: ${PRETTY_NAME:-desconhecido}"
[ "$(dpkg --print-architecture)" = amd64 ] || erro "somente amd64"
[ -n "$USUARIO" ] && [ "$USUARIO" != root ] || erro "informe --usuario com uma conta comum"
id "$USUARIO" >/dev/null 2>&1 || erro "a conta $USUARIO não existe"
[[ "$USUARIO" =~ ^[a-z_][a-z0-9_-]*$ ]] || erro "nome de conta incomum: $USUARIO"
UID_USUARIO="$(id -u "$USUARIO")"
CASA="$(getent passwd "$USUARIO" | cut -d: -f6)"

# Um pacote ao lado do script (script extraído do próprio pacote) é usado direto.
if [ -z "$PACOTE" ] && [ -z "$VERSAO" ] && [ -x "$AQUI/palantird" ] && [ -f "$AQUI/VERSAO" ]; then
	TEMPORARIO="$(mktemp -d)"
	trap 'rm -rf "$TEMPORARIO"' EXIT
	PACOTE="$TEMPORARIO/pacote.tar.gz"
	tar --create --gzip --file "$PACOTE" --directory "$(dirname "$AQUI")" "$(basename "$AQUI")"
fi
[ -z "$PACOTE" ] || [ -f "$PACOTE" ] || erro "pacote não encontrado: $PACOTE"

passo "Pacotes do Debian"
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install --yes --no-install-recommends \
	cage wlr-randr chromium chromium-l10n curl ca-certificates \
	podman uidmap passt netavark aardvark-dns crun containers-storage \
	dbus-user-session libpam-systemd kbd \
	network-manager \
	fonts-inter fonts-noto-core fonts-noto-color-emoji fonts-noto-mono \
	libgl1-mesa-dri
# Firmware da placa de vídeo AMD e do Wi-Fi Intel AX200 do HP.
apt-get install --yes --no-install-recommends firmware-amd-graphics firmware-iwlwifi ||
	echo "AVISO: firmware não instalado; habilite o componente non-free-firmware no apt."

passo "Conta $USUARIO"
grep -q "^$USUARIO:" /etc/subuid || usermod --add-subuids 200000-265535 "$USUARIO"
grep -q "^$USUARIO:" /etc/subgid || usermod --add-subgids 200000-265535 "$USUARIO"
loginctl enable-linger "$USUARIO"
systemctl start "user@$UID_USUARIO.service"
# Um gerenciador de usuário iniciado antes dos pacotes não tem o barramento
# D-Bus, de que o Podman precisa.
if [ ! -S "/run/user/$UID_USUARIO/bus" ]; then
	systemctl restart "user@$UID_USUARIO.service"
	for _ in $(seq 50); do
		[ -S "/run/user/$UID_USUARIO/bus" ] && break
		sleep 0.2
	done
	[ -S "/run/user/$UID_USUARIO/bus" ] || erro "o barramento da sessão de $USUARIO não iniciou (dbus-user-session)"
fi
install -d -o "$USUARIO" -g "$(id -gn "$USUARIO")" -m 700 \
	"$CASA/Palantir" "$CASA/Palantir/Entrada" "$CASA/Palantir/Exportacoes"
install -d -m 755 /etc/palantir /opt/palantir
echo "$USUARIO" >/etc/palantir/usuario

passo "Versão do Palantír"
# O gerenciador de versões vem do pacote, quando há um, ou deste repositório.
GERENCIADOR="$AQUI/palantir-versao"
if [ ! -f "$GERENCIADOR" ]; then
	erro "palantir-versao não está ao lado deste script"
fi
install -m 755 "$GERENCIADOR" /usr/local/sbin/palantir-versao
if [ -z "$PACOTE" ] && [ -z "$VERSAO" ]; then
	VERSAO="$(curl --fail --silent --show-error --location https://api.github.com/repos/wkoch/Palantir/releases/latest |
		sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1)"
	[ -n "$VERSAO" ] || erro "nenhuma release encontrada; use --pacote"
fi
if [ -n "$PACOTE" ]; then
	VERSAO="$(tar --extract --gzip --file "$PACOTE" --to-stdout --wildcards '*/VERSAO')"
fi
[ -n "$VERSAO" ] || erro "não foi possível descobrir a versão"
if [ "$(basename "$(readlink -f /opt/palantir/atual 2>/dev/null || echo nenhuma)")" != "$VERSAO" ]; then
	/usr/local/sbin/palantir-versao instalar "${PACOTE:-$VERSAO}"
fi
PASTA="/opt/palantir/versoes/$VERSAO"
install -m 755 "$PASTA/palantir-versao" /usr/local/sbin/palantir-versao
ln -sfn /opt/palantir/atual/palantird /usr/local/bin/palantird

passo "Serviços e sessão"
install -m 644 "$PASTA/palantird.service" /etc/systemd/user/palantird.service
install -d /etc/systemd/user/palantird.service.d
printf '[Unit]\nConditionUser=%s\n' "$USUARIO" >/etc/systemd/user/palantird.service.d/usuario.conf
systemctl --global enable palantird.service
sed -e "s/@USUARIO@/$USUARIO/g" -e "s/@UID@/$UID_USUARIO/g" "$PASTA/palantir-sessao.service" \
	>/etc/systemd/system/palantir-sessao.service
install -m 644 "$PASTA/pam-palantir-sessao" /etc/pam.d/palantir-sessao
install -d /etc/chromium/policies/managed
install -m 644 "$PASTA/chromium-politica.json" /etc/chromium/policies/managed/palantir.json
if [ ! -f /etc/palantir/quiosque.env ]; then
	cat >/etc/palantir/quiosque.env <<'FIM'
# Escala da interface na tela de 13 polegadas do HP.
PALANTIR_ESCALA=1.25
# Se a tela demorar a atualizar (comum em máquinas virtuais), descomente:
# WLR_RENDERER=pixman
FIM
fi

passo "Ativação"
/usr/local/sbin/palantir-versao ativar "$VERSAO"
systemctl daemon-reload
systemctl enable palantir-sessao.service
systemctl set-default graphical.target

if [ -n "$CHAVE_SSH" ]; then
	passo "SSH só por chave, na rede local"
	apt-get install --yes --no-install-recommends openssh-server
	install -d -o "$USUARIO" -g "$(id -gn "$USUARIO")" -m 700 "$CASA/.ssh"
	AUTORIZADAS="$CASA/.ssh/authorized_keys"
	grep -qxF "$CHAVE_SSH" "$AUTORIZADAS" 2>/dev/null || echo "$CHAVE_SSH" >>"$AUTORIZADAS"
	chown "$USUARIO:$(id -gn "$USUARIO")" "$AUTORIZADAS"
	chmod 600 "$AUTORIZADAS"
	cat >/etc/ssh/sshd_config.d/10-palantir.conf <<FIM
PasswordAuthentication no
KbdInteractiveAuthentication no
PermitRootLogin no
AllowUsers $USUARIO@10.0.0.0/8 $USUARIO@172.16.0.0/12 $USUARIO@192.168.0.0/16 $USUARIO@fe80::/10 $USUARIO@fd00::/8
FIM
	sshd -t
	systemctl enable --now ssh.service
	systemctl reload ssh.service
fi

if [ "$NM" -eq 1 ]; then
	passo "Rede pelo NetworkManager"
	if [ -f /etc/network/interfaces ]; then
		cp --no-clobber /etc/network/interfaces /etc/network/interfaces.antes-do-palantir
		printf 'auto lo\niface lo inet loopback\n' >/etc/network/interfaces
	fi
	systemctl enable NetworkManager.service
	echo "Depois de reiniciar, abra o console (Ctrl+Alt+F2) e configure a rede com: nmtui"
elif grep -Eq '^[[:space:]]*iface[[:space:]]+(en|wl|eth)' /etc/network/interfaces 2>/dev/null; then
	echo
	echo "A rede continua configurada em /etc/network/interfaces."
	echo "Para usar o nmtui, execute de novo com --usar-networkmanager."
fi

passo "Conferência"
runuser -u "$USUARIO" -- env -C / HOME="$CASA" XDG_RUNTIME_DIR="/run/user/$UID_USUARIO" \
	DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/$UID_USUARIO/bus" /opt/palantir/atual/palantird diagnostico ||
	erro "o diagnóstico encontrou falhas"

cat <<FIM

Palantír $VERSAO instalado para $USUARIO.
Depois de reiniciar, a fila de atenção aparece sozinha no tty1.
Console de emergência: Ctrl+Alt+F2.
Primeiro passo na tela: Ajustes → chave do OpenRouter.
FIM
if [ "$REINICIAR" -eq 1 ] && [ -t 0 ]; then
	read -r -p "Reiniciar agora? [s/N] " resposta
	[[ "$resposta" =~ ^[sS] ]] && systemctl reboot
fi
