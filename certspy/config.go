package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// Config é a forma validada da linha de comando. Como nas outras ferramentas,
// só confere a FORMA da entrada; crt.sh fora do ar, host que não responde e
// destino do publish inalcançável só aparecem no runtime.
type Config struct {
	Domains      []string // domínios-alvo consultados no CT, repetível
	Hosts        []string // HOST[:PORT] de onde extrair SANs ao vivo, repetível
	NoCT         bool     // não consulta o crt.sh
	NoLive       bool     // não faz handshake ao vivo
	Resolve      bool     // resolve DNS e marca vivos (padrão ligado)
	NoResolve    bool     // desliga a resolução DNS; vence --resolve
	Port         int      // porta do handshake quando o host não traz porta
	Concurrency  int      // resoluções/handshakes em paralelo
	TimeoutS     int      // timeout por request/conexão, em segundos
	OutTXT       string   // lista .txt; vazio = nome automático
	OutJSONL     string   // .jsonl com metadados; vazio = nome automático
	OutMD        string   // relatório .md; vazio = nome automático
	NoMD         bool     // não gera o markdown
	PublishURL   string   // se vazio, nada é publicado
	PublishToken string   // valor do header Authorization no publish
	PublishAll   bool     // inclui no publish nomes fora do apex do alvo
	Insecure     bool     // não valida TLS no handshake ao vivo e no publish
	UserAgent    string   // User-Agent das requests HTTP
	Quiet        bool     // não imprime o progresso por domínio
}

const (
	defaultUserAgent   = "certspy/1.0 (+https://github.com/jhowbhz/certspy)"
	defaultPort        = 443
	defaultConcurrency = 8
	defaultTimeoutS    = 20 // crt.sh costuma ser lento
	maxConcurrency     = 64
)

// listaFlag coleta uma flag repetível num slice de strings.
type listaFlag []string

func (l *listaFlag) String() string { return strings.Join(*l, ", ") }
func (l *listaFlag) Set(v string) error {
	*l = append(*l, v)
	return nil
}

// parseConfig lê os argumentos, monta a Config e valida a forma.
func parseConfig(args []string) (Config, error) {
	fs := flag.NewFlagSet("certspy", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // o chamador decide como reportar o erro

	var c Config
	var domains, hosts listaFlag
	fs.Var(&domains, "domain", "domínio-alvo para consultar no CT (repetível)")
	fs.Var(&hosts, "host", "HOST[:PORT] para extrair SANs do cert ao vivo (repetível)")
	fs.BoolVar(&c.NoCT, "no-ct", false, "não consulta o crt.sh (só SANs ao vivo)")
	fs.BoolVar(&c.NoLive, "no-live", false, "não faz handshake ao vivo (só CT)")
	fs.BoolVar(&c.Resolve, "resolve", true, "resolve DNS e marca quais nomes estão vivos")
	fs.BoolVar(&c.NoResolve, "no-resolve", false, "desliga a resolução DNS")
	fs.IntVar(&c.Port, "port", defaultPort, "porta do handshake ao vivo quando o host não traz porta")
	fs.IntVar(&c.Concurrency, "concurrency", defaultConcurrency, "resoluções/handshakes em paralelo (1..64)")
	fs.IntVar(&c.TimeoutS, "timeout", defaultTimeoutS, "timeout por request/conexão, em segundos")
	fs.StringVar(&c.OutTXT, "out-txt", "", "lista .txt; vazio = certspy-<ts>.txt")
	fs.StringVar(&c.OutJSONL, "out-jsonl", "", ".jsonl com metadados; vazio = certspy-<ts>.jsonl")
	fs.StringVar(&c.OutMD, "out-md", "", "relatório .md; vazio = certspy-<ts>.md")
	fs.BoolVar(&c.NoMD, "no-md", false, "não gera o markdown")
	fs.StringVar(&c.PublishURL, "publish-url", "", "endpoint que recebe o resultado por POST (vazio = não publica)")
	fs.StringVar(&c.PublishToken, "publish-token", "", "valor do header Authorization no publish")
	fs.BoolVar(&c.PublishAll, "publish-all", false, "inclui no publish nomes fora do apex do alvo")
	fs.BoolVar(&c.Insecure, "insecure", false, "não valida TLS no handshake ao vivo e no publish")
	fs.StringVar(&c.UserAgent, "user-agent", defaultUserAgent, "User-Agent das requests HTTP")
	fs.BoolVar(&c.Quiet, "quiet", false, "não imprime o progresso por domínio")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	c.Domains = domains
	c.Hosts = hosts
	if err := c.validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// validate confere só a forma: o que dá pra saber sem rede nem disco.
func (c Config) validate() error {
	if len(c.Domains) == 0 && len(c.Hosts) == 0 {
		return errors.New("preciso de uma fonte: --domain=<dominio> e/ou --host=<host[:porta]>")
	}
	if c.NoCT && c.NoLive {
		return errors.New("sem fonte: --no-ct e --no-live juntos não deixam nada pra fazer")
	}
	// Fonte ligada mas sem entrada (ex.: só --domain com --no-ct) também não
	// teria o que fazer.
	if (c.NoCT || len(c.Domains) == 0) && (c.NoLive || len(c.Hosts) == 0) {
		return errors.New("nenhuma fonte ativa tem entrada: --domain alimenta o CT e --host alimenta o handshake ao vivo")
	}
	for _, d := range c.Domains {
		if n := normalizarNome(d); !nomeValido(n) || strings.Contains(n, ":") {
			return fmt.Errorf("--domain inválido (quero só o nome, ex.: meusite.com.br): %q", d)
		}
	}
	for _, h := range c.Hosts {
		if strings.TrimSpace(h) == "" || strings.Contains(h, "://") {
			return fmt.Errorf("--host inválido (quero host ou host:porta, sem scheme): %q", h)
		}
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("--port fora de 1..65535: %d", c.Port)
	}
	if c.Concurrency < 1 || c.Concurrency > maxConcurrency {
		return fmt.Errorf("--concurrency fora de 1..%d: %d", maxConcurrency, c.Concurrency)
	}
	if c.TimeoutS < 1 {
		return fmt.Errorf("--timeout precisa ser >= 1: %d", c.TimeoutS)
	}
	if c.PublishURL != "" {
		u, err := url.Parse(c.PublishURL)
		if err != nil {
			return fmt.Errorf("--publish-url inválida: %v", err)
		}
		if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("--publish-url precisa de scheme http:// ou https:// e host: %q", c.PublishURL)
		}
	}
	return nil
}

// resolverDNS junta --resolve e --no-resolve: o "não" sempre vence.
func (c Config) resolverDNS() bool { return c.Resolve && !c.NoResolve }

// usaCT e usaLive dizem se a fonte roda de fato (ligada E com entrada).
func (c Config) usaCT() bool   { return !c.NoCT && len(c.Domains) > 0 }
func (c Config) usaLive() bool { return !c.NoLive && len(c.Hosts) > 0 }

// apexes são os domínios que definem "o alvo": os --domain ou, sem eles, o
// apex aproximado de cada --host.
func (c Config) apexes() []string {
	var out []string
	for _, d := range c.Domains {
		if n := normalizarNome(d); n != "" && !contem(out, n) {
			out = append(out, n)
		}
	}
	if len(out) > 0 {
		return out
	}
	for _, h := range c.Hosts {
		host, _ := hostPorta(h, c.Port)
		if a := apexDeHost(host); a != "" && !contem(out, a) {
			out = append(out, a)
		}
	}
	return out
}
