package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Config é a forma validada da linha de comando. Como nos irmãos, só confere a
// forma da entrada (campos presentes, bem-formados); rede e disco só no runtime.
type Config struct {
	Paths     []string // caminhos posicionais (arquivos/pastas) a varrer
	URLs      []string // --url repetível: URLs avulsas a buscar e varrer
	Stdin     bool     // --stdin: lê conteúdo da entrada padrão
	Headers   []string // --header repetível ("Nome: Valor")
	UserAgent string   // User-Agent das buscas
	Insecure  bool     // não valida o cert TLS ao buscar URLs
	Entropy   bool     // liga o detector genérico de alta entropia (ruidoso)
	ShowFull  bool     // mostra o valor inteiro do segredo (sem mascarar)
	OnlySec   bool     // reporta só achados da classe secreto
	Out       string   // arquivo-resumo; vazio = _CREDENCIAIS-ENCONTRADAS.txt
	LogFile   string   // manifesto JSONL; vazio = download automático
	Quiet     bool     // não imprime o relatório no console
}

const defaultUserAgent = "spyder-credentials/1.0 (+https://github.com/jhowbhz/spyder-credentials)"

// multiFlag coleta uma flag repetível num slice.
type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ", ") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

// parseConfig lê os argumentos, monta a Config e valida a forma.
func parseConfig(args []string) (Config, error) {
	fs := flag.NewFlagSet("spyder-credentials", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var c Config
	var urls, headers multiFlag
	fs.Var(&urls, "url", "URL avulsa a buscar e varrer (repetível)")
	fs.Var(&headers, "header", "header de request 'Nome: Valor' (repetível)")
	fs.BoolVar(&c.Stdin, "stdin", false, "lê conteúdo da entrada padrão")
	fs.StringVar(&c.UserAgent, "user-agent", defaultUserAgent, "User-Agent das buscas")
	fs.BoolVar(&c.Insecure, "insecure", false, "não valida o cert TLS ao buscar URLs")
	fs.BoolVar(&c.Entropy, "entropy", false, "liga o detector genérico de alta entropia (ruidoso)")
	fs.BoolVar(&c.ShowFull, "show-full", false, "mostra o valor inteiro do segredo (sem mascarar)")
	fs.BoolVar(&c.OnlySec, "only-secrets", false, "reporta só achados da classe 'secreto'")
	fs.StringVar(&c.Out, "out", "", "arquivo-resumo; vazio = _CREDENCIAIS-ENCONTRADAS.txt")
	fs.StringVar(&c.LogFile, "log-file", "", "manifesto JSONL; vazio = spyder-credentials-<ts>.jsonl")
	fs.BoolVar(&c.Quiet, "quiet", false, "não imprime o relatório no console")

	// Parse em laço para aceitar flags e caminhos em QUALQUER ordem: o flag do
	// Go para no primeiro posicional, então consumimos esse caminho e seguimos
	// parseando o resto. Assim `spyder dump/ --only-secrets` funciona.
	var paths []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return Config{}, err
		}
		rest = fs.Args()
		if len(rest) == 0 {
			break
		}
		paths = append(paths, rest[0])
		rest = rest[1:]
	}
	c.URLs = urls
	c.Headers = headers
	c.Paths = paths

	if err := c.validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// validate confere o que dá pra saber sem tocar rede/disco.
func (c Config) validate() error {
	if len(c.Paths) == 0 && len(c.URLs) == 0 && !c.Stdin {
		return errors.New("nada para varrer: passe um caminho, --url ou --stdin")
	}
	for _, u := range c.URLs {
		p, err := url.Parse(u)
		if err != nil || (p.Scheme != "http" && p.Scheme != "https") || p.Host == "" {
			return fmt.Errorf("--url precisa ter scheme http/https e host: %q", u)
		}
	}
	if _, err := c.parsedHeaders(); err != nil {
		return err
	}
	return nil
}

// parsedHeaders transforma os --header crus num http.Header.
func (c Config) parsedHeaders() (http.Header, error) {
	h := http.Header{}
	for _, raw := range c.Headers {
		nome, valor, ok := strings.Cut(raw, ":")
		if !ok || strings.TrimSpace(nome) == "" {
			return nil, fmt.Errorf("--header malformado (quero 'Nome: Valor'): %q", raw)
		}
		h.Add(strings.TrimSpace(nome), strings.TrimSpace(valor))
	}
	return h, nil
}
