package eventos

import "testing"

func TestCentral(t *testing.T) {
	c := Nova()
	c.Publicar(Evento{Tipo: Acervo}) // sem assinantes
	canal, encerrar := c.Assinar()
	c.Publicar(Evento{Tipo: Atividade, Atividade: 3})
	if e := <-canal; e.Atividade != 3 {
		t.Error(e)
	}
	// Um assinante atrasado recebe um pedido de recarga e nunca trava.
	for range capacidade * 3 {
		c.Publicar(Evento{Tipo: Acervo})
	}
	var tipos []string
	for len(canal) > 0 {
		tipos = append(tipos, (<-canal).Tipo)
	}
	if len(tipos) == 0 || len(tipos) > capacidade || tipos[0] != Tudo {
		t.Error(len(tipos), tipos)
	}
	encerrar()
	encerrar()
	c.Publicar(Evento{Tipo: Acervo})
	if len(canal) != 0 {
		t.Error("recebeu depois de encerrar")
	}
}
