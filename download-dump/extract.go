package main

import (
	"bytes"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// extract descobre para onde uma página aponta — é a fonte do que o crawler
// vai enfileirar. Só devolve URLs http/https absolutas e resolvidas contra a
// base; esquemas como mailto:, tel:, javascript: e data:, além de âncoras
// puras (#frag), são descartados porque não há o que baixar neles.

// tagsComLink mapeia cada tag HTML aos atributos que carregam uma referência.
var tagsComLink = map[string][]string{
	"a":      {"href"},
	"link":   {"href"},
	"script": {"src"},
	"img":    {"src", "srcset"},
	"source": {"src", "srcset"},
	"iframe": {"src"},
	"embed":  {"src"},
	"video":  {"src", "poster"},
	"audio":  {"src"},
	"track":  {"src"},
}

// extractLinks roteia pelo content-type: HTML e CSS têm links a seguir; o
// resto (imagem, pdf, binário) é folha e não gera descoberta.
func extractLinks(base *url.URL, contentType string, body []byte) []*url.URL {
	mt := strings.ToLower(contentType)
	switch {
	case strings.Contains(mt, "html"):
		return extractHTML(base, body)
	case strings.Contains(mt, "css"):
		return extractCSS(base, body)
	default:
		return nil
	}
}

// extractHTML varre o HTML com o tokenizer do x/net e colhe as referências
// de tags conhecidas, o CSS embutido em <style> e os url() de style="".
func extractHTML(base *url.URL, body []byte) []*url.URL {
	colet := novoColetor(base)
	z := html.NewTokenizer(bytes.NewReader(body))
	var emStyle bool

	for {
		switch z.Next() {
		case html.ErrorToken:
			return colet.urls
		case html.StartTagToken, html.SelfClosingTagToken:
			nome, temAttr := z.TagName()
			tag := string(nome)
			attrs := map[string]string{}
			for temAttr {
				var k, v []byte
				k, v, temAttr = z.TagAttr()
				attrs[string(k)] = string(v)
			}
			for _, a := range tagsComLink[tag] {
				if val, ok := attrs[a]; ok {
					if a == "srcset" {
						colet.addSrcset(val)
					} else {
						colet.add(val)
					}
				}
			}
			if s := attrs["style"]; s != "" {
				colet.addCSS([]byte(s))
			}
			if tag == "style" {
				emStyle = true
			}
		case html.EndTagToken:
			if nome, _ := z.TagName(); string(nome) == "style" {
				emStyle = false
			}
		case html.TextToken:
			if emStyle {
				colet.addCSS(z.Text())
			}
		}
	}
}

var (
	reCSSURL    = regexp.MustCompile(`url\(\s*['"]?([^'")]+?)['"]?\s*\)`)
	reCSSImport = regexp.MustCompile(`@import\s+['"]([^'"]+)['"]`)
)

// extractCSS pega os url(...) e os @import "..." de uma folha de estilo.
func extractCSS(base *url.URL, body []byte) []*url.URL {
	colet := novoColetor(base)
	colet.addCSS(body)
	return colet.urls
}

// coletor acumula URLs resolvidas contra a base, já deduplicadas e filtradas.
type coletor struct {
	base *url.URL
	urls []*url.URL
	seen map[string]bool
}

func novoColetor(base *url.URL) *coletor {
	return &coletor{base: base, seen: map[string]bool{}}
}

// add resolve uma referência crua contra a base e guarda se for http/https
// absoluta, inédita e sem o fragmento (que não muda o recurso baixado).
func (c *coletor) add(ref string) {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(ref, "#") {
		return
	}
	if temEsquemaIgnorado(ref) {
		return
	}
	u, err := c.base.Parse(ref)
	if err != nil {
		return
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return
	}
	u.Fragment = ""
	chave := u.String()
	if c.seen[chave] {
		return
	}
	c.seen[chave] = true
	c.urls = append(c.urls, u)
}

// addSrcset trata o atributo srcset: "url 1x, url 2x, url 100w" — pega só a
// parte de URL de cada candidato.
func (c *coletor) addSrcset(val string) {
	for _, cand := range strings.Split(val, ",") {
		campos := strings.Fields(strings.TrimSpace(cand))
		if len(campos) > 0 {
			c.add(campos[0])
		}
	}
}

// addCSS extrai de um trecho de CSS (folha, <style> ou style="") os url() e
// @import e os adiciona ao coletor.
func (c *coletor) addCSS(body []byte) {
	for _, m := range reCSSURL.FindAllSubmatch(body, -1) {
		c.add(string(m[1]))
	}
	for _, m := range reCSSImport.FindAllSubmatch(body, -1) {
		c.add(string(m[1]))
	}
}

// temEsquemaIgnorado barra referências que não são recursos baixáveis.
func temEsquemaIgnorado(ref string) bool {
	for _, p := range []string{"mailto:", "tel:", "javascript:", "data:", "about:", "blob:"} {
		if strings.HasPrefix(strings.ToLower(ref), p) {
			return true
		}
	}
	return false
}
