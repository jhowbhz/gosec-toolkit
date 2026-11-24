package main

import (
	"net/url"
	"testing"
)

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", raw, err)
	}
	return u
}

// contem confere que todas as URLs esperadas estão presentes (ignora ordem).
func contem(got []*url.URL, want ...string) map[string]bool {
	set := map[string]bool{}
	for _, u := range got {
		set[u.String()] = true
	}
	return set
}

func TestExtractHTMLResolveEFiltra(t *testing.T) {
	base := mustURL(t, "https://site.com/a/")
	body := []byte(`<html><head>
	<link rel="stylesheet" href="/style.css">
	<script src="//site.com/j.js"></script>
	</head><body>
	<a href="/abs">x</a>
	<a href="rel.html">y</a>
	<a href="https://other.com/x">z</a>
	<img src="img/p.png">
	<img srcset="a.png 1x, b.png 2x">
	<a href="mailto:x@y.com">mail</a>
	<a href="#frag">frag</a>
	<a href="javascript:void(0)">js</a>
	</body></html>`)

	got := extractHTML(base, body)
	set := contem(got)

	quer := []string{
		"https://site.com/abs",
		"https://site.com/a/rel.html",
		"https://other.com/x",
		"https://site.com/a/img/p.png",
		"https://site.com/style.css",
		"https://site.com/j.js",
		"https://site.com/a/a.png",
		"https://site.com/a/b.png",
	}
	for _, w := range quer {
		if !set[w] {
			t.Errorf("esperava %s nos links extraídos", w)
		}
	}
	naoQuer := []string{"mailto:x@y.com", "javascript:void(0)"}
	for _, n := range naoQuer {
		if set[n] {
			t.Errorf("não deveria extrair %s", n)
		}
	}
	for u := range set {
		if u == "https://site.com/a/" || u == "https://site.com/a/#frag" {
			t.Errorf("fragmento não deveria virar link próprio: %s", u)
		}
	}
}

func TestExtractCSSPegaURLseImports(t *testing.T) {
	base := mustURL(t, "https://site.com/css/")
	body := []byte(`
	@import "base.css";
	.x { background: url('../img/bg.png'); }
	.y { background: url(http://cdn.com/f.png); }
	@font-face { src: url("fonts/f.woff2"); }
	`)
	got := extractCSS(base, body)
	set := contem(got)
	quer := []string{
		"https://site.com/css/base.css",
		"https://site.com/img/bg.png",
		"http://cdn.com/f.png",
		"https://site.com/css/fonts/f.woff2",
	}
	for _, w := range quer {
		if !set[w] {
			t.Errorf("esperava %s no CSS extraído", w)
		}
	}
}

func TestExtractLinksRoteiaPorContentType(t *testing.T) {
	base := mustURL(t, "https://site.com/")
	html := extractLinks(base, "text/html; charset=utf-8", []byte(`<a href="/p">x</a>`))
	if len(html) != 1 || html[0].String() != "https://site.com/p" {
		t.Errorf("roteamento HTML falhou: %v", html)
	}
	css := extractLinks(base, "text/css", []byte(`.a{background:url(/bg.png)}`))
	if len(css) != 1 || css[0].String() != "https://site.com/bg.png" {
		t.Errorf("roteamento CSS falhou: %v", css)
	}
	bin := extractLinks(base, "image/png", []byte{0x89, 0x50})
	if len(bin) != 0 {
		t.Errorf("binário não deveria gerar links: %v", bin)
	}
}
