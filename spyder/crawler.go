package main

import (
	"context"
	"net/url"
	"sync"
	"time"
)

// crawler percorre o site a partir do seed, renderizando cada página (via
// fetcher), gravando as rotas descobertas e emitindo eventos ao vivo.
// Segue só páginas dentro do escopo; XHR, assets e externos são registrados
// mas não aprofundados.
type crawler struct {
	cfg    Config
	seed   *url.URL
	fetch  fetcher
	store  *store
	broker *broker
	robots *robotsPolicy
}

func newCrawler(cfg Config, f fetcher, st *store, b *broker, robots *robotsPolicy) (*crawler, error) {
	seed, err := cfg.seedURL()
	if err != nil {
		return nil, err
	}
	if robots == nil {
		robots = &robotsPolicy{}
	}
	return &crawler{cfg: cfg, seed: seed, fetch: f, store: st, broker: b, robots: robots}, nil
}

type queued struct {
	u     *url.URL
	depth int
}

// run faz o BFS com um pool de workers. Termina quando a fila esvazia (todas
// as páginas alcançáveis dentro dos limites foram processadas) ou no cancel.
func (c *crawler) run(ctx context.Context) error {
	queue := make(chan queued, c.cfg.MaxPages+1)
	seen := make(map[string]bool)
	var mu sync.Mutex
	enfileiradas := 0
	var itens sync.WaitGroup

	enfileirar := func(u *url.URL, depth int) {
		if depth > c.cfg.MaxDepth {
			return
		}
		key := u.String()
		mu.Lock()
		if seen[key] || enfileiradas >= c.cfg.MaxPages {
			mu.Unlock()
			return
		}
		seen[key] = true
		enfileiradas++
		itens.Add(1)
		mu.Unlock()
		queue <- queued{u, depth} // buffer = MaxPages+1, nunca bloqueia
	}

	enfileirar(c.seed, 0)

	var workers sync.WaitGroup
	for i := 0; i < c.cfg.Concurrency; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for q := range queue {
				c.processar(ctx, q, enfileirar)
				itens.Done()
			}
		}()
	}

	go func() { itens.Wait(); close(queue) }()
	workers.Wait()
	return nil
}

// processar renderiza uma página, grava a rota dela e classifica tudo que ela
// revelou (links e requisições de rede).
func (c *crawler) processar(ctx context.Context, q queued, enfileirar func(*url.URL, int)) {
	select {
	case <-ctx.Done():
		return
	default:
	}

	if !c.cfg.IgnoreRobots && !c.robots.allowed(caminho(q.u)) {
		return // robots.txt proíbe; respeita
	}
	if c.cfg.Delay > 0 {
		time.Sleep(c.cfg.Delay)
	}

	res, _ := c.fetch.fetch(ctx, q.u.String(), c.cfg.Headers)

	// a própria página vira rota
	c.registrar(route{
		URL: q.u.String(), Host: q.u.Hostname(), Method: "GET",
		Status: res.Status, Kind: kindPage, Depth: q.depth, Title: res.Title,
	})

	// links do DOM: segue os do escopo, registra os externos
	for _, href := range res.Links {
		u, ok := normalizar(q.u, href)
		if !ok {
			continue
		}
		if dentroDoEscopo(u, c.seed, c.cfg.Scope) {
			c.registrar(route{URL: u.String(), Host: u.Hostname(), Method: "GET",
				Kind: kindPage, Depth: q.depth + 1, SourceURL: q.u.String()})
			enfileirar(u, q.depth+1)
		} else {
			c.registrar(route{URL: u.String(), Host: u.Hostname(), Method: "GET",
				Kind: kindExternal, Depth: q.depth + 1, SourceURL: q.u.String()})
		}
	}

	// requisições de rede: os endpoints reais (XHR/fetch) e assets
	for _, ri := range res.Requests {
		u, ok := normalizar(q.u, ri.URL)
		if !ok {
			continue
		}
		metodo := ri.Method
		if metodo == "" {
			metodo = "GET"
		}
		c.registrar(route{URL: u.String(), Host: u.Hostname(), Method: metodo,
			Status: ri.Status, Kind: c.classificar(u, ri), Depth: q.depth + 1, SourceURL: q.u.String()})
	}
}

// registrar grava a rota e, se for inédita, emite o evento ao vivo. No foco
// em API, descarta rotas de front (páginas/assets) — elas ainda são navegadas
// pelo crawler pra achar as chamadas de API, só não entram no resultado.
func (c *crawler) registrar(r route) {
	if c.cfg.ApiOnly && !ehAPI(r) {
		return
	}
	novo, err := c.store.salvar(r)
	if err != nil {
		return
	}
	if novo {
		c.broker.publish(r)
	}
}

// classificar decide o tipo de uma requisição de rede. Chamada XHR/fetch é
// API mesmo cross-origin (é o endpoint que a SPA usa — ex.: Vercel/Supabase),
// então vem antes do escopo. O resto fora do escopo é externo.
func (c *crawler) classificar(u *url.URL, ri requestInfo) string {
	switch ri.ResourceType {
	case "xhr", "fetch":
		return kindXHR
	}
	if !dentroDoEscopo(u, c.seed, c.cfg.Scope) {
		return kindExternal
	}
	if ri.ResourceType == "document" {
		return kindPage
	}
	return kindAsset
}

func caminho(u *url.URL) string {
	p := u.EscapedPath()
	if p == "" {
		return "/"
	}
	return p
}
