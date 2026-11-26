// Package servidor é a API HTTP e WebSocket local, além da interface web.
package servidor

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/wkoch/Palantir/internal/acervo"
	"github.com/wkoch/Palantir/internal/atividades"
	"github.com/wkoch/Palantir/internal/config"
	"github.com/wkoch/Palantir/internal/confirmacoes"
	"github.com/wkoch/Palantir/internal/daemon"
	"github.com/wkoch/Palantir/internal/discos"
	"github.com/wkoch/Palantir/internal/segredos"
)

// Cabecalho precisa acompanhar toda requisição que altera algo. Um site de
// outra origem não consegue enviá-lo sem uma permissão que nunca é dada.
const Cabecalho = "X-Palantir"

// Servidor atende a interface.
type Servidor struct {
	D      *daemon.Daemon
	Versao string
	// Interface são os arquivos compilados da interface web.
	Interface fs.FS

	bilhetes bilheteria
}

type tratador func(w http.ResponseWriter, r *http.Request) (any, error)

// erroHTTP é um erro com código definido.
type erroHTTP struct {
	codigo int
	msg    string
}

func (e erroHTTP) Error() string { return e.msg }

func codigoDe(err error) int {
	var h erroHTTP
	var grande *http.MaxBytesError
	switch {
	case errors.As(err, &h):
		return h.codigo
	case errors.As(err, &grande), errors.Is(err, acervo.ErrGrande):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, atividades.ErrNaoEncontrada), errors.Is(err, acervo.ErrNaoEncontrado),
		errors.Is(err, fs.ErrNotExist):
		return http.StatusNotFound
	case errors.Is(err, atividades.ErrPedidoInvalido), errors.Is(err, segredos.ErrDesconhecido),
		errors.Is(err, acervo.ErrNomeInvalido), errors.Is(err, acervo.ErrVazio),
		errors.Is(err, acervo.ErrPastaProibida), errors.Is(err, discos.ErrDesconhecido):
		return http.StatusBadRequest
	case errors.Is(err, atividades.ErrEstado), errors.Is(err, confirmacoes.ErrJaDecidida),
		errors.Is(err, daemon.ErrCopiaEmAndamento),
		errors.Is(err, acervo.ErrOriginais):
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

func responder(w http.ResponseWriter, codigo int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(codigo)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Servidor) api(f tratador) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, err := f(w, r)
		if err != nil {
			codigo := codigoDe(err)
			if codigo >= 500 {
				s.D.Log.Error("API", "rota", r.URL.Path, "erro", err)
			}
			responder(w, codigo, map[string]string{"erro": s.D.Diario.Limpar(err.Error())})
			return
		}
		if v == arquivoEnviado {
			return
		}
		if v == nil {
			v = map[string]bool{"ok": true}
		}
		responder(w, http.StatusOK, v)
	}
}

// arquivoEnviado indica que o tratador já escreveu a resposta.
var arquivoEnviado = new(struct{})

func corpo[T any](w http.ResponseWriter, r *http.Request) (T, error) {
	var v T
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(&v); err != nil {
		var grande *http.MaxBytesError
		if errors.As(err, &grande) {
			return v, err
		}
		return v, erroHTTP{http.StatusBadRequest, "corpo inválido: " + err.Error()}
	}
	return v, nil
}

func idDe(r *http.Request, nome string) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue(nome), 10, 64)
	if err != nil || id <= 0 {
		return 0, erroHTTP{http.StatusBadRequest, "identificador inválido"}
	}
	return id, nil
}

// Handler monta as rotas com as proteções.
func (s *Servidor) Handler() http.Handler {
	m := http.NewServeMux()
	rota := func(padrao string, f tratador) { m.HandleFunc(padrao, s.api(f)) }

	rota("GET /api/saude", s.saude)
	rota("GET /api/estado", s.estado)
	m.HandleFunc("GET /api/eventos", s.eventos)

	rota("POST /api/atividades", s.criar)
	rota("GET /api/atividades/{id}", s.atividade)
	rota("POST /api/atividades/{id}/estado", s.mudarEstado)
	rota("POST /api/atividades/{id}/orcamento", s.mudarOrcamento)
	rota("POST /api/atividades/{id}/acoes/{acao}", s.agir)
	rota("POST /api/confirmacoes/{id}", s.decidir)

	rota("GET /api/acervo/documentos", s.documentos)
	rota("GET /api/acervo/documentos/{id}", s.documento)
	rota("GET /api/acervo/documentos/{id}/original", s.original)
	rota("POST /api/acervo/documentos/{id}/reextrair", s.reextrair)
	rota("POST /api/acervo/importar", s.importar)
	rota("POST /api/acervo/importar-pasta", s.importarPasta)
	rota("GET /api/acervo/busca", s.busca)
	rota("GET /api/acervo/visoes", s.visoes)
	rota("POST /api/acervo/colecoes", s.criarColecao)
	rota("POST /api/acervo/colecoes/{id}", s.naColecao)
	rota("POST /api/acervo/regenerar", s.regenerar)
	rota("POST /api/acervo/verificar", s.verificar)
	rota("GET /api/exportar", s.exportar)
	rota("POST /api/bilhetes", s.emitirBilhete)
	rota("GET /api/discos", s.discos)
	rota("POST /api/copia", s.copiar)

	rota("GET /api/ajustes", s.ajustes)
	rota("PUT /api/ajustes", s.gravarAjustes)
	rota("PUT /api/segredos/{nome}", s.definirSegredo)
	rota("DELETE /api/segredos/{nome}", s.removerSegredo)
	rota("GET /api/diario", s.diario)
	rota("GET /api/painel", s.painel)

	m.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		responder(w, http.StatusNotFound, map[string]string{"erro": "rota desconhecida"})
	})
	m.Handle("/", s.arquivos())
	return s.proteger(m)
}

