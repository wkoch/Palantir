package acervo

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"slices"
	"time"
)

// Problema descreve um original ausente ou alterado.
type Problema struct {
	Documento int64  `json:"documento"`
	Nome      string `json:"nome"`
	SHA256    string `json:"sha256"`
	Motivo    string `json:"motivo"`
}

func hashDoArquivo(caminho string) (string, error) {
	f, err := os.Open(caminho)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (a *Acervo) todos(ctx context.Context) ([]Documento, error) {
	return a.documentos(ctx, `SELECT `+colunas+` FROM documentos ORDER BY id`)
}

// Verificar recalcula o hash de todos os originais.
func (a *Acervo) Verificar(ctx context.Context) (conferidos int, problemas []Problema, err error) {
	docs, err := a.todos(ctx)
	if err != nil {
		return 0, nil, err
	}
	problemas = []Problema{}
	for _, d := range docs {
		if err := ctx.Err(); err != nil {
			return conferidos, problemas, err
		}
		p := Problema{Documento: d.ID, Nome: d.Nome, SHA256: d.SHA256}
		switch h, err := hashDoArquivo(a.CaminhoOriginal(d.SHA256)); {
		case err != nil:
			p.Motivo = err.Error()
		case h != d.SHA256:
			p.Motivo = "o conteúdo mudou: " + h
		default:
			conferidos++
			continue
		}
		problemas = append(problemas, p)
	}
	return conferidos, problemas, nil
}

// ErrOriginais indica que há originais ausentes ou alterados.
var ErrOriginais = errors.New("há originais ausentes ou alterados")

// Regenerar descarta todos os derivados e os refaz a partir dos originais, que
// são conferidos antes e depois.
func (a *Acervo) Regenerar(ctx context.Context) error {
	conferir := func(quando string) error {
		_, problemas, err := a.Verificar(ctx)
		if err != nil {
			return err
		}
		if len(problemas) > 0 {
			return fmt.Errorf("%w %s da regeneração: %s (%s)", ErrOriginais, quando, problemas[0].Nome, problemas[0].Motivo)
		}
		return nil
	}
	if err := conferir("antes"); err != nil {
		return err
	}
	err := func() error {
		a.processamento.Lock()
		defer a.processamento.Unlock()
		tx, err := a.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, q := range []string{
			`DELETE FROM vetores`, `DELETE FROM trechos`, `DELETE FROM entidades`, `DELETE FROM cache_modelos`,
			`DELETE FROM colecao_documentos WHERE colecao_id IN (SELECT id FROM colecoes WHERE criada_por = 'ia')`,
			`DELETE FROM colecoes WHERE criada_por = 'ia'`,
			`INSERT INTO trechos_fts(trechos_fts) VALUES ('rebuild')`,
			`UPDATE documentos SET estado = 'pendente', erro = '', resumo = '', paginas = 0, enriquecido = 0`,
		} {
			if _, err := tx.ExecContext(ctx, q); err != nil {
				return fmt.Errorf("regenerar: %w", err)
			}
		}
		return tx.Commit()
	}()
	if err != nil {
		return err
	}
	if err := a.Processar(ctx); err != nil {
		return err
	}
	return conferir("depois")
}

// Manifesto é o metadados.json da exportação.
type Manifesto struct {
	Formato    int                 `json:"formato"`
	CriadoEm   time.Time           `json:"criado_em"`
	Documentos []DocumentoCompleto `json:"documentos"`
	Colecoes   []Colecao           `json:"colecoes"`
}

// DocumentoCompleto é um documento com as visões que o citam.
type DocumentoCompleto struct {
	Documento
	Arquivo   string     `json:"arquivo"`
	Entidades []Entidade `json:"entidades"`
	Colecoes  []string   `json:"colecoes"`
}

// Exportar grava um ZIP com os originais e os metadados. extras são arquivos
// JSON adicionais, como o histórico das atividades, e copias, arquivos do disco
// por nome dentro do ZIP.
func (a *Acervo) Exportar(ctx context.Context, w io.Writer, extras map[string]any, copias map[string]string) error {
	docs, err := a.todos(ctx)
	if err != nil {
		return err
	}
	colecoes, err := a.Colecoes(ctx)
	if err != nil {
		return err
	}
	m := Manifesto{Formato: 1, CriadoEm: time.Now().UTC(), Colecoes: colecoes, Documentos: []DocumentoCompleto{}}
	membros := map[int64][]string{}
	rows, err := a.DB.QueryContext(ctx, `SELECT cd.documento_id, c.nome FROM colecao_documentos cd
		JOIN colecoes c ON c.id = cd.colecao_id ORDER BY c.nome`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		var nome string
		if err := rows.Scan(&id, &nome); err != nil {
			rows.Close()
			return err
		}
		membros[id] = append(membros[id], nome)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}

	z := zip.NewWriter(w)
	for _, d := range docs {
		if err := ctx.Err(); err != nil {
			return err
		}
		entidades, err := a.Entidades(ctx, "", "", d.ID)
		if err != nil {
			return err
		}
		arquivo := "originais/" + d.SHA256 + "/" + d.Nome
		m.Documentos = append(m.Documentos, DocumentoCompleto{Documento: d, Arquivo: arquivo,
			Entidades: entidades, Colecoes: append([]string{}, membros[d.ID]...)})
		f, err := os.Open(a.CaminhoOriginal(d.SHA256))
		if err != nil {
			return fmt.Errorf("exportar %s: %w", d.Nome, err)
		}
		destino, err := z.CreateHeader(&zip.FileHeader{Name: arquivo, Method: zip.Deflate, Modified: d.ImportadoEm})
		if err == nil {
			_, err = io.Copy(destino, f)
		}
		if err := errors.Join(err, f.Close()); err != nil {
			return err
		}
	}
	arquivos := map[string]any{"metadados.json": m}
	for nome, v := range extras {
		arquivos[nome] = v
	}
	for nome, v := range arquivos {
		destino, err := z.Create(nome)
		if err != nil {
			return err
		}
		enc := json.NewEncoder(destino)
		enc.SetIndent("", "  ")
		if err := enc.Encode(v); err != nil {
			return err
		}
	}
	for _, nome := range slices.Sorted(maps.Keys(copias)) {
		if err := acrescentar(z, nome, copias[nome]); err != nil {
			return fmt.Errorf("exportar %s: %w", nome, err)
		}
	}
	return z.Close()
}

// acrescentar põe no ZIP um arquivo do disco; um que não existe é ignorado.
func acrescentar(z *zip.Writer, nome, caminho string) error {
	f, err := os.Open(caminho)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	destino, err := z.CreateHeader(&zip.FileHeader{Name: nome, Method: zip.Deflate, Modified: time.Now()})
	if err != nil {
		return err
	}
	_, err = io.Copy(destino, f)
	return err
}
