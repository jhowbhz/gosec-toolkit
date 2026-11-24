package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// Config é a forma validada da linha de comando. Como nas outras ferramentas,
// só confere a FORMA da entrada; ler o arquivo de hosts e falar com a rede
// são decisões de runtime e não acontecem aqui.
type Config struct {
	Hosts      []string // alvos HOST[:PORT], repetíveis
	HostsFile  string   // arquivo com um alvo por linha
	Port       int      // porta usada quando o alvo não traz porta
	ServerName string   // força o SNI; vazio = usa o host do alvo
	TimeoutS   int      // timeout por conexão, em segundos
	FullChain  bool     // reporta todos os certs da cadeia, não só a folha
	NoHSTS     bool     // não faz o GET de checagem de HSTS
	LogFile    string   // caminho do JSONL; vazio = automático
	Quiet      bool     // não imprime o resumo por host no console
}

const (
	defaultPort     = 443
	defaultTimeoutS = 15
)

// hostFlag coleta --host repetível num slice.
type hostFlag []string

func (h *hostFlag) String() string { return strings.Join(*h, ", ") }
func (h *hostFlag) Set(v string) error {
	*h = append(*h, v)
	return nil
}

// parseConfig lê os argumentos, monta a Config e valida a forma.
func parseConfig(args []string) (Config, error) {
	fs := flag.NewFlagSet("tlsscan", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // o chamador decide como reportar o erro

	var c Config
	var hosts hostFlag
	fs.Var(&hosts, "host", "host a escanear, HOST[:PORT] (repetível)")
	fs.StringVar(&c.HostsFile, "hosts-file", "", "arquivo com um host por linha")
	fs.IntVar(&c.Port, "port", defaultPort, "porta usada quando o host não traz porta")
	fs.StringVar(&c.ServerName, "servername", "", "força o SNI; vazio = o próprio host")
	fs.IntVar(&c.TimeoutS, "timeout", defaultTimeoutS, "timeout por conexão, em segundos")
	fs.BoolVar(&c.FullChain, "full-chain", false, "mostra todos os certs da cadeia (padrão: só a folha)")
	fs.BoolVar(&c.NoHSTS, "no-hsts", false, "não faz o GET de checagem de HSTS")
	fs.StringVar(&c.LogFile, "log-file", "", "caminho do JSONL; vazio = tlsscan-<ts>.jsonl")
	fs.BoolVar(&c.Quiet, "quiet", false, "não imprime o resumo por host no console")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	c.Hosts = hosts
	if err := c.validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// validate confere só a forma: alvo informado e números coerentes. Se o
// arquivo existe ou se o host responde só aparece no runtime.
func (c Config) validate() error {
	if len(c.Hosts) == 0 && strings.TrimSpace(c.HostsFile) == "" {
		return errors.New("--host ou --hosts-file é obrigatório (ex.: --host=meusite.com.br)")
	}
	for _, h := range c.Hosts {
		if strings.TrimSpace(h) == "" {
			return errors.New("--host não pode ser vazio")
		}
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("--port fora de 1..65535: %d", c.Port)
	}
	if c.TimeoutS < 1 {
		return fmt.Errorf("--timeout precisa ser >= 1: %d", c.TimeoutS)
	}
	return nil
}
