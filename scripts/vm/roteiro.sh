#!/bin/bash
# Roda dentro do contêiner: instala o pacote num Debian 13 virtual, entra pela
# tela de login como uma pessoa e confere a sessão. Capturas de tela ficam em
# /saida/vm.
set -euo pipefail

PACOTE="/saida/$1"
TRABALHO=/trabalho
BASE=/cache/debian-13-generic-amd64.qcow2
MONITOR=$TRABALHO/monitor.sock
# Senha da conta criada só para esta máquina virtual descartável.
SENHA=testedavm1
OPCOES=(-i "$TRABALHO/chave" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR
	-o ConnectTimeout=5)

mkdir -p "$TRABALHO" /saida/vm
cd "$TRABALHO"

dizer() { printf '\n== %s\n' "$*"; }
falhar() {
	echo "FALHOU: $*" >&2
	tela falha || true
	vm 'sudo journalctl -b --no-pager -u greetd | tail -30; journalctl --user -b --no-pager | tail -60' || true
	# Na depuração a máquina continua ligada para inspeção.
	[ -z "${PARAR_APOS_BOOT:-}${MANTER_SE_FALHAR:-}" ] || { echo "máquina mantida para depuração"; sleep "${DURACAO_DEPURACAO:-3000}"; }
	exit 1
}
vm() { ssh "${OPCOES[@]}" -p 2222 teste@127.0.0.1 "$@"; }
copiar() { scp "${OPCOES[@]}" -P 2222 "$1" "teste@127.0.0.1:$2"; }
monitor() { printf '%s\n' "$1" | socat -t 2 - "UNIX-CONNECT:$MONITOR" >/dev/null; }
tela() {
	sleep 1
	monitor "screendump /saida/vm/$1.png -f png"
	sleep 1
	echo "captura: saida/vm/$1.png"
}
tecla() { monitor "sendkey $1"; sleep 0.08; }
digitar() {
	local texto="$1" i c
	for ((i = 0; i < ${#texto}; i++)); do
		c="${texto:i:1}"
		case "$c" in
		' ') tecla spc ;;
		.) tecla dot ;;
		[a-z0-9]) tecla "$c" ;;
		[A-Z]) tecla "shift-${c,,}" ;;
		*) falhar "caractere sem tecla: $c" ;;
		esac
	done
}
esperar() {
	local descricao="$1" tentativas="$2"
	shift 2
	for _ in $(seq "$tentativas"); do
		if "$@" >/dev/null 2>&1; then return 0; fi
		sleep 2
	done
	falhar "tempo esgotado: $descricao"
}
# api fala com o daemon como o dono da conta.
api() {
	local rota="$1"
	shift
	vm "curl -sf $* -H 'X-Palantir: 1' -H \"Authorization: Bearer \$(cat ~/.config/palantir/acesso)\" \
		-H 'Content-Type: application/json' 'http://127.0.0.1:7700$rota'"
}
api_contem() { api "$1" | grep -q "$2"; }

dizer "Máquina virtual Debian 13"
ssh-keygen -q -t ed25519 -N '' -f chave
cat >user-data <<FIM
#cloud-config
hostname: palantir-vm
keyboard: {layout: br}
users:
  - name: teste
    uid: 1000
    gecos: Pessoa de Teste
    sudo: "ALL=(ALL) NOPASSWD:ALL"
    shell: /bin/bash
    lock_passwd: false
    ssh_authorized_keys: ["$(cat chave.pub)"]
chpasswd:
  expire: false
  users:
    - {name: teste, password: $SENHA, type: text}
