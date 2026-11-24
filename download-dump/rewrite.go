package main

import (
	"bytes"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/net/html"
)

// rewrite é o passe que torna o espelho navegável offline: cada referência a
// um recurso que foi baixado (está no mapa resolve) passa a apontar para o
// arquivo local por caminho relativo; o que não foi baixado (externo, pulado,
// sensível) fica com a URL original, então a página ainda o referencia online.
//
// resolve recebe a URL absoluta (já resolvida contra a base, sem fragmento) e
// devolve o caminho local do espelho e true quando o recurso existe lá.

// attrsDeLink diz, por tag, quais atributos carregam referência a reescrever.
var attrsDeLink = map[string]map[string]bool{
	"a":      {"href": true},
	"link":   {"href": true},
	"script": {"src": true},
	"img":    {"src": true, "srcset": true},
	"source": {"src": true, "srcset": true},
	"iframe": {"src": true},
	"embed":  {"src": true},
	"video":  {"src": true, "poster": true},
	"audio":  {"src": true},
	"track":  {"src": true},
}

// rewriteHTML reescreve as referências do HTML. Tokens não tocados são
// recopiados crus (z.Raw()) para preservar o documento; só a tag cujos
// atributos mudam é reserializada, e o conteúdo de <script>/<style> nunca é
// reescapado (escapar quebraria JS/CSS).
func rewriteHTML(base *url.URL, docLocal string, body []byte, resolve func(*url.URL) (string, bool)) []byte {
	var out bytes.Buffer
	z := html.NewTokenizer(bytes.NewReader(body))
	var emStyle bool

	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			return out.Bytes()
		}
		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			tok := z.Token()
			mudou := reescreverAttrs(base, docLocal, &tok, resolve)
			if mudou {
				out.WriteString(tok.String())
			} else {
				out.Write(z.Raw())
			}
			if tok.Data == "style" && tt == html.StartTagToken {
				emStyle = true
			}
		case html.TextToken:
			if emStyle {
				out.Write(rewriteCSS(base, docLocal, z.Raw(), resolve))
			} else {
				out.Write(z.Raw())
			}
		case html.EndTagToken:
			if name, _ := z.TagName(); string(name) == "style" {
				emStyle = false
			}
			out.Write(z.Raw())
		default:
			out.Write(z.Raw())
		}
	}
}

// reescreverAttrs reescreve in-place os atributos de uma tag e diz se mudou
// algo (para decidir entre reserializar ou copiar cru).
func reescreverAttrs(base *url.URL, docLocal string, tok *html.Token, resolve func(*url.URL) (string, bool)) bool {
	attrs := attrsDeLink[tok.Data]
	mudou := false
	for i := range tok.Attr {
		a := &tok.Attr[i]
		key := strings.ToLower(a.Key)
		switch {
		case attrs[key] && key == "srcset":
			if nv := reescreverSrcset(base, docLocal, a.Val, resolve); nv != a.Val {
				a.Val = nv
				mudou = true
			}
		case attrs[key]:
			if nv := reescreverRef(base, docLocal, a.Val, resolve); nv != a.Val {
				a.Val = nv
				mudou = true
			}
		case key == "style":
			if nv := string(rewriteCSS(base, docLocal, []byte(a.Val), resolve)); nv != a.Val {
				a.Val = nv
				mudou = true
			}
		}
	}
	return mudou
}

// reescreverRef resolve uma referência; se o alvo está no espelho, troca pela
// relativa local (preservando o #fragmento). Senão, devolve o valor original.
func reescreverRef(base *url.URL, docLocal, val string, resolve func(*url.URL) (string, bool)) string {
	ref := strings.TrimSpace(val)
	if ref == "" || strings.HasPrefix(ref, "#") || temEsquemaIgnorado(ref) {
		return val
	}
	u, err := base.Parse(ref)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return val
	}
	frag := u.Fragment
	u.Fragment = ""
	local, ok := resolve(u)
	if !ok {
		return val
	}
	rel := caminhoRelativo(docLocal, local)
	if frag != "" {
		rel += "#" + frag
	}
	return rel
}

// reescreverSrcset reescreve a parte de URL de cada candidato do srcset,
// mantendo o descritor (1x, 2x, 100w).
func reescreverSrcset(base *url.URL, docLocal, val string, resolve func(*url.URL) (string, bool)) string {
	cands := strings.Split(val, ",")
	for i, cand := range cands {
		campos := strings.Fields(strings.TrimSpace(cand))
		if len(campos) == 0 {
			continue
		}
		campos[0] = reescreverRef(base, docLocal, campos[0], resolve)
		cands[i] = strings.Join(campos, " ")
	}
	return strings.Join(cands, ", ")
}

// rewriteCSS reescreve os url() e @import de um trecho de CSS para os caminhos
// locais, deixando intactas as referências fora do espelho.
func rewriteCSS(base *url.URL, docLocal string, body []byte, resolve func(*url.URL) (string, bool)) []byte {
	out := reCSSURL.ReplaceAllFunc(body, func(m []byte) []byte {
		sub := reCSSURL.FindSubmatch(m)
		if sub == nil {
			return m
		}
		rel := reescreverRef(base, docLocal, string(sub[1]), resolve)
		return []byte("url(" + rel + ")")
	})
	out = reCSSImport.ReplaceAllFunc(out, func(m []byte) []byte {
		sub := reCSSImport.FindSubmatch(m)
		if sub == nil {
			return m
		}
		rel := reescreverRef(base, docLocal, string(sub[1]), resolve)
		return []byte(`@import "` + rel + `"`)
	})
	return out
}

// caminhoRelativo devolve o caminho de 'to' visto a partir do diretório de
// 'from' (ambos relativos à raiz do espelho, com '/'), sempre com '/'.
func caminhoRelativo(from, to string) string {
	fromDir := path.Dir(from)
	rel, err := filepath.Rel(filepath.FromSlash(fromDir), filepath.FromSlash(to))
	if err != nil {
		return to
	}
	return filepath.ToSlash(rel)
}
