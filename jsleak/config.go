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

// Config é a forma validada da linha de comando. Como no download-dump, só
// confere a FORMA da entrada (fonte presente, URL com scheme, header
// bem-formado); não faz I/O nem rede — se o diretório existe ou se o host
// responde só aparece no runtime.
type Config struct {
	Dirs         []string // diretórios varridos recursivamente
	Files        []string // arquivos varridos um a um
	URLs         []string // URLs baixadas via GET passivo e varridas
	Headers      []string // headers crus "Nome: Valor" para as requests de --url
	Ext          string   // extensões varridas em --dir, separadas por vírgula
	MaxBodyKB    int      // ignora arquivo/resposta maior que isso; 0 = sem limite
	ContextChars int      // chars de contexto de cada lado do match
	Redact       bool     // redige o segredo na saída (padrão ligado)
	NoRedact     bool     // mostra o match inteiro (só no SEU alvo)
	TimeoutSec   int      // timeout por request de --url, em segundos
	Insecure     bool     // não valida o cert TLS do destino nas URLs
	LogFile      string   // caminho do manifesto JSONL; vazio = automático
	Quiet        bool     // não imprime cada achado no console
}

const (
	defaultExt        = "js,html,json,txt,css,map,env,xml,yml,yaml,ts"
	defaultContext    = 40
	defaultTimeoutSec = 15
)

// listaFlag coleta uma flag repetível (--dir, --file, --url, --header)
// num slice de strings cruas.
type listaFlag []string

func (l *listaFlag) String() string { return strings.Join(*l, ", ") }
func (l *listaFlag) Set(v string) error {
	*l = append(*l, v)
	return nil
}

// parseConfig lê os argumentos, monta a Config e valida a forma.
func parseConfig(args []string) (Config, error) {
	fs := flag.NewFlagSet("jsleak", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // o chamador decide como reportar o erro

	var c Config
	var dirs, files, urls, headers listaFlag
	fs.Var(&dirs, "dir", "diretório varrido recursivamente (repetível)")
	fs.Var(&files, "file", "arquivo varrido (repetível)")
	fs.Var(&urls, "url", "URL baixada via GET passivo e varrida (repetível)")
	fs.Var(&headers, "header", "header de request 'Nome: Valor' para --url (repetível)")
	fs.StringVar(&c.Ext, "ext", defaultExt, "extensões varridas em --dir, separadas por vírgula")
	fs.IntVar(&c.MaxBodyKB, "max-body", 0, "ignora arquivo/resposta maior que isso em KB; 0 = sem limite")
	fs.IntVar(&c.ContextChars, "context", defaultContext, "chars de contexto em volta do match")
	fs.BoolVar(&c.Redact, "redact", true, "redige o segredo na saída (padrão ligado)")
	fs.BoolVar(&c.NoRedact, "no-redact", false, "mostra o match inteiro (só no seu alvo)")
	fs.IntVar(&c.TimeoutSec, "timeout", defaultTimeoutSec, "timeout por request de --url, em segundos")
	fs.BoolVar(&c.Insecure, "insecure", false, "não valida o cert TLS do destino nas URLs")
	fs.StringVar(&c.LogFile, "log-file", "", "caminho do manifesto JSONL; vazio = jsleak-<ts>.jsonl")
	fs.BoolVar(&c.Quiet, "quiet", false, "não imprime cada achado no console")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	c.Dirs, c.Files, c.URLs, c.Headers = dirs, files, urls, headers
	if c.NoRedact {
		c.Redact = false // --no-redact vence o padrão ligado
	}
	if err := c.validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// validate confere a verdade estática: o que dá pra saber sem tocar rede
// nem disco (fonte presente, scheme conhecido, header bem-formado, números
// não-negativos).
func (c Config) validate() error {
	if len(c.Dirs)+len(c.Files)+len(c.URLs) == 0 {
		return errors.New("preciso de pelo menos uma fonte: --dir, --file ou --url")
	}
	for _, raw := range c.URLs {
		if err := validarURLAlvo(raw); err != nil {
			return err
		}
	}
	if _, err := c.parsedHeaders(); err != nil {
		return err
	}
	if c.MaxBodyKB < 0 {
		return fmt.Errorf("--max-body não pode ser negativo: %d", c.MaxBodyKB)
	}
	if c.ContextChars < 0 {
		return fmt.Errorf("--context não pode ser negativo: %d", c.ContextChars)
	}
	if c.TimeoutSec < 0 {
		return fmt.Errorf("--timeout não pode ser negativo: %d", c.TimeoutSec)
	}
	return nil
}

// validarURLAlvo exige scheme http/https e host: sem eles não há o que baixar.
func validarURLAlvo(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("--url inválida: %v", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("--url precisa de scheme http:// ou https://: %q", raw)
	}
	if u.Host == "" {
		return fmt.Errorf("--url sem host: %q", raw)
	}
	return nil
}

// parsedHeaders transforma os --header crus num http.Header. Cada um precisa
// ter um ':' separando nome e valor; o valor pode conter ':' (ex.: URL).
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

// extensoes devolve o conjunto de extensões de --ext, normalizadas
// (minúsculas, sem ponto). Normalizar aqui deixa "JS", ".js" e "js" iguais.
func (c Config) extensoes() map[string]bool {
	set := map[string]bool{}
	for _, e := range strings.Split(c.Ext, ",") {
		e = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(e), "."))
		if e != "" {
			set[e] = true
		}
	}
	return set
}

// maxBodyBytes é o teto por arquivo/resposta em bytes; 0 = sem limite.
func (c Config) maxBodyBytes() int64 {
	return int64(c.MaxBodyKB) * 1024
}
