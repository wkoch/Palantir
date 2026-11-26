package acervo

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wkoch/Palantir/internal/atividades"
	"github.com/wkoch/Palantir/internal/diario"
	"github.com/wkoch/Palantir/internal/esquema"
	"github.com/wkoch/Palantir/internal/extrator"
	"github.com/wkoch/Palantir/internal/ferramentas"
	"github.com/wkoch/Palantir/internal/modelos"
	"github.com/wkoch/Palantir/internal/proveniencia"
)

// Processar leva os documentos pendentes até o fim: extração, índices e, se os
// modelos estiverem disponíveis, vetores e entidades.
func (a *Acervo) Processar(ctx context.Context) error {
	a.processamento.Lock()
	defer a.processamento.Unlock()
	pendentes, err := a.documentos(ctx, `SELECT `+colunas+` FROM documentos WHERE estado = ? ORDER BY id`, Pendente)
	if err != nil {
		return err
	}
	for _, d := range pendentes {
		if err := a.extrair(ctx, d); err != nil {
			return err
		}
		a.avisar()
	}
	return errors.Join(a.vetorizar(ctx), a.enriquecer(ctx))
}

// extrair grava os trechos de um documento. Falhas do documento ficam
// registradas nele; só erros do próprio sistema são devolvidos.
func (a *Acervo) extrair(ctx context.Context, d Documento) error {
	var r extrator.Resultado
	err := ferramentas.Chamar(ctx, a.Executor, ferramentas.Execucao{
		Servidor: ferramentas.Extrair, Entrada: a.CaminhoOriginal(d.SHA256),
	}, "extrair", map[string]any{"nome": d.Nome}, &r)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		if errReg := a.Diario.Registrar(diario.Entrada{Tipo: "ferramenta", Nome: "extrair", Decisao: "erro",
			Detalhe: d.Nome + ": " + err.Error()}); errReg != nil {
			return errReg
		}
		_, errDB := a.DB.ExecContext(ctx, `UPDATE documentos SET estado = ?, erro = ? WHERE id = ?`,
			Erro, err.Error(), d.ID)
		return errDB
	}
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM trechos WHERE documento_id = ?`, d.ID); err != nil {
		return err
	}
	ordem := 0
	for _, p := range r.Paginas {
		for _, parte := range Dividir(p.Texto) {
			ordem++
			if _, err := tx.ExecContext(ctx, `INSERT INTO trechos (documento_id, pagina, ordem, texto)
				VALUES (?, ?, ?, ?)`, d.ID, p.Numero, ordem, parte); err != nil {
				return err
			}
		}
	}
	aviso := r.Aviso
	if ordem == 0 && aviso == "" {
		aviso = "nenhum texto encontrado"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE documentos SET estado = ?, erro = ?, paginas = ?, enriquecido = 0
		WHERE id = ?`, Pronto, aviso, len(r.Paginas), d.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// Reextrair devolve à fila um documento que falhou.
func (a *Acervo) Reextrair(ctx context.Context, id int64) error {
	res, err := a.DB.ExecContext(ctx, `UPDATE documentos SET estado = ?, erro = '' WHERE id = ?`, Pendente, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNaoEncontrado
	}
	a.Acordar()
	return nil
}

func codificar(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(x))
	}
	return b
}

const lote = 32

