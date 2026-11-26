package servidor

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wkoch/Palantir/internal/acervo"
	"github.com/wkoch/Palantir/internal/atividades"
	"github.com/wkoch/Palantir/internal/config"
	"github.com/wkoch/Palantir/internal/segredos"
)

func (s *Servidor) documentos(_ http.ResponseWriter, r *http.Request) (any, error) {
	q := r.URL.Query()
	colecao, _ := strconv.ParseInt(q.Get("colecao"), 10, 64)
	lista, err := s.D.Acervo.Listar(r.Context(), acervo.Filtro{Nome: q.Get("nome"), Colecao: colecao,
		Entidade: q.Get("entidade")})
	if err != nil {
		return nil, err
	}
	numeros, err := s.D.Acervo.Numeros(r.Context())
	return map[string]any{"documentos": lista, "numeros": numeros,
		"pasta_entrada": config.Expandir(s.D.Config().Acervo.PastaEntrada)}, err
}

func (s *Servidor) documento(_ http.ResponseWriter, r *http.Request) (any, error) {
	id, err := idDe(r, "id")
	if err != nil {
		return nil, err
	}
	ctx := r.Context()
	d, err := s.D.Acervo.Documento(ctx, id)
	if err != nil {
		return nil, err
	}
	trechos, err := s.D.Acervo.TrechosDoDocumento(ctx, id)
	if err != nil {
		return nil, err
	}
	entidades, err := s.D.Acervo.Entidades(ctx, "", "", id)
	return map[string]any{"documento": d, "trechos": trechos, "entidades": entidades}, err
}

// tiposExibiveis são abertos no próprio navegador. O resto, inclusive HTML e
// SVG, é entregue como texto ou como arquivo, sem executar nada.
var tiposExibiveis = map[string]string{
	"application/pdf": "application/pdf",
	"image/png":       "image/png",
	"image/jpeg":      "image/jpeg",
	"text/plain":      "text/plain; charset=utf-8",
	"text/markdown":   "text/plain; charset=utf-8",
	"text/csv":        "text/plain; charset=utf-8",
	"text/html":       "text/plain; charset=utf-8",
	"message/rfc822":  "text/plain; charset=utf-8",
}

func (s *Servidor) original(w http.ResponseWriter, r *http.Request) (any, error) {
	id, err := idDe(r, "id")
	if err != nil {
		return nil, err
	}
	d, err := s.D.Acervo.Documento(r.Context(), id)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(s.D.Acervo.CaminhoOriginal(d.SHA256))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tipo, exibivel := tiposExibiveis[d.Tipo]
	disposicao := "inline"
	if !exibivel {
		tipo, disposicao = "application/octet-stream", "attachment"
	}
	h := w.Header()
	h.Set("Content-Type", tipo)
	h.Set("Content-Disposition", mime.FormatMediaType(disposicao, map[string]string{"filename": d.Nome}))
	// O visualizador de PDF do Chromium não funciona com sandbox.
	if tipo != "application/pdf" {
		h.Set("Content-Security-Policy", "sandbox; default-src 'none'; frame-ancestors 'self'")
	}
	http.ServeContent(w, r, "", d.ImportadoEm, f)
	return arquivoEnviado, nil
}

func (s *Servidor) reextrair(_ http.ResponseWriter, r *http.Request) (any, error) {
	id, err := idDe(r, "id")
	if err != nil {
		return nil, err
	}
	return nil, s.D.Acervo.Reextrair(r.Context(), id)
}

const maxEnvio = 1 << 30

