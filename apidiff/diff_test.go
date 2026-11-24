package main

import "testing"

func mapa(es ...entrada) map[string]entrada {
	m := map[string]entrada{}
	for _, e := range es {
		m[e.Key] = e
	}
	return m
}

func cenario() (velho, novo map[string]entrada) {
	velho = mapa(
		entrada{Key: "/igual", URL: "/igual", Status: 200, Hash: "h1"},
		entrada{Key: "/some", URL: "/some", Status: 200, Hash: "h2"},
		entrada{Key: "/muda-status", URL: "/muda-status", Status: 200, Hash: "h3"},
		entrada{Key: "/muda-hash", URL: "/muda-hash", Status: 200, Hash: "antes"},
	)
	novo = mapa(
		entrada{Key: "/igual", URL: "/igual", Status: 200, Hash: "h1"},
		entrada{Key: "/surge", URL: "/surge", Status: 200, Hash: "h9"},
		entrada{Key: "/muda-status", URL: "/muda-status", Status: 403, Hash: "h3"},
		entrada{Key: "/muda-hash", URL: "/muda-hash", Status: 200, Hash: "depois"},
	)
	return
}

func TestDiffClassificaAdicionadoRemovidoAlterado(t *testing.T) {
	velho, novo := cenario()
	r := diff(velho, novo, nil)

	if len(r.Adicionados) != 1 || r.Adicionados[0].Key != "/surge" {
		t.Errorf("adicionados = %+v, quero só /surge", r.Adicionados)
	}
	if len(r.Removidos) != 1 || r.Removidos[0].Key != "/some" {
		t.Errorf("removidos = %+v, quero só /some", r.Removidos)
	}
	if len(r.Alterados) != 2 {
		t.Fatalf("alterados = %+v, quero 2", r.Alterados)
	}
	// ordenado por chave: /muda-hash antes de /muda-status
	if m := r.Alterados[0]; m.Key != "/muda-hash" || m.Campo != "sha256" || m.De != "antes" || m.Para != "depois" {
		t.Errorf("alterado[0] = %+v", m)
	}
	if m := r.Alterados[1]; m.Key != "/muda-status" || m.Campo != "status" || m.De != "200" || m.Para != "403" {
		t.Errorf("alterado[1] = %+v", m)
	}
}

func TestDiffScansIguaisNaoGeramNada(t *testing.T) {
	velho, _ := cenario()
	r := diff(velho, velho, nil)
	if len(r.Adicionados)+len(r.Removidos)+len(r.Alterados) != 0 {
		t.Errorf("scan contra ele mesmo deveria dar vazio: %+v", r)
	}
}

func TestDiffFiltroOnly(t *testing.T) {
	velho, novo := cenario()

	r := diff(velho, novo, map[string]bool{mudAdicionado: true})
	if len(r.Adicionados) != 1 || len(r.Removidos) != 0 || len(r.Alterados) != 0 {
		t.Errorf("--only=added vazou outros tipos: %+v", r)
	}

	r = diff(velho, novo, map[string]bool{mudRemovido: true, mudAlterado: true})
	if len(r.Adicionados) != 0 || len(r.Removidos) != 1 || len(r.Alterados) != 2 {
		t.Errorf("--only=removed,changed errado: %+v", r)
	}
}

// Mesma URL com métodos diferentes são rotas distintas no spyder: a chave já
// carrega o método, então GET igual + POST novo = 1 adicionado, 0 alterados.
func TestDiffMetodoDiferenteEhRotaDiferente(t *testing.T) {
	velho := mapa(entrada{Key: "/a\tGET", URL: "/a", Method: "GET", Status: 200, Kind: "xhr"})
	novo := mapa(
		entrada{Key: "/a\tGET", URL: "/a", Method: "GET", Status: 200, Kind: "xhr"},
		entrada{Key: "/a\tPOST", URL: "/a", Method: "POST", Status: 200, Kind: "xhr"},
	)
	r := diff(velho, novo, nil)
	if len(r.Adicionados) != 1 || r.Adicionados[0].Method != "POST" || len(r.Alterados) != 0 {
		t.Errorf("resultado inesperado: %+v", r)
	}
}

func TestDiffKindAlteradoNoDB(t *testing.T) {
	velho := mapa(entrada{Key: "/a\tGET", URL: "/a", Method: "GET", Status: 200, Kind: "page"})
	novo := mapa(entrada{Key: "/a\tGET", URL: "/a", Method: "GET", Status: 200, Kind: "xhr"})
	r := diff(velho, novo, nil)
	if len(r.Alterados) != 1 || r.Alterados[0].Campo != "kind" || r.Alterados[0].De != "page" || r.Alterados[0].Para != "xhr" {
		t.Errorf("alterados = %+v", r.Alterados)
	}
}
