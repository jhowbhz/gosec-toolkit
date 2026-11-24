package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
)

// Config é a forma validada da linha de comando. Só guarda e confere a
// FORMA da entrada (campos existem, bem-formados); não faz I/O nem rede —
// porta ocupada e upstream fora do ar só aparecem no bind/no runtime.
type Config struct {
	From         string // host que o proxy escuta (ex.: meusite.com.br)
	To           string // destino real, com scheme (ex.: https://203.0.113.5)
	HTTPAddr     string // endereço de escuta HTTP; vazio = não sobe HTTP
	HTTPSAddr    string // endereço de escuta HTTPS; vazio = não sobe HTTPS
	UpstreamAddr string     // IP:porta real pra discar, mantendo o Host/SNI do --to
	Headers      headerList // headers extras injetados no request pro destino
	PreserveHost bool       // mantém o Host original no request pro upstream
	Insecure     bool   // não valida o cert TLS do upstream
	CertFile     string // cert próprio pra escuta HTTPS (senão auto-assinado)
	KeyFile      string // chave do cert próprio
	LogFile      string // caminho do JSONL; vazio = nome automático
	Quiet        bool   // não imprime resumo por request no console
	MaxBodyKB    int    // teto de corpo logado, em KB
}

const defaultMaxBodyKB = 256

// headerKV é um header extra a mandar pro destino.
type headerKV struct{ Name, Value string }

// headerList coleta os --header repetidos. Implementa flag.Value pra validar
// o formato "Nome: valor" já na hora do parse.
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

