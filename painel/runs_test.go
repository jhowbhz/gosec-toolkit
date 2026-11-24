package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCriarEObterRun(t *testing.T) {
	s := novoStore(t.TempDir())
	r, err := s.criarRun("download-dump", map[string][]string{"url": {"https://x"}}, []string{"--url=https://x"})
	if err != nil {
		t.Fatalf("criarRun: %v", err)
	}
	if r.ID == "" || r.Status != "rodando" {
		t.Errorf("run inicial inesperado: %+v", r)
	}
	lido, err := s.obter(r.ID)
	if err != nil {
		t.Fatalf("obter: %v", err)
	}
	if lido.Tool != "download-dump" || len(lido.Argv) != 1 {
		t.Errorf("meta relido errado: %+v", lido)
	}
}

func TestCaminhoArtefatoBloqueiaTraversal(t *testing.T) {
	s := novoStore(t.TempDir())
	r, _ := s.criarRun("t", nil, nil)
	// cria um arquivo legítimo
	os.WriteFile(filepath.Join(s.dirDoRun(r.ID), "saida.jsonl"), []byte("{}"), 0o644)

	if _, err := s.caminhoArtefato(r.ID, "saida.jsonl"); err != nil {
		t.Errorf("arquivo legítimo deveria ser aceito: %v", err)
	}
	for _, mau := range []string{"../../etc/passwd", "..\\..\\x", "/etc/passwd"} {
		if _, err := s.caminhoArtefato(r.ID, mau); err == nil {
			t.Errorf("caminho %q deveria ser rejeitado", mau)
		}
	}
}

func TestListarArtefatos(t *testing.T) {
	s := novoStore(t.TempDir())
	r, _ := s.criarRun("t", nil, nil)
	dir := s.dirDoRun(r.ID)
	os.WriteFile(filepath.Join(dir, "a.jsonl"), []byte("{}"), 0o644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("x"), 0o644)

	arts := s.artefatos(r.ID)
	if len(arts) < 2 {
		t.Errorf("esperava ao menos 2 artefatos (a.jsonl, sub/b.txt), veio %v", arts)
	}
}