// hostLocal confere o cabeçalho Host, o que impede que um site aponte um nome
// seu para a máquina local (DNS rebinding).
func hostLocal(hostporta string) bool {
	host, _, err := net.SplitHostPort(hostporta)
	if err != nil {
		host = strings.Trim(hostporta, "[]")
	}
	return config.EnderecoLocal(host)
}

const politicaDeConteudo = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob:; connect-src 'self'; frame-src 'self'; object-src 'self'; " +
	"base-uri 'none'; form-action 'none'; frame-ancestors 'self'"

func (s *Servidor) proteger(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", politicaDeConteudo)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		recusar := func(motivo string) {
			responder(w, http.StatusForbidden, map[string]string{"erro": motivo})
		}
		if !hostLocal(r.Host) {
			recusar("host não permitido")
			return
		}
		origem := r.Header.Get("Origin")
		if origem != "" && origem != "http://"+r.Host {
			recusar("origem não permitida")
			return
		}
		if sitio := r.Header.Get("Sec-Fetch-Site"); sitio == "cross-site" || sitio == "same-site" {
			recusar("requisição de outro site")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get(Cabecalho) == "" {
			recusar("falta o cabeçalho " + Cabecalho)
			return
		}
		// A interface é pública; os dados, só para quem tem a chave.
		if strings.HasPrefix(r.URL.Path, "/api/") && !s.autorizado(r) {
			responder(w, http.StatusUnauthorized, map[string]string{"erro": "abra o Palantír pelo atalho do sistema"})
			return
		}
		proximo.ServeHTTP(w, r)
	})
}

func (s *Servidor) arquivos() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "método não permitido", http.StatusMethodNotAllowed)
			return
		}
		nome := strings.TrimPrefix(r.URL.Path, "/")
		if nome == "" {
			nome = "index.html"
		}
		if s.Interface == nil {
			http.Error(w, "A interface não foi compilada (execute scripts/construir.sh).", http.StatusNotFound)
			return
		}
		if _, err := fs.Stat(s.Interface, nome); err != nil || strings.HasPrefix(nome, ".") {
			if nome == "index.html" {
				http.Error(w, "A interface não foi compilada (execute scripts/construir.sh).", http.StatusNotFound)
				return
			}
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, s.Interface, nome)
	})
}

// eventos envia os avisos de mudança por WebSocket.
func (s *Servidor) eventos(w http.ResponseWriter, r *http.Request) {
	// A origem já foi conferida; Accept a confere de novo.
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{Subprotocolo}})
	if err != nil {
		return
	}
	defer c.CloseNow()
	canal, encerrar := s.D.Eventos.Assinar()
	defer encerrar()
	// A interface não envia nada; ler detecta o fechamento.
	ctx := c.CloseRead(r.Context())
	pulso := time.NewTicker(30 * time.Second)
	defer pulso.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-canal:
			prazo, cancelar := context.WithTimeout(ctx, 10*time.Second)
			err := wsjson.Write(prazo, c, e)
			cancelar()
			if err != nil {
				return
			}
		case <-pulso.C:
			prazo, cancelar := context.WithTimeout(ctx, 10*time.Second)
			err := c.Ping(prazo)
			cancelar()
			if err != nil {
				return
			}
		}
	}
}

// Escutar abre o endereço configurado.
func (s *Servidor) Escutar() (net.Listener, error) {
	return net.Listen("tcp", s.D.Config().Servidor.Endereco)
}

// Servir atende até o contexto acabar.
func (s *Servidor) Servir(ctx context.Context, l net.Listener) error {
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute,
		// Cancela as conexões WebSocket no encerramento.
		BaseContext: func(net.Listener) context.Context { return ctx }}
	fim := make(chan error, 1)
	go func() { fim <- srv.Serve(l) }()
	select {
	case err := <-fim:
		return err
	case <-ctx.Done():
		prazo, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelar()
		if err := srv.Shutdown(prazo); err != nil {
			srv.Close()
		}
		<-fim
		return nil
	}
}
