package main

import (
	"strings"
	"testing"
)

func TestMascararMantemPontasEEscondeMeio(t *testing.T) {
	v := "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9ABCDEFGHxcFAw"
	m := mascarar(v)
	if !strings.HasPrefix(m, "eyJhbGci") {
		t.Errorf("deveria começar com as primeiras 8: %q", m)
	}
	if !strings.Contains(m, "…") {
		t.Errorf("deveria ter reticências: %q", m)
	}
	if !strings.HasSuffix(strings.Split(m, " ")[0], "xcFAw"[1:]) {
		// termina com as últimas 4 antes do " (len"
		t.Errorf("deveria terminar com as últimas 4: %q", m)
	}
	if strings.Contains(m, "IsInR5cCI6") {
		t.Errorf("o miolo não pode aparecer: %q", m)
	}
	if !strings.Contains(m, "len ") {
		t.Errorf("deveria indicar o tamanho: %q", m)
	}
}

func TestPreviewRespeitaShowFull(t *testing.T) {
	v := "sk_live_0123456789ABCDEFGHIJ"
	if preview(v, true) != v {
		t.Error("com showFull=true deveria devolver o valor inteiro")
	}
	if preview(v, false) == v {
		t.Error("com showFull=false deveria mascarar")
	}
}
