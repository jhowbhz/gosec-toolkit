package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalPathCasos(t *testing.T) {
	casos := []struct {
		url, ct, quer string
	}{
		{"https://site.com/", "text/html", "site.com/index.html"},
		{"https://site.com", "text/html", "site.com/index.html"},
		{"https://site.com/a/b.html", "text/html", "site.com/a/b.html"},
		{"https://site.com/a/", "text/html", "site.com/a/index.html"},
		{"https://site.com/about", "text/html", "site.com/about.html"},
		{"https://site.com/img/p.png", "image/png", "site.com/img/p.png"},
		{"https://site.com:8080/x.css", "text/css", "site.com_8080/x.css"},
	}
	for _, c := range casos {
		got := localPath(mustURL(t, c.url), c.ct)
		if got != c.quer {
			t.Errorf("localPath(%q) = %q, quero %q", c.url, got, c.quer)
		}
	}
}

func TestLocalPathQueryViraParteDoNome(t *testing.T) {
	a := localPath(mustURL(t, "https://site.com/app.js?v=1"), "application/javascript")
	b := localPath(mustURL(t, "https://site.com/app.js?v=2"), "application/javascript")
	if a == b {
		t.Fatalf("query diferente deveria gerar nome diferente: %q == %q", a, b)
	}
	if !strings.HasSuffix(a, ".js") || !strings.HasSuffix(b, ".js") {
		t.Errorf("deveria preservar a extensão .js: %q / %q", a, b)
	}
}

func TestLocalPathSanitizaTraversalEIlegais(t *testing.T) {
	got := localPath(mustURL(t, "https://site.com/..%2f..%2fetc/pa*ss?x"), "text/html")
	if strings.Contains(got, "..") {
		t.Errorf("não pode sobrar '..' no caminho: %q", got)
	}
	if strings.ContainsAny(got, `*?`) {
		t.Errorf("não pode sobrar char ilegal do Windows: %q", got)
	}
}

func TestSalvarCriaDiretorioEGrava(t *testing.T) {
	root := t.TempDir()
	full, err := salvar(root, "site.com/a/b.html", []byte("oi"))
	if err != nil {
		t.Fatalf("salvar: %v", err)
	}
	if filepath.Dir(full) == root {
		t.Error("deveria ter criado subdiretório")
	}
	data, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "oi" {
		t.Errorf("conteúdo = %q, quero oi", data)
	}
}
