package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

// Config é a forma validada da linha de comando do spyder.
type Config struct {
	URL          string        // seed (ponto de partida)
	Out          string        // arquivo SQLite; vazio = nome automático
	Dashboard    string        // endereço da área web ao vivo
	Headers      headerList    // headers extras em toda request
	MaxPages     int           // teto de páginas renderizadas
	MaxDepth     int           // profundidade máx. a partir do seed
	Concurrency  int           // abas headless em paralelo
	Delay        time.Duration // espera entre requests (polidez)
	Scope        string        // same-host | subdomains
	IgnoreRobots bool          // ignora robots.txt (só no que é autorizado)
	ChromePath   string        // caminho do Chrome; vazio = auto-detecta
	Open         bool          // abre o dashboard no navegador ao iniciar
	ApiOnly      bool          // foco em API: só grava rotas de API, ignora front
	Forever      bool          // descoberta contínua por wordlist (nunca para)
	Fetch        bool          // baixa sozinho o catálogo de wordlists (cacheado)
	Wordlists    stringList    // arquivos de wordlist locais (repetível)
	WordlistURLs stringList    // URLs de wordlist pra baixar (repetível)
}

// stringList coleta flags de string repetidas (--wordlist, --wordlist-url).
type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error {
	v = strings.TrimSpace(v)
	if v != "" {
		*s = append(*s, v)
	}
	return nil
}

// headerKV e headerList: --header repetível, validado como "Nome: valor".
type headerKV struct{ Name, Value string }

type headerList []headerKV

func (h *headerList) String() string {
	nomes := make([]string, len(*h))
	for i, kv := range *h {
		nomes[i] = kv.Name
	}
	return strings.Join(nomes, ",")
}

func (h *headerList) Set(s string) error {
	name, value, ok := strings.Cut(s, ":")
	name = strings.TrimSpace(name)
	value = strings.TrimSpace(value)
	if !ok || name == "" {
		return fmt.Errorf(`esperado "Nome: valor", veio %q`, s)
	}
	*h = append(*h, headerKV{Name: name, Value: value})
	return nil
}

func parseConfig(args []string) (Config, error) {
	fs := flag.NewFlagSet("spyder", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var c Config
	fs.StringVar(&c.URL, "url", "", "ponto de partida (seed), com http:// ou https://")
	fs.StringVar(&c.Out, "out", "", "arquivo SQLite (padrão: spyder-<ts>.db)")
	fs.StringVar(&c.Dashboard, "dashboard", "127.0.0.1:8750", "endereço da área web ao vivo")
	fs.Var(&c.Headers, "header", `header extra em toda request, "Nome: valor" (repetível)`)
	fs.IntVar(&c.MaxPages, "max-pages", 300, "teto de páginas renderizadas")
	fs.IntVar(&c.MaxDepth, "max-depth", 4, "profundidade máxima a partir do seed")
	fs.IntVar(&c.Concurrency, "concurrency", 2, "abas headless em paralelo")
	fs.DurationVar(&c.Delay, "delay", 300*time.Millisecond, "espera entre requests (polidez)")
	fs.StringVar(&c.Scope, "scope", scopeSameHost, "escopo: same-host | subdomains")
	fs.BoolVar(&c.IgnoreRobots, "ignore-robots", false, "ignora robots.txt (só no que é SEU/autorizado)")
	fs.StringVar(&c.ChromePath, "chrome", "", "caminho do Chrome (senão auto-detecta)")
	fs.BoolVar(&c.Open, "open", false, "abre o dashboard no navegador ao iniciar")
	fs.BoolVar(&c.ApiOnly, "api-only", true, "foco em API: grava só rotas de API (--api-only=false inclui o front)")
	fs.BoolVar(&c.Forever, "forever", false, "descoberta contínua por wordlist: nunca para (Ctrl+C encerra)")
	fs.BoolVar(&c.Fetch, "fetch", true, "baixa sozinho o catálogo de wordlists (cacheado)")
	fs.Var(&c.Wordlists, "wordlist", "arquivo de wordlist local (repetível)")
	fs.Var(&c.WordlistURLs, "wordlist-url", "URL de wordlist pra baixar (repetível)")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if err := c.validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (c Config) validate() error {
	if strings.TrimSpace(c.URL) == "" {
		return errors.New("--url é obrigatório (ex.: https://site.com)")
	}
	if _, err := c.seedURL(); err != nil {
		return err
	}
	if c.Scope != scopeSameHost && c.Scope != scopeSubdomains {
		return fmt.Errorf("--scope deve ser %q ou %q, veio %q", scopeSameHost, scopeSubdomains, c.Scope)
	}
	if c.MaxPages < 1 || c.MaxDepth < 1 || c.Concurrency < 1 {
		return errors.New("--max-pages, --max-depth e --concurrency devem ser >= 1")
	}
	if c.Delay < 0 {
		return errors.New("--delay não pode ser negativo")
	}
	return nil
}

// seedURL interpreta --url exigindo scheme http/https e host.
func (c Config) seedURL() (*url.URL, error) {
	u, err := url.Parse(c.URL)
	if err != nil {
		return nil, fmt.Errorf("--url inválido: %v", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("--url precisa de http:// ou https://: %q", c.URL)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("--url sem host: %q", c.URL)
	}
	return u, nil
}
