package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

// registro é uma linha do JSONL: o resultado da auditoria de um token. Não
// guarda o token inteiro — ele é uma credencial viva, e o relatório costuma
// ser anexado a tickets. TokenID (prefixo do sha256) basta para correlacionar.
// WeakSecret é a exceção deliberada: é o achado que o usuário veio buscar.
type registro struct {
	Timestamp  string  `json:"ts"`
	TokenID    string  `json:"token_id"`
	Alg        string  `json:"alg,omitempty"`
	Kid        string  `json:"kid,omitempty"`
	Iss        string  `json:"iss,omitempty"`
	Sub        string  `json:"sub,omitempty"`
	Aud        any     `json:"aud,omitempty"`
	Exp        string  `json:"exp,omitempty"`
	Iat        string  `json:"iat,omitempty"`
	Nbf        string  `json:"nbf,omitempty"`
	Validade   string  `json:"validade,omitempty"`
	Issues     []issue `json:"issues"`
	WeakSecret string  `json:"weak_secret,omitempty"`
	Error      string  `json:"error,omitempty"`
}

// recorder serializa a escrita do JSONL e do console, conta o que passou e
// guarda a pior severidade para o resumo final.
type recorder struct {
	mu      sync.Mutex
	jsonl   io.Writer
	console io.Writer
	quiet   bool

	total         int
	erros         int
	fracos        int
	porSeveridade map[string]int
}

func newRecorder(jsonl, console io.Writer, quiet bool) *recorder {
	return &recorder{jsonl: jsonl, console: console, quiet: quiet, porSeveridade: map[string]int{}}
}

// record persiste o registro, atualiza contadores e, salvo quiet, imprime.
func (r *recorder) record(reg registro) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if line, err := json.Marshal(reg); err == nil {
		r.jsonl.Write(line)
		r.jsonl.Write([]byte("\n"))
	}

	r.total++
	if reg.Error != "" {
		r.erros++
	}
	if reg.WeakSecret != "" {
		r.fracos++
	}
	for _, i := range reg.Issues {
		r.porSeveridade[i.Severity]++
	}

	if !r.quiet {
		fmt.Fprintln(r.console, linhaConsole(reg))
	}
}

// resumo devolve os contadores finais.
func (r *recorder) resumo() (total, erros, fracos int, porSeveridade map[string]int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copia := make(map[string]int, len(r.porSeveridade))
	for k, v := range r.porSeveridade {
		copia[k] = v
	}
	return r.total, r.erros, r.fracos, copia
}

// linhaConsole formata o resultado de um token para acompanhamento ao vivo.
func linhaConsole(reg registro) string {
	if reg.Error != "" {
		return fmt.Sprintf("ERRO  %s  (%s)", reg.TokenID, reg.Error)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s  alg=%s  kid=%s  sub=%s  validade=%s", reg.TokenID, reg.Alg, reg.Kid, reg.Sub, reg.Validade)
	for _, i := range reg.Issues {
		fmt.Fprintf(&b, "\n    [%s] %s: %s", i.Severity, i.Check, i.Detalhe)
	}
	if reg.WeakSecret != "" {
		fmt.Fprintf(&b, "\n    [critical] SEGREDO FRACO: '%s' valida a assinatura", reg.WeakSecret)
	}
	return b.String()
}
