package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// registro é uma linha do manifesto: o que aconteceu com uma URL durante o
// crawl. É a evidência do teste, então guarda status, hash, de onde o link
// veio e por que algo foi pulado. O conteúdo em si vai pro espelho; aqui só
// metadados. Arquivo sensível vira registro com Sensitive=true e sem
// SavedPath — a política é avisar, não baixar.
type registro struct {
	Timestamp      string `json:"ts,omitempty"`
	URL            string `json:"url"`
	Status         int    `json:"status"`
	ContentType    string `json:"content_type,omitempty"`
	Size           int64  `json:"size,omitempty"`
	SHA256         string `json:"sha256,omitempty"`
	SavedPath      string `json:"saved_path,omitempty"`
	DiscoveredFrom string `json:"from,omitempty"`
	Depth          int    `json:"depth,omitempty"`
	Skipped        bool   `json:"skipped,omitempty"`
	SkipReason     string `json:"skip_reason,omitempty"`
	Sensitive      bool   `json:"sensitive,omitempty"`
	SensitiveNote  string `json:"sensitive_note,omitempty"`
	Error          string `json:"error,omitempty"`
	DurationMS     int64  `json:"duration_ms,omitempty"`
}

// recorder serializa a escrita do manifesto JSONL e do resumo no console
// (chegam de vários workers), conta o que passou e junta os sensíveis para
// o relatório final.
type recorder struct {
	mu       sync.Mutex
	jsonl    io.Writer
	console  io.Writer
	quiet    bool
	sensveis []registro

	baixados int
	pulados  int
	erros    int
}

func newRecorder(jsonl, console io.Writer, quiet bool) *recorder {
	return &recorder{jsonl: jsonl, console: console, quiet: quiet}
}

// record persiste o registro no JSONL, atualiza contadores e, salvo quiet,
// imprime um resumo de uma linha.
func (r *recorder) record(reg registro) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if line, err := json.Marshal(reg); err == nil {
		r.jsonl.Write(line)
		r.jsonl.Write([]byte("\n"))
	}

	switch {
	case reg.Error != "":
		r.erros++
	case reg.Sensitive:
		r.sensveis = append(r.sensveis, reg)
	case reg.Skipped:
		r.pulados++
	default:
		r.baixados++
	}

	if !r.quiet {
		fmt.Fprintln(r.console, linhaConsole(reg))
	}
}

// relatorioSensiveis devolve os registros sensíveis coletados (cópia).
func (r *recorder) relatorioSensiveis() []registro {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]registro, len(r.sensveis))
	copy(out, r.sensveis)
	return out
}

// resumo devolve os contadores finais.
func (r *recorder) resumo() (baixados, pulados, erros, sensiveis int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.baixados, r.pulados, r.erros, len(r.sensveis)
}

// linhaConsole formata o resumo de uma troca para acompanhamento ao vivo.
func linhaConsole(reg registro) string {
	switch {
	case reg.Error != "":
		return fmt.Sprintf("ERRO  %s  (%s)", reg.URL, reg.Error)
	case reg.Sensitive:
		return fmt.Sprintf("SENS  %s  [%d]  <%s>  (não baixado)", reg.URL, reg.Status, reg.SensitiveNote)
	case reg.Skipped:
		return fmt.Sprintf("PULA  %s  (%s)", reg.URL, reg.SkipReason)
	default:
		return fmt.Sprintf("%-4d  %s  -> %s", reg.Status, reg.URL, reg.SavedPath)
	}
}
