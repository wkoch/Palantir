// Package telas é a biblioteca de telas: cada uma tem um esquema JSON, e o
// daemon valida os dados antes de entregá-los à interface.
package telas

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/wkoch/Palantir/internal/esquema"
)

//go:embed esquemas/*.json
var arquivos embed.FS

// ErrTelaDesconhecida indica um nome fora da biblioteca.
var ErrTelaDesconhecida = errors.New("tela desconhecida")

var carregar = sync.OnceValues(func() (map[string]*esquema.Validador, error) {
	itens, err := arquivos.ReadDir("esquemas")
	if err != nil {
		return nil, err
	}
	m := map[string]*esquema.Validador{}
	for _, it := range itens {
		b, err := arquivos.ReadFile(path.Join("esquemas", it.Name()))
		if err != nil {
			return nil, err
		}
		nome := strings.TrimSuffix(it.Name(), ".json")
		if m[nome], err = esquema.Compilar(nome, b); err != nil {
			return nil, err
		}
	}
	return m, nil
})

// Nomes lista as telas da biblioteca.
func Nomes() []string {
	m, err := carregar()
	if err != nil {
		return nil
	}
	nomes := make([]string, 0, len(m))
	for n := range m {
		nomes = append(nomes, n)
	}
	slices.Sort(nomes)
	return nomes
}

// Esquema devolve o esquema JSON de uma tela.
func Esquema(nome string) (json.RawMessage, error) {
	b, err := arquivos.ReadFile("esquemas/" + nome + ".json")
	if err != nil {
		return nil, fmt.Errorf("%w: %q", ErrTelaDesconhecida, nome)
	}
	return b, nil
}

// Validar confere os dados de uma tela.
func Validar(nome string, dados any) error {
	m, err := carregar()
	if err != nil {
		return err
	}
	v, ok := m[nome]
	if !ok {
		return fmt.Errorf("%w: %q", ErrTelaDesconhecida, nome)
	}
	return v.Validar(dados)
}

// DoModelo são as telas que um modelo pode escolher e preencher numa conversa.
var DoModelo = []string{"indicadores", "tabela"}
