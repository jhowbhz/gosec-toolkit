package main

import (
	"encoding/base64"
	"testing"
)

// montarJWT cria um token sintético (header.payload.assinatura) com o payload
// JSON dado — só para exercitar o parser, sem embutir segredo real no repo.
func montarJWT(payloadJSON string) string {
	b64 := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	return b64(`{"alg":"ES256","typ":"JWT"}`) + "." + b64(payloadJSON) + ".QUFB"
}

func TestAnalisarJWTClassificaServiceRole(t *testing.T) {
	tok := montarJWT(`{"role":"service_role","exp":1795488794,"email":"svc@x.com"}`)
	j := analisarJWT(tok)
	if !j.Valido {
		t.Fatal("deveria decodificar")
	}
	if j.Role != "service_role" {
		t.Errorf("role = %q, quero service_role", j.Role)
	}
	if j.Exp != 1795488794 {
		t.Errorf("exp = %d, quero 1795488794", j.Exp)
	}
}

func TestAnalisarJWTAnon(t *testing.T) {
	j := analisarJWT(montarJWT(`{"role":"anon"}`))
	if !j.Valido || j.Role != "anon" {
		t.Errorf("esperava role anon válido, tenho %+v", j)
	}
}

func TestAnalisarJWTInvalido(t *testing.T) {
	for _, s := range []string{"nao.e.jwt!!!", "soisso", "a.b", "eyJ.###.zzz"} {
		if analisarJWT(s).Valido {
			t.Errorf("%q não deveria ser JWT válido", s)
		}
	}
}
