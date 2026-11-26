package estudo

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"time"
)

// LinhaAssunto é um assunto com os números do desempenho.
type LinhaAssunto struct {
	ID       int    `json:"id"`
	Pai      int    `json:"pai"`
	Titulo   string `json:"titulo"`
	Folha    bool   `json:"folha"`
	Material int    `json:"material"`
	Cartoes  int    `json:"cartoes"`
	Vencidos int    `json:"vencidos"`
	Questoes int    `json:"questoes"`
	// Respostas e Acertos contam cartões e questões.
	Respostas int    `json:"respostas"`
	Acertos   int    `json:"acertos"`
	Pendente  bool   `json:"pendente"`
	Aviso     string `json:"aviso"`
}

// Cartao é um cartão a revisar.
type Cartao struct {
	ID      int64  `json:"id"`
	Assunto int    `json:"assunto"`
	Frente  string `json:"frente"`
	Verso   string `json:"verso"`
	Fonte   fonte  `json:"fonte"`
	Novo    bool   `json:"novo"`
}

// Questao é uma questão ainda sem o gabarito.
type Questao struct {
	ID           int64    `json:"id"`
	Assunto      int      `json:"assunto"`
	Enunciado    string   `json:"enunciado"`
	Alternativas []string `json:"alternativas"`
	// Situacao é "nova", "errada" ou "certa", pela última resposta.
	Situacao string `json:"situacao"`
}

// Painel resume o desempenho.
type Painel struct {
	Cartoes     int        `json:"cartoes"`
	Vencidos    int        `json:"vencidos"`
	Questoes    int        `json:"questoes"`
	Respostas   int        `json:"respostas"`
	Acertos     int        `json:"acertos"`
	Hoje        int        `json:"hoje"`
	ProximaEm   *time.Time `json:"proxima_em"`
	MaisFracoID int        `json:"mais_fraco"`
}

// Visao são os dados da tela de estudo.
type Visao struct {
	Extraido bool           `json:"extraido"`
	Assuntos []LinhaAssunto `json:"assuntos"`
	Cartoes  []Cartao       `json:"cartoes"`
	Questoes []Questao      `json:"questoes"`
	Painel   Painel         `json:"painel"`
}

const (
	maxCartoes  = 100
	maxQuestoes = 100
)

// aproveitamento ordena os assuntos do mais fraco ao mais forte. Sem respostas,
// o assunto fica no meio.
func aproveitamento(l LinhaAssunto) float64 {
	return float64(l.Acertos+1) / float64(l.Respostas+2)
}

