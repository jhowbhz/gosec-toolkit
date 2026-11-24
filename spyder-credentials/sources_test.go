package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestEhProvavelTexto(t *testing.T) {
	if !ehProvavelTexto([]byte("const x = 'eyJ...';")) {
		t.Error("texto comum deveria ser considerado texto")
	}
	if ehProvavelTexto([]byte{0x89, 0x50, 0x4e, 0x47, 0x00, 0x01, 0x02}) {
		t.Error("conteúdo com NUL deveria ser considerado binário")
	}
}

func TestLerArquivosDeIgnoraBinario(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.js"), []byte(`token="abc"`), 0o644)
	os.MkdirAll(filepath.Join(dir, "img"), 0o755)
	os.WriteFile(filepath.Join(dir, "img", "logo.png"), []byte{0x89, 0x50, 0x00, 0x01}, 0o644)

	fs, err := lerArquivosDe([]string{dir})
	if err != nil {
		t.Fatalf("lerArquivosDe: %v", err)
	}
	if len(fs) != 1 {
		t.Fatalf("esperava só 1 arquivo de texto, veio %d: %+v", len(fs), fs)
	}
	if filepath.Base(fs[0].nome) != "app.js" {
		t.Errorf("deveria ser app.js, veio %s", fs[0].nome)
	}
}

func TestFetchURLComHeader(t *testing.T) {
	var visto string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		visto = r.Header.Get("Authorization")
		w.Write([]byte(`secret="eyJabc"`))
	}))
	defer srv.Close()

	h := http.Header{}
	h.Set("Authorization", "Bearer TESTE")
	f, err := fetchURL(srv.Client(), srv.URL+"/app.js", h, "spyder-test")
	if err != nil {
		t.Fatalf("fetchURL: %v", err)
	}
	if string(f.dados) != `secret="eyJabc"` {
		t.Errorf("corpo inesperado: %q", f.dados)
	}
	if visto != "Bearer TESTE" {
		t.Errorf("header Authorization não propagado: %q", visto)
	}
}
