package discos_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wkoch/Palantir/internal/discos"
)

// arvore imita o lsblk: o disco do sistema cifrado, um pendrive com duas
// partições, um segundo disco interno e o que nunca serve.
const arvore = `{"blockdevices":[
 {"path":"/dev/nvme0n1","type":"disk","tran":"nvme","size":500,"children":[
   {"path":"/dev/nvme0n1p1","fstype":"vfat","type":"part","size":10,"mountpoints":["/boot/efi"]},
   {"path":"/dev/nvme0n1p2","fstype":"ext4","type":"part","size":10,"mountpoints":[null]},
   {"path":"/dev/nvme0n1p3","fstype":"crypto_LUKS","type":"part","size":480,"children":[
     {"path":"/dev/mapper/raiz","fstype":"ext4","type":"crypt","size":480,"mountpoints":["/"]}]}]},
 {"path":"/dev/sda","type":"disk","tran":"usb","rm":true,"model":"Pendrive USB  ","size":"64","children":[
   {"path":"/dev/sda1","fstype":"vfat","label":"FOTOS","type":"part","size":"30","rm":"1","mountpoints":["/media/eu/FOTOS"]},
   {"path":"/dev/sda2","fstype":"exfat","type":"part","size":"34","mountpoints":[]},
   {"path":"/dev/sda3","fstype":"ext4","type":"part","size":"1","ro":true},
   {"path":"/dev/sda4","fstype":"swap","type":"part","size":null}]},
 {"path":"/dev/sdb","type":"disk","tran":"sata","model":"Segundo","size":900,"children":[
   {"path":"/dev/sdb1","fstype":"ext4","type":"part","size":900,"ro":"0","mountpoints":["/srv/dados"]}]},
 {"path":"/dev/loop0","type":"loop","fstype":"ext4","size":1},
 {"path":"/dev/zram0","type":"disk","fstype":"ext4","size":1}
]}`

// falsos grava um lsblk e um udisksctl de mentira. O udisksctl monta e
// desmonta o /dev/sda2 reescrevendo a resposta do lsblk.
func falsos(t *testing.T) (discos.Sistema, string) {
	t.Helper()
	dir := t.TempDir()
	dados := filepath.Join(dir, "lsblk.json")
	if err := os.WriteFile(dados, []byte(arvore), 0o600); err != nil {
		t.Fatal(err)
	}
	escrever := func(nome, corpo string) string {
		p := filepath.Join(dir, nome)
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"+corpo), 0o700); err != nil {
			t.Fatal(err)
		}
		return p
	}
	antes := `"fstype":"exfat","type":"part","size":"34","mountpoints":[]`
	depois := `"fstype":"exfat","type":"part","size":"34","mountpoints":["/media/eu/disco"]`
	return discos.Sistema{
		Lsblk: escrever("lsblk", "cat "+dados+"\n"),
		Udisksctl: escrever("udisksctl", `echo "$@" >> `+dir+`/chamadas
[ -e `+dir+`/falhar ] && { echo "Error: not authorized" >&2; exit 1; }
[ -e `+dir+`/nada ] && exit 0
if [ "$1" = mount ]; then sed -i 's|`+strings.ReplaceAll(antes, "[]", "\\[\\]")+`|`+depois+`|' `+dados+`
else sed -i 's|`+strings.ReplaceAll(depois, `["/media/eu/disco"]`, `\["/media/eu/disco"\]`)+`|`+antes+`|' `+dados+`; fi
`),
	}, dir
}

func TestListarSoOQueNaoEDoSistema(t *testing.T) {
	s, _ := falsos(t)
	l, err := s.Listar(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var nomes []string
	for _, d := range l {
		nomes = append(nomes, d.Dispositivo)
	}
	if got := strings.Join(nomes, " "); got != "/dev/sda1 /dev/sda2 /dev/sdb1" {
		t.Fatal(got)
	}
	if l[0].Nome != "FOTOS" || !l[0].Removivel || l[0].Tamanho != 30 || l[0].Montagem != "/media/eu/FOTOS" {
		t.Errorf("%+v", l[0])
	}
	if l[1].Nome != "Pendrive USB" || l[1].Montagem != "" || !l[1].Removivel || l[2].Removivel {
		t.Errorf("%+v %+v", l[1], l[2])
	}
}

func TestMontarEDesmontar(t *testing.T) {
	s, dir := falsos(t)
	ctx := context.Background()
	// Já montado: usa onde está e não desmonta o que não montou.
	pasta, desfazer, err := s.Montar(ctx, "/dev/sda1")
	if err != nil || pasta != "/media/eu/FOTOS" || desfazer(ctx) != nil {
		t.Fatal(pasta, err)
	}
	pasta, desfazer, err = s.Montar(ctx, "/dev/sda2")
	if err != nil || pasta != "/media/eu/disco" {
		t.Fatal(pasta, err)
	}
	if err := desfazer(ctx); err != nil {
		t.Fatal(err)
	}
	chamadas, _ := os.ReadFile(filepath.Join(dir, "chamadas"))
	if got := string(chamadas); got != "mount --no-user-interaction -b /dev/sda2\nunmount --no-user-interaction -b /dev/sda2\n" {
		t.Errorf("%q", got)
	}
	// O disco do sistema e um dispositivo inventado são recusados.
	for _, d := range []string{"/dev/nvme0n1p2", "/dev/mapper/raiz", "/dev/inexistente", "-o", ""} {
		if _, _, err := s.Montar(ctx, d); !errors.Is(err, discos.ErrDesconhecido) {
			t.Error(d, err)
		}
	}
	os.WriteFile(filepath.Join(dir, "nada"), nil, 0o600)
	if _, _, err := s.Montar(ctx, "/dev/sda2"); err == nil || !strings.Contains(err.Error(), "não apareceu montado") {
		t.Error(err)
	}
	os.WriteFile(filepath.Join(dir, "falhar"), nil, 0o600)
	if _, _, err := s.Montar(ctx, "/dev/sda2"); err == nil || !strings.Contains(err.Error(), "not authorized") {
		t.Error(err)
	}
}

func TestFalhasDoLsblk(t *testing.T) {
	ctx := context.Background()
	if _, err := (discos.Sistema{Lsblk: "/nao/existe"}).Listar(ctx); err == nil {
		t.Error("sem lsblk")
	}
	ruim := filepath.Join(t.TempDir(), "x")
	for _, corpo := range []string{"echo texto", `echo '{"blockdevices":[{"path":"a","size":"muito"}]}'`} {
		os.WriteFile(ruim, []byte("#!/bin/sh\n"+corpo+"\n"), 0o700)
		if _, err := (discos.Sistema{Lsblk: ruim}).Listar(ctx); err == nil {
			t.Error(corpo)
		}
		if _, _, err := (discos.Sistema{Lsblk: ruim}).Montar(ctx, "a"); err == nil {
			t.Error(corpo)
		}
	}
}
