package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// crawler rastreia a partir de uma semente, dentro do escopo e respeitando o
// robots.txt, baixa cada recurso para o espelho e, no fim, reescreve os links
// para navegação offline. Arquivos sensíveis são só sinalizados, nunca
// baixados. A estrutura carrega o estado compartilhado entre os workers;
// cada campo com acesso concorrente tem seu mutex.
type crawler struct {
	cfg     Config
	seed    *url.URL
	client  *http.Client
	headers http.Header
	rec     *recorder
	ctx     context.Context

	include, exclude *regexp.Regexp
	baseDominio      string

	visitadosMu sync.Mutex
	visitados   map[string]bool

	robotsMu sync.Mutex
	robots   map[string]*robots

	hostsMu sync.Mutex
	hosts   map[string]*portaoHost

	mirrorMu sync.Mutex
	mirror   map[string]string // URL absoluta -> caminho local relativo

	baseMu sync.Mutex
	bases  map[string]*baseline // linha de base de soft-404 por host

	textoMu sync.Mutex
	texto   []textAsset // HTML/CSS salvos, a reescrever no fim
}

// textAsset é um recurso textual salvo que será reescrito: o caminho local, a
// URL base para resolver suas referências e se é CSS (ou HTML).
type textAsset struct {
	local string
	base  *url.URL
	isCSS bool
}

// item é uma unidade da fila de rastreamento.
type item struct {
	u     *url.URL
	depth int
	from  string
	probe bool // veio da sonda ativa de caminhos sensíveis
}

// portaoHost serializa e espaça os requests a um mesmo host (educação).
type portaoHost struct {
	mu   sync.Mutex
	last time.Time
}

// assinatura resume uma resposta (tamanho + hash do corpo) para comparar
// duas respostas sem guardar o conteúdo.
type assinatura struct {
	tam  int
	hash string
}

// baseline é a calibração de soft-404 de um host: se um caminho sabidamente
// inexistente responde "sucesso" (soft=true), guardamos a assinatura dessa
// resposta para distinguir, depois, um arquivo que existe de verdade (corpo
// diferente) de um catch-all/SPA que devolve sempre a mesma página.
type baseline struct {
	soft bool
	sig  assinatura
}

const (
	timeoutRequest = 30 * time.Second
	capAssinatura  = 256 * 1024 // teto de leitura para assinar uma resposta
)

func newCrawler(cfg Config, rec *recorder) (*crawler, error) {
	seed, err := cfg.seedURL()
	if err != nil {
		return nil, err
	}
	headers, err := cfg.parsedHeaders()
	if err != nil {
		return nil, err
	}
	tr := &http.Transport{}
	if cfg.Insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	c := &crawler{
		cfg:         cfg,
		seed:        seed,
		client:      &http.Client{Transport: tr, Timeout: timeoutRequest},
		headers:     headers,
		rec:         rec,
		include:     cfg.includeRe(),
		exclude:     cfg.excludeRe(),
		baseDominio: dominioBase(seed.Host),
		visitados:   map[string]bool{},
		robots:      map[string]*robots{},
		hosts:       map[string]*portaoHost{},
		mirror:      map[string]string{},
		bases:       map[string]*baseline{},
	}
	return c, nil
}

// run faz o crawl inteiro e depois o passe de reescrita para navegação offline.
// O contexto permite encerrar graciosamente (Ctrl+C): os requests em voo são
// cancelados e os workers drenam a fila sem iniciar trabalho novo.
func (c *crawler) run(ctx context.Context) error {
	c.ctx = ctx
	var wg sync.WaitGroup
	tasks := make(chan item, 1024)

	var enqueue func(it item)
	enqueue = func(it item) {
		chave := it.u.String()
		c.visitadosMu.Lock()
		if c.visitados[chave] {
			c.visitadosMu.Unlock()
			return
		}
		c.visitados[chave] = true
		c.visitadosMu.Unlock()

		wg.Add(1)
		go func() { tasks <- it }()
	}

	for i := 0; i < c.cfg.Concurrency; i++ {
		go func() {
			for it := range tasks {
				c.processar(it, enqueue)
				wg.Done()
			}
		}()
	}

	enqueue(item{u: c.seed, depth: 0, from: ""})
	if c.cfg.ProbeSensitive {
		c.semearSondas(enqueue)
	}

	wg.Wait()
	close(tasks)

	return c.reescreverTudo()
}

