package main

import (
	"context"
	"sync"
	"testing"
)

// fakeFetcher devolve um grafo fixo e anota o que foi realmente buscado,
// pra provar que o crawler segue só páginas do mesmo host.
type fakeFetcher struct {
	mu      sync.Mutex
	pages   map[string]pageResult
	fetched []string
}

func (f *fakeFetcher) fetch(_ context.Context, rawurl string, _ []headerKV) (pageResult, error) {
	f.mu.Lock()
	f.fetched = append(f.fetched, rawurl)
	f.mu.Unlock()
	if pr, ok := f.pages[rawurl]; ok {
		return pr, nil
	}
	return pageResult{FinalURL: rawurl, Status: 200}, nil
}

func (f *fakeFetcher) buscou(url string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.fetched {
		if u == url {
			return true
		}
	}
	return false
}

func TestCrawlerDescobreGravaEmiteESegueSoMesmoHost(t *testing.T) {
	fake := &fakeFetcher{pages: map[string]pageResult{
		"https://site.com": {
			Status: 200, Title: "Home",
			Links:    []string{"/a", "/b", "https://ext.com/x"},
			Requests: []requestInfo{{URL: "https://site.com/api/data", Method: "GET", Status: 200, ResourceType: "xhr"}},
		},
		"https://site.com/a": {Status: 200, Links: []string{"/a/deep"}},
		"https://site.com/b": {Status: 200},
		"https://site.com/a/deep": {Status: 200},
	}}

	// --api-only=false: aqui testamos a descoberta COMPLETA (o filtro de API é
	// testado à parte, em api_test.go).
	cfg, err := parseConfig([]string{"--url=https://site.com", "--delay=0", "--concurrency=2", "--api-only=false"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	st := abrirStoreTeste(t)
	b := newBroker()
	eventos, cancelar := b.subscribe()
	defer cancelar()

	cr, err := newCrawler(cfg, fake, st, b, &robotsPolicy{})
	if err != nil {
		t.Fatalf("newCrawler: %v", err)
	}
	if err := cr.run(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}

	rotas, err := st.listar()
	if err != nil {
		t.Fatalf("listar: %v", err)
	}
	porURL := map[string]route{}
	for _, r := range rotas {
		porURL[r.URL] = r
	}

	// páginas do mesmo host foram descobertas e seguidas
	for _, u := range []string{"https://site.com", "https://site.com/a", "https://site.com/b", "https://site.com/a/deep"} {
		if porURL[u].Kind != kindPage {
			t.Errorf("%s deveria estar gravada como page, veio %q", u, porURL[u].Kind)
		}
		if !fake.buscou(u) {
			t.Errorf("%s deveria ter sido renderizada", u)
		}
	}
	// o endpoint XHR virou rota, mas NÃO foi renderizado
	if porURL["https://site.com/api/data"].Kind != kindXHR {
		t.Errorf("api/data deveria ser xhr, veio %q", porURL["https://site.com/api/data"].Kind)
	}
	if fake.buscou("https://site.com/api/data") {
		t.Error("o crawler não deveria renderizar uma chamada XHR")
	}
	// externo é registrado mas NÃO seguido
	if porURL["https://ext.com/x"].Kind != kindExternal {
		t.Errorf("ext.com/x deveria ser external, veio %q", porURL["https://ext.com/x"].Kind)
	}
	if fake.buscou("https://ext.com/x") {
		t.Error("o crawler não deveria seguir um host externo")
	}

	// cada rota nova gerou um evento ao vivo (bufferizados no assinante)
	n := 0
	for drenando := true; drenando; {
		select {
		case <-eventos:
			n++
		default:
			drenando = false
		}
	}
	if n != len(rotas) {
		t.Errorf("eventos emitidos = %d, quero %d (um por rota nova)", n, len(rotas))
	}
}
