package servidor

import "net/http"

// discos lista os destinos possíveis e o estado da cópia.
func (s *Servidor) discos(_ http.ResponseWriter, r *http.Request) (any, error) {
	lista, err := s.D.Discos.Listar(r.Context())
	if err != nil {
		return nil, err
	}
	return map[string]any{"discos": lista, "copia": s.D.Copia()}, nil
}

// copiar inicia a cópia que o usuário confirmou na tela.
func (s *Servidor) copiar(w http.ResponseWriter, r *http.Request) (any, error) {
	p, err := corpo[struct {
		Dispositivo string `json:"dispositivo"`
		Confirmar   bool   `json:"confirmar"`
	}](w, r)
	if err != nil {
		return nil, err
	}
	if !p.Confirmar {
		return nil, erroHTTP{http.StatusBadRequest, "a cópia precisa da sua confirmação"}
	}
	return nil, s.D.Copiar(r.Context(), p.Dispositivo)
}
