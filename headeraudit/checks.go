package main

import (
	"fmt"
	"net/http"
	"strings"
)

// Severidades dos achados. Ficam como string porque vão direto pro JSONL e
// pro console; a ordem para comparar mora em pesoSeveridade.
const (
	sevCritica = "critical"
	sevAlta    = "high"
	sevMedia   = "medium"
	sevBaixa   = "low"
	sevInfo    = "info"
)

// pesoSeveridade ordena as severidades. A nota é a PIOR severidade achada,
// então precisa de uma comparação — string não ordena sozinha.
var pesoSeveridade = map[string]int{
	sevInfo: 0, sevBaixa: 1, sevMedia: 2, sevAlta: 3, sevCritica: 4,
}

// notaPorPeso mapeia o peso da pior severidade para a nota. Info não
// derruba a nota: é só informativo, não é falha de postura.
var notaPorPeso = []string{"A", "B", "C", "D", "F"}

// issue é um achado de uma checagem. Presente diz se o header/cookie existe
// (um X-Content-Type-Options com valor errado está presente e mesmo assim
// é achado), para o relatório separar "faltou" de "veio errado".
type issue struct {
	Check      string `json:"check"`
	Severidade string `json:"severity"`
	Presente   bool   `json:"present"`
	Detalhe    string `json:"detail"`
}

// Todas as funções abaixo são puras (sem rede nem relógio): recebem o que a
// resposta trouxe e devolvem só os problemas. Assim cada regra é testável
// com um http.Header montado à mão.

// avaliarHeadersSeguranca confere os headers de endurecimento do navegador.
// HSTS só vale em https — cobrar em http seria achado falso, o navegador
// ignora o header em conexão sem TLS.
func avaliarHeadersSeguranca(h http.Header, https bool) []issue {
	var out []issue

	if https && h.Get("Strict-Transport-Security") == "" {
		out = append(out, issue{"Strict-Transport-Security", sevAlta, false,
			"HSTS ausente: o navegador aceita voltar a http e fica exposto a downgrade/SSL strip"})
	}

	csp := h.Get("Content-Security-Policy")
	if csp == "" {
		out = append(out, issue{"Content-Security-Policy", sevMedia, false,
			"CSP ausente: nada limita de onde scripts rodam, o XSS tem caminho livre"})
	}

	// frame-ancestors do CSP substitui o X-Frame-Options nos navegadores
	// modernos; só cobra o XFO se nenhum dos dois protege contra clickjacking.
	if h.Get("X-Frame-Options") == "" && !strings.Contains(strings.ToLower(csp), "frame-ancestors") {
		out = append(out, issue{"X-Frame-Options", sevMedia, false,
			"sem X-Frame-Options nem frame-ancestors no CSP: a página pode ser embutida em iframe (clickjacking)"})
	}

	if xcto := h.Get("X-Content-Type-Options"); !strings.EqualFold(strings.TrimSpace(xcto), "nosniff") {
		out = append(out, issue{"X-Content-Type-Options", sevBaixa, xcto != "",
			"deveria ser 'nosniff' para o navegador não adivinhar o tipo do conteúdo"})
	}

	if h.Get("Referrer-Policy") == "" {
		out = append(out, issue{"Referrer-Policy", sevBaixa, false,
			"Referrer-Policy ausente: a URL completa pode vazar no Referer para terceiros"})
	}

	if h.Get("Permissions-Policy") == "" {
		out = append(out, issue{"Permissions-Policy", sevInfo, false,
			"Permissions-Policy ausente: recursos do navegador (câmera, geolocalização...) não são restringidos"})
	}

	return out
}

// avaliarCookies confere as flags de cada Set-Cookie. Reaproveita o parser
// do net/http (via http.Response) em vez de quebrar a string na mão: atributos
// em caixa variada e espaços são o tipo de detalhe que parser caseiro erra.
func avaliarCookies(setCookies []string, https bool) []issue {
	resp := http.Response{Header: http.Header{"Set-Cookie": setCookies}}

	var out []issue
	for _, c := range resp.Cookies() {
		check := "cookie:" + c.Name

		if https && !c.Secure {
			out = append(out, issue{check, sevAlta, true,
				"cookie sem Secure: pode trafegar em http puro e ser interceptado"})
		}
		if !c.HttpOnly {
			out = append(out, issue{check, sevMedia, true,
				"cookie sem HttpOnly: legível por JavaScript, então um XSS rouba a sessão"})
		}
		// SameSite=None só é aceito pelo navegador junto com Secure; sem
		// Secure é cookie descartado ou, na prática, sem proteção de CSRF.
		switch {
		// Zero é "atributo ausente": o net/http só preenche SameSiteDefaultMode
		// quando vem "SameSite" sem valor.
		case c.SameSite == 0 || c.SameSite == http.SameSiteDefaultMode:
			out = append(out, issue{check, sevMedia, true,
				"cookie sem SameSite: sem proteção explícita contra CSRF"})
		case c.SameSite == http.SameSiteNoneMode && !c.Secure:
			out = append(out, issue{check, sevMedia, true,
				"SameSite=None sem Secure: o navegador rejeita ou o cookie fica enviado em qualquer contexto"})
		}
	}
	return out
}

// avaliarCORS confere a resposta (ao OPTIONS) quando mandamos um Origin de
// teste. Refletir uma origem arbitrária já é grave; refletir E permitir
// credenciais deixa qualquer site lido com a sessão da vítima — crítico.
// Refletir "*" não entra: navegador não envia credenciais nesse caso.
func avaliarCORS(origem string, resposta http.Header) []issue {
	if resposta.Get("Access-Control-Allow-Origin") != origem {
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(resposta.Get("Access-Control-Allow-Credentials")), "true") {
		return []issue{{"CORS", sevCritica, true,
			fmt.Sprintf("reflete a origem arbitrária %s COM Allow-Credentials:true: qualquer site lê respostas autenticadas", origem)}}
	}
	return []issue{{"CORS", sevAlta, true,
		fmt.Sprintf("reflete a origem arbitrária %s em Access-Control-Allow-Origin", origem)}}
}

// nota resume os achados na pior severidade: A (nada além de info) a F
// (crítico). Pior caso e não média, porque um achado crítico não é
// "compensado" por cinco headers corretos.
func nota(issues []issue) string {
	pior := 0
	for _, i := range issues {
		if p := pesoSeveridade[i.Severidade]; p > pior {
			pior = p
		}
	}
	return notaPorPeso[pior]
}