FIM
printf 'instance-id: palantir-vm-1\nlocal-hostname: palantir-vm\n' >meta-data
xorriso -as mkisofs -quiet -output seed.iso -volid cidata -joliet -rock user-data meta-data
qemu-img create -q -f qcow2 -F qcow2 -b "$BASE" disco.qcow2 28G
qemu-img create -q -f raw pendrive.img 2G
qemu-system-x86_64 -machine q35,accel=kvm -cpu host -smp 4 -m 6144 \
	-drive file=disco.qcow2,if=virtio -drive file=seed.iso,if=virtio,format=raw,readonly=on \
	-nic user,model=virtio-net-pci,hostfwd=tcp:127.0.0.1:2222-:22 \
	-device virtio-vga,xres=1920,yres=1080 -device virtio-keyboard-pci -device virtio-tablet-pci \
	-device qemu-xhci -drive if=none,id=pendrive,file=pendrive.img,format=raw \
	-device usb-storage,drive=pendrive,removable=on \
	-display none -monitor "unix:$MONITOR,server,nowait" -serial "file:/saida/vm/serial.log" -daemonize
esperar "SSH" 90 vm true
vm 'cloud-init status --wait >/dev/null || true; . /etc/os-release; echo "$PRETTY_NAME"'

dizer "Pós-instalação, como no guia"
copiar "$PACOTE" /tmp/pacote.tar.gz
vm 'cp /tmp/pacote.tar.gz ~/pacote.tar.gz && mkdir -p /tmp/p && tar -xzf /tmp/pacote.tar.gz -C /tmp/p'
# O script roda de dentro do pacote extraído, e a conta vem do sudo.
vm 'sudo /tmp/p/*/pos-instalacao.sh --sem-reiniciar' || falhar "pos-instalacao.sh"
dizer "A pós-instalação pode ser repetida"
vm 'sudo /tmp/p/*/pos-instalacao.sh --usuario teste --pacote /tmp/pacote.tar.gz --sem-reiniciar >/tmp/segunda.log 2>&1' ||
	{ vm 'tail -30 /tmp/segunda.log'; falhar "segunda execução"; }
vm 'apt-cache policy openssl | grep -q pkg.noctalia.dev' && falhar "o repositório de terceiros alcança outros pacotes"

reiniciar() {
	local antes
	antes="$(vm 'cat /proc/sys/kernel/random/boot_id')"
	vm 'sudo systemctl reboot' || true
	esperar "reinício" 90 bash -c "n=\$(ssh ${OPCOES[*]} -p 2222 teste@127.0.0.1 cat /proc/sys/kernel/random/boot_id) && [ \"\$n\" != '$antes' ]"
}

# no_usuario roda um comando com o barramento da sessão, como um terminal dela.
no_usuario() {
	vm "export XDG_RUNTIME_DIR=/run/user/1000 DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus; $1"
}
sessao() { vm 'loginctl list-sessions --no-legend | awk "/seat0/ && /teste/ {print \$1}"'; }
bloqueada() { [ "$(vm "loginctl show-session $(sessao) -p LockedHint --value")" = "$1" ]; }
# O navegador só mantém o canal de avisos aberto se a chave de acesso foi aceita.
interface_aberta() { vm 'ss -tnp | grep -q "127.0.0.1:7700 .*chromium"'; }
na_tela_de_login() { vm 'systemctl is-active --quiet greetd && pgrep -x noctalia-greeter >/dev/null || pgrep -f /usr/bin/noctalia-greeter\$ >/dev/null'; }
entrar() {
	digitar "$SENHA"
	tecla ret
	esperar "sessão do usuário" 60 vm 'pgrep -u teste -x umbriel >/dev/null && pgrep -u teste -x noctalia >/dev/null'
	esperar "Palantír aberto e autorizado" 60 interface_aberta
	sleep 12
}

dizer "Reinício: aparece a tela de login"
reiniciar
esperar "tela de login" 60 na_tela_de_login
sleep 15
tela 01-login

# Para depurar: deixa a máquina ligada na tela de login.
if [ -n "${PARAR_APOS_BOOT:-}" ]; then
	echo "VM ligada; SSH na porta 2222 do contêiner."
	sleep "${DURACAO_DEPURACAO:-3000}"
	exit 0
fi

