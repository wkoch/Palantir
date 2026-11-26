#!/bin/bash
# Pós-instalação do Palantír sobre um Debian 13 (trixie) sem desktop: a sessão
# Noctalia, com login e tela de bloqueio, o Palantír de cada usuário e o
# mecanismo de busca. Pode ser executado de novo: cada passo confere o que já
# está feito.
set -euo pipefail

AQUI="$(cd "$(dirname "$(readlink -f "$0")")" && pwd)"

# Impressão digital da chave que assina o repositório pkg.noctalia.dev.
CHAVE_NOCTALIA=C2A7E07FB3D746BDF2A245CD683B67D7BC99A9B7
REPO_NOCTALIA=https://pkg.noctalia.dev/deb

uso() {
	cat <<'FIM'
uso: sudo ./pos-instalacao.sh [opções]

  --usuario NOME       conta a preparar (padrão: quem chamou o sudo); repita
                       o comando para preparar outras
  --pacote ARQUIVO     instala este pacote .tar.gz em vez de baixar a release
  --versao vX.Y.Z      baixa esta release (padrão: a mais recente)
  --manter-rede        não passa a rede para o NetworkManager (o Wi-Fi não
                       aparecerá no Noctalia)
  --sem-busca          não instala o mecanismo de busca
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
NM=1
BUSCA=1
CHAVE_SSH=""
REINICIAR=1
while [ $# -gt 0 ]; do
	case "$1" in
	--usuario) USUARIO="${2:-}"; shift 2 ;;
	--pacote) PACOTE="${2:-}"; shift 2 ;;
	--versao) VERSAO="${2:-}"; shift 2 ;;
	--manter-rede) NM=0; shift ;;
	--sem-busca) BUSCA=0; shift ;;
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
[ "$UID_USUARIO" -ge 1000 ] || erro "$USUARIO é uma conta do sistema"
GRUPO="$(id -gn "$USUARIO")"
CASA="$(getent passwd "$USUARIO" | cut -d: -f6)"

TEMPORARIO="$(mktemp -d)"
trap 'rm -rf "$TEMPORARIO"' EXIT

# Um pacote ao lado do script (script extraído do próprio pacote) é usado direto.
if [ -z "$PACOTE" ] && [ -z "$VERSAO" ] && [ -x "$AQUI/palantird" ] && [ -f "$AQUI/VERSAO" ]; then
	PACOTE="$TEMPORARIO/pacote.tar.gz"
	tar --create --gzip --file "$PACOTE" --directory "$(dirname "$AQUI")" "$(basename "$AQUI")"
fi
[ -z "$PACOTE" ] || [ -f "$PACOTE" ] || erro "pacote não encontrado: $PACOTE"
[ -f "$AQUI/palantir-versao" ] || erro "palantir-versao não está ao lado deste script"
[ -d "$AQUI/sessao" ] || erro "a pasta sessao não está ao lado deste script"

export DEBIAN_FRONTEND=noninteractive
# Um espelho que falha por um instante não derruba a instalação.
baixar_pacotes() { apt-get -o Acquire::Retries=5 "$@"; }

passo "Repositórios: Debian backports e Noctalia"
baixar_pacotes update
baixar_pacotes install --yes --no-install-recommends ca-certificates curl gpg
echo 'deb http://deb.debian.org/debian trixie-backports main contrib non-free-firmware' \
	>/etc/apt/sources.list.d/palantir-backports.list
curl --fail --silent --show-error --location --output "$TEMPORARIO/chave.deb" "$REPO_NOCTALIA/nickh-archive-keyring.deb"
# O pacote da chave não é instalado às cegas: só o arquivo da chave é extraído,
# e a impressão digital precisa ser a conhecida.
dpkg-deb --extract "$TEMPORARIO/chave.deb" "$TEMPORARIO/chave"
ARQUIVO_CHAVE="$TEMPORARIO/chave/usr/share/keyrings/nickh-archive-keyring.gpg"
IMPRESSOES="$(GNUPGHOME="$(mktemp -d -p "$TEMPORARIO")" gpg --batch --quiet --show-keys --with-colons "$ARQUIVO_CHAVE" |
	awk -F: '$1=="pub"{p=1} $1=="fpr"&&p{print $10; p=0}')"
