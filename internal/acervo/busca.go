package acervo

import (
	"cmp"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode"
)

// Achado é um resultado de busca.
type Achado struct {
	Trecho
	Pontos float64 `json:"pontos"`
}

var palavrasVazias = func() map[string]bool {
	m := map[string]bool{}
	for _, p := range strings.Fields(`a ao aos as à às o os um uma uns umas de do da dos das em no na nos nas
		por para com sem sob sobre entre e ou que se meu minha meus minhas seu sua tem tenho há ha
		qual quais como quando onde quem é são foi ser estar isso isto esse essa este esta eu me
		the of and to in is`) {
		m[p] = true
	}
	return m
}()

// ConsultaFTS transforma texto livre numa consulta FTS5 segura: cada palavra
// vira um termo entre aspas, com busca por prefixo nas mais longas.
func ConsultaFTS(texto string) string {
	palavras := strings.FieldsFunc(strings.ToLower(texto), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	var termos, reserva []string
	vistos := map[string]bool{}
	for _, p := range palavras {
		if vistos[p] || len(termos) >= 24 {
			continue
		}
		vistos[p] = true
		termo := `"` + p + `"`
		if len([]rune(p)) >= 4 {
			termo += "*"
		}
		if palavrasVazias[p] {
			reserva = append(reserva, termo)
		} else {
			termos = append(termos, termo)
		}
	}
	if len(termos) == 0 {
		termos = reserva
	}
	return strings.Join(termos, " OR ")
}

const candidatos = 50

func (a *Acervo) buscaTextual(ctx context.Context, consulta string) ([]int64, error) {
	q := ConsultaFTS(consulta)
	if q == "" {
		return nil, nil
	}
	rows, err := a.DB.QueryContext(ctx, `SELECT rowid FROM trechos_fts WHERE trechos_fts MATCH ?
		ORDER BY bm25(trechos_fts), rowid LIMIT ?`, q, candidatos)
	if err != nil {
		return nil, fmt.Errorf("busca textual: %w", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// similaridadeMinima descarta vizinhos distantes demais para serem úteis.
const similaridadeMinima = 0.30

func cosseno(a []float32, b []byte) float64 {
	if len(b) != 4*len(a) {
		return -1
	}
	var ab, aa, bb float64
	for i, x := range a {
		y := float64(math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:])))
		ab += float64(x) * y
		aa += float64(x) * float64(x)
		bb += y * y
	}
	if aa == 0 || bb == 0 {
		return -1
	}
	return ab / math.Sqrt(aa*bb)
}

// buscaVetorial compara a consulta com todos os vetores. Para o acervo de uma
// pessoa a varredura completa é rápida e dispensa um índice próprio.
func (a *Acervo) buscaVetorial(ctx context.Context, atividade int64, consulta string) ([]int64, error) {
	if a.Modelos == nil || a.Config().Embeddings == "" {
		return nil, nil
	}
	modelo := a.Config().Embeddings
	var ha bool
	if err := a.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM vetores WHERE modelo = ?)`, modelo).Scan(&ha); err != nil || !ha {
		return nil, err
	}
	v, _, err := a.Modelos.Embeddings(ctx, atividade, []string{consulta})
	if err != nil {
		return nil, nil // sem rede ou sem crédito, vale só a busca textual
	}
	rows, err := a.DB.QueryContext(ctx, `SELECT trecho_id, vetor FROM vetores WHERE modelo = ?`, modelo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type par struct {
		id     int64
		pontos float64
	}
	var pares []par
	for rows.Next() {
		var id int64
		var bruto []byte
		if err := rows.Scan(&id, &bruto); err != nil {
			return nil, err
		}
		if p := cosseno(v[0], bruto); p >= similaridadeMinima {
			pares = append(pares, par{id, p})
		}
	}
	slices.SortFunc(pares, func(x, y par) int { return cmp.Or(cmp.Compare(y.pontos, x.pontos), cmp.Compare(x.id, y.id)) })
	if len(pares) > candidatos {
		pares = pares[:candidatos]
	}
	ids := make([]int64, len(pares))
	for i, p := range pares {
		ids[i] = p.id
	}
	return ids, rows.Err()
}

// Buscar combina a busca textual e a semântica por fusão de posições.
func (a *Acervo) Buscar(ctx context.Context, atividade int64, consulta string, limite int) ([]Achado, error) {
	if limite <= 0 || limite > 50 {
		limite = 10
	}
	textual, err := a.buscaTextual(ctx, consulta)
	if err != nil {
		return nil, err
	}
	vetorial, err := a.buscaVetorial(ctx, atividade, consulta)
	if err != nil {
		return nil, err
	}
	pontos := map[int64]float64{}
	for _, lista := range [][]int64{textual, vetorial} {
		for i, id := range lista {
			pontos[id] += 1 / float64(60+i+1)
		}
	}
	ids := make([]int64, 0, len(pontos))
	for id := range pontos {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(x, y int64) int { return cmp.Or(cmp.Compare(pontos[y], pontos[x]), cmp.Compare(x, y)) })
	if len(ids) > limite {
		ids = ids[:limite]
	}
	trechos, err := a.Trechos(ctx, ids)
	if err != nil {
		return nil, err
	}
	achados := make([]Achado, len(trechos))
	for i, t := range trechos {
		achados[i] = Achado{Trecho: t, Pontos: pontos[t.ID]}
	}
	return achados, nil
}
