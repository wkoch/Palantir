// Package eventos avisa a interface de que algo mudou. Os avisos não carregam
// dados: a interface busca o estado novo pela API.
package eventos

import "sync"

// Tipos de aviso.
const (
	Atividade = "atividade"
	Acervo    = "acervo"
	Ajustes   = "ajustes"
	Copia     = "copia"
	// Tudo pede que a interface recarregue o que mostra.
	Tudo = "tudo"
)

// Evento é um aviso.
type Evento struct {
	Tipo      string `json:"tipo"`
	Atividade int64  `json:"atividade,omitempty"`
}

// Central distribui avisos aos assinantes.
type Central struct {
	mu         sync.Mutex
	assinantes map[chan Evento]struct{}
}

// Nova cria uma central.
func Nova() *Central { return &Central{assinantes: map[chan Evento]struct{}{}} }

const capacidade = 64

// Assinar devolve um canal de avisos e a função que o encerra.
func (c *Central) Assinar() (<-chan Evento, func()) {
	canal := make(chan Evento, capacidade)
	c.mu.Lock()
	c.assinantes[canal] = struct{}{}
	c.mu.Unlock()
	var uma sync.Once
	return canal, func() {
		uma.Do(func() {
			c.mu.Lock()
			delete(c.assinantes, canal)
			c.mu.Unlock()
		})
	}
}

// Publicar nunca bloqueia. Um assinante atrasado recebe um único pedido de
// recarga no lugar dos avisos perdidos.
func (c *Central) Publicar(e Evento) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for canal := range c.assinantes {
		select {
		case canal <- e:
		default:
			for len(canal) > 0 {
				select {
				case <-canal:
				default:
				}
			}
			select {
			case canal <- Evento{Tipo: Tudo}:
			default:
			}
		}
	}
}