[ "$IMPRESSOES" = "$CHAVE_NOCTALIA" ] ||
	erro "a chave do repositório Noctalia não é a esperada: ${IMPRESSOES:-nenhuma}"
install -D -m 644 "$ARQUIVO_CHAVE" /usr/share/keyrings/nickh-archive-keyring.gpg
cat >/etc/apt/sources.list.d/noctalia.sources <<FIM
Types: deb
URIs: $REPO_NOCTALIA/
Signed-By: /usr/share/keyrings/nickh-archive-keyring.gpg
Suites: trixie/ trixie-backports/
Architectures: amd64
FIM
install -m 644 "$AQUI/sessao/apt-noctalia.pref" /etc/apt/preferences.d/palantir-noctalia
baixar_pacotes update

passo "Pacotes da sessão e do Palantír"
# -t trixie-backports: o Umbriel exige bibliotecas gráficas mais novas.
baixar_pacotes install --yes --no-install-recommends -t trixie-backports \
	noctalia umbriel noctalia-greeter xdg-desktop-portal-umbriel luciole nickh-archive-keyring greetd \
	kitty chromium chromium-l10n xwayland xdg-desktop-portal xdg-desktop-portal-gtk xdg-utils \
	network-manager bluez pipewire pipewire-pulse pipewire-audio wireplumber libspa-0.2-bluetooth rtkit \
	gnome-keyring libpam-gnome-keyring libsecret-tools polkitd \
	udisks2 upower power-profiles-daemon sound-theme-freedesktop wl-clipboard \
	podman uidmap passt netavark aardvark-dns nftables crun containers-storage \
	dbus-user-session libpam-systemd libgl1-mesa-dri mesa-vulkan-drivers \
	fonts-inter fonts-noto-core fonts-noto-color-emoji fonts-noto-mono
# Firmware da placa de vídeo AMD e do Wi-Fi Intel AX200 do HP.
baixar_pacotes install --yes --no-install-recommends firmware-amd-graphics firmware-iwlwifi ||
	echo "AVISO: firmware não instalado; habilite o componente non-free-firmware no apt."

passo "Versão do Palantír"
install -m 755 "$AQUI/palantir-versao" /usr/local/sbin/palantir-versao
install -d -m 755 /etc/palantir /opt/palantir
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
install -m 644 "$PASTA/palantird.service" /etc/systemd/user/palantird.service
systemctl --global enable palantird.service

passo "Sessão, aparência e atalhos"
install -D -m 644 "$PASTA/sessao/palantir.desktop" /usr/local/share/applications/palantir.desktop
install -D -m 644 "$PASTA/sessao/icone.svg" /usr/local/share/icons/hicolor/scalable/apps/palantir.svg
install -D -m 644 "$PASTA/sessao/mimeapps.list" /etc/xdg/mimeapps.list
install -D -m 644 "$PASTA/sessao/chromium" /etc/chromium.d/palantir
install -D -m 644 "$PASTA/sessao/50-palantir-montagem.rules" /etc/polkit-1/rules.d/50-palantir-montagem.rules

