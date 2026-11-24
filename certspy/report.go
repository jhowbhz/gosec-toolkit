package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// gravarTXT escreve um domínio por linha. mesclar já devolve ordenado, então
// o arquivo é estável entre execuções e serve de entrada para outras
// ferramentas (ex.: --url do download-dump).
func gravarTXT(path string, doms []dominio) error {
	return escrever(path, func(w *bufio.Writer) error {
		for _, d := range doms {
			fmt.Fprintln(w, d.Nome)
		}
		return nil
	})
}

// gravarJSONL escreve um objeto dominio por linha.
func gravarJSONL(path string, doms []dominio) error {
	return escrever(path, func(w *bufio.Writer) error {
		enc := json.NewEncoder(w)
		for _, d := range doms {
			if err := enc.Encode(d); err != nil {
				return err
			}
		}
		return nil
	})
}

// gravarMD escreve o relatório legível: cabeçalho, resumo e tabela.
func gravarMD(path string, doms []dominio, alvo []string, fontesAtivas []string, quando time.Time) error {
	return escrever(path, func(w *bufio.Writer) error {
		fmt.Fprintf(w, "# certspy — %s\n\n", strings.Join(alvo, ", "))
		fmt.Fprintf(w, "Gerado em %s. Fontes ativas: %s.\n\n", quando.Format("2006-01-02 15:04:05"), strings.Join(fontesAtivas, ", "))

		total, vivos, sobApex := 0, 0, 0
		porFonte := map[string]int{}
		for _, d := range doms {
			total++
			if d.Vivo {
				vivos++
			}
			if d.SobApex {
				sobApex++
			}
			for _, f := range d.Fontes {
				porFonte[f]++
			}
		}
		fmt.Fprintln(w, "## Resumo")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "- Total: %d\n- Vivos (A/AAAA): %d\n- Sob o apex do alvo: %d\n", total, vivos, sobApex)
		fmt.Fprintf(w, "- Por fonte: ct=%d, live=%d\n\n", porFonte[fonteCT], porFonte[fonteLive])

		fmt.Fprintln(w, "## Domínios")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "| domínio | vivo | fontes | IPs |")
		fmt.Fprintln(w, "|---|---|---|---|")
		for _, d := range doms {
			vivo := "não"
			if d.Vivo {
				vivo = "sim"
			}
			fmt.Fprintf(w, "| %s | %s | %s | %s |\n", d.Nome, vivo, strings.Join(d.Fontes, ", "), strings.Join(d.IPs, ", "))
		}
		return nil
	})
}

// escrever cria o arquivo, entrega um writer com buffer e só considera
// gravado se o flush e o close também deram certo.
func escrever(path string, f func(*bufio.Writer) error) error {
	arq, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("não consegui criar %s: %w", path, err)
	}
	w := bufio.NewWriter(arq)
	if err := f(w); err != nil {
		arq.Close()
		return fmt.Errorf("gravando %s: %w", path, err)
	}
	if err := w.Flush(); err != nil {
		arq.Close()
		return fmt.Errorf("gravando %s: %w", path, err)
	}
	return arq.Close()
}