// vetorizar calcula os vetores que faltam para o modelo configurado.
func (a *Acervo) vetorizar(ctx context.Context) error {
	if a.Modelos == nil || a.Config().Embeddings == "" {
		return nil
	}
	modelo := a.Config().Embeddings
	for {
		faltam, err := a.trechos(ctx, `LEFT JOIN vetores v ON v.trecho_id = t.id AND v.modelo = ?
			WHERE v.trecho_id IS NULL ORDER BY t.id LIMIT ?`, modelo, lote)
		if err != nil || len(faltam) == 0 {
			return err
		}
		textos := make([]string, len(faltam))
		for i, t := range faltam {
			textos[i] = t.Texto
		}
		vetores, _, err := a.Modelos.Embeddings(ctx, 0, textos)
		if err != nil {
			// Sem modelo, a busca textual continua funcionando.
			if atividades.Indisponivel(err) {
				return nil
			}
			return fmt.Errorf("vetorizar: %w", err)
		}
		tx, err := a.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		for i, t := range faltam {
			if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO vetores (trecho_id, modelo, vetor)
				VALUES (?, ?, ?)`, t.ID, modelo, codificar(vetores[i])); err != nil {
				tx.Rollback()
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
}

// TiposDeEntidade são as categorias extraídas dos documentos.
var TiposDeEntidade = []string{"pessoa", "organizacao", "data", "valor", "contrato", "assunto"}

var esquemaEnriquecimento = esquema.JSON(esquema.Objeto(map[string]any{
	"resumo": esquema.Texto,
	"entidades": esquema.Lista(esquema.Objeto(map[string]any{
		"tipo": esquema.Enum(TiposDeEntidade...), "valor": esquema.Texto,
	})),
	"colecoes": esquema.Lista(esquema.Texto),
}))

const sistemaEnriquecimento = `Você cataloga documentos do usuário. Para o documento recebido, escreva um resumo
de até duas frases, liste as entidades presentes (pessoas, organizações, datas, valores, contratos e assuntos)
e sugira até duas coleções temáticas curtas, reaproveitando as existentes quando servirem.`

const maxAmostra = 8000

func curto(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > n {
		s = string([]rune(s)[:n])
	}
	return s
}

func (a *Acervo) enriquecer(ctx context.Context) error {
	if a.Modelos == nil || !a.Config().Enriquecer {
		return nil
	}
	pendentes, err := a.documentos(ctx, `SELECT `+colunas+` FROM documentos
		WHERE estado = ? AND enriquecido = 0 AND paginas > 0 ORDER BY id`, Pronto)
	if err != nil {
		return err
	}
	for _, d := range pendentes {
		err := a.enriquecerUm(ctx, d)
		switch {
		case err == nil:
		case atividades.Indisponivel(err):
			return nil
		case errors.Is(err, modelos.ErrRespostaRuim):
			// Não insiste num documento que o modelo não consegue catalogar.
			if _, err := a.DB.ExecContext(ctx, `UPDATE documentos SET enriquecido = 1 WHERE id = ?`, d.ID); err != nil {
				return err
			}
		default:
			return err
		}
		a.avisar()
	}
	return nil
}

func (a *Acervo) enriquecerUm(ctx context.Context, d Documento) error {
	trechos, err := a.TrechosDoDocumento(ctx, d.ID)
	if err != nil {
		return err
	}
	var amostra strings.Builder
	for _, t := range trechos {
		if amostra.Len() > maxAmostra {
			break
		}
		amostra.WriteString(t.Texto + "\n")
	}
	colecoes, err := a.Colecoes(ctx)
	if err != nil {
		return err
	}
	nomes := make([]string, len(colecoes))
	for i, c := range colecoes {
		nomes[i] = c.Nome
	}
	var v struct {
		Resumo    string     `json:"resumo"`
		Entidades []Entidade `json:"entidades"`
		Colecoes  []string   `json:"colecoes"`
	}
	err = modelos.CompletarJSON(ctx, a.Modelos, modelos.Pedido{Nivel: modelos.Barato, Finalidade: "acervo",
		Cache: true, Esquema: &modelos.Esquema{Nome: "enriquecimento", Definicao: esquemaEnriquecimento},
		Mensagens: []modelos.Mensagem{
			{Papel: modelos.PapelSistema, Conteudo: sistemaEnriquecimento + "\n\n" + proveniencia.Instrucao},
			{Papel: modelos.PapelUsuario, Conteudo: "Coleções existentes: " + strings.Join(nomes, ", ") + "\n\n" +
				proveniencia.Embrulhar(proveniencia.Documento, d.Nome, amostra.String())},
		}}, &v)
	if err != nil {
		return err
	}
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE documentos SET resumo = ?, enriquecido = 1 WHERE id = ?`,
		curto(v.Resumo, 500), d.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM entidades WHERE documento_id = ?`, d.ID); err != nil {
		return err
	}
	for i, e := range v.Entidades {
		if i >= 60 {
			break
		}
		if valor := curto(e.Valor, 120); valor != "" {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO entidades (documento_id, tipo, valor)
				VALUES (?, ?, ?)`, d.ID, e.Tipo, valor); err != nil {
				return err
			}
		}
	}
	// Coleções criadas pela IA são refeitas; as do usuário não são tocadas.
	if _, err := tx.ExecContext(ctx, `DELETE FROM colecao_documentos WHERE documento_id = ?
		AND colecao_id IN (SELECT id FROM colecoes WHERE criada_por = 'ia')`, d.ID); err != nil {
		return err
	}
	for i, nome := range v.Colecoes {
		nome = curto(nome, 60)
		if i >= 2 || nome == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO colecoes (nome, criada_por, criada_em)
			VALUES (?, 'ia', ?)`, nome, time.Now().UnixMilli()); err != nil {
			return err
		}
		// A IA não acrescenta documentos a uma coleção feita pelo usuário.
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO colecao_documentos (colecao_id, documento_id)
			SELECT id, ? FROM colecoes WHERE nome = ? AND criada_por = 'ia'`, d.ID, nome); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type marcaArquivo struct{ tamanho, modificado int64 }