func (e *Executor) visao(ctx context.Context, id int64, d Dados) (Visao, error) {
	v := Visao{Extraido: d.Extraido, Assuntos: []LinhaAssunto{}, Cartoes: []Cartao{}, Questoes: []Questao{}}
	linhas := map[int]*LinhaAssunto{}
	for _, a := range d.Assuntos {
		v.Assuntos = append(v.Assuntos, LinhaAssunto{ID: a.ID, Pai: a.Pai, Titulo: a.Titulo, Folha: d.folha(a.ID),
			Material: len(a.Trechos), Pendente: a.Pendente, Aviso: a.Aviso})
	}
	for i := range v.Assuntos {
		linhas[v.Assuntos[i].ID] = &v.Assuntos[i]
	}
	agora := e.agora()
	// somar aplica um incremento ao assunto e ao seu principal.
	somar := func(assunto int, f func(*LinhaAssunto)) {
		if l := linhas[assunto]; l != nil {
			f(l)
			if p := linhas[l.Pai]; p != nil {
				f(p)
			}
		}
	}
	consultar := func(q string, ler func(scan func(...any) error) error, args ...any) error {
		rows, err := e.DB.QueryContext(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			if err := ler(rows.Scan); err != nil {
				return err
			}
		}
		return rows.Err()
	}

	inicioDoDia := time.Date(agora.Year(), agora.Month(), agora.Day(), 0, 0, 0, 0, agora.Location()).UnixMilli()
	situacao := map[int64]bool{}
	respondida := map[int64]bool{}
	if err := consultar(`SELECT assunto, tipo, item_id, correta, quando FROM respostas
		WHERE atividade_id = ? ORDER BY id`, func(scan func(...any) error) error {
		var chave, tipo string
		var item, quando int64
		var correta bool
		if err := scan(&chave, &tipo, &item, &correta, &quando); err != nil {
			return err
		}
		if tipo == "questao" {
			respondida[item], situacao[item] = true, correta
		}
		v.Painel.Respostas++
		if correta {
			v.Painel.Acertos++
		}
		if quando >= inicioDoDia {
			v.Painel.Hoje++
		}
		n, _ := strconv.Atoi(chave)
		somar(n, func(l *LinhaAssunto) {
			l.Respostas++
			if correta {
				l.Acertos++
			}
		})
		return nil
	}, id); err != nil {
		return v, err
	}

	var proximas []int64
	if err := consultar(`SELECT id, assunto, frente, verso, fonte, repeticoes, proxima_em FROM cartoes
		WHERE atividade_id = ? ORDER BY proxima_em, id`, func(scan func(...any) error) error {
		var c Cartao
		var chave, f string
		var repeticoes int
		var proxima int64
		if err := scan(&c.ID, &chave, &c.Frente, &c.Verso, &f, &repeticoes, &proxima); err != nil {
			return err
		}
		c.Assunto, _ = strconv.Atoi(chave)
		c.Novo = repeticoes == 0
		vencido := proxima <= agora.UnixMilli()
		v.Painel.Cartoes++
		somar(c.Assunto, func(l *LinhaAssunto) {
			l.Cartoes++
			if vencido {
				l.Vencidos++
			}
		})
		if !vencido {
			proximas = append(proximas, proxima)
			return nil
		}
		v.Painel.Vencidos++
		v.Cartoes = append(v.Cartoes, c)
		return json.Unmarshal([]byte(f), &v.Cartoes[len(v.Cartoes)-1].Fonte)
	}, id); err != nil {
		return v, err
	}
	if len(proximas) > 0 {
		t := time.UnixMilli(slices.Min(proximas)).UTC()
		v.Painel.ProximaEm = &t
	}

	if err := consultar(`SELECT id, assunto, enunciado, alternativas FROM questoes
		WHERE atividade_id = ? ORDER BY id`, func(scan func(...any) error) error {
		var q Questao
		var chave, alternativas string
		if err := scan(&q.ID, &chave, &q.Enunciado, &alternativas); err != nil {
			return err
		}
		q.Assunto, _ = strconv.Atoi(chave)
		q.Situacao = "nova"
		if respondida[q.ID] {
			q.Situacao = "errada"
			if situacao[q.ID] {
				q.Situacao = "certa"
			}
		}
		v.Painel.Questoes++
		somar(q.Assunto, func(l *LinhaAssunto) { l.Questoes++ })
		if err := json.Unmarshal([]byte(alternativas), &q.Alternativas); err != nil {
			return errors.Join(errors.New("alternativas corrompidas"), err)
		}
		v.Questoes = append(v.Questoes, q)
		return nil
	}, id); err != nil {
		return v, err
	}

	// Os assuntos mais fracos vêm primeiro nas revisões e nas questões.
	forca := func(assunto int) float64 {
		if l := linhas[assunto]; l != nil {
			return aproveitamento(*l)
		}
		return 1
	}
	slices.SortStableFunc(v.Cartoes, func(a, b Cartao) int { return cmp.Compare(forca(a.Assunto), forca(b.Assunto)) })
	ordem := map[string]int{"nova": 0, "errada": 1, "certa": 2}
	slices.SortStableFunc(v.Questoes, func(a, b Questao) int {
		return cmp.Or(cmp.Compare(ordem[a.Situacao], ordem[b.Situacao]), cmp.Compare(forca(a.Assunto), forca(b.Assunto)))
	})
	v.Cartoes = v.Cartoes[:min(len(v.Cartoes), maxCartoes)]
	v.Questoes = v.Questoes[:min(len(v.Questoes), maxQuestoes)]

	pior := 2.0
	for _, l := range v.Assuntos {
		if l.Folha && l.Respostas > 0 && aproveitamento(l) < pior {
			pior, v.Painel.MaisFracoID = aproveitamento(l), l.ID
		}
	}
	return v, nil
}
