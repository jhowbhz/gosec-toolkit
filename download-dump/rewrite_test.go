package main

import (
	"net/url"
	"strings"
	"testing"
)

// resolverDe devolve um resolve(abs) baseado num mapa URL->caminho local.
func resolverDe(m map[string]string) func(*url.URL) (string, bool) {
	return func(u *url.URL) (string, bool) {
		p, ok := m[u.String()]
		return p, ok
	}
}

func TestRewriteHTMLApontaProLocalEDeixaExternoIntacto(t *testing.T) {
	base := mustURL(t, "https://site.com/")
	resolve := resolverDe(map[string]string{
		"https://site.com/sobre":     "site.com/sobre.html",
		"https://site.com/style.css": "site.com/style.css",
	})
	body := []byte(`<link rel="stylesheet" href="/style.css">` +
		`<a href="/sobre">s</a>` +
		`<a href="https://other.com/x">e</a>`)

	out := string(rewriteHTML(base, "site.com/index.html", body, resolve))

	if !strings.Contains(out, `href="sobre.html"`) {
		t.Errorf("link interno /sobre deveria virar sobre.html; saída: %s", out)
	}
	if !strings.Contains(out, `href="style.css"`) {
		t.Errorf("/style.css deveria virar style.css relativo; saída: %s", out)
	}
	if !strings.Contains(out, `href="https://other.com/x"`) {
		t.Errorf("link externo deveria ficar intacto; saída: %s", out)
	}
}

func TestRewriteHTMLNaoCorrompeScript(t *testing.T) {
	base := mustURL(t, "https://site.com/")
	resolve := resolverDe(map[string]string{})
	body := []byte(`<script>if (a < b && c > d) { x(); }</script>`)
	out := string(rewriteHTML(base, "site.com/index.html", body, resolve))
	if !strings.Contains(out, `a < b && c > d`) {
		t.Errorf("conteúdo de <script> não pode ser escapado; saída: %s", out)
	}
}

func TestRewriteCSSRecalculaRelativoeMantemExterno(t *testing.T) {
	base := mustURL(t, "https://site.com/assets/css/")
	resolve := resolverDe(map[string]string{
		"https://site.com/img/bg.png": "site.com/img/bg.png",
		"https://site.com/base.css":   "site.com/base.css",
	})
	body := []byte(`@import "/base.css";` +
		`.x{background:url('/img/bg.png')}` +
		`.y{background:url(https://cdn.com/f.png)}`)

	out := string(rewriteCSS(base, "site.com/assets/css/app.css", body, resolve))

	if !strings.Contains(out, "../../img/bg.png") {
		t.Errorf("bg.png deveria recalcular pra ../../img/bg.png; saída: %s", out)
	}
	if !strings.Contains(out, "../../base.css") {
		t.Errorf("@import deveria recalcular pra ../../base.css; saída: %s", out)
	}
	if !strings.Contains(out, "https://cdn.com/f.png") {
		t.Errorf("url externa deveria ficar intacta; saída: %s", out)
	}
}
