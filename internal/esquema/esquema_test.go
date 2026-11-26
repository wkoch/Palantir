package esquema

import "testing"

func TestEsquema(t *testing.T) {
	v, err := Compilar("x", JSON(Objeto(map[string]any{"b": Texto, "a": Inteiro})))
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Validar(map[string]any{"a": 1, "b": "x"}); err != nil {
		t.Error(err)
	}
	for _, ruim := range []string{`{"a":1}`, `{"a":1,"b":"x","c":2}`, `{`, `[]`} {
		if v.ValidarJSON([]byte(ruim)) == nil {
			t.Errorf("%s deveria ser recusado", ruim)
		}
	}
	if v.Validar(func() {}) == nil {
		t.Error("valor não codificável")
	}
	for _, ruim := range []string{`{`, `{"type": 3}`, `{"$ref":"http://exemplo.org/x.json"}`, `{"$ref":"file:///etc/passwd"}`} {
		if _, err := Compilar("ruim", []byte(ruim)); err == nil {
			t.Errorf("%s deveria falhar", ruim)
		}
	}
	defer func() {
		if recover() == nil {
			t.Error("esperava pânico")
		}
	}()
	JSON(map[string]any{"f": func() {}})
}
