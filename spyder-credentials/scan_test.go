package main

import (
	"strings"
	"testing"
)

// porDetector indexa os achados pelo nome do detector (último vence).
func porDetector(as []achado) map[string]achado {
	m := map[string]achado{}
	for _, a := range as {
		m[a.Detector] = a
	}
	return m
}

func TestVarrerAchaEClassifica(t *testing.T) {
	jwtSvc := montarJWT(`{"role":"service_role","exp":1795488794}`)
	jwtAnon := montarJWT(`{"role":"anon"}`)
	txt := strings.Join([]string{
		`const svc = "` + jwtSvc + `";`,                                 // 1 JWT service_role -> crítico/secreto
		`const pub = "sb_publishable_McQq2r064N6wpycBkJhqhQ_cEv_ZpjN";`, // 2 Supabase publicável
		`const anon = "` + jwtAnon + `";`,                               // 3 JWT anon -> publicável
		`aws_key = "AKIAIOSFODNN7EXAMPLE"`,                              // 4 AWS -> secreto
		`password = "changeme"`,                                         // 5 placeholder -> ignorar
		`-----BEGIN RSA PRIVATE KEY-----`,                               // 6 chave privada
	}, "\n")

	todos := varrer(txt, "app.js", detectoresPadrao(), false)
	m := porDetector(todos)

	// Há dois JWT (service_role e anon); confere cada classe na lista inteira.
	var temSvc, temAnon bool
	for _, a := range todos {
		if a.Detector != "jwt" {
			continue
		}
		if strings.Contains(a.Extra, "service_role") {
			temSvc = true
			if a.Classe != classeSecreto {
				t.Errorf("JWT service_role deveria ser secreto, veio %q", a.Classe)
			}
		}
		if strings.Contains(a.Extra, "role=anon") {
			temAnon = true
			if a.Classe != classePublicavel {
				t.Errorf("JWT anon deveria ser publicável, veio %q", a.Classe)
			}
		}
	}
	if !temSvc || !temAnon {
		t.Fatalf("esperava detectar JWT service_role e anon; svc=%v anon=%v", temSvc, temAnon)
	}

	if a := m["supabase-publishable"]; a.Classe != classePublicavel {
		t.Errorf("sb_publishable deveria ser publicável, veio %q (det=%+v)", a.Classe, a)
	}
	if _, ok := m["aws-access-key"]; !ok {
		t.Error("AWS access key não detectada")
	}
	if a, ok := m["chave-privada"]; !ok || a.Linha != 6 {
		t.Errorf("chave privada deveria ser detectada na linha 6, veio %+v", a)
	}

	for _, a := range varrer(txt, "app.js", detectoresPadrao(), false) {
		if a.Detector == "atribuicao-generica" && strings.Contains(a.Valor, "changeme") {
			t.Error("placeholder 'changeme' não deveria ser reportado")
		}
	}
}

func TestVarrerIgnoraPlaceholdersGenericos(t *testing.T) {
	txt := strings.Join([]string{
		`api_key = "your_api_key_here"`,
		`secret = "xxxxxxxx"`,
		`token = "example"`,
		`password = "12345678"`,
		`client_secret = "s3Cr3t-V4lor-Real-AQUI-987"`, // esse é plausível -> reportar
	}, "\n")
	as := varrer(txt, "cfg.js", detectoresPadrao(), false)
	var reais int
	for _, a := range as {
		if a.Detector == "atribuicao-generica" {
			reais++
			if !strings.Contains(a.Valor, "s3Cr3t") {
				t.Errorf("reportou placeholder como real: %q", a.Valor)
			}
		}
	}
	if reais != 1 {
		t.Errorf("esperava 1 atribuição genérica real, tenho %d", reais)
	}
}
