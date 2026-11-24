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

// Config é a forma validada da linha de comando. Como nas outras ferramentas,
// só confere a FORMA da entrada (campos presentes, números dentro da faixa);
// não faz I/O nem rede — arquivo de corpo ilegível ou alvo fora do ar só
// aparecem no runtime.
type Config struct {
	URL         string   // endpoint medido, com scheme (ex.: https://api.meusite.com/v1/x)
	Method      string   // método HTTP dos requests
	Headers     []string // headers crus "Nome: Valor", repetíveis
	Body        string   // corpo inline (POST/PUT)
	BodyFile    string   // corpo lido de arquivo; exclusivo com Body
	Rate        int      // requests por segundo alvo, até tetoRate
	Ramp        bool     // sobe de 1/s até Rate em degraus
	RampSteps   int      // nº de degraus da rampa
	Total       int      // total de requests, até tetoTotal
	Duration    int      // para após N segundos; 0 = só o Total manda
	Concurrency int      // requests em voo ao mesmo tempo, até tetoConcurrency
	StopOn429   bool     // para no primeiro 429 (padrão ligado)
	Timeout     int      // timeout por request, em segundos
	Insecure    bool     // não valida o cert TLS do destino
	LogFile     string   // caminho do JSONL; vazio = automático
	Quiet       bool     // não imprime uma linha por request
}

// Tetos rígidos: existem para que a ferramenta, mesmo mal usada, continue
// sendo medição de quota e não vire gerador de carga. Não há flag para
// subi-los de propósito — quem precisar de mais deve usar ferramenta de carga
// própria, com outro nível de controle e consentimento.
const (
	tetoRate        = 50
	tetoTotal       = 5000
	tetoConcurrency = 20
)

const (
	defaultMethod      = "GET"
	defaultRate        = 1
	defaultTotal       = 60
	defaultConcurrency = 1
	defaultRampSteps   = 5
	defaultTimeout     = 15
)

// headerFlag coleta --header repetível num slice de strings cruas.
type headerFlag []string

func (h *headerFlag) String() string { return strings.Join(*h, ", ") }
func (h *headerFlag) Set(v string) error {
	*h = append(*h, v)
	return nil
}

// parseConfig lê os argumentos, monta a Config e valida a forma.
func parseConfig(args []string) (Config, error) {
	fs := flag.NewFlagSet("ratemeter", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // o chamador decide como reportar o erro

	var c Config
	var headers headerFlag
	var continuar429 bool
	fs.StringVar(&c.URL, "url", "", "endpoint medido, com scheme (ex.: https://api.site.com/v1/x)")
	fs.StringVar(&c.Method, "method", defaultMethod, "método HTTP")
	fs.Var(&headers, "header", "header de request 'Nome: Valor' (repetível)")
	fs.StringVar(&c.Body, "body", "", "corpo do request (POST/PUT)")
	fs.StringVar(&c.BodyFile, "body-file", "", "corpo do request lido de arquivo")
	fs.IntVar(&c.Rate, "rate", defaultRate, fmt.Sprintf("requests por segundo alvo (teto %d)", tetoRate))
	fs.BoolVar(&c.Ramp, "ramp", false, "sobe de 1/s até --rate em degraus")
	fs.IntVar(&c.RampSteps, "ramp-steps", defaultRampSteps, "nº de degraus da rampa")
	fs.IntVar(&c.Total, "total", defaultTotal, fmt.Sprintf("total de requests (teto %d)", tetoTotal))
	fs.IntVar(&c.Duration, "duration", 0, "para após N segundos; 0 = só --total manda")
	fs.IntVar(&c.Concurrency, "concurrency", defaultConcurrency, fmt.Sprintf("requests em voo em paralelo (teto %d)", tetoConcurrency))
	fs.BoolVar(&c.StopOn429, "stop-on-429", true, "para assim que receber 429 (padrão ligado)")
	fs.BoolVar(&continuar429, "continue-on-429", false, "desliga o stop-on-429 (só no SEU alvo, com cautela)")
	fs.IntVar(&c.Timeout, "timeout", defaultTimeout, "timeout por request, em segundos")
	fs.BoolVar(&c.Insecure, "insecure", false, "não valida o cert TLS do destino")
	fs.StringVar(&c.LogFile, "log-file", "", "caminho do JSONL; vazio = ratemeter-<ts>.jsonl")
	fs.BoolVar(&c.Quiet, "quiet", false, "não imprime uma linha por request")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	c.Headers = headers
	// --continue-on-429 vence --stop-on-429: é o opt-out explícito, e o
	// padrão seguro só cai quando alguém pede isso por extenso.
	if continuar429 {
		c.StopOn429 = false
	}
	c.Method = strings.ToUpper(strings.TrimSpace(c.Method))
	if err := c.validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// validate confere a verdade estática: o que dá pra saber sem tocar rede
// nem disco. Os tetos moram aqui para que um valor abusivo seja recusado
// antes de qualquer request sair.
func (c Config) validate() error {
	if strings.TrimSpace(c.URL) == "" {
		return errors.New("--url é obrigatório (endpoint medido, ex.: https://api.site.com/v1/x)")
	}
	if _, err := c.targetURL(); err != nil {
		return err
	}
	if c.Method == "" || strings.ContainsAny(c.Method, " \t") {
		return fmt.Errorf("--method inválido: %q", c.Method)
	}
	if c.Rate < 1 {
		return fmt.Errorf("--rate precisa ser >= 1: %d", c.Rate)
	}
	if c.Rate > tetoRate {
		return fmt.Errorf("--rate acima do teto de segurança %d: %d", tetoRate, c.Rate)
	}
	if c.Total < 1 {
		return fmt.Errorf("--total precisa ser >= 1: %d", c.Total)
	}
	if c.Total > tetoTotal {
		return fmt.Errorf("--total acima do teto de segurança %d: %d", tetoTotal, c.Total)
	}
	if c.Concurrency < 1 {
		return fmt.Errorf("--concurrency precisa ser >= 1: %d", c.Concurrency)
	}
	if c.Concurrency > tetoConcurrency {
		return fmt.Errorf("--concurrency acima do teto de segurança %d: %d", tetoConcurrency, c.Concurrency)
	}
	if c.RampSteps < 1 {
		return fmt.Errorf("--ramp-steps precisa ser >= 1: %d", c.RampSteps)
	}
	if c.Duration < 0 {
		return fmt.Errorf("--duration não pode ser negativa: %d", c.Duration)
	}
	if c.Timeout < 1 {
		return fmt.Errorf("--timeout precisa ser >= 1: %d", c.Timeout)
	}
	if c.Body != "" && c.BodyFile != "" {
		return errors.New("--body e --body-file são exclusivos: escolha um")
	}
	if _, err := c.parsedHeaders(); err != nil {
		return err
	}
	return nil
}

// targetURL devolve o endpoint como URL. Exige scheme http/https e host,
// porque sem eles não há para onde enviar.
func (c Config) targetURL() (*url.URL, error) {
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
