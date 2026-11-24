package main

import (
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"
)

// limites são os sinais de quota que o servidor expõe nos headers. Ficam como
// string crua: o formato do Reset varia (segundos, epoch, data HTTP) e a
// ferramenta mede o que o alvo diz, não reinterpreta.
type limites struct {
	RetryAfter string
	Limit      string
	Remaining  string
	Reset      string
}

// parseRateLimitHeaders extrai os sinais de quota. Lê o padrão IETF
// (RateLimit-*) primeiro e cai para o legado (X-RateLimit-*), que ainda é o
// mais comum em APIs na prática.
func parseRateLimitHeaders(h http.Header) limites {
	pegar := func(nomes ...string) string {
		for _, n := range nomes {
			if v := h.Get(n); v != "" {
				return v
			}
		}
		return ""
	}
	return limites{
		RetryAfter: h.Get("Retry-After"),
		Limit:      pegar("RateLimit-Limit", "X-RateLimit-Limit"),
		Remaining:  pegar("RateLimit-Remaining", "X-RateLimit-Remaining"),
		Reset:      pegar("RateLimit-Reset", "X-RateLimit-Reset"),
	}
}

// histogramaStatus conta quantas vezes cada status apareceu. Status 0
// representa request que nem chegou a ter resposta (erro de rede/timeout).
func histogramaStatus(regs []registro) map[int]int {
	h := map[int]int{}
	for _, r := range regs {
		h[r.Status]++
	}
	return h
}

// percentil usa o método nearest-rank sobre uma cópia ordenada, para não
// reordenar a fatia de quem chamou. p em (0,100]; vazio devolve 0.
func percentil(amostras []time.Duration, p float64) time.Duration {
	if len(amostras) == 0 {
		return 0
	}
	ord := append([]time.Duration(nil), amostras...)
	sort.Slice(ord, func(i, j int) bool { return ord[i] < ord[j] })
	idx := int(math.Ceil(p/100*float64(len(ord)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(ord) {
		idx = len(ord) - 1
	}
	return ord[idx]
}

// latencias extrai as durações dos requests que tiveram resposta; erro de
// rede não entra porque seu "tempo" é o do timeout, não o do servidor.
func latencias(regs []registro) []time.Duration {
	var out []time.Duration
	for _, r := range regs {
		if r.Status != 0 {
			out = append(out, time.Duration(r.DurationMS)*time.Millisecond)
		}
	}
	return out
}

// primeiro429 acha o 429 de menor seq. Com concorrência as respostas chegam
// fora de ordem, então "primeiro" é pela ordem de envio, não de chegada.
func primeiro429(regs []registro) (reg registro, achou bool) {
	for _, r := range regs {
		if r.Status == http.StatusTooManyRequests && (!achou || r.Seq < reg.Seq) {
			reg, achou = r, true
		}
	}
	return reg, achou
}

// limitesObservados resume o que os headers disseram ao longo do teste: o
// primeiro Limit visto, o MENOR Remaining (o ponto mais perto do estouro), o
// último Reset e o Retry-After do primeiro 429 (que é o que importa).
func limitesObservados(regs []registro) limites {
	var out limites
	menor := math.MaxInt
	for _, r := range regs {
		if out.Limit == "" {
			out.Limit = r.RateLimitLimit
		}
		if n, err := strconv.Atoi(r.RateLimitRemaining); err == nil && n < menor {
			menor = n
			out.Remaining = r.RateLimitRemaining
		}
		if r.RateLimitReset != "" {
			out.Reset = r.RateLimitReset
		}
	}
	if r, ok := primeiro429(regs); ok {
		out.RetryAfter = r.RetryAfter
	}
	return out
}
