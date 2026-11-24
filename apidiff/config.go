package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// Config é a forma validada da linha de comando. Como nas outras ferramentas,
// só confere a FORMA (campos presentes, extensão conhecida, enum válido); não
// abre os scans. Arquivo ausente ou banco corrompido só aparecem no runtime.
type Config struct {
	Old     string // scan antigo (.jsonl do download-dump ou .db do spyder)
	New     string // scan novo, no mesmo formato de entrada
	Format  string // text | jsonl
	Only    string // csv de tipos de mudança: added,removed,changed
	LogFile string // grava o jsonl também em arquivo (só com --format=jsonl)
	Quiet   bool   // no modo text, imprime só o resumo
}

const (
	formatoText  = "text"
	formatoJSONL = "jsonl"

	mudAdicionado = "added"
	mudRemovido   = "removed"
	mudAlterado   = "changed"
)

var formatosValidos = map[string]bool{formatoText: true, formatoJSONL: true}

var tiposDeMudanca = []string{mudAdicionado, mudRemovido, mudAlterado}

// parseConfig lê os argumentos, monta a Config e valida a forma.
func parseConfig(args []string) (Config, error) {
	fs := flag.NewFlagSet("apidiff", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // o chamador decide como reportar o erro

	var c Config
	fs.StringVar(&c.Old, "old", "", "scan antigo (.jsonl ou .db)")
	fs.StringVar(&c.New, "new", "", "scan novo (.jsonl ou .db)")
	fs.StringVar(&c.Format, "format", formatoText, "formato de saída: text | jsonl")
	fs.StringVar(&c.Only, "only", "", "filtra tipos de mudança, csv: added,removed,changed (vazio = todos)")
	fs.StringVar(&c.LogFile, "log-file", "", "grava o jsonl também neste arquivo (só com --format=jsonl)")
	fs.BoolVar(&c.Quiet, "quiet", false, "no modo text, imprime só o resumo")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if err := c.validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// validate confere só a forma. Não abre --old/--new: existência de arquivo é
// verdade fresca e pertence ao runtime.
func (c Config) validate() error {
	for _, f := range []struct{ flag, path string }{{"--old", c.Old}, {"--new", c.New}} {
		if strings.TrimSpace(f.path) == "" {
			return fmt.Errorf("%s é obrigatório (scan .jsonl do download-dump ou .db do spyder)", f.flag)
		}
		if _, err := tipoDoScan(f.path); err != nil {
			return fmt.Errorf("%s: %w", f.flag, err)
		}
	}
	if !formatosValidos[c.Format] {
		return fmt.Errorf("--format inválido: %q (quero text ou jsonl)", c.Format)
	}
	if c.LogFile != "" && c.Format != formatoJSONL {
		return errors.New("--log-file só faz sentido com --format=jsonl")
	}
	if _, err := c.onlySet(); err != nil {
		return err
	}
	return nil
}

// onlySet transforma --only no conjunto de tipos de mudança a mostrar.
// Vazio significa todos.
func (c Config) onlySet() (map[string]bool, error) {
	set := map[string]bool{}
	if strings.TrimSpace(c.Only) == "" {
		for _, t := range tiposDeMudanca {
			set[t] = true
		}
		return set, nil
	}
	validos := map[string]bool{}
	for _, t := range tiposDeMudanca {
		validos[t] = true
	}
	for _, item := range strings.Split(c.Only, ",") {
		item = strings.TrimSpace(item)
		if !validos[item] {
			return nil, fmt.Errorf("--only inválido: %q (quero added, removed ou changed)", item)
		}
		set[item] = true
	}
	return set, nil
}
