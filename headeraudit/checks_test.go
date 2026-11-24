package main

import (
	"net/http"
	"testing"
)

// headersCompletos é uma resposta sem nenhum achado: base para cada teste
// remover (ou estragar) um header e ver só a regra correspondente disparar.
func headersCompletos() http.Header {
	h := http.Header{}
	h.Set("Strict-Transport-Security", "max-age=31536000")
	h.Set("Content-Security-Policy", "default-src 'self'")
	h.Set("X-Frame-Options", "DENY")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Permissions-Policy", "camera=()")
	return h
}

// achado devolve o primeiro issue do check, ou nil.
func achado(issues []issue, check string) *issue {
	for i := range issues {
		if issues[i].Check == check {
			return &issues[i]
		}
	}
	return nil
}

func TestHeadersCompletosNaoGeramAchado(t *testing.T) {
	if got := avaliarHeadersSeguranca(headersCompletos(), true); len(got) != 0 {
		t.Errorf("esperava nenhum achado, veio %+v", got)
	}
}

func TestHeadersAusentesPorSeveridade(t *testing.T) {
	casos := []struct {
		header, check, severidade string
	}{
		{"Strict-Transport-Security", "Strict-Transport-Security", sevAlta},
		{"Content-Security-Policy", "Content-Security-Policy", sevMedia},
		{"X-Frame-Options", "X-Frame-Options", sevMedia},
		{"X-Content-Type-Options", "X-Content-Type-Options", sevBaixa},
		{"Referrer-Policy", "Referrer-Policy", sevBaixa},
		{"Permissions-Policy", "Permissions-Policy", sevInfo},
	}
	for _, c := range casos {
		t.Run(c.header, func(t *testing.T) {
			h := headersCompletos()
			h.Del(c.header)
			i := achado(avaliarHeadersSeguranca(h, true), c.check)
			if i == nil {
				t.Fatalf("sem %s deveria gerar achado", c.header)
			}
			if i.Severidade != c.severidade {
				t.Errorf("severidade = %s, quero %s", i.Severidade, c.severidade)
			}
			if i.Presente {
				t.Error("header ausente não pode vir como Presente")
			}
		})
	}
}

func TestHSTSNaoSeCobraEmHTTP(t *testing.T) {
	h := headersCompletos()
	h.Del("Strict-Transport-Security")
	if i := achado(avaliarHeadersSeguranca(h, false), "Strict-Transport-Security"); i != nil {
		t.Errorf("HSTS não vale em http, mas gerou achado: %+v", i)
	}
}

func TestFrameAncestorsDispensaXFrameOptions(t *testing.T) {
	h := headersCompletos()
	h.Del("X-Frame-Options")
	h.Set("Content-Security-Policy", "default-src 'self'; Frame-Ancestors 'none'")
	if i := achado(avaliarHeadersSeguranca(h, true), "X-Frame-Options"); i != nil {
		t.Errorf("frame-ancestors no CSP deveria bastar: %+v", i)
	}
}

func TestXContentTypeOptionsComValorErrado(t *testing.T) {
	h := headersCompletos()
	h.Set("X-Content-Type-Options", "sniff")
	i := achado(avaliarHeadersSeguranca(h, true), "X-Content-Type-Options")
	if i == nil || !i.Presente {
		t.Fatalf("valor errado deveria ser achado com Presente=true: %+v", i)
	}
}

func TestCookieSemFlags(t *testing.T) {
	got := avaliarCookies([]string{"sid=abc; Path=/"}, true)
	if len(got) != 3 {
		t.Fatalf("cookie pelado deveria ter 3 achados, veio %d: %+v", len(got), got)
	}
	for _, i := range got {
		if i.Check != "cookie:sid" {
			t.Errorf("check = %q, quero cookie:sid", i.Check)
		}
	}
	var altas, medias int
	for _, i := range got {
		switch i.Severidade {
		case sevAlta:
			altas++
		case sevMedia:
			medias++
		}
	}
	if altas != 1 || medias != 2 {
		t.Errorf("altas=%d medias=%d, quero 1 e 2", altas, medias)
	}
}

func TestCookieBemConfiguradoNaoGeraAchado(t *testing.T) {
	got := avaliarCookies([]string{"sid=abc; Secure; HttpOnly; SameSite=Lax"}, true)
	if len(got) != 0 {
		t.Errorf("esperava nenhum achado, veio %+v", got)
	}
}

func TestCookieSecureNaoSeCobraEmHTTP(t *testing.T) {
	got := avaliarCookies([]string{"sid=abc; HttpOnly; SameSite=Lax"}, false)
	if len(got) != 0 {
		t.Errorf("Secure não vale em http, mas gerou %+v", got)
	}
}

func TestCookieSameSiteNoneSemSecure(t *testing.T) {
	got := avaliarCookies([]string{"sid=abc; HttpOnly; SameSite=None"}, false)
	if len(got) != 1 || got[0].Severidade != sevMedia {
		t.Errorf("SameSite=None sem Secure deveria dar 1 achado medium: %+v", got)
	}
}

func TestCORSOrigemRefletida(t *testing.T) {
	const origem = "https://attacker.example"
	h := http.Header{}
	h.Set("Access-Control-Allow-Origin", origem)

	got := avaliarCORS(origem, h)
	if len(got) != 1 || got[0].Severidade != sevAlta {
		t.Fatalf("reflexão simples deveria ser high: %+v", got)
	}
}

func TestCORSComCredenciaisEhCritico(t *testing.T) {
	const origem = "https://attacker.example"
	h := http.Header{}
	h.Set("Access-Control-Allow-Origin", origem)
	h.Set("Access-Control-Allow-Credentials", "true")

	got := avaliarCORS(origem, h)
	if len(got) != 1 || got[0].Severidade != sevCritica {
		t.Fatalf("reflexão com credenciais deveria ser critical: %+v", got)
	}
}

func TestCORSSemReflexaoNaoGeraAchado(t *testing.T) {
	const origem = "https://attacker.example"
	casos := map[string]string{
		"sem header":  "",
		"origem fixa": "https://meusite.com.br",
		"curinga":     "*",
		"origem nula": "null",
	}
	for nome, valor := range casos {
		t.Run(nome, func(t *testing.T) {
			h := http.Header{}
			if valor != "" {
				h.Set("Access-Control-Allow-Origin", valor)
			}
			h.Set("Access-Control-Allow-Credentials", "true")
			if got := avaliarCORS(origem, h); len(got) != 0 {
				t.Errorf("esperava nenhum achado, veio %+v", got)
			}
		})
	}
}

func TestNota(t *testing.T) {
	casos := []struct {
		nome   string
		issues []issue
		quero  string
	}{
		{"sem achados", nil, "A"},
		{"só info", []issue{{Severidade: sevInfo}}, "A"},
		{"baixa", []issue{{Severidade: sevBaixa}}, "B"},
		{"média", []issue{{Severidade: sevMedia}}, "C"},
		{"alta", []issue{{Severidade: sevAlta}}, "D"},
		{"crítica", []issue{{Severidade: sevCritica}}, "F"},
		{"pior vence", []issue{{Severidade: sevBaixa}, {Severidade: sevCritica}, {Severidade: sevMedia}}, "F"},
		{"muitas baixas não somam", []issue{{Severidade: sevBaixa}, {Severidade: sevBaixa}, {Severidade: sevBaixa}}, "B"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := nota(c.issues); got != c.quero {
				t.Errorf("nota = %s, quero %s", got, c.quero)
			}
		})
	}
}
