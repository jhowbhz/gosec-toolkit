package main

import (
	"regexp"
	"strings"
	"time"
)

// detectoresPadrao é o conjunto curado de detectores de alto sinal. A ordem
// não importa (tudo roda); a classificação de cada um reflete o risco real de
// exposição: secreto = nunca deveria aparecer; publicavel = público por
// desenho; suspeito = precisa de conferência humana.
func detectoresPadrao() []detector {
	return []detector{
		{
			nome:   "jwt",
			re:     regexp.MustCompile(`eyJ[A-Za-z0-9_-]{6,}\.eyJ[A-Za-z0-9_-]{4,}\.[A-Za-z0-9_-]{4,}`),
			refina: refinaJWT,
		},
		{
			nome:   "chave-privada",
			re:     regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA |OPENSSH |PGP |ENCRYPTED )?PRIVATE KEY-----`),
			sev:    sevCritico,
			classe: classeSecreto,
		},
		{
			nome:   "supabase-secret",
			re:     regexp.MustCompile(`sb_secret_[A-Za-z0-9_-]{10,}`),
			sev:    sevCritico,
			classe: classeSecreto,
		},
		{
			nome:   "supabase-publishable",
			re:     regexp.MustCompile(`sb_publishable_[A-Za-z0-9_-]{10,}`),
			sev:    sevInfo,
			classe: classePublicavel,
		},
		{
			nome:   "aws-access-key",
			re:     regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
			sev:    sevAlto,
			classe: classeSecreto,
		},
		{
			nome:   "google-api-key",
			re:     regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`),
			sev:    sevAlto,
			classe: classeSecreto,
		},
		{
			nome:   "github-token",
			re:     regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36,}`),
			sev:    sevCritico,
			classe: classeSecreto,
		},
		{
			nome:   "slack-token",
			re:     regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`),
			sev:    sevCritico,
			classe: classeSecreto,
		},
		{
			nome:   "stripe-secret",
			re:     regexp.MustCompile(`sk_live_[0-9A-Za-z]{16,}`),
			sev:    sevCritico,
			classe: classeSecreto,
		},
		{
			nome:   "stripe-publishable",
			re:     regexp.MustCompile(`pk_live_[0-9A-Za-z]{16,}`),
			sev:    sevInfo,
			classe: classePublicavel,
		},
		{
			nome:   "sendgrid-key",
			re:     regexp.MustCompile(`SG\.[A-Za-z0-9_-]{22}\.[A-Za-z0-9_-]{43}`),
			sev:    sevCritico,
			classe: classeSecreto,
		},
		{
			nome:   "conn-string",
			re:     regexp.MustCompile(`(?:postgres(?:ql)?|mysql|mongodb(?:\+srv)?|redis|amqp)://[^\s:@/]+:[^\s:@/]+@[^\s/]+`),
			sev:    sevCritico,
			classe: classeSecreto,
		},
		{
			nome: "atribuicao-generica",
			// A classe de valor exclui os caracteres de código (parênteses,
			// operadores, cifrão, crase) que só aparecem em JS, nunca no valor
			// de uma credencial — isso corta o ruído de bundles minificados.
			re:     regexp.MustCompile("(?i)(?:access[_-]?token|client[_-]?secret|auth[_-]?token|api[_-]?key|apikey|secret|token|password|passwd|senha)[\"']?\\s*[:=]\\s*[\"']?([^\\s\"';,}{()=<>|&$`]{8,})"),
			refina: refinaGenerica,
		},
	}
}

// refinaJWT classifica um JWT pela role do payload: service_role é segredo
// crítico; anon/authenticated é publicável; o resto fica como suspeito. Anexa
// a role e marca se já expirou.
func refinaJWT(val, full string) (string, classe, string, bool) {
	j := analisarJWT(full)
	extra := ""
	if j.Valido {
		extra = "role=" + nvl(j.Role, "(sem role)")
		if j.Exp > 0 {
			if time.Now().Unix() > j.Exp {
				extra += " exp=expirado"
			} else {
				extra += " exp=válido"
			}
		}
	}
	switch j.Role {
	case "service_role":
		return sevCritico, classeSecreto, extra, true
	case "anon", "authenticated":
		return sevInfo, classePublicavel, extra, true
	default:
		return sevMedio, classeSuspeito, extra, true
	}
}

// placeholdersGenericos casa valores que claramente não são segredo real.
var placeholdersGenericos = regexp.MustCompile(`(?i)^(?:changeme|change_me|your_?[a-z_]*|example[a-z_]*|sample|placeholder|dummy|test\d*|xx+|\*+|\.\.\.+|null|none|undefined|true|false|senha|password|secret|token|\d+)$`)

// refinaGenerica descarta placeholders óbvios e valores de baixa variedade;
// o que sobra vira achado suspeito (precisa de conferência humana).
func refinaGenerica(val, full string) (string, classe, string, bool) {
	v := strings.Trim(val, `"'`)
	if len(v) < 8 || placeholdersGenericos.MatchString(v) || umCaractereSo(v) {
		return "", "", "", false
	}
	return sevMedio, classeSuspeito, "atribuição em código", true
}

func umCaractereSo(s string) bool {
	for _, r := range s {
		if r != rune(s[0]) {
			return false
		}
	}
	return true
}

func nvl(s, alt string) string {
	if s == "" {
		return alt
	}
	return s
}