dizer "Login com a senha: o Palantír abre sozinho, já autorizado"
entrar
tela 02-palantir
vm 'noctalia config validate' || falhar "configuração do Noctalia"
vm 'umbriel config validate -c /etc/xdg/umbriel/config.toml' || falhar "configuração do Umbriel"
vm 'grep -q "^layout = \"br\"" /etc/xdg/umbriel/config.toml' || falhar "o teclado do Debian não chegou ao compositor"

dizer "Chave guardada no keyring destravado pelo login"
no_usuario 'echo chave-de-teste-da-vm | timeout 40 palantird segredo definir openrouter' || falhar "keyring"
vm 'test -f ~/.local/share/keyrings/login.keyring && test ! -e ~/.config/palantir/segredos.json' ||
	falhar "a chave não está no keyring"

dizer "Uso pelo teclado"
tecla n
sleep 2
digitar "Teste feito no teclado da maquina virtual"
tecla ret
esperar "atividade criada pela interface" 20 api_contem /api/estado 'Teste feito no teclado'
sleep 3
tela 03-atividade

dizer "Um endereço sozinho abre o site"
tecla g
tecla f
sleep 2
tecla n
sleep 2
digitar debian.org
tecla ret
sleep 20
tela 04-site
api_contem /api/estado 'debian.org' && falhar "o endereço virou atividade"
tecla ctrl-w
sleep 3

dizer "Bloqueio de tela: só a senha libera"
vm "loginctl lock-session $(sessao)" || falhar "pedido de bloqueio"
esperar "bloqueio" 20 bloqueada yes
sleep 5
tela 05-bloqueio
# Com a tela bloqueada, o que se digita não chega ao Palantír.
tecla n
digitar intruso
tecla ret
sleep 8
bloqueada yes || falhar "uma senha errada liberou a sessão"
api_contem /api/estado intruso && falhar "o teclado atravessou o bloqueio"
tecla esc
sleep 3
digitar "$SENHA"
tecla ret
esperar "desbloqueio com a senha" 30 bloqueada no
sleep 3

dizer "Ferramentas isoladas e busca"
no_usuario 'palantird diagnostico' | tee /saida/vm/diagnostico.txt
grep -q "ok    ferramentas" /saida/vm/diagnostico.txt || falhar "ferramentas não isoladas"
vm 'printf "Contrato de aluguel de teste. O aluguel vence no dia cinco.\n" >~/Palantir/Entrada/contrato.txt'
esperar "documento extraído num contêiner sem rede" 40 api_contem /api/acervo/documentos '"estado":"pronto"'
esperar "mecanismo de busca" 40 vm 'curl -sf -m 30 "http://127.0.0.1:8888/search?q=debian&format=json" | grep -q "\"url\""'
vm 'sudo ss -ltn | grep -E ":(7700|8888) " | grep -vq "127.0.0.1:" && exit 1 || exit 0' ||
	falhar "o daemon e a busca só podem escutar em 127.0.0.1"

dizer "Cópia para um pendrive"
vm 'sudo DEBIAN_FRONTEND=noninteractive apt-get -o Acquire::Retries=5 install -y -qq dosfstools >/dev/null'
PEN="$(vm "lsblk -nrpo NAME,TRAN | awk '\$2==\"usb\" {print \$1}'")"
[ -n "$PEN" ] || falhar "o pendrive virtual não apareceu"
vm "sudo mkfs.vfat -I -n PENDRIVE $PEN >/dev/null && sudo udevadm settle"
sleep 3
api_contem /api/discos "$PEN" || falhar "o pendrive não está entre os destinos"
api /api/discos | grep -q '/dev/vda' && falhar "o disco do sistema foi oferecido"
api /api/copia -X POST -d "'{\"dispositivo\":\"$PEN\",\"confirmar\":true}'" >/dev/null || falhar "pedido de cópia"
esperar "cópia concluída" 60 api_contem /api/discos '"arquivo":"palantir-'
vm "udisksctl mount -b $PEN --no-user-interaction >/dev/null && \
	python3 -c 'import glob,zipfile; z=zipfile.ZipFile(glob.glob(\"/media/teste/PENDRIVE/Palantir-copias/*.zip\")[0]); assert z.testzip() is None and \"banco/palantir.db\" in z.namelist()' && \
	udisksctl unmount -b $PEN --no-user-interaction >/dev/null" || falhar "a cópia no pendrive"

