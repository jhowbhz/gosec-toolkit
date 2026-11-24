package main

import (
	"fmt"
	"strings"
)

// redact decide quanto de um segredo aparece no relatório. O padrão é mascarar
// — mostrar só as pontas e o tamanho — para que o próprio relatório não vire
// um depósito de segredos em claro. --show-full desliga o mascaramento.

// preview devolve o valor cru (showFull) ou a versão mascarada.
func preview(valor string, showFull bool) string {
	if showFull {
		return valor
	}
	return mascarar(valor)
}

// mascarar mostra as primeiras 8 e as últimas 4 posições do valor, com o
// tamanho ao final. Valores curtos aparecem mais escondidos, porque pontas
// demais de um valor curto já entregam o segredo.
func mascarar(v string) string {
	n := len(v)
	switch {
	case n <= 4:
		return strings.Repeat("•", n)
	case n <= 12:
		return v[:2] + "…" + v[n-2:] + fmt.Sprintf(" (len %d)", n)
	default:
		return v[:8] + "…" + v[n-4:] + fmt.Sprintf(" (len %d)", n)
	}
}