// semearSondas enfileira os caminhos sensíveis comuns (opt-in) como itens de
// sonda, resolvidos contra a raiz do host da semente.
func (c *crawler) semearSondas(enqueue func(item)) {
	raiz := &url.URL{Scheme: c.seed.Scheme, Host: c.seed.Host, Path: "/"}
	for _, p := range caminhosSensiveisComuns() {
		if u, err := raiz.Parse(p); err == nil {
			enqueue(item{u: u, depth: 0, from: "(sonda)", probe: true})
		}
	}
}

// processar trata um item: filtros de escopo/robots/include, depois sinaliza
// sensível (sem baixar) ou baixa e, se for HTML/CSS, descobre novos links.
func (c *crawler) processar(it item, enqueue func(item)) {
	if c.ctx.Err() != nil {
		return // encerrando: não começa trabalho novo
	}
	u := it.u

	// Item de sonda é sempre só checagem de existência — nunca baixa, mesmo
	// que o classificador não reconheça o padrão (a lista de sondagem é mais
	// ampla que as regras de ehSensivel de propósito).
	if it.probe {
		motivo := "caminho sensível comum (sonda)"
		if sens, m := ehSensivel(u); sens {
			motivo = m
		}
		c.conferirSensivel(it, motivo)
		return
	}
	if sens, motivo := ehSensivel(u); sens {
		c.conferirSensivel(it, motivo)
		return
	}
	if !c.cfg.IgnoreRobots && !c.robotsDoHost(u.Host).allowed(u.Path) {
		c.rec.record(registro{URL: u.String(), DiscoveredFrom: it.from, Skipped: true, SkipReason: "barrado pelo robots.txt"})
		return
	}
	if c.filtrado(u) {
		c.rec.record(registro{URL: u.String(), DiscoveredFrom: it.from, Skipped: true, SkipReason: "fora de --include/--exclude"})
		return
	}

	c.baixar(it, enqueue)
}

// conferirSensivel confirma a existência via HEAD e registra o achado SEM
// baixar o conteúdo. Sonda que dá 404 é silenciada (ruído); link sensível
// descoberto no site é registrado de todo jeito.
func (c *crawler) conferirSensivel(it item, motivo string) {
	u := it.u

	// Respeita o robots.txt também na checagem de sensíveis: HEAD é um fetch,
	// e o robots vale para todo fetch. Se barrado, uma sonda fica silenciosa
	// (não dá pra confirmar existência); um link descoberto é sinalizado
	// assim mesmo (ele existe — foi referenciado), mas não é sondado.
	if !c.cfg.IgnoreRobots && !c.robotsDoHost(u.Host).allowed(u.Path) {
		if it.probe {
			return
		}
		c.rec.record(registro{
			URL:            u.String(),
			DiscoveredFrom: it.from,
			Sensitive:      true,
			Skipped:        true,
			SensitiveNote:  motivo + " (barrado por robots — não sondado)",
		})
		return
	}

	c.aguardarHost(u.Host)
	existe, status, legivel := c.confereExistencia(u)
	if !existe {
		return // inexistente ou soft-404: não reporta (evita falso positivo)
	}
	if !legivel {
		motivo += " (existe mas protegido)"
	}
	c.rec.record(registro{
		URL:            u.String(),
		Status:         status,
		DiscoveredFrom: it.from,
		Sensitive:      true,
		Skipped:        true,
		SensitiveNote:  motivo,
	})
}

