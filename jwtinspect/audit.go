package main

import (
	"fmt"
	"strings"
	"time"
)

// Severidades, da pior para a mais leve.
const (
	sevCritical = "critical"
	sevMedium   = "medium"
	sevLow      = "low"
	sevInfo     = "info"
)

// vidaMuitoLonga separa "vida longa" (low) de "vida muito longa" (medium):
// além de 30 dias, um token vazado fica útil por tempo demais para ser só nota.
const vidaMuitoLonga = 30 * 24 * time.Hour

// issue é um achado da auditoria de um token.
type issue struct {
	Check    string `json:"check"`
	Severity string `json:"severity"`
	Detalhe  string `json:"detalhe"`
}

// claimTempo lê um claim NumericDate (segundos desde a época). JSON decodifica
// número como float64, que representa epoch em segundos sem perda.
func claimTempo(payload map[string]any, nome string) (time.Time, bool) {
	v, ok := payload[nome].(float64)
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(int64(v), 0), true
}

// situacao resume a janela de validade num rótulo para o JSONL.
func situacao(payload map[string]any, agora time.Time, leeway time.Duration) string {
	exp, temExp := claimTempo(payload, "exp")
	if !temExp {
		return "sem_exp"
	}
	if agora.After(exp.Add(leeway)) {
		return "expirado"
	}
	if nbf, ok := claimTempo(payload, "nbf"); ok && nbf.After(agora.Add(leeway)) {
		return "ainda_nao_valido"
	}
	return "valido"
}

// avaliarClaims audita o alg e a janela de validade. É pura (recebe "agora")
// para o teste não depender do relógio. maxLife == 0 desliga a checagem de
// vida longa.
func avaliarClaims(alg string, payload map[string]any, agora time.Time, leeway, maxLife time.Duration) []issue {
	var issues []issue

	if strings.EqualFold(alg, "none") {
		issues = append(issues, issue{"alg_none", sevCritical,
			"alg \"none\": o token não tem assinatura; qualquer um forja claims se o servidor aceitar"})
	}
	if algSimetrico(alg) {
		issues = append(issues, issue{"alg_simetrico", sevInfo,
			fmt.Sprintf("%s é simétrico: confirme que o verificador fixa o algoritmo e não aceita o alg do header (risco de alg-confusion com chave pública usada como segredo)", alg)})
	}

	exp, temExp := claimTempo(payload, "exp")
	if !temExp {
		issues = append(issues, issue{"exp_ausente", sevMedium,
			"sem claim exp: o token nunca expira sozinho"})
	} else if agora.After(exp.Add(leeway)) {
		issues = append(issues, issue{"expirado", sevInfo,
			fmt.Sprintf("expirou em %s (há %s)", exp.UTC().Format(time.RFC3339), agora.Sub(exp).Round(time.Second))})
	}

	if nbf, ok := claimTempo(payload, "nbf"); ok && nbf.After(agora.Add(leeway)) {
		issues = append(issues, issue{"nbf_futuro", sevInfo,
			fmt.Sprintf("ainda não é válido: nbf %s", nbf.UTC().Format(time.RFC3339))})
	}

	iat, temIat := claimTempo(payload, "iat")
	if temIat && iat.After(agora.Add(leeway)) {
		issues = append(issues, issue{"iat_futuro", sevLow,
			fmt.Sprintf("emitido no futuro (iat %s): relógio do emissor desalinhado ou claim forjado", iat.UTC().Format(time.RFC3339))})
	}

	if temExp && temIat && maxLife > 0 {
		if vida := exp.Sub(iat); vida > maxLife {
			sev := sevLow
			if vida > vidaMuitoLonga {
				sev = sevMedium
			}
			issues = append(issues, issue{"vida_longa", sev,
				fmt.Sprintf("vida de %s (exp - iat) acima do limite de %s", vida, maxLife)})
		}
	}

	return issues
}

// testarSegredos confere offline, contra a assinatura do PRÓPRIO token, cada
// candidato fornecido. Só HS*: sem alg simétrico não há segredo a adivinhar.
func testarSegredos(t token, candidatos []string) (bool, string) {
	alg := t.alg()
	if !algSimetrico(alg) {
		return false, ""
	}
	for _, c := range candidatos {
		if verificarHMAC(t.signingInput, t.assinatura, c, alg) {
			return true, c
		}
	}
	return false, ""
}
