package modelos

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wkoch/Palantir/internal/esquema"
)

// CompletarJSON pede uma resposta no esquema indicado, valida e decodifica em
// destino. Uma resposta inválida recebe uma única chance de correção.
func CompletarJSON(ctx context.Context, c Completador, p Pedido, destino any) error {
	if p.Esquema == nil {
		return fmt.Errorf("CompletarJSON exige um esquema")
	}
	validador, err := esquema.Compilar(p.Esquema.Nome, p.Esquema.Definicao)
	if err != nil {
		return err
	}
	for tentativa := 1; ; tentativa++ {
		resp, err := c.Completar(ctx, p)
		if err != nil {
			return err
		}
		texto := semCerca(resp.Conteudo)
		err = validador.ValidarJSON([]byte(texto))
		if err == nil {
			err = json.Unmarshal([]byte(texto), destino)
		}
		if err == nil {
			return nil
		}
		if tentativa == 2 {
			return fmt.Errorf("%w: %v", ErrRespostaRuim, err)
		}
		p.Cache = false
		p.Mensagens = append(append([]Mensagem(nil), p.Mensagens...),
			Mensagem{Papel: PapelAssistente, Conteudo: resp.Conteudo},
			Mensagem{Papel: PapelUsuario, Conteudo: "A resposta não segue o esquema: " + err.Error() +
				"\nResponda novamente apenas com o JSON corrigido."})
	}
}

// semCerca remove a cerca de código que alguns modelos colocam em volta do JSON.
func semCerca(s string) string {
	s = strings.TrimSpace(s)
	if resto, ok := strings.CutPrefix(s, "```"); ok {
		if i := strings.IndexByte(resto, '\n'); i >= 0 {
			resto = resto[i+1:]
		}
		s = strings.TrimSpace(strings.TrimSuffix(resto, "```"))
	}
	return s
}
