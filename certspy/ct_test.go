package main

import (
	"reflect"
	"strings"
	"testing"
)

const amostraCrtSh = `[
 {"id":1,"name_value":"alvo.com\nwww.alvo.com","not_before":"2024-03-01T10:00:00"},
 {"id":2,"name_value":"*.api.alvo.com\nAPI.alvo.com\ndono@alvo.com","not_before":"2023-07-15T08:30:00"}
]`

func TestParseCrtShNomesEDatas(t *testing.T) {
	got, err := parseCrtSh([]byte(amostraCrtSh))
	if err != nil {
		t.Fatalf("parseCrtSh: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("quero 2 certificados, vieram %d", len(got))
	}
	if want := []string{"alvo.com", "www.alvo.com"}; !reflect.DeepEqual(got[0].Nomes, want) {
		t.Errorf("cert 1 nomes = %v, quero %v", got[0].Nomes, want)
	}
	// wildcard perde o "*.", a caixa baixa e o e-mail é descartado
	if want := []string{"api.alvo.com", "api.alvo.com"}; !reflect.DeepEqual(got[1].Nomes, want) {
		t.Errorf("cert 2 nomes = %v, quero %v", got[1].Nomes, want)
	}
	if got[1].NotBefore != "2023-07-15T08:30:00" {
		t.Errorf("not_before = %q", got[1].NotBefore)
	}
}

func TestParseCrtShToleraNaoJSON(t *testing.T) {
	for nome, corpo := range map[string]string{
		"vazio": "  \n",
		"html":  "<html><body>502 Bad Gateway</body></html>",
	} {
		_, err := parseCrtSh([]byte(corpo))
		if err == nil {
			t.Errorf("%s: esperava erro", nome)
			continue
		}
		if !strings.Contains(err.Error(), "crt.sh") {
			t.Errorf("%s: erro deveria citar crt.sh: %v", nome, err)
		}
	}
}

func TestParseCrtShJSONQuebrado(t *testing.T) {
	if _, err := parseCrtSh([]byte(`[{"name_value":`)); err == nil {
		t.Fatal("esperava erro com JSON truncado")
	}
}
