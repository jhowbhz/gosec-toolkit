package main

import (
	"strings"
	"testing"
)

func TestParseConfigPrecisaDeEntrada(t *testing.T) {
	_, err := parseConfig(nil)
	if err == nil {
		t.Fatal("sem caminho, --url ou --stdin deveria dar erro")
	}
}

func TestParseConfigStdinSozinhoOK(t *testing.T) {
	c, err := parseConfig([]string{"--stdin"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if !c.Stdin {
		t.Error("Stdin deveria estar ligado")
	}
}

func TestParseConfigPathPosicional(t *testing.T) {
	c, err := parseConfig([]string{"dump/"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if len(c.Paths) != 1 || c.Paths[0] != "dump/" {
		t.Errorf("Paths = %v, quero [dump/]", c.Paths)
	}
}

func TestParseConfigURLeFlags(t *testing.T) {
	c, err := parseConfig([]string{"--url=https://x.com/app.js", "--show-full", "--entropy", "dump"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if len(c.URLs) != 1 || !c.ShowFull || !c.Entropy {
		t.Errorf("flags não aplicadas: %+v", c)
	}
	if len(c.Paths) != 1 {
		t.Errorf("deveria ter pego o path posicional depois das flags: %v", c.Paths)
	}
}

func TestParseConfigFlagsDepoisDoPath(t *testing.T) {
	c, err := parseConfig([]string{"dump/", "--only-secrets", "--quiet"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if len(c.Paths) != 1 || c.Paths[0] != "dump/" {
		t.Errorf("Paths = %v, quero [dump/]", c.Paths)
	}
	if !c.OnlySec || !c.Quiet {
		t.Errorf("flags depois do caminho deveriam valer: %+v", c)
	}
}

func TestParseConfigPathsEFlagsIntercalados(t *testing.T) {
	c, err := parseConfig([]string{"a/", "--quiet", "b/", "--entropy"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if len(c.Paths) != 2 {
		t.Errorf("deveria pegar 2 caminhos intercalados: %v", c.Paths)
	}
	if !c.Quiet || !c.Entropy {
		t.Errorf("flags intercaladas não aplicadas: %+v", c)
	}
}

func TestParseConfigURLSemSchemeErro(t *testing.T) {
	_, err := parseConfig([]string{"--url=x.com"})
	if err == nil {
		t.Fatal("URL sem scheme deveria dar erro")
	}
}

func TestParseConfigHeaderMalformado(t *testing.T) {
	_, err := parseConfig([]string{"--stdin", "--header=semdoispontos"})
	if err == nil {
		t.Fatal("header sem ':' deveria dar erro")
	}
}

func TestParseConfigHeadersViramHTTP(t *testing.T) {
	c, err := parseConfig([]string{"--stdin", "--header=Authorization: Bearer x"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	h, err := c.parsedHeaders()
	if err != nil {
		t.Fatalf("parsedHeaders: %v", err)
	}
	if got := h.Get("Authorization"); got != "Bearer x" {
		t.Errorf("Authorization = %q", got)
	}
	_ = strings.TrimSpace
}
