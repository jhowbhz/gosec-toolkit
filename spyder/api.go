package main

import (
	"net/url"
	"strings"
)

// ehAPI diz se uma rota é parte da superfície de API (o foco do spyder), e não
// front-end. É API quando foi uma chamada XHR/fetch, ou quando o caminho cheira
// a API (/api, /rest, /graphql, .json). Página comum e asset não são.
func ehAPI(r route) bool {
	if r.Kind == kindXHR {
		return true
	}
	u, err := url.Parse(r.URL)
	if err != nil {
		return false
	}
	p := strings.ToLower(u.Path)
	switch {
	case strings.Contains(p, "/api"):
		return true
	case strings.Contains(p, "/rest/"):
		return true
	case strings.Contains(p, "/graphql"):
		return true
	case strings.HasSuffix(p, ".json"):
		return true
	}
	return false
}
