package estudo

import (
	"math"
	"time"
)

// Repetição espaçada pelo algoritmo FSRS 4.5, com os parâmetros padrão. É
// código determinístico e funciona sem rede.

// Notas de uma revisão.
const (
	Errei   = 1
	Dificil = 2
	Bom     = 3
	Facil   = 4
)

var w = [17]float64{0.4872, 1.4003, 3.7145, 13.8206, 5.1618, 1.2298, 0.8975, 0.031,
	1.6474, 0.1367, 1.0461, 2.1072, 0.0793, 0.3246, 1.587, 0.2272, 2.8755}

const (
	decaimento = -0.5
	fator      = 19.0 / 81.0
	retencao   = 0.9
	maxDias    = 36500
	// Depois de errar, o cartão volta na mesma sessão.
	reaprender = 10 * time.Minute
)

// Memoria é o estado de um cartão.
type Memoria struct {
	Estabilidade float64
	Dificuldade  float64
	Repeticoes   int
	Lapsos       int
	RevisadoEm   time.Time
}

func limitar(v, minimo, maximo float64) float64 { return math.Max(minimo, math.Min(maximo, v)) }

func dificuldadeInicial(nota int) float64 { return limitar(w[4]-float64(nota-3)*w[5], 1, 10) }

// Lembranca estima a chance de lembrar depois de dias sem revisar.
func Lembranca(dias, estabilidade float64) float64 {
	if estabilidade <= 0 {
		return 0
	}
	return math.Pow(1+fator*math.Max(dias, 0)/estabilidade, decaimento)
}

func intervalo(estabilidade float64) int {
	dias := estabilidade / fator * (math.Pow(retencao, 1/decaimento) - 1)
	return int(limitar(math.Round(dias), 1, maxDias))
}

// Revisar aplica uma nota e devolve o novo estado e quando revisar de novo.
func Revisar(m Memoria, nota int, agora time.Time) (Memoria, time.Time) {
	nota = int(limitar(float64(nota), Errei, Facil))
	if m.Repeticoes == 0 || m.Estabilidade <= 0 {
		m.Estabilidade, m.Dificuldade = w[nota-1], dificuldadeInicial(nota)
	} else {
		dias := agora.Sub(m.RevisadoEm).Hours() / 24
		r := Lembranca(dias, m.Estabilidade)
		d, s := m.Dificuldade, m.Estabilidade
		proxima := d - w[6]*float64(nota-3)
		m.Dificuldade = limitar(w[7]*dificuldadeInicial(Bom)+(1-w[7])*proxima, 1, 10)
		if nota == Errei {
			novo := w[11] * math.Pow(d, -w[12]) * (math.Pow(s+1, w[13]) - 1) * math.Exp(w[14]*(1-r))
			m.Estabilidade = limitar(novo, 0.01, s)
			m.Lapsos++
		} else {
			penalidade, bonus := 1.0, 1.0
			if nota == Dificil {
				penalidade = w[15]
			}
			if nota == Facil {
				bonus = w[16]
			}
			m.Estabilidade = s * (1 + math.Exp(w[8])*(11-d)*math.Pow(s, -w[9])*(math.Exp(w[10]*(1-r))-1)*penalidade*bonus)
		}
	}
	m.Repeticoes++
	m.RevisadoEm = agora
	if nota == Errei {
		return m, agora.Add(reaprender)
	}
	return m, agora.AddDate(0, 0, intervalo(m.Estabilidade))
}