// confereExistencia decide se um recurso realmente existe, sem salvar nada.
// Em host normal (um caminho inexistente dá erro), basta o status do HEAD.
// Em host catch-all/SPA (inexistente responde "sucesso"), compara o corpo com
// a linha de base: só existe se a resposta DIFERE da página devolvida a todo
// caminho. legivel=false sinaliza existe-mas-protegido (401/403).
func (c *crawler) confereExistencia(u *url.URL) (existe bool, status int, legivel bool) {
	bl := c.baselineDoHost(u.Host)
	if !bl.soft {
		st := c.statusExiste(u)
		switch st {
		case 200, 206:
			return true, st, true
		case 401, 403:
			return true, st, false
		default:
			return false, st, false
		}
	}
	st, sig, err := c.getAssinatura(u)
	if err != nil || st >= 400 {
		return false, st, false
	}
	difere := sig.tam != bl.sig.tam || sig.hash != bl.sig.hash
	return difere, st, difere
}

// baselineDoHost calibra (e memoiza) o comportamento de soft-404 de um host,
// pedindo um caminho aleatório que não deve existir.
func (c *crawler) baselineDoHost(host string) *baseline {
	c.baseMu.Lock()
	if b, ok := c.bases[host]; ok {
		c.baseMu.Unlock()
		return b
	}
	c.baseMu.Unlock()

	u := &url.URL{Scheme: c.seed.Scheme, Host: host, Path: "/" + caminhoInexistente()}
	b := &baseline{}
	if resp, err := c.fetch(http.MethodGet, u); err == nil {
		if resp.StatusCode < 400 {
			b.soft = true
			body, _, _ := lerCorpo(resp.Body, capAssinatura)
			b.sig = assinar(body)
		}
		resp.Body.Close()
	}

	c.baseMu.Lock()
	c.bases[host] = b
	c.baseMu.Unlock()
	return b
}

// statusExiste devolve o status de existência via HEAD, caindo para GET se o
// servidor não suportar HEAD (405/501).
func (c *crawler) statusExiste(u *url.URL) int {
	st, err := c.head(u)
	if err != nil {
		return 0
	}
	if st == http.StatusMethodNotAllowed || st == http.StatusNotImplemented {
		if resp, err := c.fetch(http.MethodGet, u); err == nil {
			resp.Body.Close()
			return resp.StatusCode
		}
	}
	return st
}

// getAssinatura busca o recurso e devolve o status e a assinatura do corpo.
// O corpo é lido para comparar e DESCARTADO — nunca salvo nem registrado.
func (c *crawler) getAssinatura(u *url.URL) (int, assinatura, error) {
	resp, err := c.fetch(http.MethodGet, u)
	if err != nil {
		return 0, assinatura{}, err
	}
	defer resp.Body.Close()
	body, _, err := lerCorpo(resp.Body, capAssinatura)
	if err != nil {
		return resp.StatusCode, assinatura{}, err
	}
	return resp.StatusCode, assinar(body), nil
}

// assinar resume um corpo por tamanho e hash.
func assinar(body []byte) assinatura {
	s := sha256.Sum256(body)
	return assinatura{tam: len(body), hash: hex.EncodeToString(s[:])}
}

// caminhoInexistente gera um caminho aleatório improvável de existir, para
// calibrar a linha de base de soft-404.
func caminhoInexistente() string {
	var b [12]byte
	rand.Read(b[:])
	return "zz-" + hex.EncodeToString(b[:]) + "-404.html"
}

