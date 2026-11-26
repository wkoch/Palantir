#!/bin/bash
# Roda dentro do contêiner: instala o pacote num Debian 13 virtual e confere os
# critérios da fase P0. Capturas de tela ficam em /saida.
set -euo pipefail

PACOTE="/saida/$1"
TRABALHO=/trabalho
BASE=/cache/debian-13-generic-amd64.qcow2
MONITOR=$TRABALHO/monitor.sock
OPCOES=(-i "$TRABALHO/chave" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR
	-o ConnectTimeout=5)

mkdir -p "$TRABALHO" /saida/vm
cd "$TRABALHO"

dizer() { printf '\n== %s\n' "$*"; }
falhar() {
	echo "FALHOU: $*" >&2
	tela falha || true
	vm 'sudo journalctl -b --no-pager -u palantir-sessao | tail -40; journalctl --user -b --no-pager -u palantird | tail -20' || true
	exit 1
}
vm() { ssh "${OPCOES[@]}" -p 2222 palantir@127.0.0.1 "$@"; }
copiar() { scp "${OPCOES[@]}" -P 2222 "$1" "palantir@127.0.0.1:$2"; }
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

dizer "Máquina virtual Debian 13"
ssh-keygen -q -t ed25519 -N '' -f chave
cat >user-data <<FIM
#cloud-config
hostname: palantir-vm
keyboard: {layout: br}
users:
  - name: palantir
    sudo: "ALL=(ALL) NOPASSWD:ALL"
    shell: /bin/bash
    ssh_authorized_keys: ["$(cat chave.pub)"]
FIM
printf 'instance-id: palantir-vm-1\nlocal-hostname: palantir-vm\n' >meta-data
xorriso -as mkisofs -quiet -output seed.iso -volid cidata -joliet -rock user-data meta-data
qemu-img create -q -f qcow2 -F qcow2 -b "$BASE" disco.qcow2 16G
qemu-system-x86_64 -machine q35,accel=kvm -cpu host -smp 4 -m 4096 \
	-drive file=disco.qcow2,if=virtio -drive file=seed.iso,if=virtio,format=raw,readonly=on \
	-nic user,model=virtio-net-pci,hostfwd=tcp:127.0.0.1:2222-:22 \
	-device virtio-vga,xres=1920,yres=1080 -device virtio-keyboard-pci -device virtio-tablet-pci \
	-display none -monitor "unix:$MONITOR,server,nowait" -serial "file:/saida/vm/serial.log" -daemonize
esperar "SSH" 90 vm true
vm 'cloud-init status --wait >/dev/null || true; . /etc/os-release; echo "$PRETTY_NAME"'

dizer "Pós-instalação"
copiar "$PACOTE" /tmp/pacote.tar.gz
vm 'cp /tmp/pacote.tar.gz ~/pacote.tar.gz'
vm 'mkdir -p /tmp/p && tar -xzf /tmp/pacote.tar.gz -C /tmp/p'
vm 'sudo /tmp/p/*/pos-instalacao.sh --usuario palantir --pacote /tmp/pacote.tar.gz --sem-reiniciar' ||
	falhar "pos-instalacao.sh"
dizer "A pós-instalação pode ser repetida"
vm 'sudo /tmp/p/*/pos-instalacao.sh --usuario palantir --pacote /tmp/pacote.tar.gz --sem-reiniciar >/tmp/segunda.log 2>&1' ||
	{ vm 'tail -30 /tmp/segunda.log'; falhar "segunda execução"; }

reiniciar() {
	local antes
	antes="$(vm 'cat /proc/sys/kernel/random/boot_id')"
	vm 'sudo systemctl reboot' || true
	esperar "reinício" 90 bash -c "n=\$(ssh ${OPCOES[*]} -p 2222 palantir@127.0.0.1 cat /proc/sys/kernel/random/boot_id) && [ \"\$n\" != '$antes' ]"
}
sessao_no_ar() { vm 'systemctl is-active --quiet palantir-sessao && pgrep -f "chromium.*--kiosk" >/dev/null'; }
saude() { vm 'curl -sf http://127.0.0.1:7700/api/saude'; }
api() { vm "curl -sf -H 'X-Palantir: 1' -H 'Content-Type: application/json' $*"; }