dizer "Segundo usuário, isolado"
# O /tmp é apagado a cada reinício.
vm 'rm -rf /tmp/p && mkdir -p /tmp/p && tar -xzf ~/pacote.tar.gz -C /tmp/p'
vm "sudo useradd -m -s /bin/bash visita && sudo bash -c '/tmp/p/*/pos-instalacao.sh --usuario visita --pacote /home/teste/pacote.tar.gz --sem-busca --sem-reiniciar' >/tmp/visita.log 2>&1" ||
	{ vm 'tail -30 /tmp/visita.log'; falhar "instalação do segundo usuário"; }
vm 'sudo loginctl enable-linger visita'
esperar "daemon do segundo usuário" 90 vm 'curl -sf http://127.0.0.1:7701/api/saude'
[ "$(vm 'curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $(cat ~/.config/palantir/acesso)" http://127.0.0.1:7701/api/estado')" = 401 ] ||
	falhar "a chave de um usuário abriu o Palantír do outro"
vm 'sudo -u visita cat /home/teste/.config/palantir/acesso' && falhar "um usuário leu a chave do outro"
vm 'sudo loginctl disable-linger visita'

dizer "Atualizar e voltar de versão"
VERSAO="$(vm 'cat /opt/palantir/atual/VERSAO')"
# O padrão ancorado não casa com o próprio shell que executa o comando.
em_execucao() { [ "$(vm 'readlink /proc/$(pgrep -u teste -f "^/opt/palantir/atual/palantird servir")/exe')" = "/opt/palantir/versoes/$1/palantird" ]; }
vm 'rm -rf /tmp/n && mkdir /tmp/n && tar -xzf ~/pacote.tar.gz -C /tmp/n && echo v9.9.9 >$(ls -d /tmp/n/*)/VERSAO && cd /tmp/n && tar -czf /tmp/nova.tar.gz *'
vm 'sudo palantir-versao instalar /tmp/nova.tar.gz && sudo palantir-versao ativar v9.9.9 >/tmp/ativar.log 2>&1' || falhar "ativar"
esperar "daemon na versão nova" 90 em_execucao v9.9.9
vm 'sudo palantir-versao voltar && sudo palantir-versao listar' || falhar "voltar"
esperar "daemon na versão anterior" 90 em_execucao "$VERSAO"

dizer "Reiniciar preserva atividades e chaves"
reiniciar
esperar "tela de login" 60 na_tela_de_login
sleep 15
# Com dois usuários, a tela de login pede primeiro a escolha: Enter confirma o
# primeiro da lista.
tela 06-escolha-de-usuario
tecla ret
sleep 3
entrar
api_contem /api/estado 'Teste feito no teclado' || falhar "a atividade se perdeu"
no_usuario 'palantird segredo listar' | grep -q '^openrouter   definido' || falhar "a chave se perdeu"
tela 07-apos-reiniciar

# Por último: a placa de vídeo virtual não se recupera da suspensão. A tela
# fica apagada, as teclas se perdem e um desligamento trava o kernel da máquina
# virtual; por isso aqui só se confere que o computador volta bloqueado.
dizer "Suspensão: volta na tela de bloqueio"
vm '(sleep 1; sudo systemctl suspend) >/dev/null 2>&1 &'
suspensa() { printf 'info status\n' | socat -t 2 - "UNIX-CONNECT:$MONITOR" | grep -q suspended; }
esperar "suspensão" 30 suspensa
monitor system_wakeup
esperar "retorno" 30 vm true
sleep 8
bloqueada yes || falhar "voltou sem bloqueio"

monitor quit || true
dizer "Todos os critérios conferidos na máquina virtual"
