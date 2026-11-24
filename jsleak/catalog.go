package main

import (
	"regexp"
	"sync"
)

// Severidades das regras, da mais grave à menos grave.
const (
	sevCritical = "critical"
	sevHigh     = "high"
	sevMedium   = "medium"
	sevLow      = "low"
)

// ordemSeveridades fixa a ordem do resumo final: o que dói mais aparece
// primeiro.
var ordemSeveridades = []string{sevCritical, sevHigh, sevMedium, sevLow}

// regra é uma entrada do catálogo: um padrão determinístico que reconhece
// um tipo de segredo ou endpoint vazado.
type regra struct {
	Nome       string
	Severidade string
	Re         *regexp.Regexp
	Nota       string // o que o achado significa e o que fazer, em pt-BR
}

// defRegra é a forma "crua" (padrão ainda como texto) usada só para declarar
// o catálogo; compilarCatalogo a transforma em regra.
type defRegra struct {
	nome, severidade, padrao, nota string
}

// Trechos base64url de "service_role" nos três alinhamentos possíveis dentro
// do payload de um JWT. O JSON vem codificado, então "service_role" não
// aparece em texto puro; sem cobrir os três alinhamentos a regra falharia
// conforme o que vem antes no payload.
const serviceRoleB64 = `(?:c2VydmljZV9yb2xl|cnZpY2Vfcm9s|ZXJ2aWNlX3Jv)`

// definicoes é o catálogo. Padrões em RE2 (sem lookahead): ancorados em
// prefixos de formato fixo sempre que o provedor tem um, porque é isso que
// mantém o falso-positivo baixo. As genéricas (atribuição, Bearer) ficam
// com severidade menor pelo mesmo motivo.
var definicoes = []defRegra{
	{"aws-access-key", sevCritical, `\bAKIA[0-9A-Z]{16}\b`,
		"Chave de acesso AWS; revogue e gere outra, depois audite o CloudTrail."},
	{"google-api-key", sevHigh, `AIza[0-9A-Za-z\-_]{35}`,
		"Chave de API do Google; confira se tem restrição de origem/API no console."},
	{"github-token", sevCritical, `\bgh[pousr]_[0-9A-Za-z]{30,}`,
		"Token do GitHub; revogue em Settings > Developer settings."},
	{"slack-token", sevHigh, `\bxox[baprs]-[0-9A-Za-z-]{10,}`,
		"Token do Slack; revogue e reinstale o app."},
	{"jwt", sevMedium, `eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`,
		"JWT embutido; decodifique o payload para ver escopo e expiração."},
	{"private-key", sevCritical, `-----BEGIN [A-Z ]*PRIVATE KEY-----`,
		"Cabeçalho de chave privada; o corpo da chave está nas linhas seguintes. Troque o par de chaves."},
	{"bearer-token", sevHigh, `(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{20,}`,
		"Token Bearer em header; confira se ainda é válido e a quem pertence."},
	{"connection-string", sevCritical, `(?i)\b(?:postgres(?:ql)?|mysql|mongodb(?:\+srv)?|redis|amqp)://[^:\s/]+:[^@\s]+@`,
		"String de conexão com senha embutida; troque a senha e tire do código."},
	{"url-basic-auth", sevHigh, `https?://[^:\s/]+:[^@\s/]+@`,
		"Usuário e senha embutidos em URL; troque a credencial."},
	{"generic-secret", sevLow, `(?i)(?:api[_-]?key|secret|token|password|passwd|authorization)["'\s:=]+[A-Za-z0-9._\-]{8,}`,
		"Atribuição com cara de segredo; pode ser falso-positivo, confira o valor."},
	{"supabase-key", sevMedium, `\bsb[a-z]*_[A-Za-z0-9_]{20,}`,
		"Chave Supabase (publishable/secret); a secret nunca deve ir ao cliente."},
	{"supabase-service-role", sevCritical, `eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*` + serviceRoleB64 + `[A-Za-z0-9_-]*\.[A-Za-z0-9_-]+`,
		"JWT com role service_role: ignora RLS e dá acesso total ao banco. Rotacione o JWT secret."},
}

var (
	catalogoOnce sync.Once
	catalogoVal  []regra
)

// catalogo compila as definições uma única vez e devolve as regras. Padrão
// inválido é erro de programação, não de entrada: MustCompile derruba no
// primeiro uso (e o teste do catálogo pega antes de chegar ao usuário).
func catalogo() []regra {
	catalogoOnce.Do(func() {
		catalogoVal = compilarCatalogo(definicoes)
	})
	return catalogoVal
}

func compilarCatalogo(defs []defRegra) []regra {
	out := make([]regra, 0, len(defs))
	for _, d := range defs {
		out = append(out, regra{
			Nome:       d.nome,
			Severidade: d.severidade,
			Re:         regexp.MustCompile(d.padrao),
			Nota:       d.nota,
		})
	}
	return out
}
