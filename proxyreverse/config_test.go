package main

import (
	"reflect"
	"testing"
)

func TestParseConfigMinimo(t *testing.T) {
	cfg, err := parseConfig([]string{"--from=meusite.com.br", "--to=https://203.0.113.5"})
	if err != nil {
		t.Fatalf("parseConfig falhou com entrada válida: %v", err)
	}
	if cfg.From != "meusite.com.br" {
		t.Errorf("From = %q, quero %q", cfg.From, "meusite.com.br")
	}
	if cfg.To != "https://203.0.113.5" {
		t.Errorf("To = %q, quero %q", cfg.To, "https://203.0.113.5")
	}
	// Sem flags de escuta, o padrão é HTTP :80 + HTTPS :443.
	if h, s := cfg.listenAddrs(); h != ":80" || s != ":443" {
		t.Errorf("listenAddrs default = (%q,%q), quero (\":80\",\":443\")", h, s)
	}
	if cfg.PreserveHost || cfg.Insecure {
		t.Errorf("flags booleanas deveriam ser false por default: preserveHost=%v insecure=%v", cfg.PreserveHost, cfg.Insecure)
	}
}

func TestParseConfigExigeFrom(t *testing.T) {
	_, err := parseConfig([]string{"--to=https://203.0.113.5"})
	if err == nil {
		t.Fatal("quero erro quando --from falta, veio nil")
	}
}

func TestParseConfigExigeTo(t *testing.T) {
	_, err := parseConfig([]string{"--from=meusite.com.br"})
	if err == nil {
		t.Fatal("quero erro quando --to falta, veio nil")
	}
}

func TestParseConfigToPrecisaDeScheme(t *testing.T) {
	// "forma": sem http:// ou https:// não dá pra saber o protocolo do upstream.
	_, err := parseConfig([]string{"--from=site.local", "--to=meudev.local:3000"})
	if err == nil {
		t.Fatal("quero erro quando --to não tem scheme, veio nil")
	}
}

func TestParseConfigToSchemeInvalido(t *testing.T) {
	_, err := parseConfig([]string{"--from=site.local", "--to=ftp://meudev.local"})
	if err == nil {
		t.Fatal("quero erro quando --to usa scheme fora de http/https, veio nil")
	}
}

func TestUpstreamURL(t *testing.T) {
	cfg, err := parseConfig([]string{"--from=site.local", "--to=http://meudev.local:3000/base"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	u, err := cfg.upstreamURL()
	if err != nil {
		t.Fatalf("upstreamURL: %v", err)
	}
	if u.Scheme != "http" || u.Host != "meudev.local:3000" {
		t.Errorf("upstream = %s://%s, quero http://meudev.local:3000", u.Scheme, u.Host)
	}
}

func TestFromHostTiraPorta(t *testing.T) {
	cfg, err := parseConfig([]string{"--from=site.local:8080", "--to=https://x.com"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if got := cfg.fromHost(); got != "site.local" {
		t.Errorf("fromHost() = %q, quero %q", got, "site.local")
	}
}

func TestListenSoHTTP(t *testing.T) {
	cfg, err := parseConfig([]string{"--from=site.local", "--to=https://x.com", "--http=127.0.0.1:8080"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	// Pediu só --http: não sobe HTTPS (comum no dev sem admin, porta alta).
	if h, s := cfg.listenAddrs(); h != "127.0.0.1:8080" || s != "" {
		t.Errorf("listenAddrs = (%q,%q), quero (\"127.0.0.1:8080\",\"\")", h, s)
	}
}

func TestListenSoHTTPS(t *testing.T) {
	cfg, err := parseConfig([]string{"--from=site.local", "--to=https://x.com", "--https=127.0.0.1:8443"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if h, s := cfg.listenAddrs(); h != "" || s != "127.0.0.1:8443" {
		t.Errorf("listenAddrs = (%q,%q), quero (\"\",\"127.0.0.1:8443\")", h, s)
	}
}

func TestAcessoURLsPadrao(t *testing.T) {
	cfg, err := parseConfig([]string{"--from=meusite.com.br", "--to=https://x.com"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	got := cfg.acessoURLs()
	want := []string{"http://meusite.com.br/", "https://meusite.com.br/"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("acessoURLs() = %v, quero %v", got, want)
	}
}

func TestAcessoURLsPortaCustomAparece(t *testing.T) {
	cfg, err := parseConfig([]string{"--from=site.local", "--to=https://x.com", "--http=127.0.0.1:8080"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	got := cfg.acessoURLs()
	want := []string{"http://site.local:8080/"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("acessoURLs() = %v, quero %v (porta não-padrão aparece)", got, want)
	}
}

func TestParseConfigHeaderRepetivel(t *testing.T) {
	cfg, err := parseConfig([]string{"--from=x.local", "--to=https://x.com", "--header", "X-Pentest: autorizado", "--header", "X-Tool: proxyreverse"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if len(cfg.Headers) != 2 {
		t.Fatalf("Headers = %d, quero 2", len(cfg.Headers))
	}
	if cfg.Headers[0].Name != "X-Pentest" || cfg.Headers[0].Value != "autorizado" {
		t.Errorf("Headers[0] = %+v, quero {X-Pentest autorizado}", cfg.Headers[0])
	}
}

func TestParseConfigHeaderMalformado(t *testing.T) {
	_, err := parseConfig([]string{"--from=x.local", "--to=https://x.com", "--header", "sem-dois-pontos"})
	if err == nil {
		t.Fatal(`quero erro quando --header não tem "Nome: valor", veio nil`)
	}
}

func TestParseConfigUpstreamAddr(t *testing.T) {
	cfg, err := parseConfig([]string{"--from=meusite.com.br", "--to=https://meusite.com.br", "--upstream-addr=203.0.113.5:443"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.UpstreamAddr != "203.0.113.5:443" {
		t.Errorf("UpstreamAddr = %q, quero %q", cfg.UpstreamAddr, "203.0.113.5:443")
	}
}

func TestUpstreamAddrPrecisaDePorta(t *testing.T) {
	// forma: sem porta não dá pra discar; exige host:porta.
	_, err := parseConfig([]string{"--from=x.local", "--to=https://x.com", "--upstream-addr=203.0.113.5"})
	if err == nil {
		t.Fatal("quero erro quando --upstream-addr não tem porta, veio nil")
	}
}

func TestListenInvalido(t *testing.T) {
	_, err := parseConfig([]string{"--from=site.local", "--to=https://x.com", "--http=porta-errada"})
	if err == nil {
		t.Fatal("quero erro quando --http é malformado, veio nil")
	}
}
