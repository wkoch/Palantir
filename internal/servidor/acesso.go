package servidor

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Subprotocolo é o nome anunciado pelo WebSocket de avisos. O navegador não
// envia cabeçalhos num WebSocket, então a chave vai como um segundo
// subprotocolo, que o servidor nunca devolve.
const (
	Subprotocolo    = "palantir"
	prefixoDaChave  = "chave."
	validadeBilhete = 5 * time.Minute
	maxBilhetes     = 200
)

// semChave são as rotas que não revelam nem mudam nada.
var semChave = map[string]bool{"/api/saude": true}

// comBilhete são os endereços que o navegador abre sozinho (um quadro ou um
// download), onde não há como mandar um cabeçalho.
var comBilhete = regexp.MustCompile(`^/api/(acervo/documentos/[0-9]+/original|exportar)$`)

type bilhete struct {
	caminho string
	ate     time.Time
}

type bilheteria struct {
	mu       sync.Mutex
	emitidos map[string]bilhete
}

func (b *bilheteria) emitir(caminho string) (string, error) {
	aleatorio := make([]byte, 24)
	if _, err := rand.Read(aleatorio); err != nil {
		return "", err
	}
	codigo := base64.RawURLEncoding.EncodeToString(aleatorio)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.emitidos == nil {
		b.emitidos = map[string]bilhete{}
	}
	agora := time.Now()
	for k, v := range b.emitidos {
		if agora.After(v.ate) || len(b.emitidos) >= maxBilhetes {
			delete(b.emitidos, k)
		}
	}
	b.emitidos[codigo] = bilhete{caminho, agora.Add(validadeBilhete)}
	return codigo, nil
}

func (b *bilheteria) vale(codigo, caminho string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	v, ok := b.emitidos[codigo]
	return ok && v.caminho == caminho && time.Now().Before(v.ate)
}

func (s *Servidor) confere(chave string) bool {
	return s.D.Chave != "" && subtle.ConstantTimeCompare([]byte(chave), []byte(s.D.Chave)) == 1
}

// autorizado informa se o pedido à API vem do dono desta instalação.
func (s *Servidor) autorizado(r *http.Request) bool {
	if semChave[r.URL.Path] {
		return true
	}
	if chave, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		return s.confere(chave)
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if r.URL.Path == "/api/eventos" {
		for _, valor := range r.Header.Values("Sec-WebSocket-Protocol") {
			for _, p := range strings.Split(valor, ",") {
				if chave, ok := strings.CutPrefix(strings.TrimSpace(p), prefixoDaChave); ok {
					return s.confere(chave)
				}
			}
		}
		return false
	}
	codigo := r.URL.Query().Get("bilhete")
	return codigo != "" && s.bilhetes.vale(codigo, r.URL.Path)
}

// emitirBilhete dá acesso temporário a um único endereço de leitura.
func (s *Servidor) emitirBilhete(w http.ResponseWriter, r *http.Request) (any, error) {
	p, err := corpo[struct {
		Caminho string `json:"caminho"`
	}](w, r)
	if err != nil {
		return nil, err
	}
	if !comBilhete.MatchString(p.Caminho) {
		return nil, erroHTTP{http.StatusBadRequest, "não há bilhete para esse endereço"}
	}
	codigo, err := s.bilhetes.emitir(p.Caminho)
	if err != nil {
		return nil, err
	}
	return map[string]string{"endereco": p.Caminho + "?bilhete=" + codigo}, nil
}