// baixar busca o recurso, grava no espelho e descobre novos links se o
// content-type for HTML/CSS e a profundidade permitir.
func (c *crawler) baixar(it item, enqueue func(item)) {
	u := it.u
	c.aguardarHost(u.Host)

	inicio := time.Now()
	resp, err := c.fetch(http.MethodGet, u)
	if err != nil {
		c.rec.record(registro{URL: u.String(), DiscoveredFrom: it.from, Error: err.Error()})
		return
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	max := c.cfg.maxBodyBytes()
	if max > 0 && resp.ContentLength > max {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		c.rec.record(registro{URL: u.String(), Status: resp.StatusCode, ContentType: ct, DiscoveredFrom: it.from, Skipped: true, SkipReason: "maior que --max-body"})
		return
	}
	body, grande, err := lerCorpo(resp.Body, max)
	if err != nil {
		c.rec.record(registro{URL: u.String(), Status: resp.StatusCode, DiscoveredFrom: it.from, Error: "lendo corpo: " + err.Error()})
		return
	}
	if grande {
		c.rec.record(registro{URL: u.String(), Status: resp.StatusCode, ContentType: ct, DiscoveredFrom: it.from, Skipped: true, SkipReason: "maior que --max-body"})
		return
	}

	rel := localPath(u, ct)
	if _, err := salvar(c.cfg.Out, rel, body); err != nil {
		c.rec.record(registro{URL: u.String(), Status: resp.StatusCode, DiscoveredFrom: it.from, Error: "salvando: " + err.Error()})
		return
	}
	soma := sha256.Sum256(body)
	c.registrarMirror(u, rel, ct)
	c.rec.record(registro{
		URL:            u.String(),
		Status:         resp.StatusCode,
		ContentType:    ct,
		Size:           int64(len(body)),
		SHA256:         hex.EncodeToString(soma[:]),
		SavedPath:      rel,
		DiscoveredFrom: it.from,
		Depth:          it.depth,
		DurationMS:     time.Since(inicio).Milliseconds(),
	})

	if c.cfg.Depth >= 0 && it.depth >= c.cfg.Depth {
		return
	}
	for _, link := range extractLinks(u, ct, body) {
		if c.emEscopo(link) {
			enqueue(item{u: link, depth: it.depth + 1, from: u.String()})
		}
	}
}

// registrarMirror guarda o mapeamento URL->local e marca HTML/CSS para o
// passe de reescrita final.
func (c *crawler) registrarMirror(u *url.URL, rel, ct string) {
	chave := *u
	chave.Fragment = ""
	c.mirrorMu.Lock()
	c.mirror[chave.String()] = rel
	c.mirrorMu.Unlock()

	mt := strings.ToLower(ct)
	ehHTML := strings.Contains(mt, "html")
	ehCSS := strings.Contains(mt, "css")
	if ehHTML || ehCSS {
		c.textoMu.Lock()
		c.texto = append(c.texto, textAsset{local: rel, base: u, isCSS: ehCSS})
		c.textoMu.Unlock()
	}
}

// reescreverTudo roda o passe de reescrita sobre cada HTML/CSS salvo, agora
// que o mapa URL->local está completo.
func (c *crawler) reescreverTudo() error {
	resolve := func(u *url.URL) (string, bool) {
		c.mirrorMu.Lock()
		defer c.mirrorMu.Unlock()
		p, ok := c.mirror[u.String()]
		return p, ok
	}
	for _, t := range c.texto {
		full := filepath.Join(c.cfg.Out, filepath.FromSlash(t.local))
		data, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		var novo []byte
		if t.isCSS {
			novo = rewriteCSS(t.base, t.local, data, resolve)
		} else {
			novo = rewriteHTML(t.base, t.local, data, resolve)
		}
		if err := os.WriteFile(full, novo, 0o644); err != nil {
			return fmt.Errorf("reescrevendo %s: %w", t.local, err)
		}
	}
	return nil
}

// emEscopo diz se a URL entra no crawl, conforme --scope. Fora do escopo o
// link não é seguido (fica externo no espelho).
func (c *crawler) emEscopo(u *url.URL) bool {
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	switch c.cfg.Scope {
	case "domain":
		return u.Host == c.seed.Host || strings.HasSuffix(strings.ToLower(u.Host), "."+c.baseDominio)
	case "prefix":
		return u.Host == c.seed.Host && strings.HasPrefix(u.Path, prefixoDir(c.seed.Path))
	default: // host
		return u.Host == c.seed.Host
	}
}

// filtrado aplica --include/--exclude ao caminho da URL.
func (c *crawler) filtrado(u *url.URL) bool {
	if c.include != nil && !c.include.MatchString(u.Path) {
		return true
	}
	if c.exclude != nil && c.exclude.MatchString(u.Path) {
		return true
	}
	return false
}

// robotsDoHost busca e memoiza o robots.txt de um host. Falha de rede = sem
// restrição (permite tudo), mesma postura de um navegador.
func (c *crawler) robotsDoHost(host string) *robots {
	c.robotsMu.Lock()
	if r, ok := c.robots[host]; ok {
		c.robotsMu.Unlock()
		return r
	}
	c.robotsMu.Unlock()

	u := &url.URL{Scheme: c.seed.Scheme, Host: host, Path: "/robots.txt"}
	var body []byte
	if resp, err := c.fetch(http.MethodGet, u); err == nil {
		body, _, _ = lerCorpo(resp.Body, 512*1024)
		resp.Body.Close()
	}
	r := parseRobots(body, c.cfg.UserAgent)

	c.robotsMu.Lock()
	c.robots[host] = r
	c.robotsMu.Unlock()
	return r
}

// aguardarHost espaça os requests a um host pelo maior entre --delay e o
// Crawl-delay do robots.
func (c *crawler) aguardarHost(host string) {
	atraso := time.Duration(c.cfg.DelayMS) * time.Millisecond
	if d := c.robotsDoHost(host).crawlDelay(); d > atraso {
		atraso = d
	}
	if atraso <= 0 {
		return
	}
	c.hostsMu.Lock()
	g, ok := c.hosts[host]
	if !ok {
		g = &portaoHost{}
		c.hosts[host] = g
	}
	c.hostsMu.Unlock()

	g.mu.Lock()
	defer g.mu.Unlock()
	if espera := atraso - time.Since(g.last); espera > 0 {
		time.Sleep(espera)
	}
	g.last = time.Now()
}

// fetch monta e envia um request com os headers e o User-Agent configurados.
func (c *crawler) fetch(metodo string, u *url.URL) (*http.Response, error) {
	req, err := http.NewRequestWithContext(c.ctx, metodo, u.String(), nil)
	if err != nil {
		return nil, err
	}
	for k, vs := range c.headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", c.cfg.UserAgent)
	}
	return c.client.Do(req)
}

