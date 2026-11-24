package main

import "testing"

func TestEhAPI(t *testing.T) {
	casos := []struct {
		r    route
		want bool
	}{
		{route{Kind: kindXHR, URL: "https://x.com/qualquer"}, true},          // XHR é sempre API
		{route{Kind: kindPage, URL: "https://x.com/api/conta"}, true},        // caminho /api
		{route{Kind: kindPage, URL: "https://x.com/API/Consulta"}, true},     // case-insensitive
		{route{Kind: kindPage, URL: "https://x.com/dados.json"}, true},       // resposta .json
		{route{Kind: kindPage, URL: "https://x.com/home"}, false},            // front
		{route{Kind: kindAsset, URL: "https://x.com/app.js"}, false},         // asset
		{route{Kind: kindExternal, URL: "https://cdn.com/fonts.css"}, false}, // externo não-api
	}
	for _, c := range casos {
		if got := ehAPI(c.r); got != c.want {
			t.Errorf("ehAPI(%s kind=%s) = %v, quero %v", c.r.URL, c.r.Kind, got, c.want)
		}
	}
}

func TestRegistrarApiOnlyFiltraFront(t *testing.T) {
	st := abrirStoreTeste(t)
	cr := &crawler{cfg: Config{ApiOnly: true}, store: st, broker: newBroker()}

	cr.registrar(route{URL: "https://x.com/home", Method: "GET", Kind: kindPage})       // front
	cr.registrar(route{URL: "https://x.com/api/conta", Method: "GET", Kind: kindXHR})   // api
	cr.registrar(route{URL: "https://x.com/app.js", Method: "GET", Kind: kindAsset})    // asset

	rotas, _ := st.listar()
	if len(rotas) != 1 || rotas[0].URL != "https://x.com/api/conta" {
		t.Errorf("api-only deveria gravar só a API, veio %+v", rotas)
	}
}

func TestRegistrarSemApiOnlyGravaTudo(t *testing.T) {
	st := abrirStoreTeste(t)
	cr := &crawler{cfg: Config{ApiOnly: false}, store: st, broker: newBroker()}
	cr.registrar(route{URL: "https://x.com/home", Method: "GET", Kind: kindPage})
	cr.registrar(route{URL: "https://x.com/api/x", Method: "GET", Kind: kindXHR})
	rotas, _ := st.listar()
	if len(rotas) != 2 {
		t.Errorf("sem api-only deveria gravar tudo, veio %d", len(rotas))
	}
}
