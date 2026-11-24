package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	userAgent = "headeraudit/1.0 (+https://github.com/jhowbhz/headeraudit)"

	// maxRedirects limita a corrente seguida: o que interessa é a postura
	// de onde a URL pousa, não perseguir redirect infinito. Passou disso,
	// avalia a última resposta recebida e registra o status dela.
	maxRedirects = 5

	// maxDrenagem é quanto do corpo lemos antes de fechar. Só os headers
	// importam, mas ler um pouco permite reaproveitar a conexão sem baixar
	// uma página inteira.
	maxDrenagem = 64 * 1024
)

// auditor roda GET + OPTIONS por URL e transforma as respostas em notas.
type auditor struct {
	cfg     Config
	headers http.Header
	client  *http.Client
	portao  *portaoHost
	rec     *recorder
}

func newAuditor(cfg Config, rec *recorder) (*auditor, error) {
	headers, err := cfg.parsedHeaders()
	if err != nil {
		return nil, err
	}
	// Clone do transport padrão: herda proxy/HTTP2 do ambiente, e só o
	// TLS é ajustado quando o usuário pede --insecure.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.Insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &auditor{
		cfg:     cfg,
		headers: headers,
		portao:  newPortaoHost(cfg.delay()),
		rec:     rec,
		client: &http.Client{
			Timeout:   cfg.timeout(),
			Transport: transport,
			CheckRedirect: func(_ *http.Request, via []*http.Request) error {
				if len(via) >= maxRedirects {
					return http.ErrUseLastResponse
				}
				return nil
			},
		},
	}, nil
}

// run audita todas as URLs com um pool de workers e só volta quando termina
// ou o contexto é cancelado.
func (a *auditor) run(ctx context.Context, urls []string) {
	fila := make(chan string)
	var wg sync.WaitGroup
	for i := 0; i < a.cfg.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for u := range fila {
				a.rec.record(a.auditar(ctx, u))
			}
		}()
	}

	for _, u := range urls {
		select {
		case fila <- u:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(fila)
	wg.Wait()
}

// auditar faz o GET (headers e cookies) e o OPTIONS com Origin de teste
// (CORS) de uma URL. Falha no OPTIONS não derruba a URL: o GET já deu
// resultado útil, e a falta do teste de CORS vira um achado informativo.
func (a *auditor) auditar(ctx context.Context, alvo string) registro {
	inicio := time.Now()
	reg := registro{Timestamp: inicio.UTC().Format(time.RFC3339), URL: alvo}
	defer func() { reg.DurationMS = time.Since(inicio).Milliseconds() }()

	resp, err := a.pedir(ctx, http.MethodGet, alvo, "")
	if err != nil {
		reg.Error = err.Error()
		return reg
	}
	descartar(resp)

	final := resp.Request.URL
	https := final.Scheme == "https"
	reg.Status = resp.StatusCode
	if final.String() != alvo {
		reg.FinalURL = final.String()
	}

	reg.Issues = append(reg.Issues, avaliarHeadersSeguranca(resp.Header, https)...)
	reg.Issues = append(reg.Issues, avaliarCookies(resp.Header.Values("Set-Cookie"), https)...)

	opt, err := a.pedir(ctx, http.MethodOptions, final.String(), a.cfg.Origin)
	if err != nil {
		reg.Issues = append(reg.Issues, issue{"CORS", sevInfo, false,
			fmt.Sprintf("teste de CORS não executado: %v", err)})
	} else {
		descartar(opt)
		reg.Issues = append(reg.Issues, avaliarCORS(a.cfg.Origin, opt.Header)...)
	}

	reg.Grade = nota(reg.Issues)
	return reg
}

// pedir monta e envia uma request respeitando o delay por host. Com origem
// preenchida vira um preflight de CORS (Origin + Access-Control-Request-Method).
func (a *auditor) pedir(ctx context.Context, metodo, alvo, origem string) (*http.Response, error) {
	u, err := url.Parse(alvo)
	if err != nil {
		return nil, err
	}
	if err := a.portao.esperar(ctx, u.Host); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, metodo, alvo, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	for nome, valores := range a.headers {
		req.Header[nome] = append([]string(nil), valores...)
	}
	if origem != "" {
		req.Header.Set("Origin", origem)
		req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	}
	return a.client.Do(req)
}

func descartar(resp *http.Response) {
	io.CopyN(io.Discard, resp.Body, maxDrenagem)
	resp.Body.Close()
}

// portaoHost impõe a pausa mínima entre requests ao MESMO host, mesmo com
// vários workers: cada um reserva o próximo horário livre do host sob lock e
// dorme fora dele, então a fila de espera não trava os outros hosts.
type portaoHost struct {
	mu      sync.Mutex
	pausa   time.Duration
	proximo map[string]time.Time
}

func newPortaoHost(pausa time.Duration) *portaoHost {
	return &portaoHost{pausa: pausa, proximo: map[string]time.Time{}}
}

func (p *portaoHost) esperar(ctx context.Context, host string) error {
	if p.pausa <= 0 {
		return ctx.Err()
	}
	p.mu.Lock()
	vez := time.Now()
	if t := p.proximo[host]; t.After(vez) {
		vez = t
	}
	p.proximo[host] = vez.Add(p.pausa)
	p.mu.Unlock()

	espera := time.NewTimer(time.Until(vez))
	defer espera.Stop()
	select {
	case <-espera.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
