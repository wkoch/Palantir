// Package discos lista os discos externos e outros discos que não são o do
// sistema, e os monta pelo udisks, para receber cópias de segurança.
package discos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Disco é um sistema de arquivos que pode receber uma cópia.
type Disco struct {
	Dispositivo string `json:"dispositivo"` // /dev/sdb1
	Nome        string `json:"nome"`
	Sistema     string `json:"sistema"` // vfat, ext4...
	Montagem    string `json:"montagem"`
	Tamanho     int64  `json:"tamanho"`
	Removivel   bool   `json:"removivel"`
}

// Sistema conversa com o lsblk e o udisksctl.
type Sistema struct {
	Lsblk     string
	Udisksctl string
}

// formatos são os sistemas de arquivos em que se grava com segurança.
var formatos = []string{"ext2", "ext3", "ext4", "btrfs", "xfs", "f2fs", "vfat", "exfat", "ntfs"}

// doSistema são os pontos de montagem que denunciam o disco do sistema.
var doSistema = []string{"/", "/boot", "/boot/efi", "/usr", "/var", "/home", "[SWAP]"}

// numero aceita o número como o lsblk o escreve, que varia entre versões.
type numero int64

func (n *numero) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "null" || s == "" {
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	*n = numero(v)
	return err
}

type logico bool

func (l *logico) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	*l = s == "true" || s == "1"
	return nil
}

type no struct {
	Caminho   string    `json:"path"`
	Rotulo    string    `json:"label"`
	Modelo    string    `json:"model"`
	Formato   string    `json:"fstype"`
	Tamanho   numero    `json:"size"`
	Removivel logico    `json:"rm"`
	Hotplug   logico    `json:"hotplug"`
	Via       string    `json:"tran"`
	Tipo      string    `json:"type"`
	Leitura   logico    `json:"ro"`
	Montagens []*string `json:"mountpoints"`
	Filhos    []no      `json:"children"`
}

func (n no) montagens() []string {
	var m []string
	for _, p := range n.Montagens {
		if p != nil && *p != "" {
			m = append(m, *p)
		}
	}
	return m
}

func (n no) temDoSistema() bool {
	if slices.ContainsFunc(n.montagens(), func(m string) bool { return slices.Contains(doSistema, m) }) {
		return true
	}
	return slices.ContainsFunc(n.Filhos, no.temDoSistema)
}

func (s Sistema) executar(ctx context.Context, programa string, args ...string) ([]byte, error) {
	ctx, cancelar := context.WithTimeout(ctx, time.Minute)
	defer cancelar()
	cmd := exec.CommandContext(ctx, programa, args...)
	var saida, erros bytes.Buffer
	cmd.Stdout, cmd.Stderr = &saida, &erros
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s %s: %w: %s", programa, args[0], err, strings.TrimSpace(erros.String()))
	}
	return saida.Bytes(), nil
}

func ou(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// Listar devolve os sistemas de arquivos dos discos que não são o do sistema.
func (s Sistema) Listar(ctx context.Context) ([]Disco, error) {
	b, err := s.executar(ctx, ou(s.Lsblk, "lsblk"), "-J", "-b", "-o",
		"PATH,LABEL,MODEL,FSTYPE,SIZE,RM,HOTPLUG,TRAN,TYPE,RO,MOUNTPOINTS")
	if err != nil {
		return nil, err
	}
	var arvore struct {
		Dispositivos []no `json:"blockdevices"`
	}
	if err := json.Unmarshal(b, &arvore); err != nil {
		return nil, fmt.Errorf("resposta do lsblk: %w", err)
	}
	lista := []Disco{}
	vistos := map[string]bool{}
	for _, disco := range arvore.Dispositivos {
		if disco.Tipo == "loop" || strings.HasPrefix(disco.Caminho, "/dev/zram") || disco.temDoSistema() {
			continue
		}
		removivel := bool(disco.Removivel || disco.Hotplug) || disco.Via == "usb"
		var percorrer func(n no)
		percorrer = func(n no) {
			if slices.Contains(formatos, n.Formato) && !bool(n.Leitura) && !vistos[n.Caminho] {
				vistos[n.Caminho] = true
				d := Disco{Dispositivo: n.Caminho, Nome: ou(n.Rotulo, ou(strings.TrimSpace(disco.Modelo), n.Caminho)),
					Sistema: n.Formato, Tamanho: int64(n.Tamanho), Removivel: removivel}
				if m := n.montagens(); len(m) > 0 {
					d.Montagem = m[0]
				}
				lista = append(lista, d)
			}
			for _, f := range n.Filhos {
				percorrer(f)
			}
		}
		percorrer(disco)
	}
	return lista, nil
}

// ErrDesconhecido indica um dispositivo que não está entre os destinos.
var ErrDesconhecido = errors.New("este disco não está disponível para cópias")

// Achar devolve o destino com esse dispositivo. Só o que Listar mostra vale.
func (s Sistema) Achar(ctx context.Context, dispositivo string) (Disco, error) {
	lista, err := s.Listar(ctx)
	if err != nil {
		return Disco{}, err
	}
	i := slices.IndexFunc(lista, func(d Disco) bool { return d.Dispositivo == dispositivo })
	if i < 0 {
		return Disco{}, ErrDesconhecido
	}
	return lista[i], nil
}

// Montar garante que o disco esteja montado. A função devolvida desfaz a
// montagem feita aqui, o que também grava no disco o que faltava.
func (s Sistema) Montar(ctx context.Context, dispositivo string) (string, func(context.Context) error, error) {
	nada := func(context.Context) error { return nil }
	d, err := s.Achar(ctx, dispositivo)
	if err != nil {
		return "", nada, err
	}
	if d.Montagem != "" {
		return d.Montagem, nada, nil
	}
	udisks := ou(s.Udisksctl, "udisksctl")
	if _, err := s.executar(ctx, udisks, "mount", "--no-user-interaction", "-b", d.Dispositivo); err != nil {
		return "", nada, fmt.Errorf("montar o disco: %w", err)
	}
	desfazer := func(ctx context.Context) error {
		_, err := s.executar(ctx, udisks, "unmount", "--no-user-interaction", "-b", d.Dispositivo)
		return err
	}
	montado, err := s.Achar(ctx, dispositivo)
	if err != nil || montado.Montagem == "" {
		return "", nada, errors.Join(errors.New("o disco não apareceu montado"), err, desfazer(ctx))
	}
	return montado.Montagem, desfazer, nil
}
