package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// corpoMax é o quanto do corpo lemos pra medir o tamanho (soft-404). Suficiente
// pra distinguir a página de catch-all de uma página real.
const corpoMax = 512 * 1024

// baseline é a assinatura de "não existe" do alvo, medida com caminhos
// aleatórios. Num site normal é 404; numa SPA costuma ser 200 + a index.
type baseline struct {
	estavel bool
	status  int
	tamanho int64
}

// resposta é o resultado medido de uma requisição de probe.
type resposta struct {
	status  int
	tamanho int64
}

// probe faz força bruta de caminhos por HTTP cru (sem Chrome) — é o jeito
// rápido de achar rotas que não estão linkadas em lugar nenhum.
type probe struct {
	client  *http.Client
	headers []headerKV
	delay   time.Duration
	workers int
}

func newProbe(cfg Config) *probe {
	return &probe{
		client: &http.Client{
			Timeout: 15 * time.Second,
			// Não segue redirect: 301/302 são respostas que interessam registrar.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		headers: cfg.Headers,
		delay:   cfg.Delay,
		workers: cfg.Concurrency,
	}
}

// probar calibra a linha de base do alvo e então testa cada path, chamando
// emit só pros que DIFEREM da assinatura de "não existe" — assim o catch-all
// 200 de uma SPA não vira um monte de falso-positivo. Respeita concorrência,
// delay e cancelamento.
func (p *probe) probar(ctx context.Context, base *url.URL, paths []string, emit func(route)) {
	bl := p.calibrar(ctx, base)

	jobs := make(chan string)
	var wg sync.WaitGroup
	for i := 0; i < p.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				p.um(ctx, base, path, bl, emit)
			}
		}()
	}

	for _, path := range paths {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return
		case jobs <- path:
		}
	}
	close(jobs)
	wg.Wait()
}

// calibrar mede como o alvo responde a caminhos aleatórios (que não existem).
// Se dois deles batem (mesmo status, tamanho parecido), essa é a assinatura de
// "não existe"; senão, não há assinatura estável e caímos no critério de status.
func (p *probe) calibrar(ctx context.Context, base *url.URL) baseline {
	a := p.buscar(ctx, base, caminhoAleatorio())
	b := p.buscar(ctx, base, caminhoAleatorio())
	if a == nil || b == nil {
		return baseline{}
	}
	if a.status == b.status && parecido(a.tamanho, b.tamanho) {
		return baseline{estavel: true, status: a.status, tamanho: (a.tamanho + b.tamanho) / 2}
	}
	return baseline{}
}

func (p *probe) um(ctx context.Context, base *url.URL, path string, bl baseline, emit func(route)) {
	if p.delay > 0 {
		select {
		case <-ctx.Done():
			return
		case <-time.After(p.delay):
		}
	}
	u, ok := normalizar(base, path)
	if !ok {
		return
	}
	r := p.buscar(ctx, base, path)
	if r == nil {
		return
	}
	if r.status == http.StatusNotFound || r.status == http.StatusGone {
		return // não existe
	}
	// catch-all: mesma cara da resposta de "não existe" → não é rota de verdade
	if bl.estavel && r.status == bl.status && parecido(r.tamanho, bl.tamanho) {
		return
	}
	achada := route{
		URL: u.String(), Host: u.Hostname(), Method: "GET",
		Status: r.status, Kind: kindPage, SourceURL: "wordlist",
	}
	if ehAPI(achada) { // caminho com cara de API (/api, /rest, .json) → marca como API
		achada.Kind = kindXHR
	}
	emit(achada)
}

// buscar faz o GET e mede status + tamanho do corpo (limitado), que é o que
// a detecção de soft-404 precisa comparar.
func (p *probe) buscar(ctx context.Context, base *url.URL, path string) *resposta {
	select {
	case <-ctx.Done():
		return nil
	default:
	}
	u, ok := normalizar(base, path)
	if !ok {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil
	}
	for _, h := range p.headers {
		req.Header.Set(h.Name, h.Value)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	n, _ := io.Copy(io.Discard, io.LimitReader(resp.Body, corpoMax))
	return &resposta{status: resp.StatusCode, tamanho: n}
}

// parecido diz se dois tamanhos de corpo são "o mesmo" pra fins de catch-all,
// com folga pequena (nonce/timestamp embutido não deve enganar).
func parecido(a, b int64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	tol := (a + b) / 100 // ~2%
	if tol < 48 {
		tol = 48
	}
	return d <= tol
}

func caminhoAleatorio() string {
	var x [8]byte
	rand.Read(x[:])
	return "zz" + hex.EncodeToString(x[:]) + "-naoexiste"
}
