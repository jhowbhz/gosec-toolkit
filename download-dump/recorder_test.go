package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRecorderGravaJSONLeConta(t *testing.T) {
	var jsonl, console bytes.Buffer
	rec := newRecorder(&jsonl, &console, true)

	rec.record(registro{URL: "https://site.com/", Status: 200, SavedPath: "site.com/index.html"})
	rec.record(registro{URL: "https://site.com/x.bin", Status: 200, Skipped: true, SkipReason: "maior que --max-body"})
	rec.record(registro{URL: "https://site.com/falha", Error: "connection refused"})

	linhas := strings.Split(strings.TrimSpace(jsonl.String()), "\n")
	if len(linhas) != 3 {
		t.Fatalf("esperava 3 linhas JSONL, tenho %d", len(linhas))
	}
	var primeira registro
	if err := json.Unmarshal([]byte(linhas[0]), &primeira); err != nil {
		t.Fatalf("linha 0 não é JSON válido: %v", err)
	}
	if primeira.URL != "https://site.com/" || primeira.Status != 200 {
		t.Errorf("linha 0 inesperada: %+v", primeira)
	}

	baixados, pulados, erros, sensiveis := rec.resumo()
	if baixados != 1 || pulados != 1 || erros != 1 || sensiveis != 0 {
		t.Errorf("resumo = baixados=%d pulados=%d erros=%d sensiveis=%d; quero 1/1/1/0",
			baixados, pulados, erros, sensiveis)
	}
}

func TestRecorderColetaSensiveis(t *testing.T) {
	var jsonl, console bytes.Buffer
	rec := newRecorder(&jsonl, &console, true)

	rec.record(registro{URL: "https://site.com/.env", Status: 200, Sensitive: true, Skipped: true, SensitiveNote: "arquivo de ambiente (.env)"})
	rec.record(registro{URL: "https://site.com/index.html", Status: 200, SavedPath: "site.com/index.html"})

	sens := rec.relatorioSensiveis()
	if len(sens) != 1 {
		t.Fatalf("esperava 1 sensível coletado, tenho %d", len(sens))
	}
	if sens[0].URL != "https://site.com/.env" {
		t.Errorf("sensível coletado errado: %+v", sens[0])
	}
	if sens[0].SavedPath != "" {
		t.Error("sensível NÃO pode ter saved_path (não se baixa o conteúdo)")
	}

	_, _, _, sensiveis := rec.resumo()
	if sensiveis != 1 {
		t.Errorf("contador de sensíveis = %d, quero 1", sensiveis)
	}
}