# O padrão do Umbriel, vindo do pacote, com os acréscimos do Palantír. Cada
# usuário pode copiá-lo para ~/.config/umbriel e mudar.
UMBRIEL=/etc/xdg/umbriel/config.toml
if [ ! -f "$UMBRIEL" ] || grep -q '^# Gerado pelo Palantír' "$UMBRIEL"; then
	TECLADO_LAYOUT="" TECLADO_VARIANTE="" TECLADO_MODELO=""
	if [ -r /etc/default/keyboard ]; then
		TECLADO_LAYOUT="$(sed -n 's/^XKBLAYOUT="\{0,1\}\([^"]*\).*/\1/p' /etc/default/keyboard)"
		TECLADO_VARIANTE="$(sed -n 's/^XKBVARIANT="\{0,1\}\([^"]*\).*/\1/p' /etc/default/keyboard)"
		TECLADO_MODELO="$(sed -n 's/^XKBMODEL="\{0,1\}\([^"]*\).*/\1/p' /etc/default/keyboard)"
	fi
	NOVO="$TEMPORARIO/umbriel.toml"
	{
		echo '# Gerado pelo Palantír a partir de /usr/share/umbriel/config.toml.'
		awk -v layout="$TECLADO_LAYOUT" -v variante="$TECLADO_VARIANTE" -v modelo="$TECLADO_MODELO" '
			{ print }
			/^\[general\][[:space:]]*$/ {
				print "autostart = [\"noctalia\", \"palantird abrir\"]"
				# A lista de atalhos cobriria o Palantír no primeiro login.
				print "show_cheatsheet = false"
			}
			/^\[keybinds\][[:space:]]*$/ { print "\"Mod+Shift+P\" = { action = \"spawn palantird abrir\", repeat = false }" }
			/^\[input\.keyboard\][[:space:]]*$/ {
				if (layout != "") print "layout = \"" layout "\""
				if (variante != "") print "variant = \"" variante "\""
				if (modelo != "") print "model = \"" modelo "\""
			}
		' /usr/share/umbriel/config.toml
		printf '\n# Tela de 13 polegadas do HP.\n[output.eDP-1]\nscale = 1.25\n'
		printf '\n# O navegador abre ocupando a tela.\n[[window_rule]]\nmatch.app_id = "^chromium"\ndefault_maximize = true\n'
	} >"$NOVO"
	for esperado in 'autostart = \["noctalia"' 'Mod+Shift+P'; do
		grep -q "$esperado" "$NOVO" || erro "a configuração padrão do Umbriel mudou de formato; não achei onde acrescentar $esperado"
	done
	umbriel config validate -c "$NOVO" || erro "a configuração gerada para o Umbriel não é válida"
	install -D -m 644 "$NOVO" "$UMBRIEL"
fi

# Tela de login do Noctalia, destravando o keyring com a senha digitada.
PAM=/etc/pam.d/greetd
grep -q 'pam_gnome_keyring.so' "$PAM" || sed -i \
	-e '/^@include common-auth/a auth optional pam_gnome_keyring.so' \
	-e '/^@include common-session/a session optional pam_gnome_keyring.so auto_start' "$PAM"
grep -q 'auto_start' "$PAM" || erro "não foi possível ligar o keyring ao login em $PAM"
# O keyring é iniciado pela tela de login, com a senha. Iniciado pelo systemd, no
# primeiro login ele não enxergava o cofre recém-criado e abria um pedido de
# senha que não aparecia.
systemctl --global disable gnome-keyring-daemon.socket gnome-keyring-daemon.service
systemctl enable greetd.service
systemctl set-default graphical.target

# Restos da sessão kiosk da v0.1.0.
if [ -f /etc/systemd/system/palantir-sessao.service ]; then
	systemctl disable palantir-sessao.service || true
	rm -f /etc/systemd/system/palantir-sessao.service /etc/pam.d/palantir-sessao \
		/etc/chromium/policies/managed/palantir.json /etc/palantir/quiosque.env /etc/palantir/usuario
	rm -rf /etc/systemd/user/palantird.service.d
fi

passo "Conta $USUARIO"
install -D -m 644 "$PASTA/sessao/noctalia.toml" /etc/skel/.config/noctalia/config.toml
usermod --append --groups plugdev,netdev "$USUARIO"
grep -q "^$USUARIO:" /etc/subuid || usermod --add-subuids 200000-265535 "$USUARIO"
grep -q "^$USUARIO:" /etc/subgid || usermod --add-subgids 200000-265535 "$USUARIO"
for pasta in "$CASA/.config" "$CASA/.config/noctalia" "$CASA/Palantir" "$CASA/Palantir/Entrada"; do
	[ -d "$pasta" ] || install -d -o "$USUARIO" -g "$GRUPO" -m 700 "$pasta"
done
[ -f "$CASA/.config/noctalia/config.toml" ] ||
	install -o "$USUARIO" -g "$GRUPO" -m 644 "$PASTA/sessao/noctalia.toml" "$CASA/.config/noctalia/config.toml"

passo "Ativação"
/usr/local/sbin/palantir-versao ativar "$VERSAO"

if [ "$BUSCA" -eq 1 ]; then
	passo "Mecanismo de busca (SearXNG)"
	CONFIG_BUSCA=/etc/palantir/busca-settings.yml
	if [ ! -f "$CONFIG_BUSCA" ]; then
		SEGREDO="$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')"
		(umask 077 && sed "s/@SEGREDO@/$SEGREDO/" "$PASTA/busca/settings.yml" >"$CONFIG_BUSCA")
		# O contêiner roda como o usuário 977.
		chown root:977 "$CONFIG_BUSCA"
		chmod 640 "$CONFIG_BUSCA"
	fi
	install -D -m 644 "$PASTA/busca/palantir-busca.container" /etc/containers/systemd/palantir-busca.container
	systemctl daemon-reload
	systemctl restart palantir-busca.service ||
		echo "AVISO: o mecanismo de busca não iniciou; veja: sudo journalctl -u palantir-busca"
fi

if [ -n "$CHAVE_SSH" ]; then
	passo "SSH só por chave, na rede local"
	baixar_pacotes install --yes --no-install-recommends openssh-server
	install -d -o "$USUARIO" -g "$GRUPO" -m 700 "$CASA/.ssh"
	AUTORIZADAS="$CASA/.ssh/authorized_keys"
	grep -qxF "$CHAVE_SSH" "$AUTORIZADAS" 2>/dev/null || echo "$CHAVE_SSH" >>"$AUTORIZADAS"
	chown "$USUARIO:$GRUPO" "$AUTORIZADAS"
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
	if grep -Eq '^[[:space:]]*iface[[:space:]]+(en|wl|eth)' /etc/network/interfaces 2>/dev/null; then
		cp --no-clobber /etc/network/interfaces /etc/network/interfaces.antes-do-palantir
		printf 'auto lo\niface lo inet loopback\n' >/etc/network/interfaces
		echo "Depois de reiniciar, o cabo conecta sozinho; o Wi-Fi é escolhido na barra do Noctalia."
	fi
	systemctl enable NetworkManager.service
fi

passo "Conferência"
# O gerenciador do usuário roda o Podman sem root.
systemctl start "user@$UID_USUARIO.service"
# Um gerenciador iniciado antes dos pacotes não tem o barramento D-Bus.
if [ ! -S "/run/user/$UID_USUARIO/bus" ]; then
	systemctl restart "user@$UID_USUARIO.service"
	for _ in $(seq 50); do
		[ -S "/run/user/$UID_USUARIO/bus" ] && break
		sleep 0.2
	done
	[ -S "/run/user/$UID_USUARIO/bus" ] || erro "o barramento da sessão de $USUARIO não iniciou (dbus-user-session)"
fi
como_usuario() {
	runuser -u "$USUARIO" -- env -C / HOME="$CASA" XDG_RUNTIME_DIR="/run/user/$UID_USUARIO" \
		DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/$UID_USUARIO/bus" "$@"
}
como_usuario /opt/palantir/atual/palantir-imagem
como_usuario /opt/palantir/atual/palantird diagnostico --instalacao || erro "o diagnóstico encontrou falhas"

cat <<FIM

Palantír $VERSAO instalado.
Reinicie: aparece a tela de login. Entre com a sua senha; o Palantír abre sozinho.
Atalhos: Super abre os programas, Super+Shift+P abre o Palantír, Super+Return o terminal.
Primeiro passo: Ajustes → chave do OpenRouter.
FIM
if [ "$REINICIAR" -eq 1 ] && [ -t 0 ]; then
	read -r -p "Reiniciar agora? [s/N] " resposta
	[[ "$resposta" =~ ^[sS] ]] && systemctl reboot
fi
