package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// maxCorpoCT limita a leitura: domínios grandes devolvem dezenas de MB, mas
// um corpo sem teto deixaria um crt.sh defeituoso estourar a memória.
const maxCorpoCT = 128 << 20

// certEntry é um certificado do CT já reduzido ao que interessa: os nomes que
// ele cobre (name_value vem com vários, separados por \n) e quando passou a
// valer.
type certEntry struct {
	Nomes     []string
	NotBefore string
}

// parseCrtSh converte o JSON do crt.sh em certEntry. É pura (sem rede) para
// poder ser testada com amostra. O crt.sh devolve HTML ou vazio quando está
// sob carga; isso vira erro claro em vez de "invalid character '<'".
func parseCrtSh(corpo []byte) ([]certEntry, error) {
	corpo = []byte(strings.TrimSpace(string(corpo)))
	if len(corpo) == 0 {
		return nil, errors.New("crt.sh devolveu resposta vazia (serviço sob carga?)")
	}
	if corpo[0] != '[' {
		inicio := string(corpo)
		if len(inicio) > 60 {
			inicio = inicio[:60]
		}
		return nil, fmt.Errorf("crt.sh não devolveu JSON (serviço sob carga?): %q", inicio)
	}

	var brutos []struct {
		NameValue string `json:"name_value"`
		NotBefore string `json:"not_before"`
	}
	if err := json.Unmarshal(corpo, &brutos); err != nil {
		return nil, fmt.Errorf("JSON do crt.sh malformado: %w", err)
	}

	entradas := make([]certEntry, 0, len(brutos))
	for _, b := range brutos {
		var nomes []string
		for _, n := range strings.Split(b.NameValue, "\n") {
			if n = normalizarNome(n); nomeValido(n) {
				nomes = append(nomes, n)
			}
		}
		entradas = append(entradas, certEntry{Nomes: nomes, NotBefore: b.NotBefore})
	}
	return entradas, nil
}

// consultarCT faz um GET no crt.sh para a consulta dada e entrega o corpo ao
// parse. O fetch fica separado do parse para o parse ser testável sem rede.
func consultarCT(ctx context.Context, cli *http.Client, userAgent, consulta string) ([]certEntry, error) {
	q := url.Values{"q": {consulta}, "output": {"json"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://crt.sh/?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("crt.sh %q: %w", consulta, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("crt.sh %q: status %d", consulta, resp.StatusCode)
	}
	corpo, err := io.ReadAll(io.LimitReader(resp.Body, maxCorpoCT))
	if err != nil {
		return nil, fmt.Errorf("crt.sh %q: leitura: %w", consulta, err)
	}
	entradas, err := parseCrtSh(corpo)
	if err != nil {
		return nil, fmt.Errorf("crt.sh %q: %w", consulta, err)
	}
	return entradas, nil
}

// achadosDeCT achata os certificados em achados da fonte "ct".
func achadosDeCT(entradas []certEntry) []achado {
	var out []achado
	for _, e := range entradas {
		for _, n := range e.Nomes {
			out = append(out, achado{Nome: n, Fonte: fonteCT, NotBefore: e.NotBefore})
		}
	}
	return out
}
