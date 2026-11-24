package main

import (
	"strings"
	"testing"
)

// A atribuição genérica é a mais ruidosa: em JS minificado ela casa trechos de
// código (arrow functions, operadores). Um valor de credencial de verdade não
// contém '(', ')', '=>', '||' etc., então esses devem ser descartados.
func TestGenericaIgnoraCodigoMinificado(t *testing.T) {
	txt := strings.Join([]string{
		`password=()=>ocupado()`,        // arrow function -> ignorar
		`token=aa||bb||cc||dd`,          // operadores -> ignorar
		`secret=a>b&&c<d&&eee`,          // comparação -> ignorar
		`client_secret="aB3xZ9kQ7mP2w"`, // token plausível -> reportar
	}, "\n")

	var reais []string
	for _, a := range varrer(txt, "min.js", detectoresPadrao(), false) {
		if a.Detector == "atribuicao-generica" {
			reais = append(reais, a.Valor)
		}
	}
	if len(reais) != 1 {
		t.Fatalf("esperava 1 atribuição real, veio %d: %v", len(reais), reais)
	}
	if !strings.Contains(reais[0], "aB3xZ9kQ") {
		t.Errorf("reportou código em vez do token real: %q", reais[0])
	}
}
