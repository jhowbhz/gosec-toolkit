package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash"
	"strings"
)

// token é um JWT já aberto. Guarda o signingInput e a assinatura crus porque
// a verificação HMAC precisa dos bytes originais — reserializar o header/payload
// mudaria a ordem das chaves e invalidaria qualquer assinatura legítima.
type token struct {
	Header  map[string]any
	Payload map[string]any

	signingInput string
	assinatura   []byte
}

func (t token) alg() string {
	s, _ := t.Header["alg"].(string)
	return s
}

func (t token) kid() string {
	s, _ := t.Header["kid"].(string)
	return s
}

// decodificar abre um JWT compacto (header.payload.assinatura). Não valida
// assinatura nenhuma: só separa e decodifica, que é o que dá pra fazer sem
// segredo ou chave pública.
func decodificar(raw string) (token, error) {
	partes := strings.Split(strings.TrimSpace(raw), ".")
	if len(partes) != 3 {
		return token{}, fmt.Errorf("JWT precisa de 3 partes separadas por '.', veio %d", len(partes))
	}

	header, err := decodificarJSON(partes[0])
	if err != nil {
		return token{}, fmt.Errorf("header: %w", err)
	}
	payload, err := decodificarJSON(partes[1])
	if err != nil {
		return token{}, fmt.Errorf("payload: %w", err)
	}
	// "alg":"none" vem com a terceira parte vazia; decodeSegment("") devolve
	// slice vazio, e é isso que verificarHMAC trata como "sem assinatura".
	assinatura, err := decodeSegment(partes[2])
	if err != nil {
		return token{}, fmt.Errorf("assinatura: %w", err)
	}

	return token{
		Header:       header,
		Payload:      payload,
		signingInput: partes[0] + "." + partes[1],
		assinatura:   assinatura,
	}, nil
}

// decodeSegment decodifica base64url sem padding. Remove "=" antes porque
// emissores fora do padrão RFC 7515 às vezes deixam o padding, e recusar o
// token por isso esconderia justamente os tokens mal emitidos que queremos ver.
func decodeSegment(seg string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(seg, "="))
}

func decodificarJSON(seg string) (map[string]any, error) {
	bruto, err := decodeSegment(seg)
	if err != nil {
		return nil, fmt.Errorf("base64url inválido: %w", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(bruto, &obj); err != nil {
		return nil, fmt.Errorf("não é um objeto JSON: %w", err)
	}
	return obj, nil
}

// funcaoHash devolve o hash do algoritmo HS*; nil para qualquer outro alg.
// Só HS* entra: RSA/EC exigem chave pública, que não é o escopo desta ferramenta.
func funcaoHash(alg string) func() hash.Hash {
	switch alg {
	case "HS256":
		return sha256.New
	case "HS384":
		return sha512.New384
	case "HS512":
		return sha512.New
	}
	return nil
}

func algSimetrico(alg string) bool { return funcaoHash(alg) != nil }

// verificarHMAC diz se o segredo reproduz a assinatura. hmac.Equal compara em
// tempo constante; aqui não há atacante remoto, mas é o hábito certo e custa nada.
func verificarHMAC(signingInput string, assinatura []byte, segredo, alg string) bool {
	novo := funcaoHash(alg)
	if novo == nil || len(assinatura) == 0 {
		return false
	}
	mac := hmac.New(novo, []byte(segredo))
	mac.Write([]byte(signingInput))
	return hmac.Equal(mac.Sum(nil), assinatura)
}
