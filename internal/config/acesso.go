package config

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// portaBase é a porta do primeiro usuário comum (uid 1000).
const portaBase = 7700

// EnderecoDoUsuario é o endereço usado quando a configuração não escolhe um.
// Cada usuário da máquina tem o próprio daemon, numa porta estável.
func EnderecoDoUsuario(uid int) string {
	n := (uid - 1000) % 1000
	if n < 0 {
		n += 1000
	}
	return "127.0.0.1:" + strconv.Itoa(portaBase+n)
}

// ChaveDeAcesso é o arquivo com o segredo que a interface apresenta à API.
func (c Caminhos) ChaveDeAcesso() string { return filepath.Join(c.Config, "acesso") }

// Endereco é o arquivo em que o daemon em execução anota onde atende.
func (c Caminhos) Endereco() string { return filepath.Join(c.Execucao, "endereco") }

// criarInteiro grava um arquivo novo, que só aparece depois de completo: o
// daemon e um comando podem criá-lo ao mesmo tempo. Se já existir, devolve
// fs.ErrExist sem tocar nele.
func criarInteiro(arquivo string, conteudo []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(arquivo), ".novo-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(conteudo)
	if err := errors.Join(err, tmp.Close()); err != nil {
		return err
	}
	return os.Link(tmp.Name(), arquivo)
}

// LerChaveDeAcesso devolve a chave, criando-a na primeira vez. Só o dono lê o
// arquivo, o que separa os usuários da máquina.
func (c Caminhos) LerChaveDeAcesso() (string, error) {
	arquivo := c.ChaveDeAcesso()
	b, err := os.ReadFile(arquivo)
	if errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(c.Config, 0o700); err != nil {
			return "", err
		}
		aleatorio := make([]byte, 32)
		if _, err := rand.Read(aleatorio); err != nil {
			return "", err
		}
		novo := base64.RawURLEncoding.EncodeToString(aleatorio)
		err := criarInteiro(arquivo, []byte(novo+"\n"))
		if errors.Is(err, fs.ErrExist) {
			// Outro processo criou ao mesmo tempo.
			return c.LerChaveDeAcesso()
		}
		return novo, err
	}
	if err != nil {
		return "", err
	}
	chave := strings.TrimSpace(string(b))
	if len(chave) < 32 {
		return "", fmt.Errorf("%s está vazio ou curto demais; apague-o para gerar outro", arquivo)
	}
	return chave, nil
}
