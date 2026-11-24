package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// exchange é uma troca completa (request + response) que passou pelo proxy.
// É o registro que vira evidência do teste, então guarda headers e corpo
// dos dois lados; omitempty mantém a linha JSONL enxuta quando não há valor.
type exchange struct {
	Timestamp         string      `json:"ts"`
	ID                string      `json:"id"`
	Method            string      `json:"method"`
	URL               string      `json:"url"`
	ReqHeaders        http.Header `json:"req_headers,omitempty"`
	ReqBody           string      `json:"req_body,omitempty"`
	ReqBodyTruncated  bool        `json:"req_body_truncated,omitempty"`
	Status            int         `json:"status"`
	RespHeaders       http.Header `json:"resp_headers,omitempty"`
	RespBody          string      `json:"resp_body,omitempty"`
	RespBodyTruncated bool        `json:"resp_body_truncated,omitempty"`
	DurationMS        int64       `json:"duration_ms"`
	Error             string      `json:"error,omitempty"`
}

// recorder grava cada troca em dois destinos: o JSONL completo (a evidência)
// e um resumo de uma linha no console (o acompanhamento ao vivo). Os dois
// lados chegam concorrentes, então a escrita é serializada por um mutex.
type recorder struct {
	mu      sync.Mutex
	jsonl   io.Writer
	console io.Writer
	quiet   bool
}

func newRecorder(jsonl, console io.Writer, quiet bool) *recorder {
	return &recorder{jsonl: jsonl, console: console, quiet: quiet}
}

// record persiste a troca inteira no JSONL e, salvo quiet, imprime o resumo.
func (r *recorder) record(ex exchange) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if line, err := json.Marshal(ex); err == nil {
		r.jsonl.Write(line)
		r.jsonl.Write([]byte("\n"))
	}
	if r.quiet {
		return
	}
	status := fmt.Sprintf("%d", ex.Status)
	if ex.Error != "" {
		status = "ERRO:" + ex.Error
	}
	fmt.Fprintf(r.console, "%-6s %s -> %s (%dms)\n", ex.Method, ex.URL, status, ex.DurationMS)
}

// bodyForLog devolve o corpo a registrar, cortado no teto de bytes. O proxy
// repassa o corpo inteiro; só o que vai pro log é limitado, pra não estourar
// com upload grande ou stream binário.
func bodyForLog(full []byte, max int) (string, bool) {
	if max >= 0 && len(full) > max {
		return string(full[:max]), true
	}
	return string(full), false
}

// newID gera um identificador curto pra correlacionar request e response
// da mesma troca quando o log é lido depois.
func newID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "000000000000"
	}
	return hex.EncodeToString(b[:])
}
