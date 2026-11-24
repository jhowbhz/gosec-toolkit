package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"
)

// registro é uma linha do JSONL: o que aconteceu com um request. Status 0 +
// Error significa que não houve resposta. Elapsed é o tempo desde o início do
// teste, que é o que permite dizer "o 429 veio aos N segundos".
type registro struct {
	Timestamp          string `json:"ts"`
	Seq                int    `json:"seq"`
	Status             int    `json:"status"`
	DurationMS         int64  `json:"duration_ms"`
	ElapsedMS          int64  `json:"elapsed_ms"`
	RetryAfter         string `json:"retry_after,omitempty"`
	RateLimitLimit     string `json:"ratelimit_limit,omitempty"`
	RateLimitRemaining string `json:"ratelimit_remaining,omitempty"`
	RateLimitReset     string `json:"ratelimit_reset,omitempty"`
	Error              string `json:"error,omitempty"`
}

// recorder serializa a escrita do JSONL e da linha de console (chegam de
// várias goroutines) e guarda os registros para o resumo final. Até 5000
// registros em memória é barato e evita reler o arquivo.
type recorder struct {
	mu      sync.Mutex
	jsonl   io.Writer
	console io.Writer
	quiet   bool
	regs    []registro
}

func newRecorder(jsonl, console io.Writer, quiet bool) *recorder {
	return &recorder{jsonl: jsonl, console: console, quiet: quiet}
}

// record persiste o registro, guarda para o resumo e, salvo quiet, imprime
// uma linha de acompanhamento.
func (r *recorder) record(reg registro) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if line, err := json.Marshal(reg); err == nil {
		r.jsonl.Write(line)
		r.jsonl.Write([]byte("\n"))
	}
	r.regs = append(r.regs, reg)

	if !r.quiet {
		fmt.Fprintln(r.console, linhaConsole(reg))
	}
}

// registros devolve uma cópia ordenada por seq (a ordem de chegada varia com
// a concorrência, a de envio é a que faz sentido ler).
func (r *recorder) registros() []registro {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := append([]registro(nil), r.regs...)
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out
}

func linhaConsole(reg registro) string {
	if reg.Error != "" {
		return fmt.Sprintf("#%04d  ERRO  %6dms  (%s)", reg.Seq, reg.DurationMS, reg.Error)
	}
	linha := fmt.Sprintf("#%04d  %-4d  %6dms", reg.Seq, reg.Status, reg.DurationMS)
	if reg.RateLimitRemaining != "" {
		linha += fmt.Sprintf("  remaining=%s", reg.RateLimitRemaining)
	}
	if reg.RetryAfter != "" {
		linha += fmt.Sprintf("  retry-after=%s", reg.RetryAfter)
	}
	return linha
}

// escreverResumo imprime o fechamento do teste: quantos requests saíram, o
// histograma de status, onde (e se) o 429 apareceu e o que os headers
// revelaram sobre a quota.
func escreverResumo(w io.Writer, regs []registro, motivo string, total time.Duration) {
	fmt.Fprintf(w, "\nconcluído: %d requests em %s — %s\n", len(regs), total.Round(time.Millisecond), motivo)

	hist := histogramaStatus(regs)
	codigos := make([]int, 0, len(hist))
	for c := range hist {
		codigos = append(codigos, c)
	}
	sort.Ints(codigos)
	fmt.Fprintln(w, "status:")
	for _, c := range codigos {
		nome := fmt.Sprintf("%d", c)
		if c == 0 {
			nome = "erro"
		}
		fmt.Fprintf(w, "  %-5s %d\n", nome, hist[c])
	}

	if r, ok := primeiro429(regs); ok {
		fmt.Fprintf(w, "primeiro 429: request #%d, aos %s\n", r.Seq, (time.Duration(r.ElapsedMS) * time.Millisecond).Round(time.Millisecond))
	} else {
		fmt.Fprintln(w, "primeiro 429: não ocorreu")
	}

	lat := latencias(regs)
	fmt.Fprintf(w, "latência: p50=%s p95=%s\n", percentil(lat, 50), percentil(lat, 95))

	lim := limitesObservados(regs)
	if lim == (limites{}) {
		fmt.Fprintln(w, "headers de rate-limit: o servidor não enviou nenhum")
		return
	}
	fmt.Fprintln(w, "headers de rate-limit observados:")
	for _, par := range []struct{ nome, valor string }{
		{"limit", lim.Limit}, {"menor remaining", lim.Remaining},
		{"reset", lim.Reset}, {"retry-after (no 429)", lim.RetryAfter},
	} {
		if par.valor != "" {
			fmt.Fprintf(w, "  %s: %s\n", par.nome, par.valor)
		}
	}
}
