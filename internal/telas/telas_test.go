package telas

import (
	"errors"
	"slices"
	"testing"
)

func TestBiblioteca(t *testing.T) {
	nomes := Nomes()
	for _, n := range []string{"conversa", "estudo", "indicadores", "relatorio", "tabela", "triagem"} {
		if !slices.Contains(nomes, n) {
			t.Error("falta a tela", n)
		}
		if b, err := Esquema(n); err != nil || len(b) == 0 {
			t.Error(n, err)
		}
	}
	if err := Validar("indicadores", map[string]any{"titulo": "t", "indicadores": []any{map[string]any{"rotulo": "r", "valor": "1"}}}); err != nil {
		t.Error(err)
	}
	for _, ruim := range []any{
		map[string]any{"titulo": "t"},
		map[string]any{"titulo": "t", "indicadores": []any{}},
		map[string]any{"titulo": "t", "indicadores": []any{map[string]any{"rotulo": "r", "valor": 1}}},
		map[string]any{"titulo": "t", "indicadores": []any{map[string]any{"rotulo": "r", "valor": "1", "html": "<script>"}}},
	} {
		if Validar("indicadores", ruim) == nil {
			t.Error("aceitou", ruim)
		}
	}
	if err := Validar("nada", 1); !errors.Is(err, ErrTelaDesconhecida) {
		t.Error(err)
	}
	if _, err := Esquema("../x"); !errors.Is(err, ErrTelaDesconhecida) {
		t.Error(err)
	}
	for _, n := range DoModelo {
		if !slices.Contains(nomes, n) {
			t.Error(n)
		}
	}
}
