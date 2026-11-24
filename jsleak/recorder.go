package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// achado é uma linha do manifesto: onde o catálogo casou e o que casou. É a
// evidência da auditoria, então Match e Context já chegam REDIGIDOS (a menos
// que o operador tenha pedido --no-redact): o JSONL nunca vira um cofre de
// segredos em texto puro por acidente.
type achado struct {
	Timestamp string `json:"ts"`
	Source    string `json:"source"` // "file:<caminho>" ou a URL varrida
	Line      int    `json:"line,omitempty"`
	Rule      string `json:"rule"`
	Severity  string `json:"severity"`
	Match     string `json:"match"`
	Context   string `json:"context,omitempty"`
	Note      string `json:"note,omitempty"`
}

// recorder serializa a escrita do manifesto JSONL e da linha de console,
// conta os achados por severidade e guarda o que foi varrido e o que falhou
// para o resumo final.
type recorder struct {
	mu      sync.Mutex
	jsonl   io.Writer
	console io.Writer
	quiet   bool

	porSeveridade map[string]int
	fontes        int
	erros         int
}

func newRecorder(jsonl, console io.Writer, quiet bool) *recorder {
	return &recorder{
		jsonl:         jsonl,
		console:       console,
		quiet:         quiet,
		porSeveridade: map[string]int{},
	}
}

// record persiste o achado no JSONL, atualiza o contador da severidade e,
// salvo quiet, imprime um resumo de uma linha.
func (r *recorder) record(a achado) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if a.Timestamp == "" {
		a.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if line, err := json.Marshal(a); err == nil {
		r.jsonl.Write(line)
		r.jsonl.Write([]byte("\n"))
	}
	r.porSeveridade[a.Severity]++

	if !r.quiet {
		fmt.Fprintln(r.console, linhaConsole(a))
	}
}

// fonteVarrida conta uma fonte (arquivo ou URL) lida por inteiro.
func (r *recorder) fonteVarrida() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fontes++
}

// erro registra uma fonte que não deu pra ler. Vai sempre ao console, mesmo
// com --quiet: silenciar falha faria parecer que a fonte estava limpa.
func (r *recorder) erro(source string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.erros++
	fmt.Fprintf(r.console, "ERRO  %s  (%v)\n", source, err)
}

// resumo devolve os contadores finais (a cópia do mapa evita que o chamador
// leia enquanto outro record escreve).
func (r *recorder) resumo() (porSeveridade map[string]int, fontes, erros int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make(map[string]int, len(r.porSeveridade))
	for k, v := range r.porSeveridade {
		cp[k] = v
	}
	return cp, r.fontes, r.erros
}

// linhaConsole formata um achado para acompanhamento ao vivo.
func linhaConsole(a achado) string {
	local := a.Source
	if a.Line > 0 {
		local = fmt.Sprintf("%s:%d", a.Source, a.Line)
	}
	return fmt.Sprintf("%-8s %-22s %s  %s  | %s",
		strings.ToUpper(a.Severity), a.Rule, local, a.Match, a.Context)
}