// parseConfig lê os argumentos, monta a Config e valida a forma.
func parseConfig(args []string) (Config, error) {
	fs := flag.NewFlagSet("proxyreverse", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // o chamador decide como reportar o erro

	var c Config
	fs.StringVar(&c.From, "from", "", "host que o proxy escuta (ex.: meusite.com.br)")
	fs.StringVar(&c.To, "to", "", "destino real com scheme (ex.: https://203.0.113.5)")
	fs.StringVar(&c.HTTPAddr, "http", "", "endereço de escuta HTTP (ex.: 127.0.0.1:8080 ou :80)")
	fs.StringVar(&c.HTTPSAddr, "https", "", "endereço de escuta HTTPS (ex.: 127.0.0.1:8443 ou :443)")
	fs.StringVar(&c.UpstreamAddr, "upstream-addr", "", "IP:porta real pra discar (mantém o Host/SNI do --to); útil quando o nome aponta pra 127.0.0.1 no hosts")
	fs.Var(&c.Headers, "header", `header extra pro destino, "Nome: valor" (repetível)`)
	fs.BoolVar(&c.PreserveHost, "preserve-host", false, "mantém o Host original (--from) no request pro upstream")
	fs.BoolVar(&c.Insecure, "insecure", false, "não valida o cert TLS do upstream")
	fs.StringVar(&c.CertFile, "cert", "", "cert próprio pra escuta HTTPS")
	fs.StringVar(&c.KeyFile, "key", "", "chave do cert próprio")
	fs.StringVar(&c.LogFile, "log-file", "", "caminho do log JSONL; vazio = proxyreverse-<ts>.jsonl")
	fs.BoolVar(&c.Quiet, "quiet", false, "não imprime o resumo por request no console")
	fs.IntVar(&c.MaxBodyKB, "max-body", defaultMaxBodyKB, "tamanho máx. de corpo logado, em KB")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	// Sem nenhuma escuta pedida, assume os padrões que casam com um host
	// apontado pra 127.0.0.1 no hosts: HTTP :80 e HTTPS :443.
	if c.HTTPAddr == "" && c.HTTPSAddr == "" {
		c.HTTPAddr, c.HTTPSAddr = ":80", ":443"
	}
	if err := c.validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// validate confere a verdade estática: o que dá pra saber sem perguntar
// a ninguém (campo presente, scheme conhecido, porta no range).
func (c Config) validate() error {
	if strings.TrimSpace(c.From) == "" {
		return errors.New("--from é obrigatório (host que o proxy escuta, ex.: meusite.com.br)")
	}
	if strings.Contains(c.From, "://") || strings.ContainsAny(c.From, "/\\") {
		return fmt.Errorf("--from deve ser só o host, sem scheme nem caminho: %q", c.From)
	}
	if strings.TrimSpace(c.To) == "" {
		return errors.New("--to é obrigatório (destino real, ex.: https://203.0.113.5)")
	}
	if _, err := c.upstreamURL(); err != nil {
		return err
	}
	for _, a := range []struct{ flag, addr string }{{"--http", c.HTTPAddr}, {"--https", c.HTTPSAddr}} {
		if a.addr == "" {
			continue
		}
		if _, _, err := net.SplitHostPort(a.addr); err != nil {
			return fmt.Errorf("%s malformado (quero host:porta ou :porta): %v", a.flag, err)
		}
	}
	if (c.CertFile == "") != (c.KeyFile == "") {
		return errors.New("--cert e --key devem ser usados juntos")
	}
	if c.UpstreamAddr != "" {
		if _, _, err := net.SplitHostPort(c.UpstreamAddr); err != nil {
			return fmt.Errorf("--upstream-addr precisa ser host:porta (ex.: 203.0.113.5:443): %v", err)
		}
	}
	if c.MaxBodyKB < 0 {
		return fmt.Errorf("--max-body não pode ser negativo: %d", c.MaxBodyKB)
	}
	return nil
}

// upstreamURL devolve o destino (--to) como URL. Exige scheme http/https
// porque sem ele não dá pra saber o protocolo que o upstream fala.
func (c Config) upstreamURL() (*url.URL, error) {
	u, err := url.Parse(c.To)
	if err != nil {
		return nil, fmt.Errorf("--to inválido: %v", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("--to precisa de scheme http:// ou https://: %q", c.To)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("--to sem host: %q", c.To)
	}
	return u, nil
}

// upstreamHostLabel é o host que o upstream enxerga no Host/SNI (o do --to),
// usado só pra exibir no banner.
func (c Config) upstreamHostLabel() string {
	if u, err := c.upstreamURL(); err == nil {
		return u.Host
	}
	return c.To
}

// fromHost devolve o host de --from sem a porta opcional.
func (c Config) fromHost() string {
	if host, _, err := net.SplitHostPort(c.From); err == nil {
		return host
	}
	return c.From
}

// maxBodyBytes é o teto de corpo logado em bytes.
func (c Config) maxBodyBytes() int {
	return c.MaxBodyKB * 1024
}

// acessoURLs devolve as URLs que o usuário deve abrir no navegador — o lado
// --from, onde o proxy escuta. É o engano clássico: abrir o --to (o destino)
// em vez do --from. A porta só aparece quando não é a padrão do scheme.
func (c Config) acessoURLs() []string {
	httpAddr, httpsAddr := c.listenAddrs()
	var urls []string
	if httpAddr != "" {
		urls = append(urls, urlAcesso("http", c.fromHost(), portaDe(httpAddr), "80"))
	}
	if httpsAddr != "" {
		urls = append(urls, urlAcesso("https", c.fromHost(), portaDe(httpsAddr), "443"))
	}
	return urls
}

func urlAcesso(scheme, host, porta, portaPadrao string) string {
	if porta == "" || porta == portaPadrao {
		return scheme + "://" + host + "/"
	}
	return scheme + "://" + host + ":" + porta + "/"
}

func portaDe(addr string) string {
	if _, p, err := net.SplitHostPort(addr); err == nil {
		return p
	}
	return ""
}

// listenAddrs devolve os endereços de escuta HTTP e HTTPS já resolvidos
// (os defaults :80/:443 são aplicados no parse). Endereço vazio = aquele
// protocolo não sobe.
func (c Config) listenAddrs() (httpAddr, httpsAddr string) {
	return c.HTTPAddr, c.HTTPSAddr
}
