package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestConsolidarDedupEOrdena(t *testing.T) {
	as := []achado{
		{Detector: "jwt", Classe: classeSecreto, Severidade: sevCritico, Valor: "AAA", Local: "a.js", Linha: 1},
		{Detector: "jwt", Classe: classeSecreto, Severidade: sevCritico, Valor: "AAA", Local: "b.js", Linha: 5},
		{Detector: "supabase-publishable", Classe: classePublicavel, Severidade: sevInfo, Valor: "sb_publishable_xxxxxxxxxx", Local: "a.js", Linha: 2},
	}
	c := consolidar(as, false)
	if len(c) != 2 {
		t.Fatalf("esperava 2 após dedup, veio %d", len(c))
	}
	if c[0].Classe != classeSecreto {
		t.Errorf("secreto deveria vir primeiro na ordenação: %+v", c[0])
	}
	if c[0].Ocorrencias != 2 {
		t.Errorf("ocorrencias = %d, quero 2", c[0].Ocorrencias)
	}
	if len(c[0].Locais) != 2 {
		t.Errorf("locais = %v, quero 2", c[0].Locais)
	}
	if c[0].Preview == "AAA" {
		t.Error("preview deveria estar mascarado, não o valor cru")
	}
}

func TestExitCodeDoisSeSecreto(t *testing.T) {
	if got := exitCode([]achado{{Classe: classePublicavel}}); got != 0 {
		t.Errorf("sem secreto deveria sair 0, veio %d", got)
	}
	if got := exitCode([]achado{{Classe: classePublicavel}, {Classe: classeSecreto}}); got != 2 {
		t.Errorf("com secreto deveria sair 2, veio %d", got)
	}
}

func TestEscreverJSONLValido(t *testing.T) {
	var b bytes.Buffer
	escreverJSONL(&b, []achado{{Detector: "jwt", Preview: "eyJ…FAw", Local: "a.js", Linha: 1, Classe: classeSecreto, Severidade: sevCritico}})
	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(b.Bytes()), &m); err != nil {
		t.Fatalf("linha não é JSON válido: %v", err)
	}
	if m["detector"] != "jwt" {
		t.Errorf("campo detector errado: %v", m["detector"])
	}
	if _, ok := m["preview"]; !ok {
		t.Error("JSONL deveria ter preview")
	}
}
