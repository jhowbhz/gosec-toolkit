package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
)

func TestProbarAchaExistentesIgnora404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/admin":
			w.WriteHeader(200)
		case "/secret":
			w.WriteHeader(403)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	base, _ := url.Parse(srv.URL + "/")
	cfg, _ := parseConfig([]string{"--url=" + srv.URL, "--delay=0", "--concurrency=4"})
	p := newProbe(cfg)

	var mu sync.Mutex
	hits := map[string]int{}
	p.probar(context.Background(), base, []string{"admin", "secret", "naoexiste"}, func(r route) {
		mu.Lock()
		hits[r.URL] = r.Status
		mu.Unlock()
	})

	if hits[srv.URL+"/admin"] != 200 {
		t.Errorf("/admin deveria ser hit 200, veio %d", hits[srv.URL+"/admin"])
	}
	if hits[srv.URL+"/secret"] != 403 {
		t.Errorf("/secret (403) é hit — existe mas protegido, veio %d", hits[srv.URL+"/secret"])
	}
	if _, achou := hits[srv.URL+"/naoexiste"]; achou {
		t.Error("/naoexiste (404) não deveria ser hit")
	}
}

func TestProbarIgnoraSoft404(t *testing.T) {
	// site que devolve 200 + a MESMA página pra tudo (catch-all de SPA):
	// nenhum caminho da wordlist deveria virar hit.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("<html>catch all</html>"))
	}))
	defer srv.Close()

	base, _ := url.Parse(srv.URL + "/")
	cfg, _ := parseConfig([]string{"--url=" + srv.URL, "--delay=0", "--concurrency=3"})
	p := newProbe(cfg)

	hits := 0
	var mu sync.Mutex
	p.probar(context.Background(), base, []string{"admin", "secret", "config"}, func(route) {
		mu.Lock()
		hits++
		mu.Unlock()
	})
	if hits != 0 {
		t.Errorf("soft-404 (200 pra tudo) não deveria gerar hit, veio %d", hits)
	}
}

func TestProbarAchaPaginaQueDifereDoSoft404(t *testing.T) {
	// catch-all 200 pra tudo, MAS /admin tem conteúdo bem diferente → hit real.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		if r.URL.Path == "/admin" {
			w.Write([]byte("<html>painel do admin com muito mais conteudo que o catch-all generico</html>"))
			return
		}
		w.Write([]byte("<html>catch all</html>"))
	}))
	defer srv.Close()

	base, _ := url.Parse(srv.URL + "/")
	cfg, _ := parseConfig([]string{"--url=" + srv.URL, "--delay=0", "--concurrency=3"})
	p := newProbe(cfg)

	var mu sync.Mutex
	hits := map[string]bool{}
	p.probar(context.Background(), base, []string{"admin", "naoexiste"}, func(r route) {
		mu.Lock()
		hits[r.URL] = true
		mu.Unlock()
	})
	if !hits[srv.URL+"/admin"] {
		t.Error("/admin difere do catch-all e deveria ser hit")
	}
	if hits[srv.URL+"/naoexiste"] {
		t.Error("/naoexiste é o catch-all e não deveria ser hit")
	}
}

func TestProbarMandaHeaders(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	base, _ := url.Parse(srv.URL + "/")
	cfg, _ := parseConfig([]string{"--url=" + srv.URL, "--delay=0", "--header", "Authorization: Bearer tok"})
	p := newProbe(cfg)
	p.probar(context.Background(), base, []string{"x"}, func(route) {})

	if gotAuth != "Bearer tok" {
		t.Errorf("header do usuário não chegou no probe: %q", gotAuth)
	}
}
