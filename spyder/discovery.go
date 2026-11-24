package main

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

// pausaVarredura evita martelar o alvo repetindo a wordlist sem parar.
const pausaVarredura = 3 * time.Second

// descobrir é o modo contínuo: um crawl inicial (links/XHR) e depois varreduras
// de wordlist em cada diretório conhecido, em loop, recursivo (cada diretório
// novo entra na próxima varredura), até o ctx ser cancelado (Ctrl+C).
func (c *crawler) descobrir(ctx context.Context, pr *probe, paths []string, stdout io.Writer) {
	fmt.Fprintln(stdout, "varredura 1: crawl de links e chamadas de API…")
	c.run(ctx)

	for sweep := 2; ctx.Err() == nil; sweep++ {
		bases := c.diretorios()
		fmt.Fprintf(stdout, "varredura %d: wordlist em %d diretório(s) × %d caminhos…\n", sweep, len(bases), len(paths))
		for _, base := range bases {
			if ctx.Err() != nil {
				return
			}
			pr.probar(ctx, base, paths, c.registrar)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(pausaVarredura):
		}
	}
}

// diretorios devolve as bases (diretórios) do mesmo host pra probar: o do seed
// e, de cada rota já conhecida, o diretório pai e a própria página como base —
// é isso que torna a varredura recursiva (acha /admin → proba /admin/… depois).
func (c *crawler) diretorios() []*url.URL {
	rotas, err := c.store.listar()
	if err != nil {
		return nil
	}
	visto := make(map[string]bool)
	var out []*url.URL
	add := func(u *url.URL) {
		if k := u.String(); !visto[k] {
			visto[k] = true
			out = append(out, u)
		}
	}

	add(dirURL(c.seed))
	// bases de API comuns sob o host — faz a força bruta focar em endpoints
	for _, b := range []string{"/api/", "/api/v1/", "/api/v2/", "/rest/", "/graphql/"} {
		u := *c.seed
		u.Path, u.RawQuery, u.Fragment = b, "", ""
		add(&u)
	}
	for _, r := range rotas {
		u, err := url.Parse(r.URL)
		if err != nil || !dentroDoEscopo(u, c.seed, c.cfg.Scope) {
			continue
		}
		add(dirURL(u))
		if r.Kind == kindPage {
			add(comoDir(u))
		}
	}
	return out
}

// dirURL devolve o diretório que contém a URL (tira o último segmento).
func dirURL(u *url.URL) *url.URL {
	c := *u
	c.RawQuery, c.Fragment = "", ""
	if i := strings.LastIndex(c.Path, "/"); i >= 0 {
		c.Path = c.Path[:i+1]
	} else {
		c.Path = "/"
	}
	return &c
}

// comoDir trata a própria URL como diretório (garante a barra final).
func comoDir(u *url.URL) *url.URL {
	c := *u
	c.RawQuery, c.Fragment = "", ""
	if !strings.HasSuffix(c.Path, "/") {
		c.Path += "/"
	}
	return &c
}
