// Package esquema valida dados contra esquemas JSON.
package esquema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Validador confere dados contra um esquema compilado.
type Validador struct {
	nome    string
	esquema *jsonschema.Schema
}

// semCarga impede que um esquema busque referências em arquivos ou na rede.
type semCarga struct{}

func (semCarga) Load(url string) (any, error) {
	return nil, fmt.Errorf("referência externa não permitida: %s", url)
}

// Compilar prepara o esquema.
func Compilar(nome string, definicao []byte) (*Validador, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(definicao))
	if err != nil {
		return nil, fmt.Errorf("esquema %s: %w", nome, err)
	}
	c := jsonschema.NewCompiler()
	c.UseLoader(semCarga{})
	url := "mem:///" + nome + ".json"
	if err := c.AddResource(url, doc); err != nil {
		return nil, fmt.Errorf("esquema %s: %w", nome, err)
	}
	s, err := c.Compile(url)
	if err != nil {
		return nil, fmt.Errorf("esquema %s: %w", nome, err)
	}
	return &Validador{nome: nome, esquema: s}, nil
}

// ValidarJSON confere um documento JSON.
func (v *Validador) ValidarJSON(b []byte) error {
	dado, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("%s: JSON inválido: %w", v.nome, err)
	}
	if err := v.esquema.Validate(dado); err != nil {
		return fmt.Errorf("%s: %w", v.nome, err)
	}
	return nil
}

// Validar confere um valor Go, codificando-o antes.
func (v *Validador) Validar(dado any) error {
	b, err := json.Marshal(dado)
	if err != nil {
		return err
	}
	return v.ValidarJSON(b)
}

// Objeto monta o esquema de um objeto com todas as propriedades obrigatórias e
// nenhuma extra, como exigem as saídas estruturadas estritas.
func Objeto(propriedades map[string]any) map[string]any {
	nomes := make([]string, 0, len(propriedades))
	for n := range propriedades {
		nomes = append(nomes, n)
	}
	slices.Sort(nomes)
	return map[string]any{"type": "object", "properties": propriedades,
		"required": nomes, "additionalProperties": false}
}

// Lista monta o esquema de uma lista.
func Lista(item any) map[string]any { return map[string]any{"type": "array", "items": item} }

// Tipos simples.
var (
	Texto    = map[string]any{"type": "string"}
	Inteiro  = map[string]any{"type": "integer"}
	Numero   = map[string]any{"type": "number"}
	Booleano = map[string]any{"type": "boolean"}
)

// Enum restringe um texto às opções dadas.
func Enum(opcoes ...string) map[string]any {
	return map[string]any{"type": "string", "enum": opcoes}
}

// JSON codifica um esquema montado em Go.
func JSON(e map[string]any) json.RawMessage {
	b, err := json.Marshal(e)
	if err != nil {
		panic("esquema não codificável: " + err.Error()) // erro de programação
	}
	return b
}
