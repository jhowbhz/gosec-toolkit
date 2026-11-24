package main

import (
	"fmt"
	"strconv"
)

// registry descreve cada ferramenta e seus campos de forma declarativa. É a
// peça de segurança central: o painel SÓ monta argv a partir deste schema, e
// qualquer flag fora dele é rejeitada. Nada vindo do navegador vira flag solta.

// campo é um parâmetro de uma ferramenta exposto no formulário.
type campo struct {
	Flag       string   `json:"flag"`                 // nome da flag, sem "--"
	Label      string   `json:"label,omitempty"`      // rótulo no formulário
	Tipo       string   `json:"tipo"`                 // texto | bool | int | enum | run-ref
	Default    string   `json:"default,omitempty"`    // valor pré-preenchido
	Required   bool     `json:"required,omitempty"`   // obrigatório
	Repetivel  bool     `json:"repetivel,omitempty"`  // aceita vários valores
	Opcoes     []string `json:"opcoes,omitempty"`     // para tipo enum
	Ajuda      string   `json:"ajuda,omitempty"`      // texto de ajuda
	RunTipo    string   `json:"run_tipo,omitempty"`   // para run-ref: "dir" ou "arquivo"
	Posicional bool     `json:"posicional,omitempty"` // vai como argumento posicional (sem --flag)
}

// ferramenta é uma CLI que o painel sabe executar.
type ferramenta struct {
	ID         string  `json:"id"`
	Nome       string  `json:"nome"`
	Descricao  string  `json:"descricao"`
	Campos     []campo `json:"campos"`
	Posicional bool    `json:"posicional,omitempty"` // aceita caminhos posicionais (run-ref)

	// Campos abaixo não vão para o navegador: controlam a execução.
	Bin      string `json:"-"` // id do diretório/binário em tools/<bin>
	Lang     string `json:"-"` // "go" (padrão) ou "python"
	Script   string `json:"-"` // para python: arquivo do script
	SaidaDir string `json:"-"` // flag que recebe a pasta do run (ex.: "out")
	SaidaLog string `json:"-"` // flag do manifesto JSONL (ex.: "log-file")
}

// tiposValidos restringe o que um campo pode ser.
var tiposValidos = map[string]bool{"texto": true, "bool": true, "int": true, "enum": true, "run-ref": true}

// montarArgv transforma os valores vindos do formulário no argv validado da
// ferramenta — sem o binário e sem as flags de saída (injetadas pelo runner).
// Rejeita flag fora do schema, obrigatório ausente, int/enum inválido. Ordem
// segue a dos campos, para resultado determinístico.
func montarArgv(f ferramenta, valores map[string][]string) ([]string, error) {
	conhecidos := map[string]campo{}
	for _, c := range f.Campos {
		conhecidos[c.Flag] = c
	}
	for flag := range valores {
		if _, ok := conhecidos[flag]; !ok {
			return nil, fmt.Errorf("flag desconhecida para %q: %q", f.ID, flag)
		}
	}

	var argv, posicionais []string
	for _, c := range f.Campos {
		vals := naoVazios(valores[c.Flag])
		if len(vals) == 0 {
			if c.Required {
				return nil, fmt.Errorf("campo obrigatório ausente: --%s", c.Flag)
			}
			continue
		}
		if !c.Repetivel && len(vals) > 1 {
			return nil, fmt.Errorf("--%s não é repetível", c.Flag)
		}
		for _, v := range vals {
			if c.Posicional {
				posicionais = append(posicionais, v) // sem --flag, vai no fim
				continue
			}
			arg, err := formatarArg(c, v)
			if err != nil {
				return nil, err
			}
			if arg != "" {
				argv = append(argv, arg)
			}
		}
	}
	return append(argv, posicionais...), nil
}

// formatarArg valida um valor conforme o tipo e devolve o fragmento de argv
// ("--flag=valor", ou "--flag" para bool, ou "" quando o bool é falso).
func formatarArg(c campo, v string) (string, error) {
	switch c.Tipo {
	case "bool":
		switch v {
		case "true", "1", "on":
			return "--" + c.Flag, nil
		case "false", "0", "off", "":
			// Flag com default true precisa do explícito =false para desligar;
			// com default false, basta omitir.
			if c.Default == "true" {
				return "--" + c.Flag + "=false", nil
			}
			return "", nil
		default:
			return "", fmt.Errorf("--%s espera booleano, veio %q", c.Flag, v)
		}
	case "int":
		if _, err := strconv.Atoi(v); err != nil {
			return "", fmt.Errorf("--%s espera inteiro, veio %q", c.Flag, v)
		}
		return "--" + c.Flag + "=" + v, nil
	case "enum":
		if !contemStr(c.Opcoes, v) {
			return "", fmt.Errorf("--%s valor inválido %q (opções: %v)", c.Flag, v, c.Opcoes)
		}
		return "--" + c.Flag + "=" + v, nil
	case "texto", "run-ref":
		return "--" + c.Flag + "=" + v, nil
	default:
		return "", fmt.Errorf("tipo de campo desconhecido: %q", c.Tipo)
	}
}

func naoVazios(vs []string) []string {
	var out []string
	for _, v := range vs {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func contemStr(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
