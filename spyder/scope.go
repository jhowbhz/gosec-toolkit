package main

import (
	"net/url"
	"strings"
)

// Escopos de crawl.
const (
	scopeSameHost   = "same-host"
	scopeSubdomains = "subdomains"
)

// normalizar resolve ref (absoluto ou relativo) contra base, descarta o
// fragmento (#...) e aceita só http/https. Devolve false pro que não é uma
// rota navegável (mailto:, javascript:, tel:, etc.).
func normalizar(base *url.URL, ref string) (*url.URL, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, false
	}
	u, err := base.Parse(ref)
	if err != nil {
		return nil, false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, false
	}
	u.Fragment = ""
	u.Host = strings.ToLower(u.Host)
	return u, true
}

// dentroDoEscopo diz se a URL deve ser SEGUIDA (aprofundada). Fora do escopo
// a URL ainda é registrada como externa, só não é enfileirada.
func dentroDoEscopo(target, seed *url.URL, scope string) bool {
	th := strings.ToLower(target.Hostname())
	sh := strings.ToLower(seed.Hostname())
	if th == sh {
		return true
	}
	if scope == scopeSubdomains && strings.HasSuffix(th, "."+sh) {
		return true
	}
	return false
}
