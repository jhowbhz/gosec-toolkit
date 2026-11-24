package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// Config é a forma validada da linha de comando. Como no proxyreverse, só
// confere a FORMA da entrada (campos presentes, bem-formados); não faz I/O
// nem rede — robots.txt, destino fora do ar e disco só aparecem no runtime.
type Config struct {
	URL            string   // URL semente, com scheme (ex.: https://site.com/app)
	Out            string   // pasta de saída do espelho; vazio = nome automático
	Headers        []string // headers crus "Nome: Valor", repetíveis
	UserAgent      string   // User-Agent do crawler
	Scope          string   // host | domain | prefix — até onde segue links
	Depth          int      // profundidade máxima; -1 = ilimitado
	Concurrency    int      // workers paralelos
	DelayMS        int      // pausa mínima por host entre requests, em ms
	MaxBodyKB      int      // pula arquivos maiores que isso; 0 = sem limite
	Include        string   // regex: só segue/salva caminhos que casam
	Exclude        string   // regex: nunca segue/salva caminhos que casam
	IgnoreRobots   bool     // ignora robots.txt (só em alvo autorizado)
	Insecure       bool     // não valida o cert TLS do destino
	ProbeSensitive bool     // sonda ativa de caminhos sensíveis comuns
	LogFile        string   // caminho do manifesto JSONL; vazio = automático
	Quiet          bool     // não imprime o resumo por recurso no console
}

const (
	defaultUserAgent   = "download-dump/1.0 (+https://github.com/jhowbhz/download-dump)"
	defaultConcurrency = 4
	defaultScope       = "host"
)

// escoposValidos são as três formas de delimitar o crawl.
var escoposValidos = map[string]bool{"host": true, "domain": true, "prefix": true}

// headerFlag coleta --header repetível num slice de strings cruas.
type headerFlag []string

func (h *headerFlag) String() string { return strings.Join(*h, ", ") }
func (h *headerFlag) Set(v string) error {
	*h = append(*h, v)
	return nil
}

// parseConfig lê os argumentos, monta a Config e valida a forma.
func parseConfig(args []string) (Config, error) {
	fs := flag.NewFlagSet("download-dump", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // o chamador decide como reportar o erro

	var c Config
	var headers headerFlag
	fs.StringVar(&c.URL, "url", "", "URL semente com scheme (ex.: https://site.com/app)")
	fs.StringVar(&c.Out, "out", "", "pasta de saída do espelho; vazio = dump-<host>-<ts>")
	fs.Var(&headers, "header", "header de request 'Nome: Valor' (repetível)")
	fs.StringVar(&c.UserAgent, "user-agent", defaultUserAgent, "User-Agent do crawler")
	fs.StringVar(&c.Scope, "scope", defaultScope, "até onde segue links: host | domain | prefix")
	fs.IntVar(&c.Depth, "depth", -1, "profundidade máxima de links; -1 = ilimitado")
	fs.IntVar(&c.Concurrency, "concurrency", defaultConcurrency, "workers paralelos")
	fs.IntVar(&c.DelayMS, "delay", 0, "pausa mínima por host entre requests, em ms (robots Crawl-delay vence se maior)")
	fs.IntVar(&c.MaxBodyKB, "max-body", 0, "pula arquivos maiores que isso em KB; 0 = sem limite")
	fs.StringVar(&c.Include, "include", "", "regex: só segue/salva caminhos que casam")
	fs.StringVar(&c.Exclude, "exclude", "", "regex: nunca segue/salva caminhos que casam")
	fs.BoolVar(&c.IgnoreRobots, "ignore-robots", false, "ignora robots.txt (use só em alvo autorizado)")
	fs.BoolVar(&c.Insecure, "insecure", false, "não valida o cert TLS do destino")
	fs.BoolVar(&c.ProbeSensitive, "probe-sensitive", false, "sonda ativa de caminhos sensíveis comuns (não baixa, só avisa)")
	fs.StringVar(&c.LogFile, "log-file", "", "caminho do manifesto JSONL; vazio = download-dump-<ts>.jsonl")
	fs.BoolVar(&c.Quiet, "quiet", false, "não imprime o resumo por recurso no console")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	c.Headers = headers
	if err := c.validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// validate confere a verdade estática: o que dá pra saber sem tocar rede
// nem disco (campo presente, scheme conhecido, enum válido, regex compila).
func (c Config) validate() error {
	if strings.TrimSpace(c.URL) == "" {
		return errors.New("--url é obrigatório (URL semente, ex.: https://site.com)")
	}
	if _, err := c.seedURL(); err != nil {
		return err
	}
	if !escoposValidos[c.Scope] {
		return fmt.Errorf("--scope inválido: %q (quero host, domain ou prefix)", c.Scope)
	}
	if c.Concurrency < 1 {
		return fmt.Errorf("--concurrency precisa ser >= 1: %d", c.Concurrency)
	}
	if c.Depth < -1 {
		return fmt.Errorf("--depth não pode ser < -1: %d", c.Depth)
	}
	if c.DelayMS < 0 {
		return fmt.Errorf("--delay não pode ser negativo: %d", c.DelayMS)
	}
	if c.MaxBodyKB < 0 {
		return fmt.Errorf("--max-body não pode ser negativo: %d", c.MaxBodyKB)
	}
	if _, err := c.parsedHeaders(); err != nil {
		return err
	}
	for _, r := range []struct{ flag, expr string }{{"--include", c.Include}, {"--exclude", c.Exclude}} {
		if r.expr == "" {
			continue
		}
		if _, err := regexp.Compile(r.expr); err != nil {
			return fmt.Errorf("%s não é uma regex válida: %v", r.flag, err)
		}
	}
	return nil
}

// seedURL devolve a semente como URL. Exige scheme http/https e host,
// porque sem eles não há de onde começar o crawl.
func (c Config) seedURL() (*url.URL, error) {
	u, err := url.Parse(c.URL)
	if err != nil {
		return nil, fmt.Errorf("--url inválida: %v", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("--url precisa de scheme http:// ou https://: %q", c.URL)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("--url sem host: %q", c.URL)
	}
	return u, nil
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

// includeRe e excludeRe devolvem as regex já compiladas (ou nil se vazias).
// validate garante que compilam, então MustCompile aqui é seguro.
func (c Config) includeRe() *regexp.Regexp {
	if c.Include == "" {
		return nil
	}
	return regexp.MustCompile(c.Include)
}

func (c Config) excludeRe() *regexp.Regexp {
	if c.Exclude == "" {
		return nil
	}
	return regexp.MustCompile(c.Exclude)
}

// maxBodyBytes é o teto por arquivo em bytes; 0 = sem limite.
func (c Config) maxBodyBytes() int64 {
	return int64(c.MaxBodyKB) * 1024
}