func (s *Servidor) importar(w http.ResponseWriter, r *http.Request) (any, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxEnvio)
	leitor, err := r.MultipartReader()
	if err != nil {
		return nil, erroHTTP{http.StatusBadRequest, "envie os arquivos como multipart/form-data"}
	}
	rel := acervo.Relatorio{Falhas: []string{}}
	ids := []int64{}
	for {
		parte, err := leitor.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, erroHTTP{http.StatusBadRequest, "envio interrompido: " + err.Error()}
		}
		nome := parte.FileName()
		if nome == "" {
			continue
		}
		d, novo, err := s.D.Acervo.Importar(r.Context(), parte, nome, "enviado")
		if err != nil {
			rel.Falhas = append(rel.Falhas, nome+": "+err.Error())
			continue
		}
		ids = append(ids, d.ID)
		if novo {
			rel.Importados++
		} else {
			rel.Repetidos++
		}
	}
	if len(ids) == 0 && len(rel.Falhas) == 0 {
		return nil, erroHTTP{http.StatusBadRequest, "nenhum arquivo recebido"}
	}
	return map[string]any{"relatorio": rel, "documentos": ids}, nil
}

func (s *Servidor) importarPasta(w http.ResponseWriter, r *http.Request) (any, error) {
	in, err := corpo[struct {
		Caminho string `json:"caminho"`
	}](w, r)
	if err != nil {
		return nil, err
	}
	caminho := config.Expandir(strings.TrimSpace(in.Caminho))
	if !filepath.IsAbs(caminho) {
		return nil, erroHTTP{http.StatusBadRequest, "informe o caminho completo, como ~/Documentos"}
	}
	// As pastas do próprio Palantír guardam segredos e o diário.
	for _, privada := range []string{s.D.Caminhos.Config, s.D.Caminhos.Dados, s.D.Caminhos.Execucao} {
		if sobrepoem(privada, caminho) {
			return nil, erroHTTP{http.StatusBadRequest, "essa pasta contém arquivos do próprio Palantír"}
		}
	}
	rel, err := s.D.Acervo.ImportarCaminho(r.Context(), caminho, "pasta")
	return map[string]any{"relatorio": rel}, err
}

// sobrepoem informa se um caminho contém o outro, depois de resolver links.
func sobrepoem(a, b string) bool {
	resolver := func(p string) string {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
		}
		return filepath.Clean(p) + string(filepath.Separator)
	}
	a, b = resolver(a), resolver(b)
	return strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
}

func (s *Servidor) busca(_ http.ResponseWriter, r *http.Request) (any, error) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" || len(q) > 2000 {
		return nil, erroHTTP{http.StatusBadRequest, "informe o que procurar"}
	}
	achados, err := s.D.Acervo.Buscar(r.Context(), 0, q, 20)
	return map[string]any{"achados": achados}, err
}

func (s *Servidor) visoes(_ http.ResponseWriter, r *http.Request) (any, error) {
	colecoes, err := s.D.Acervo.Colecoes(r.Context())
	if err != nil {
		return nil, err
	}
	entidades, err := s.D.Acervo.Entidades(r.Context(), r.URL.Query().Get("tipo"), r.URL.Query().Get("q"), 0)
	return map[string]any{"colecoes": colecoes, "entidades": entidades}, err
}

func (s *Servidor) criarColecao(w http.ResponseWriter, r *http.Request) (any, error) {
	in, err := corpo[struct {
		Nome string `json:"nome"`
	}](w, r)
	if err != nil {
		return nil, err
	}
	id, err := s.D.Acervo.CriarColecao(r.Context(), in.Nome)
	return map[string]int64{"id": id}, err
}

func (s *Servidor) naColecao(w http.ResponseWriter, r *http.Request) (any, error) {
	id, err := idDe(r, "id")
	if err != nil {
		return nil, err
	}
	in, err := corpo[struct {
		Documento int64 `json:"documento"`
		Incluir   bool  `json:"incluir"`
	}](w, r)
	if err != nil {
		return nil, err
	}
	return nil, s.D.Acervo.NaColecao(r.Context(), id, in.Documento, in.Incluir)
}

func (s *Servidor) regenerar(_ http.ResponseWriter, r *http.Request) (any, error) {
	if err := s.D.Acervo.Regenerar(r.Context()); err != nil {
		return nil, err
	}
	n, problemas, err := s.D.Acervo.Verificar(r.Context())
	return map[string]any{"conferidos": n, "problemas": problemas}, err
}

