package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestRecordEscreveLinhaJSONL(t *testing.T) {
	var jsonl, console bytes.Buffer
	rec := newRecorder(&jsonl, &console, false)

	rec.record(exchange{
		ID:         "abc123",
		Method:     "POST",
		URL:        "https://meusite.com.br/api/conta",
		Status:     200,
		ReqBody:    `{"acao":"saldo"}`,
		RespBody:   `{"ok":true}`,
		DurationMS: 42,
		ReqHeaders: http.Header{"Authorization": {"Bearer x"}},
	})

	line := strings.TrimSpace(jsonl.String())
	if strings.Count(line, "\n") != 0 {
		t.Fatalf("quero uma linha só, veio: %q", line)
	}
	var got exchange
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("linha não é JSON válido: %v\n%s", err, line)
	}
	// O que dói se regredir: perder a evidência do pentest (corpo/headers).
	if got.Method != "POST" || got.Status != 200 || got.ReqBody != `{"acao":"saldo"}` || got.RespBody != `{"ok":true}` {
		t.Errorf("exchange round-trip divergiu: %+v", got)
	}
	if got.ReqHeaders.Get("Authorization") != "Bearer x" {
		t.Errorf("headers de request não preservados: %+v", got.ReqHeaders)
	}
}

func TestRecordConsoleResumeRequest(t *testing.T) {
	var jsonl, console bytes.Buffer
	rec := newRecorder(&jsonl, &console, false)
	rec.record(exchange{Method: "GET", URL: "https://x/api", Status: 403, DurationMS: 12})

	out := console.String()
	for _, want := range []string{"GET", "403"} {
		if !strings.Contains(out, want) {
			t.Errorf("resumo do console não tem %q: %q", want, out)
		}
	}
}

func TestRecordQuietNaoImprimeNoConsole(t *testing.T) {
	var jsonl, console bytes.Buffer
	rec := newRecorder(&jsonl, &console, true)
	rec.record(exchange{Method: "GET", URL: "https://x/api", Status: 200})

	if console.Len() != 0 {
		t.Errorf("quiet deveria silenciar o console, veio: %q", console.String())
	}
	if jsonl.Len() == 0 {
		t.Error("quiet não deve silenciar o JSONL")
	}
}

func TestBodyForLogTrunca(t *testing.T) {
	full := []byte("0123456789")
	s, truncated := bodyForLog(full, 4)
	if s != "0123" || !truncated {
		t.Errorf("bodyForLog(10 bytes, max 4) = %q,%v; quero \"0123\",true", s, truncated)
	}

	s, truncated = bodyForLog(full, 100)
	if s != "0123456789" || truncated {
		t.Errorf("bodyForLog sem estourar deveria vir inteiro: %q,%v", s, truncated)
	}
}