# A placa de vídeo virtual atrasa um quadro com o renderizador OpenGL do
# wlroots; no HP, com a GPU de verdade, o padrão é mantido.
vm 'echo WLR_RENDERER=pixman | sudo tee -a /etc/palantir/quiosque.env >/dev/null'

dizer "Reinício: a fila de atenção aparece sem interação"
reiniciar
esperar "daemon" 60 saude
esperar "sessão kiosk" 60 sessao_no_ar
sleep 12
tela 01-fila-apos-boot
vm 'echo "console ativo: $(cat /sys/class/tty/tty0/active)"'
[ "$(vm 'cat /sys/class/tty/tty0/active')" = tty1 ] || falhar "o tty1 deveria estar ativo"

# Para depurar: deixa a máquina ligada depois do primeiro boot.
if [ -n "${PARAR_APOS_BOOT:-}" ]; then
	echo "VM ligada; SSH na porta 2222 do contêiner."
	exit 0
fi

dizer "Diagnóstico com as ferramentas em contêiner"
vm 'palantird diagnostico' | tee /saida/vm/diagnostico.txt
grep -q "ok    ferramentas" /saida/vm/diagnostico.txt || falhar "ferramentas não isoladas"

dizer "Importação extraída por contêiner sem rede"
vm 'printf "Contrato de aluguel de teste. O aluguel vence no dia cinco.\n" >~/Palantir/Entrada/contrato.txt'
esperar "documento extraído" 40 bash -c "ssh ${OPCOES[*]} -p 2222 palantir@127.0.0.1 curl -sf http://127.0.0.1:7700/api/acervo/documentos | grep -q '\"estado\":\"pronto\"'"
vm "curl -sf 'http://127.0.0.1:7700/api/acervo/busca?q=aluguel'" | grep -q 'contrato.txt' || falhar "busca"
vm 'palantird verificar'

dizer "Uso pelo teclado, dentro do kiosk"
tecla n
sleep 2
digitar "Teste feito no teclado da maquina virtual"
tecla ret
esperar "atividade criada pela interface" 20 bash -c "ssh ${OPCOES[*]} -p 2222 palantir@127.0.0.1 curl -sf http://127.0.0.1:7700/api/estado | grep -q 'Teste feito no teclado'"
sleep 3
tela 02-atividade-criada-no-kiosk
tecla g
tecla a
sleep 3
tela 03-acervo
tecla g
tecla j
sleep 3
tela 04-ajustes

dizer "Leitor de PDF dentro do kiosk"
python3 - >teste.pdf <<'FIM'
import sys
fluxo = b"BT /F1 28 Tf 60 700 Td (PDF aberto dentro do kiosk do Palantir) Tj ET"
objetos = [b"<< /Type /Catalog /Pages 2 0 R >>", b"<< /Type /Pages /Kids [4 0 R] /Count 1 >>",
    b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
    b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 3 0 R >> >> /Contents 5 0 R >>",
    b"<< /Length %d >>\nstream\n%s\nendstream" % (len(fluxo), fluxo)]
s, pos = bytearray(b"%PDF-1.4\n"), []
for i, o in enumerate(objetos, 1):
    pos.append(len(s)); s += b"%d 0 obj\n%s\nendobj\n" % (i, o)
