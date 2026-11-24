package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTipoDoScan(t *testing.T) {
	casos := map[string]string{
		"a.jsonl":     tipoJSONL,
		"dir/B.JSONL": tipoJSONL,
		"spyder-1.db": tipoDB,
		"spyder-1.DB": tipoDB,
	}
	for path, quero := range casos {
		got, err := tipoDoScan(path)
		if err != nil || got != quero {
			t.Errorf("tipoDoScan(%q) = %q, %v; quero %q", path, got, err, quero)
		}
	}
	for _, ruim := range []string{"a.csv", "semextensao", "a.jsonl.bak"} {
		if _, err := tipoDoScan(ruim); err == nil {
			t.Errorf("tipoDoScan(%q) deveria recusar", ruim)
		}
	}
}

func TestCarregarJSONLNormalizaEPulaSemConteudo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scan.jsonl")
	conteudo := strings.Join([]string{
		`{"url":"https://x.com/a.js","status":200,"sha256":"abc","size":10}`,
		``,
		`{"url":"https://x.com/pulado","status":200,"skipped":true,"skip_reason":"robots"}`,
		`{"url":"https://x.com/.env","status":200,"sensitive":true}`,
		`{"url":"https://x.com/quebrou","error":"timeout"}`,
		`{"url":"https://x.com/b","status":404}`,
	}, "\n")
	if err := os.WriteFile(path, []byte(conteudo), 0o600); err != nil {
		t.Fatal(err)
	}

	m, err := carregarJSONL(path)
	if err != nil {
		t.Fatalf("carregarJSONL: %v", err)
	}
	if len(m) != 2 {
		t.Fatalf("quero 2 entradas (skipped/sensitive/error ficam de fora), veio %d: %+v", len(m), m)
	}
	a := m["https://x.com/a.js"]
	if a.Status != 200 || a.Hash != "abc" || a.URL != "https://x.com/a.js" || a.Method != "" {
		t.Errorf("a.js normalizado errado: %+v", a)
	}
	if b := m["https://x.com/b"]; b.Status != 404 || b.Hash != "" {
		t.Errorf("b normalizado errado: %+v", b)
	}
}

func TestCarregarJSONLLinhaInvalidaCitaNumero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ruim.jsonl")
	if err := os.WriteFile(path, []byte("{\"url\":\"/a\",\"status\":200}\nisto não é json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := carregarJSONL(path)
	if err == nil || !strings.Contains(err.Error(), "linha 2") {
		t.Fatalf("erro deveria citar a linha 2: %v", err)
	}
}

func TestCarregarArquivoAusenteEhErroDeRuntime(t *testing.T) {
	dir := t.TempDir()
	for _, nome := range []string{"nao-existe.jsonl", "nao-existe.db"} {
		path := filepath.Join(dir, nome)
		if _, err := carregar(path); err == nil {
			t.Errorf("carregar(%s) deveria falhar", nome)
		}
	}
	// o driver sqlite criaria o arquivo; não pode sobrar banco fantasma
	if _, err := os.Stat(filepath.Join(dir, "nao-existe.db")); err == nil {
		t.Error("carregarDB criou um banco que não existia")
	}
}

func TestCarregarDBChaveUsaMetodo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spyder.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE routes (url TEXT NOT NULL, host TEXT, method TEXT NOT NULL DEFAULT 'GET',
		status INTEGER, kind TEXT, source_url TEXT, depth INTEGER, title TEXT, first_seen TEXT,
		PRIMARY KEY (url, method));
		INSERT INTO routes (url, method, status, kind) VALUES ('https://x.com/api', 'GET', 200, 'xhr');
		INSERT INTO routes (url, method, status, kind) VALUES ('https://x.com/api', 'POST', NULL, NULL);`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	m, err := carregarDB(path)
	if err != nil {
		t.Fatalf("carregarDB: %v", err)
	}
	if len(m) != 2 {
		t.Fatalf("GET e POST da mesma URL são duas rotas, veio %d: %+v", len(m), m)
	}
	get := m["https://x.com/api\tGET"]
	if get.Status != 200 || get.Kind != "xhr" || get.Method != "GET" {
		t.Errorf("GET normalizado errado: %+v", get)
	}
	if post := m["https://x.com/api\tPOST"]; post.Status != 0 || post.Kind != "" {
		t.Errorf("NULL deveria virar zero-value: %+v", post)
	}
}
