package main

import (
	"path/filepath"
	"testing"
)

func abrirStoreTeste(t *testing.T) *store {
	t.Helper()
	s, err := openStore(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("openStore: %v", err)
	}
	t.Cleanup(func() { s.close() })
	return s
}

func TestStoreSalvaNovaRota(t *testing.T) {
	s := abrirStoreTeste(t)
	novo, err := s.salvar(route{URL: "https://x/a", Method: "GET", Kind: kindPage})
	if err != nil {
		t.Fatalf("salvar: %v", err)
	}
	if !novo {
		t.Error("primeira vez deveria contar como nova")
	}
	rotas, err := s.listar()
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	if len(rotas) != 1 || rotas[0].URL != "https://x/a" {
		t.Errorf("listar = %+v, quero 1 rota https://x/a", rotas)
	}
}

func TestStoreDedupPorUrlEMetodo(t *testing.T) {
	s := abrirStoreTeste(t)
	s.salvar(route{URL: "https://x/a", Method: "GET"})
	denovo, _ := s.salvar(route{URL: "https://x/a", Method: "GET"})
	if denovo {
		t.Error("mesma url+método deveria ser dedupada (não contar como nova)")
	}
	outroMetodo, _ := s.salvar(route{URL: "https://x/a", Method: "POST"})
	if !outroMetodo {
		t.Error("url igual mas método diferente é outra rota")
	}
	rotas, _ := s.listar()
	if len(rotas) != 2 {
		t.Errorf("quero 2 rotas (GET e POST), veio %d", len(rotas))
	}
}
