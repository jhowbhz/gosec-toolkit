package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// registro é uma linha do JSONL: o veredito de uma URL. Em erro de rede a
// URL não recebe nota (Grade vazio) — "não consegui medir" é diferente de
// "medi e está bom", e misturar os dois esconderia alvo fora do ar.
type registro struct {
	Timestamp  string  `json:"ts"`
	URL        string  `json:"url"`
	FinalURL   string  `json:"final_url,omitempty"`
	Status     int     `json:"status"`
	Grade      string  `json:"grade,omitempty"`
	Issues     []issue `json:"issues"`
	Error      string  `json:"error,omitempty"`
	DurationMS int64   `json:"duration_ms,omitempty"`
}

// recorder serializa a escrita do JSONL e do console (chegam de vários
// workers) e conta a distribuição de notas para o resumo final.
type recorder struct {
	mu      sync.Mutex
	jsonl   io.Writer
	console io.Writer
	quiet   bool

	notas map[string]int
	erros int
}

func newRecorder(jsonl, console io.Writer, quiet bool) *recorder {
	return &recorder{jsonl: jsonl, console: console, quiet: quiet, notas: map[string]int{}}
}

// record persiste o registro, atualiza os contadores e, salvo quiet,
// imprime uma linha.
func (r *recorder) record(reg registro) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if reg.Issues == nil {
		reg.Issues = []issue{} // JSON "[]" em vez de null, mais fácil de consumir
	}
	if line, err := json.Marshal(reg); err == nil {
		r.jsonl.Write(line)
		r.jsonl.Write([]byte("\n"))
	}

	if reg.Error != "" {
		r.erros++
	} else {
		r.notas[reg.Grade]++
	}

	if !r.quiet {
		fmt.Fprintln(r.console, linhaConsole(reg))
	}
}

// resumo devolve a distribuição de notas (cópia) e o número de URLs com erro.
func (r *recorder) resumo() (notas map[string]int, erros int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	notas = make(map[string]int, len(r.notas))
	for k, v := range r.notas {
		notas[k] = v
	}
	return notas, r.erros
}

// linhaConsole mostra a nota e quantos achados há por severidade, para
// bater o olho e ver onde olhar primeiro.
func linhaConsole(reg registro) string {
	if reg.Error != "" {
		return fmt.Sprintf("ERRO  %s  (%s)", reg.URL, reg.Error)
	}
	c := contarPorSeveridade(reg.Issues)
	return fmt.Sprintf("[%s]  %-3d  %s  crit=%d alta=%d média=%d baixa=%d info=%d",
		reg.Grade, reg.Status, reg.URL,
		c[sevCritica], c[sevAlta], c[sevMedia], c[sevBaixa], c[sevInfo])
}

func contarPorSeveridade(issues []issue) map[string]int {
	c := map[string]int{}
	for _, i := range issues {
		c[i.Severidade]++
	}
	return c
}
