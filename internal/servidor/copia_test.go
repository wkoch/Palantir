package servidor_test

import (
	"testing"
	"time"
)

func TestCopiaPelaInterface(t *testing.T) {
	c := montar(t)
	v := c.quer(200, "GET", "/api/discos", "")
	if len(v["discos"].([]any)) != 1 || v["copia"].(map[string]any)["rodando"] != false {
		t.Fatal(v)
	}
	c.quer(400, "POST", "/api/copia", `{"dispositivo":"/dev/teste1"}`)
	c.quer(400, "POST", "/api/copia", `{"dispositivo":"/dev/teste1","confirmar":false}`)
	c.quer(400, "POST", "/api/copia", `{"dispositivo":"/dev/nvme0n1","confirmar":true}`)
	c.quer(400, "POST", "/api/copia", `x`)
	c.quer(200, "POST", "/api/copia", `{"dispositivo":"/dev/teste1","confirmar":true}`)
	for range 1000 {
		estado := c.quer(200, "GET", "/api/discos", "")["copia"].(map[string]any)
		if estado["rodando"] == false {
			if estado["arquivo"] == "" || estado["erro"] != nil {
				t.Fatal(estado)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("a cópia não terminou")
}
