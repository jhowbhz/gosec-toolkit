package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "apidiff:", err)
		os.Exit(1)
	}
}

// run é o corpo de main sem os.Exit: lê a config, carrega os dois scans,
// calcula o drift e imprime.
func run(args []string, stdout, stderr io.Writer) error {
	cfg, err := parseConfig(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, uso)
			return nil
		}
		return err
	}
	only, _ := cfg.onlySet() // já validado em parseConfig

	velho, err := carregar(cfg.Old)
	if err != nil {
		return err
	}
	novo, err := carregar(cfg.New)
	if err != nil {
		return err
	}

	r := diff(velho, novo, only)
	if cfg.Format == formatoText {
		imprimirTexto(stdout, r, cfg.Quiet)
		return nil
	}
	return emitirJSONL(cfg, r, stdout)
}

// emitirJSONL escreve no stdout e, se pedido, no --log-file ao mesmo tempo.
func emitirJSONL(cfg Config, r resultado, stdout io.Writer) error {
	w := stdout
	if cfg.LogFile != "" {
		f, err := os.Create(cfg.LogFile)
		if err != nil {
			return fmt.Errorf("não consegui criar %s: %w", cfg.LogFile, err)
		}
		defer f.Close()
		w = io.MultiWriter(stdout, f)
	}
	return imprimirJSONL(w, r)
}

const uso = `apidiff - compara dois scans e mostra o drift da superfície (artefatos que você já gerou)

uso:
  apidiff --old=<scan-antigo> --new=<scan-novo> [opções]

exemplo:
  apidiff --old=download-dump-20260101-100000.jsonl --new=download-dump-20260201-100000.jsonl

obrigatórias:
  --old=PATH        scan antigo: .jsonl (download-dump) ou .db (spyder)
  --new=PATH        scan novo, no mesmo formato

opções:
  --format=FMT      saída: text (padrão) | jsonl
  --only=LIST       tipos de mudança, csv: added,removed,changed (padrão: todos)
  --log-file=PATH   com --format=jsonl, grava também neste arquivo
  --quiet           no modo text, imprime só o resumo
`