func (s *Servidor) verificar(_ http.ResponseWriter, r *http.Request) (any, error) {
	n, problemas, err := s.D.Acervo.Verificar(r.Context())
	return map[string]any{"conferidos": n, "problemas": problemas}, err
}

func (s *Servidor) exportar(w http.ResponseWriter, r *http.Request) (any, error) {
	// Monta o arquivo antes de responder, para que uma falha não vire um ZIP
	// pela metade.
	tmp, err := os.CreateTemp(s.D.Caminhos.Execucao, "exportacao-*.zip")
	if err != nil {
		return nil, err
	}
	defer func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}()
	if err := s.D.Exportar(r.Context(), tmp); err != nil {
		return nil, err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	nome := "palantir-" + time.Now().Format("2006-01-02") + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+nome+`"`)
	http.ServeContent(w, r, nome, time.Time{}, tmp)
	return arquivoEnviado, nil
}

type visaoAjustes struct {
	IMAP      config.IMAP       `json:"imap"`
	Orcamento config.Orcamento  `json:"orcamento"`
	Modelos   config.Modelos    `json:"modelos"`
	WebHosts  []string          `json:"web_hosts"`
	Segredos  map[string]bool   `json:"segredos"`
	Caminhos  map[string]string `json:"caminhos"`
}

// ajustes mostra a configuração. Dos segredos, só informa se existem.
func (s *Servidor) ajustes(_ http.ResponseWriter, r *http.Request) (any, error) {
	c := s.D.Config()
	v := visaoAjustes{IMAP: c.IMAP, Orcamento: c.Orcamento, Modelos: c.Modelos,
		WebHosts: append([]string{}, c.Ferramentas.Web.Hosts...), Segredos: map[string]bool{},
		Caminhos: map[string]string{"configuracao": s.D.Caminhos.Arquivo(), "dados": s.D.Caminhos.Dados,
			"entrada": config.Expandir(c.Acervo.PastaEntrada)}}
	for _, nome := range segredos.Conhecidos {
		valor, err := s.D.Cofre.Ler(r.Context(), nome)
		if err != nil {
			return nil, err
		}
		v.Segredos[nome] = valor != ""
	}
	return v, nil
}

func (s *Servidor) gravarAjustes(w http.ResponseWriter, r *http.Request) (any, error) {
	in, err := corpo[config.Ajustes](w, r)
	if err != nil {
		return nil, err
	}
	if err := s.D.Ajustar(in); err != nil {
		return nil, fmt.Errorf("%w: %v", atividades.ErrPedidoInvalido, err)
	}
	return nil, nil
}

func (s *Servidor) diario(_ http.ResponseWriter, r *http.Request) (any, error) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, erroHTTP{http.StatusBadRequest, "consulta inválida"}
	}
	n, _ := strconv.Atoi(q.Get("n"))
	if n <= 0 || n > 1000 {
		n = 200
	}
	atividade, _ := strconv.ParseInt(q.Get("atividade"), 10, 64)
	entradas, err := s.D.Diario.Ultimas(n, atividade)
	return map[string]any{"entradas": entradas}, err
}

func (s *Servidor) painel(_ http.ResponseWriter, r *http.Request) (any, error) {
	ctx := r.Context()
	porModelo, err := s.D.Livro.Resumo(ctx, "modelo")
	if err != nil {
		return nil, err
	}
	porFinalidade, err := s.D.Livro.Resumo(ctx, "finalidade")
	if err != nil {
		return nil, err
	}
	gasto, err := s.D.Livro.NoMes(ctx)
	if err != nil {
		return nil, err
	}
	numeros, err := s.D.Acervo.Numeros(ctx)
	if err != nil {
		return nil, err
	}
	lista, err := s.D.Armazem.Listar(ctx, true)
	if err != nil {
		return nil, err
	}
	estados := map[string]int{}
	for _, x := range lista {
		estados[string(x.Estado)]++
	}
	return map[string]any{"gasto_mes_usd": gasto, "teto_mes_usd": s.D.Config().Orcamento.MensalUSD,
		"por_modelo": porModelo, "por_finalidade": porFinalidade, "acervo": numeros, "atividades": estados}, nil
}
