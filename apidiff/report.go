package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

// linhaJSONL é um objeto do modo jsonl: um por mudança, com `change` dizendo
// o tipo. Campos de entrada saem em added/removed; field/from/to só em changed.
type linhaJSONL struct {
	Change string `json:"change"`
	URL    string `json:"url"`
	Method string `json:"method,omitempty"`
	Status *int   `json:"status,omitempty"`
	Kind   string `json:"kind,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	Field  string `json:"field,omitempty"`
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
}

// imprimirTexto escreve as seções e o resumo. Seção sem itens some, para o
// relatório de scans quase idênticos caber numa tela. quiet corta tudo menos
// o resumo.
func imprimirTexto(w io.Writer, r resultado, quiet bool) {
	if !quiet {
		if len(r.Adicionados) > 0 {
			fmt.Fprintf(w, "ADICIONADOS (%d)\n", len(r.Adicionados))
			for _, e := range r.Adicionados {
				fmt.Fprintf(w, "  + %s\n", descreverEntrada(e))
			}
			fmt.Fprintln(w)
		}
		if len(r.Removidos) > 0 {
			fmt.Fprintf(w, "REMOVIDOS (%d)\n", len(r.Removidos))
			for _, e := range r.Removidos {
				fmt.Fprintf(w, "  - %s\n", descreverEntrada(e))
			}
			fmt.Fprintln(w)
		}
		if len(r.Alterados) > 0 {
			fmt.Fprintf(w, "ALTERADOS (%d)\n", len(r.Alterados))
			for _, m := range r.Alterados {
				fmt.Fprintf(w, "  ~ %s  %s: %s -> %s\n", rotulo(m.URL, m.Method), m.Campo, valorOuVazio(m.De), valorOuVazio(m.Para))
			}
			fmt.Fprintln(w)
		}
	}
	fmt.Fprintf(w, "resumo: %d adicionados, %d removidos, %d alterados\n",
		len(r.Adicionados), len(r.Removidos), len(r.Alterados))
}

// imprimirJSONL escreve um objeto por mudança, sem resumo: o stream precisa
// continuar sendo JSONL puro para quem consome com jq.
func imprimirJSONL(w io.Writer, r resultado) error {
	enc := json.NewEncoder(w)
	for _, e := range r.Adicionados {
		if err := enc.Encode(linhaDeEntrada(mudAdicionado, e)); err != nil {
			return err
		}
	}
	for _, e := range r.Removidos {
		if err := enc.Encode(linhaDeEntrada(mudRemovido, e)); err != nil {
			return err
		}
	}
	for _, m := range r.Alterados {
		l := linhaJSONL{Change: mudAlterado, URL: m.URL, Method: m.Method, Field: m.Campo, From: m.De, To: m.Para}
		if err := enc.Encode(l); err != nil {
			return err
		}
	}
	return nil
}

func linhaDeEntrada(tipo string, e entrada) linhaJSONL {
	status := e.Status
	return linhaJSONL{Change: tipo, URL: e.URL, Method: e.Method, Status: &status, Kind: e.Kind, SHA256: e.Hash}
}

// descreverEntrada resume uma entrada numa linha, omitindo o que o formato
// de origem não tem (kind no jsonl, por exemplo).
func descreverEntrada(e entrada) string {
	s := rotulo(e.URL, e.Method) + "  [" + strconv.Itoa(e.Status) + "]"
	if e.Kind != "" {
		s += "  " + e.Kind
	}
	return s
}

// rotulo junta método e URL; no jsonl não há método e sai só a URL.
func rotulo(url, method string) string {
	if method == "" {
		return url
	}
	return method + " " + url
}

func valorOuVazio(v string) string {
	if v == "" {
		return "(vazio)"
	}
	return v
}
