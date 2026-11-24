package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config é a forma validada da linha de comando. Como nas outras ferramentas,
// só confere a FORMA da entrada (campos presentes, bem-formados); não lê
// arquivo nem toca rede — arquivo de URLs inexistente só aparece no runtime.
type Config struct {
	URLs        []string // URLs a auditar (--url repetível), com scheme
	URLsFile    string   // arquivo com uma URL por linha; vazio = não usa
	JSONL       string   // manifesto do download-dump de onde extrair URLs; vazio = não usa
	Headers     []string // headers crus "Nome: Valor" mandados em toda request
	Origin      string   // Origin de teste enviado no OPTIONS para o teste de CORS
	Concurrency int      // requests em paralelo
	DelayMS     int      // pausa mínima entre requests ao mesmo host, em ms
	TimeoutS    int      // timeout por request, em segundos
	Insecure    bool     // não valida o cert TLS do destino
	LogFile     string   // caminho do JSONL; vazio = automático
	Quiet       bool     // não imprime a linha por URL no console
}

const (
	defaultOrigin      = "https://attacker.example"
	defaultConcurrency = 4
	defaultTimeoutS    = 15
)

// listaFlag coleta uma flag repetível (--url, --header) num slice de strings.
type listaFlag []string

func (l *listaFlag) String() string { return strings.Join(*l, ", ") }
func (l *listaFlag) Set(v string) error {
	*l = append(*l, v)
	return nil
}

// parseConfig lê os argumentos, monta a Config e valida a forma.
func parseConfig(args []string) (Config, error) {
	fs := flag.NewFlagSet("headeraudit", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // o chamador decide como reportar o erro

	var c Config
	var urls, headers listaFlag
	fs.Var(&urls, "url", "URL a auditar, com scheme (repetível)")
	fs.StringVar(&c.URLsFile, "urls-file", "", "arquivo com uma URL por linha")
	fs.StringVar(&c.JSONL, "jsonl", "", "manifesto do download-dump de onde extrair URLs únicas")
	fs.Var(&headers, "header", "header extra 'Nome: Valor' em toda request (repetível)")
	fs.StringVar(&c.Origin, "origin", defaultOrigin, "Origin usado no teste de CORS")
	fs.IntVar(&c.Concurrency, "concurrency", defaultConcurrency, "requests em paralelo")
	fs.IntVar(&c.DelayMS, "delay", 0, "pausa mínima entre requests ao mesmo host, em ms")
	fs.IntVar(&c.TimeoutS, "timeout", defaultTimeoutS, "timeout por request, em segundos")
	fs.BoolVar(&c.Insecure, "insecure", false, "não valida o cert TLS do destino")
	fs.StringVar(&c.LogFile, "log-file", "", "caminho do JSONL; vazio = headeraudit-<ts>.jsonl")
	fs.BoolVar(&c.Quiet, "quiet", false, "não imprime a linha por URL no console")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	c.URLs = urls
	c.Headers = headers
	if err := c.validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// validate confere a verdade estática: o que dá pra saber sem tocar rede
// nem disco. Os caminhos de --urls-file/--jsonl só precisam estar
// preenchidos quando a flag foi passada; se o arquivo existe é problema
// do runtime, que enxerga o disco como ele está na hora de ler.
func (c Config) validate() error {
	if len(c.URLs) == 0 && c.URLsFile == "" && c.JSONL == "" {
		return errors.New("passe ao menos uma fonte de URLs: --url, --urls-file ou --jsonl")
	}
	for _, raw := range c.URLs {
		if err := validarURLAlvo(raw); err != nil {
			return fmt.Errorf("--url inválida: %w", err)
		}
	}
	if err := validarOrigin(c.Origin); err != nil {
		return err
	}
	if c.Concurrency < 1 {
		return fmt.Errorf("--concurrency precisa ser >= 1: %d", c.Concurrency)
	}
	if c.DelayMS < 0 {
		return fmt.Errorf("--delay não pode ser negativo: %d", c.DelayMS)
	}
	if c.TimeoutS < 1 {
		return fmt.Errorf("--timeout precisa ser >= 1: %d", c.TimeoutS)
	}
	if _, err := c.parsedHeaders(); err != nil {
		return err
	}
	return nil
}

// validarURLAlvo exige scheme http/https e host: sem eles não há request
// possível, e url.Parse aceita "exemplo.com.br" como caminho sem reclamar.
func validarURLAlvo(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%q: %v", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("precisa de scheme http:// ou https://: %q", raw)
	}
	if u.Host == "" {
		return fmt.Errorf("sem host: %q", raw)
	}
	return nil
}

// validarOrigin exige o formato de um Origin (scheme://host[:porta], sem
// caminho): é o que o navegador manda, e outro formato faria o servidor
// ignorar o teste de CORS em vez de refleti-lo.
func validarOrigin(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("--origin precisa ser scheme://host (ex.: %s): %q", defaultOrigin, raw)
	}
	if u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("--origin não leva caminho, query nem fragmento: %q", raw)
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

func (c Config) timeout() time.Duration { return time.Duration(c.TimeoutS) * time.Second }
func (c Config) delay() time.Duration   { return time.Duration(c.DelayMS) * time.Millisecond }
