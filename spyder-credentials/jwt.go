package main

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// infoJWT é o que extraímos do payload de um JWT sem validar a assinatura — o
// suficiente para classificar a gravidade de uma exposição. Um token Supabase
// com role service_role é segredo crítico; anon/authenticated é publicável.
type infoJWT struct {
	Role   string
	Exp    int64
	Email  string
	Valido bool
}

// analisarJWT decodifica o segundo segmento (payload) de um token no formato
// header.payload.assinatura. Não confere a assinatura — só lê os claims para
// orientar a classificação. Token malformado devolve Valido=false.
func analisarJWT(tok string) infoJWT {
	p := strings.Split(tok, ".")
	if len(p) != 3 {
		return infoJWT{}
	}
	raw, err := decodeBase64URL(p[1])
	if err != nil {
		return infoJWT{}
	}
	var c struct {
		Role  string `json:"role"`
		Exp   int64  `json:"exp"`
		Email string `json:"email"`
	}
	if json.Unmarshal(raw, &c) != nil {
		return infoJWT{}
	}
	return infoJWT{Role: c.Role, Exp: c.Exp, Email: c.Email, Valido: true}
}

// decodeBase64URL tolera presença ou ausência de padding (JWT usa sem).
func decodeBase64URL(s string) ([]byte, error) {
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.URLEncoding.DecodeString(s)
}
