package estudo

import (
	"math"
	"testing"
	"time"
)

func TestFSRS(t *testing.T) {
	agora := time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC)
	// Cartão novo: o intervalo cresce com a nota.
	var proximas []time.Time
	for nota := Errei; nota <= Facil; nota++ {
		m, p := Revisar(Memoria{}, nota, agora)
		if m.Repeticoes != 1 || m.Estabilidade != w[nota-1] || m.Dificuldade < 1 || m.Dificuldade > 10 {
			t.Errorf("nota %d: %+v", nota, m)
		}
		proximas = append(proximas, p)
	}
	if proximas[0] != agora.Add(10*time.Minute) {
		t.Error("errar volta na mesma sessão", proximas[0])
	}
	// Para 90% de retenção, o intervalo em dias é a estabilidade.
	if proximas[2] != agora.AddDate(0, 0, 4) || proximas[3] != agora.AddDate(0, 0, 14) {
		t.Error(proximas)
	}
	if r := Lembranca(10, 10); math.Abs(r-0.9) > 1e-9 {
		t.Error(r)
	}
	if Lembranca(1, 0) != 0 || Lembranca(-5, 3) != 1 {
		t.Error("bordas")
	}

	// Acertos sucessivos afastam as revisões.
	m, p := Revisar(Memoria{}, Bom, agora)
	anterior := p.Sub(agora)
	for range 5 {
		quando := p
		m, p = Revisar(m, Bom, quando)
		if p.Sub(quando) <= anterior {
			t.Fatal("o intervalo deveria crescer", p.Sub(quando), anterior)
		}
		anterior = p.Sub(quando)
	}
	base := m
	dificil, pd := Revisar(base, Dificil, p)
	bom, pb := Revisar(base, Bom, p)
	facil, pf := Revisar(base, Facil, p)
	if !(pd.Before(pb) && pb.Before(pf)) || !(dificil.Dificuldade > bom.Dificuldade && bom.Dificuldade > facil.Dificuldade) {
		t.Error("ordem das notas", pd, pb, pf)
	}
	errou, pe := Revisar(base, Errei, p)
	if errou.Lapsos != 1 || errou.Estabilidade >= base.Estabilidade || !pe.Before(pd) {
		t.Errorf("%+v", errou)
	}
	// Valores fora da escala são ajustados e o intervalo tem teto.
	enorme, pm := Revisar(Memoria{Estabilidade: 1e9, Dificuldade: 5, Repeticoes: 3, RevisadoEm: agora}, 99, agora.AddDate(1, 0, 0))
	if enorme.Dificuldade < 1 || pm.Sub(agora.AddDate(1, 0, 0)) > 36501*24*time.Hour {
		t.Error(pm)
	}
	if _, p := Revisar(Memoria{}, -3, agora); p != agora.Add(10*time.Minute) {
		t.Error("nota mínima")
	}
}
