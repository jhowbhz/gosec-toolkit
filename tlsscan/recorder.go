package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

// certInfo é o que importa de um certificado para o relatório.
type certInfo struct {
	Subject      string   `json:"subject"`
	SANs         []string `json:"sans,omitempty"`
	Emissor      string   `json:"emissor"`
	NotBefore    string   `json:"not_before"`
	NotAfter     string   `json:"not_after"`
	DiasParaExp  int      `json:"dias_para_expirar"`
	Assinatura   string   `json:"assinatura"`
	Chave        string   `json:"chave"`
	AutoAssinado bool     `json:"auto_assinado"`
}

// resultadoHost é uma linha do JSONL: tudo o que se descobriu de um alvo.
// Ciphers é indexado pelo nome da versão ("TLS 1.2") porque o crypto/tls só
// revela o cipher negociado, não a lista que o servidor suporta.
type resultadoHost struct {
	Timestamp     string            `json:"ts"`
	Host          string            `json:"host"`
	Versoes       []string          `json:"versoes_aceitas"`
	Ciphers       map[string]string `json:"ciphers"`
	Certs         []certInfo        `json:"certs"`
	VerificacaoOK bool              `json:"verificacao_ok"`
	VerificacaoEr string            `json:"verificacao_erro,omitempty"`
	HSTS          string            `json:"hsts"`
	HSTSErro      string            `json:"hsts_erro,omitempty"`
	Grade         string            `json:"grade"`
	Issues        []issue           `json:"issues"`
	Erro          string            `json:"erro,omitempty"`
}

// recorder grava o JSONL e o resumo no console e conta o que passou.
// O mutex existe para o escaneamento poder ser paralelizado sem mexer aqui.
type recorder struct {
	mu      sync.Mutex
	jsonl   io.Writer
	console io.Writer
	quiet   bool

	hosts    int
	falhas   int
	criticas int
	altas    int
}

func newRecorder(jsonl, console io.Writer, quiet bool) *recorder {
	return &recorder{jsonl: jsonl, console: console, quiet: quiet}
}

// record persiste o resultado, atualiza contadores e, salvo quiet, imprime.
func (r *recorder) record(res resultadoHost) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if line, err := json.Marshal(res); err == nil {
		r.jsonl.Write(line)
		r.jsonl.Write([]byte("\n"))
	}

	r.hosts++
	if res.Erro != "" {
		r.falhas++
	}
	for _, i := range res.Issues {
		switch i.Severity {
		case sevCritical:
			r.criticas++
		case sevHigh:
			r.altas++
		}
	}

	if !r.quiet {
		fmt.Fprint(r.console, blocoConsole(res))
	}
}

// resumo devolve os contadores finais.
func (r *recorder) resumo() (hosts, falhas, criticas, altas int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hosts, r.falhas, r.criticas, r.altas
}

// blocoConsole formata um host para acompanhamento ao vivo.
func blocoConsole(res resultadoHost) string {
	if res.Erro != "" {
		return fmt.Sprintf("ERRO  %s  (%s)\n", res.Host, res.Erro)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s  nota %s  versões: %s\n", res.Host, res.Grade, strings.Join(res.Versoes, ", "))
	for _, c := range res.Certs {
		fmt.Fprintf(&b, "  cert %s  emissor: %s  expira em %d dias (%s)  %s / %s\n",
			c.Subject, c.Emissor, c.DiasParaExp, c.NotAfter, c.Assinatura, c.Chave)
	}
	for _, i := range res.Issues {
		fmt.Fprintf(&b, "  [%s] %s: %s\n", i.Severity, i.Check, i.Detalhe)
	}
	return b.String()
}
