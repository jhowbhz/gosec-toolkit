package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

func stubServidor(t *testing.T) (http.Handler, string) {
	t.Helper()
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	st := novoStore(t.TempDir())
	reg := []ferramenta{{ID: "stub", Nome: "Stub", Campos: []campo{{Flag: "x", Tipo: "texto"}}}}
	rn := novoRunner(st, func(f ferramenta) (string, []string, error) {
		return os.Args[0], []string{"-test.run=TestHelperProcess", "--", "linhas"}, nil
	})
	ui := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>painel</html>")}}
	return novoServidor("segredo", reg, rn, st, ui), "segredo"
}

func TestAuthBloqueiaSemToken(t *testing.T) {
	h, _ := stubServidor(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/tools", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("sem token deveria dar 401, veio %d", rec.Code)
	}
}

func TestAuthAceitaComHeaderEListaTools(t *testing.T) {
	h, tok := stubServidor(t)
	req := httptest.NewRequest("GET", "/api/tools", nil)
	req.Header.Set("X-Painel-Token", tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("com token deveria dar 200, veio %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "stub") {
		t.Errorf("deveria listar a ferramenta stub: %s", rec.Body.String())
	}
}

func TestRodarEhReceberStream(t *testing.T) {
	h, tok := stubServidor(t)

	req := httptest.NewRequest("POST", "/api/run", strings.NewReader(`{"tool":"stub","args":{"x":["v"]}}`))
	req.Header.Set("X-Painel-Token", tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("POST /api/run deveria dar 200, veio %d (%s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.ID == "" {
		t.Fatalf("resposta do run sem id: %v (%s)", err, rec.Body.String())
	}

	sreq := httptest.NewRequest("GET", "/api/runs/"+resp.ID+"/stream?token="+tok, nil)
	srec := httptest.NewRecorder()
	h.ServeHTTP(srec, sreq)
	if !strings.Contains(srec.Body.String(), "linha stdout") {
		t.Errorf("stream deveria conter a saída do processo: %q", srec.Body.String())
	}
}

func TestStreamExigeTokenNaQuery(t *testing.T) {
	h, _ := stubServidor(t)
	// sem token na query → 401
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/runs/xyz/stream", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("stream sem token deveria dar 401, veio %d", rec.Code)
	}
}
