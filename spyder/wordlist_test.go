package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLerPathsIgnoraComentarioEVazio(t *testing.T) {
	entrada := "# comentário\nadmin\n\n  login  \n#outra\napi/conta\n"
	got := lerPaths(strings.NewReader(entrada))
	want := []string{"admin", "login", "api/conta"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("lerPaths = %v, quero %v", got, want)
	}
}

func TestMesclarPathsDedupPreservandoOrdem(t *testing.T) {
	got := mesclarPaths([]string{"admin", "login"}, []string{"login", "api", "admin"})
	want := []string{"admin", "login", "api"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("mesclarPaths = %v, quero %v", got, want)
	}
}

func TestEmbutidaTemCaminhos(t *testing.T) {
	ps := lerPaths(strings.NewReader(embeddedCommon))
	if len(ps) < 10 {
		t.Errorf("wordlist embutida muito curta: %d", len(ps))
	}
}

func TestBaixarSeFaltarUsaCacheNaSegundaVez(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte("admin\nlogin\n"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "lista.txt")
	cli := srv.Client()

	if err := baixarSeFaltar(cli, srv.URL, dest); err != nil {
		t.Fatalf("baixar 1: %v", err)
	}
	if err := baixarSeFaltar(cli, srv.URL, dest); err != nil {
		t.Fatalf("baixar 2: %v", err)
	}
	if hits != 1 {
		t.Errorf("baixou %d vezes, quero 1 (segunda usa cache)", hits)
	}
	b, _ := os.ReadFile(dest)
	if !strings.Contains(string(b), "admin") {
		t.Errorf("arquivo baixado sem conteúdo esperado: %q", string(b))
	}
}