// head confere a existência de um recurso sem baixar o corpo.
func (c *crawler) head(u *url.URL) (int, error) {
	resp, err := c.fetch(http.MethodHead, u)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}

// lerCorpo lê o corpo inteiro; com max>0, lê até max+1 e sinaliza grande=true
// se estourou (para não gravar arquivo truncado).
func lerCorpo(r io.Reader, max int64) (body []byte, grande bool, err error) {
	if max <= 0 {
		b, err := io.ReadAll(r)
		return b, false, err
	}
	b, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(b)) > max {
		return nil, true, nil
	}
	return b, false, nil
}

// dominioBase aproxima o domínio registrável pelos dois últimos rótulos
// (ex.: a.b.site.com -> site.com). Aproximação assumida: não cobre sufixos de
// dois níveis como co.uk — documentado no README.
func dominioBase(host string) string {
	if h, _, err := splitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(host)
	partes := strings.Split(host, ".")
	if len(partes) <= 2 {
		return host
	}
	return strings.Join(partes[len(partes)-2:], ".")
}

// prefixoDir devolve o diretório do caminho da semente, usado no escopo
// "prefix" (ex.: /app/x -> /app/).
func prefixoDir(p string) string {
	if p == "" {
		return "/"
	}
	if strings.HasSuffix(p, "/") {
		return p
	}
	d := path.Dir(p)
	if d == "/" || d == "." {
		return "/"
	}
	return d + "/"
}

// splitHostPort é net.SplitHostPort tolerante (host sem porta não é erro aqui).
func splitHostPort(host string) (h, p string, err error) {
	if !strings.Contains(host, ":") {
		return host, "", nil
	}
	i := strings.LastIndex(host, ":")
	return host[:i], host[i+1:], nil
}