// Vigia observa a pasta de entrada.
type Vigia struct {
	Acervo *Acervo
	Pasta  func() string
	vistos map[string]marcaArquivo
}

// Varrer importa os arquivos novos que não mudaram desde a varredura anterior,
// para não copiar um download pela metade. Nada na pasta é alterado.
func (v *Vigia) Varrer(ctx context.Context) (Relatorio, error) {
	rel := Relatorio{Falhas: []string{}}
	pasta := v.Pasta()
	if pasta == "" || v.Acervo.conferirCaminho(resolver(pasta)) != nil {
		return rel, nil
	}
	pasta = resolver(pasta)
	privadas := v.Acervo.privadas()
	agora := map[string]marcaArquivo{}
	err := filepath.WalkDir(pasta, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == pasta {
				return err
			}
			rel.Falhas = append(rel.Falhas, err.Error())
			return nil
		}
		if d.IsDir() && slices.Contains(privadas, p) {
			return fs.SkipDir
		}
		if p != pasta && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || temporario(d.Name()) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil // removido durante a varredura
		}
		m := marcaArquivo{info.Size(), info.ModTime().UnixNano()}
		agora[p] = m
		if anterior, ok := v.vistos[p]; !ok || anterior != m || m.tamanho == 0 {
			return nil
		}
		var tamanho, modificado int64
		err = v.Acervo.DB.QueryRowContext(ctx, `SELECT tamanho, modificado FROM entrada_vistos WHERE caminho = ?`, p).
			Scan(&tamanho, &modificado)
		if err == nil && tamanho == m.tamanho && modificado == m.modificado {
			return nil
		}
		novo, err := v.Acervo.importarArquivo(ctx, p, "entrada")
		if err != nil {
			rel.Falhas = append(rel.Falhas, p+": "+err.Error())
			if !errors.Is(err, ErrGrande) {
				return ctx.Err()
			}
		} else {
			rel.somar(novo)
		}
		_, err = v.Acervo.DB.ExecContext(ctx, `INSERT OR REPLACE INTO entrada_vistos (caminho, tamanho, modificado)
			VALUES (?, ?, ?)`, p, m.tamanho, m.modificado)
		return err
	})
	v.vistos = agora
	if errors.Is(err, fs.ErrNotExist) {
		return rel, nil
	}
	return rel, err
}

func temporario(nome string) bool {
	for _, s := range []string{".part", ".crdownload", ".tmp", ".partial", "~"} {
		if strings.HasSuffix(strings.ToLower(nome), s) {
			return true
		}
	}
	return false
}

// Trabalhar vigia a entrada e processa documentos até o contexto acabar.
func (a *Acervo) Trabalhar(ctx context.Context, v *Vigia, intervalo func() time.Duration, relatar func(error)) {
	if pasta := v.Pasta(); pasta != "" {
		if err := os.MkdirAll(pasta, 0o700); err != nil {
			relatar(err)
		}
	}
	for {
		if _, err := v.Varrer(ctx); err != nil && ctx.Err() == nil {
			relatar(fmt.Errorf("pasta de entrada: %w", err))
		}
		if err := a.Processar(ctx); err != nil && ctx.Err() == nil {
			relatar(fmt.Errorf("processar o acervo: %w", err))
		}
		select {
		case <-ctx.Done():
			return
		case <-a.sinal():
		case <-time.After(intervalo()):
		}
	}
}
