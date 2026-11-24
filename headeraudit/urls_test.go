package main

import (
	"strings"
	"testing"
)

func TestLerURLsDeLinhasIgnoraComentarioEVazio(t *testing.T) {
	entrada := "# alvos\nhttps://a.com/x\n\n  http://b.com  \n"
	got, err := lerURLsDeLinhas(strings.NewReader(entrada))
	if err != nil {
		t.Fatalf("lerURLsDeLinhas: %v", err)
	}
	if len(got) != 2 || got[0] != "https://a.com/x" || got[1] != "http://b.com" {
		t.Errorf("got = %v", got)
	}
}

func TestLerURLsDeLinhasAbortaEmURLSemScheme(t *testing.T) {
	_, err := lerURLsDeLinhas(strings.NewReader("https://a.com\nb.com\n"))
	if err == nil || !strings.Contains(err.Error(), "linha 2") {
		t.Errorf("esperava erro citando a linha 2, veio %v", err)
	}
}

func TestLerURLsDoManifestoPulaErrosESemStatus(t *testing.T) {
	entrada := `{"url":"https://a.com/","status":200}
{"url":"https://a.com/quebrada","status":0,"error":"timeout"}
{"url":"https://a.com/pulada","skipped":true}
{"url":"https://a.com/app.js","status":200}
`
	got, err := lerURLsDoManifesto(strings.NewReader(entrada))
	if err != nil {
		t.Fatalf("lerURLsDoManifesto: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("quero 2 URLs (a.com/ e app.js), veio %v", got)
	}
}
