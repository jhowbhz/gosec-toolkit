package main

import (
	"net/http"
	"testing"
	"time"
)

func TestPercentilConjuntoConhecido(t *testing.T) {
	var amostras []time.Duration
	for i := 10; i >= 1; i-- { // fora de ordem de propósito: percentil precisa ordenar
		amostras = append(amostras, time.Duration(i)*time.Millisecond)
	}
	if got := percentil(amostras, 50); got != 5*time.Millisecond {
		t.Errorf("p50 = %s, quero 5ms", got)
	}
	if got := percentil(amostras, 95); got != 10*time.Millisecond {
		t.Errorf("p95 = %s, quero 10ms", got)
	}
	if amostras[0] != 10*time.Millisecond {
		t.Error("percentil não pode reordenar a fatia de quem chamou")
	}
}

func TestPercentilVazio(t *testing.T) {
	if got := percentil(nil, 95); got != 0 {
		t.Errorf("p95 de vazio = %s, quero 0", got)
	}
}

func TestHistogramaStatus(t *testing.T) {
	regs := []registro{{Status: 200}, {Status: 200}, {Status: 429}, {Status: 0}}
	h := histogramaStatus(regs)
	if h[200] != 2 || h[429] != 1 || h[0] != 1 {
		t.Errorf("histograma = %v, quero 200:2 429:1 0:1", h)
	}
}

// Com concorrência a resposta do 429 pode chegar antes da de um seq menor;
// o "primeiro" tem que ser o de menor seq, senão o resumo mente sobre onde
// o limite estourou.
func TestPrimeiro429PorSeq(t *testing.T) {
	regs := []registro{
		{Seq: 7, Status: 429, ElapsedMS: 7000},
		{Seq: 5, Status: 429, ElapsedMS: 5000},
		{Seq: 6, Status: 200},
	}
	r, ok := primeiro429(regs)
	if !ok || r.Seq != 5 || r.ElapsedMS != 5000 {
		t.Errorf("primeiro429 = %+v ok=%v, quero seq 5 aos 5000ms", r, ok)
	}
	if _, ok := primeiro429([]registro{{Seq: 1, Status: 200}}); ok {
		t.Error("não há 429, não deveria achar")
	}
}

func TestParseRateLimitHeadersXPrefixado(t *testing.T) {
	h := http.Header{}
	h.Set("X-RateLimit-Limit", "100")
	h.Set("X-RateLimit-Remaining", "42")
	h.Set("X-RateLimit-Reset", "1700000000")
	h.Set("Retry-After", "30")
	got := parseRateLimitHeaders(h)
	want := limites{RetryAfter: "30", Limit: "100", Remaining: "42", Reset: "1700000000"}
	if got != want {
		t.Errorf("got %+v, quero %+v", got, want)
	}
}

func TestParseRateLimitHeadersPadraoIETFVence(t *testing.T) {
	h := http.Header{}
	h.Set("RateLimit-Limit", "10")
	h.Set("RateLimit-Remaining", "3")
	h.Set("RateLimit-Reset", "9")
	h.Set("X-RateLimit-Limit", "999")
	got := parseRateLimitHeaders(h)
	if got.Limit != "10" || got.Remaining != "3" || got.Reset != "9" {
		t.Errorf("RateLimit-* deveria vencer X-RateLimit-*: %+v", got)
	}
}

func TestParseRateLimitHeadersSemNada(t *testing.T) {
	if got := parseRateLimitHeaders(http.Header{}); got != (limites{}) {
		t.Errorf("sem headers deveria dar zero value: %+v", got)
	}
}

func TestLimitesObservadosMenorRemaining(t *testing.T) {
	regs := []registro{
		{Seq: 1, Status: 200, RateLimitLimit: "5", RateLimitRemaining: "4"},
		{Seq: 2, Status: 200, RateLimitLimit: "5", RateLimitRemaining: "1"},
		{Seq: 3, Status: 429, RateLimitRemaining: "0", RetryAfter: "12"},
	}
	got := limitesObservados(regs)
	if got.Limit != "5" || got.Remaining != "0" || got.RetryAfter != "12" {
		t.Errorf("limitesObservados = %+v", got)
	}
}

// A rampa decide em que degrau o 429 aparece; um erro aqui desloca toda a
// leitura do resultado.
func TestTaxaDoPassoRampa(t *testing.T) {
	// total 10, 5 degraus, rate 10: 2 requests por degrau -> 2,4,6,8,10
	quero := []int{2, 2, 4, 4, 6, 6, 8, 8, 10, 10}
	for i, q := range quero {
		if got := taxaDoPasso(i+1, 10, true, 5, 10); got != q {
			t.Errorf("seq %d: taxa = %d, quero %d", i+1, got, q)
		}
	}
	if got := taxaDoPasso(3, 10, false, 5, 7); got != 7 {
		t.Errorf("sem rampa a taxa é constante: %d, quero 7", got)
	}
	if got := taxaDoPasso(1, 10, true, 5, 3); got != 1 {
		t.Errorf("a rampa nunca desce de 1/s: %d", got)
	}
}
