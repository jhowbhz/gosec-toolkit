package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// siteFake sobe um mini-site com página inicial, página sem extensão, CSS que
// referencia imagem, imagens, um caminho barrado por robots e um arquivo
// sensível (.env) — o suficiente para exercitar o crawler ponta a ponta.
func siteFake(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("User-agent: *\nDisallow: /private\n"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head>
			<link rel="stylesheet" href="/style.css">
		</head><body>
			<a href="/sobre">sobre</a>
			<img src="/img/logo.png">
			<a href="/private/secret.html">barrado</a>
			<a href="/.env">env</a>
			<a href="https://example.org/externo">externo</a>
		</body></html>`))
	})
	mux.HandleFunc("/sobre", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body><a href="/">home</a></body></html>`))
	})
	mux.HandleFunc("/style.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		w.Write([]byte(`body{background:url("/img/bg.png")}`))
	})
	mux.HandleFunc("/img/logo.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("PNG-LOGO"))
	})
	mux.HandleFunc("/img/bg.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("PNG-BG"))
	})
	mux.HandleFunc("/private/secret.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html>segredo que robots barra</html>"))
	})
	mux.HandleFunc("/.env", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("DB_PASSWORD=supersecreto"))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// arquivosDoEspelho devolve o conjunto de nomes-base de arquivo sob root.
func arquivosDoEspelho(t *testing.T, root string) map[string]string {
	t.Helper()
	nomes := map[string]string{}
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			nomes[d.Name()] = p
		}
		return nil
	})
	return nomes
}

func rodarCrawl(t *testing.T, extra ...string) (string, *recorder) {
	return rodarCrawlEm(t, siteFake(t), extra...)
}

func rodarCrawlEm(t *testing.T, srv *httptest.Server, extra ...string) (root string, rec *recorder) {
	t.Helper()
	root = t.TempDir()

	args := append([]string{"--url=" + srv.URL + "/", "--out=" + root, "--quiet"}, extra...)
	cfg, err := parseConfig(args)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	var jsonl, console bytes.Buffer
	rec = newRecorder(&jsonl, &console, true)
	c, err := newCrawler(cfg, rec)
	if err != nil {
		t.Fatalf("newCrawler: %v", err)
	}
	if err := c.run(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}
	return root, rec
}

func TestCrawlBaixaEspelhoEsperado(t *testing.T) {
	root, _ := rodarCrawl(t)
	nomes := arquivosDoEspelho(t, root)
	for _, n := range []string{"index.html", "sobre.html", "style.css", "logo.png", "bg.png"} {
		if _, ok := nomes[n]; !ok {
			t.Errorf("esperava %s no espelho; tenho %v", n, chaves(nomes))
		}
	}
}

func TestCrawlReescreveLinksInternos(t *testing.T) {
	root, _ := rodarCrawl(t)
	nomes := arquivosDoEspelho(t, root)
	idx, ok := nomes["index.html"]
	if !ok {
		t.Fatal("sem index.html")
	}
	data, _ := os.ReadFile(idx)
	html := string(data)
	if strings.Contains(html, `href="/sobre"`) {
		t.Error("link interno /sobre deveria ter sido reescrito")
	}
	if !strings.Contains(html, "sobre.html") {
		t.Errorf("index deveria apontar pra sobre.html; html: %s", html)
	}
	if !strings.Contains(html, "https://example.org/externo") {
		t.Error("link externo deveria permanecer intacto")
	}
}

func TestCrawlRespeitaRobots(t *testing.T) {
	root, _ := rodarCrawl(t)
	nomes := arquivosDoEspelho(t, root)
	if _, ok := nomes["secret.html"]; ok {
		t.Error("robots barra /private — secret.html NÃO deveria ter sido baixado")
	}
}

func TestCrawlSinalizaSensivelSemBaixar(t *testing.T) {
	root, rec := rodarCrawl(t)

	// O conteúdo do .env não pode estar em lugar nenhum do espelho.
	var achouSegredo bool
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := os.ReadFile(p)
			if bytes.Contains(b, []byte("supersecreto")) {
				achouSegredo = true
			}
		}
		return nil
	})
	if achouSegredo {
		t.Error("conteúdo do .env NÃO pode ser baixado")
	}

	sens := rec.relatorioSensiveis()
	var achouEnv bool
	for _, s := range sens {
		if strings.HasSuffix(s.URL, "/.env") {
			achouEnv = true
			if s.SavedPath != "" {
				t.Error(".env não pode ter saved_path")
			}
		}
	}
	if !achouEnv {
		t.Errorf(".env deveria estar no relatório de sensíveis; tenho %+v", sens)
	}
}

func TestCrawlSondaNuncaBaixaNem404(t *testing.T) {
	root, rec := rodarCrawl(t, "--probe-sensitive")

	// A sonda tenta phpinfo.php (não existe no site): não pode virar arquivo
	// salvo nem contar como erro.
	nomes := arquivosDoEspelho(t, root)
	if _, ok := nomes["phpinfo.php"]; ok {
		t.Error("sonda de phpinfo.php (404) NÃO deveria gerar arquivo salvo")
	}
	if _, _, erros, _ := rec.resumo(); erros != 0 {
		t.Errorf("sonda 404 não deveria contar como erro; erros=%d", erros)
	}

	// Nenhum sensível pode ter saved_path.
	for _, s := range rec.relatorioSensiveis() {
		if s.SavedPath != "" {
			t.Errorf("sensível %s não pode ter saved_path", s.URL)
		}
	}
}

// siteCatchAll imita um SPA/catch-all: responde 200 com o MESMO corpo para
// qualquer rota (soft-404), menos /.env, que devolve conteúdo real diferente.
func siteCatchAll(t *testing.T) *httptest.Server {
	t.Helper()
	const shell = "<html><body>SPA shell constante</body></html>"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.env" {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("DB_PASSWORD=real-secret-999"))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(shell))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCrawlSoft404SoReportaExistente(t *testing.T) {
	root, rec := rodarCrawlEm(t, siteCatchAll(t), "--probe-sensitive")

	sens := rec.relatorioSensiveis()
	var temEnv, temPhpinfo bool
	for _, s := range sens {
		if strings.HasSuffix(s.URL, "/.env") {
			temEnv = true
		}
		if strings.HasSuffix(s.URL, "/phpinfo.php") {
			temPhpinfo = true
		}
	}
	if temPhpinfo {
		t.Error("soft-404: phpinfo.php responde o shell (não existe) — NÃO pode ser reportado")
	}
	if !temEnv {
		t.Errorf(".env tem corpo distinto (existe) — deveria ser reportado; sens=%+v", sens)
	}

	// Mesmo tendo lido o .env pra comparar, o conteúdo não pode ser salvo.
	var vazou bool
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := os.ReadFile(p)
			if bytes.Contains(b, []byte("real-secret-999")) {
				vazou = true
			}
		}
		return nil
	})
	if vazou {
		t.Error("conteúdo do .env não pode ser salvo em disco")
	}
}

func chaves(m map[string]string) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