x = len(s)
s += b"xref\n0 6\n0000000000 65535 f \n" + b"".join(b"%010d 00000 n \n" % p for p in pos)
s += b"trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n" % x
sys.stdout.buffer.write(bytes(s))
FIM
copiar teste.pdf Palantir/Entrada/leitura.pdf
esperar "PDF extraído por contêiner" 40 bash -c "ssh ${OPCOES[*]} -p 2222 palantir@127.0.0.1 curl -sf 'http://127.0.0.1:7700/api/acervo/busca?q=kiosk' | grep -q leitura.pdf"
ID_PDF="$(api "'http://127.0.0.1:7700/api/acervo/documentos?nome=leitura.pdf'" | python3 -c 'import json,sys; print(json.load(sys.stdin)["documentos"][0]["id"])')"
vm "echo PALANTIR_PAGINA=http://127.0.0.1:7700/api/acervo/documentos/$ID_PDF/original | sudo tee -a /etc/palantir/quiosque.env >/dev/null && sudo systemctl restart palantir-sessao"
esperar "sessão com o PDF" 45 sessao_no_ar
sleep 12
tela 08-pdf-no-kiosk
vm "sudo sed -i /PALANTIR_PAGINA/d /etc/palantir/quiosque.env && sudo systemctl restart palantir-sessao"
esperar "sessão kiosk" 45 sessao_no_ar
sleep 8

dizer "Chromium encerrado: a sessão volta sozinha"
ANTES="$(vm 'pgrep -of "chromium.*--kiosk"')"
vm "sudo kill -9 $ANTES"
voltou() { sessao_no_ar && [ "$(vm 'pgrep -of "chromium.*--kiosk"')" != "$ANTES" ]; }
esperar "sessão de volta" 45 voltou
sleep 10
tela 05-sessao-de-volta

dizer "Console de emergência no tty2"
tecla ctrl-alt-f2
sleep 3
tela 06-tty2
[ "$(vm 'cat /sys/class/tty/tty0/active')" = tty2 ] || falhar "Ctrl+Alt+F2 não trocou de console"
vm 'systemctl is-active getty@tty2.service' || falhar "sem login no tty2"
tecla ctrl-alt-f1
sleep 3
[ "$(vm 'cat /sys/class/tty/tty0/active')" = tty1 ] || falhar "não voltou ao tty1"

dizer "Isolamento do navegador"
vm 'cat /etc/chromium/policies/managed/palantir.json | python3 -c "import json,sys; p=json.load(sys.stdin); assert p[\"URLBlocklist\"]==[\"*\"]"'
vm 'sudo ss -ltnp | grep 7700' | grep -q '127.0.0.1:7700' || falhar "o daemon precisa escutar só em 127.0.0.1"

dizer "Atualizar e voltar de versão"
VERSAO="$(vm 'cat /opt/palantir/atual/VERSAO')"
vm 'rm -rf /tmp/n && mkdir /tmp/n && tar -xzf ~/pacote.tar.gz -C /tmp/n && echo v9.9.9 | tee /tmp/n/*/VERSAO'
vm 'cd /tmp/n && tar -czf /tmp/nova.tar.gz *'
vm 'sudo palantir-versao instalar /tmp/nova.tar.gz && sudo palantir-versao ativar v9.9.9 && sudo palantir-versao listar'
esperar "daemon após atualizar" 60 saude
vm 'sudo palantir-versao voltar && sudo palantir-versao listar'
esperar "daemon após voltar" 60 saude
[ "$(vm 'cat /opt/palantir/atual/VERSAO')" = "$VERSAO" ] || falhar "voltar não restaurou $VERSAO"

dizer "Reiniciar preserva as atividades"
reiniciar
esperar "daemon" 60 saude
esperar "sessão kiosk" 60 sessao_no_ar
api "http://127.0.0.1:7700/api/estado" | grep -q 'Teste feito no teclado' || falhar "a atividade se perdeu"
sleep 10
tela 07-apos-reiniciar

dizer "Log da sessão"
vm 'sudo journalctl -b --no-pager -u palantir-sessao | tail -15' || true
monitor quit || true
dizer "Todos os critérios conferidos na máquina virtual"
