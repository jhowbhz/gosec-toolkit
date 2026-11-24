package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"
)

// version identifica a build.
const version = "0.1.0"

// settle é quanto esperar, após o load, pra capturar os XHRs que a página
// dispara no início (SPA). Fixo e curto; o crawl inteiro respeita --delay.
const settle = 2500 * time.Millisecond

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "spyder:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	cfg, err := parseConfig(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, uso)
			return nil
		}
		return err
	}
	seed, _ := cfg.seedURL() // já validado no parse

	outPath := cfg.Out
	if outPath == "" {
		outPath = fmt.Sprintf("spyder-%s.db", time.Now().Format("20060102-150405"))
	}
	st, err := openStore(outPath)
	if err != nil {
		return fmt.Errorf("não consegui abrir o banco %s: %w", outPath, err)
	}
	defer st.close()

	robots := &robotsPolicy{}
	if !cfg.IgnoreRobots {
		robots = buscarRobots(seed, stdout)
	}

	b := newBroker()
	dash := newDashboard(st, b)
	ln, err := dash.escutar(cfg.Dashboard)
	if err != nil {
		return fmt.Errorf("não consegui escutar o dashboard em %s: %w (porta ocupada?)", cfg.Dashboard, err)
	}

	fetcher, cleanup, err := newChromeFetcher(cfg.ChromePath, settle)
	if err != nil {
		ln.Close()
		return fmt.Errorf("não consegui iniciar o Chrome headless: %w (instale o Chrome ou use --chrome=CAMINHO)", err)
	}
	defer cleanup()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go dash.serve(ctx, ln)

	url := "http://" + cfg.Dashboard + "/"
	banner(stdout, cfg, outPath, url)
	if cfg.Open {
		abrirNavegador(url)
	}

	cr, err := newCrawler(cfg, fetcher, st, b, robots)
	if err != nil {
		return err
	}

	if cfg.Forever {
		paths := carregarPaths(cfg, stdout)
		fmt.Fprintf(stdout, "%d caminho(s) na wordlist. Ctrl+C pra parar.\n", len(paths))
		cr.descobrir(ctx, newProbe(cfg), paths, stdout) // só volta no Ctrl+C
		return nil
	}

	if err := cr.run(ctx); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "\ncrawl terminado. dashboard segue no ar: %s\n  Ctrl+C pra sair.\n", url)
	<-ctx.Done() // mantém o dashboard de pé pra explorar o resultado
	return nil
}

// buscarRobots baixa e interpreta o robots.txt do host do seed. Fail-open:
// se não achar/der erro, libera tudo (não trava o crawl por causa disso).
func buscarRobots(seed *url.URL, stdout io.Writer) *robotsPolicy {
	robotsURL := seed.Scheme + "://" + seed.Host + "/robots.txt"
	cli := &http.Client{Timeout: 10 * time.Second}
	resp, err := cli.Get(robotsURL)
	if err != nil {
		fmt.Fprintf(stdout, "robots.txt: não acessível (%v) — seguindo sem restrições\n", err)
		return &robotsPolicy{}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &robotsPolicy{}
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	return parseRobots(string(body), robotsUA)
}

func banner(w io.Writer, cfg Config, outPath, url string) {
	modoRobots := "respeitando robots.txt"
	if cfg.IgnoreRobots {
		modoRobots = "IGNORANDO robots.txt (--ignore-robots)"
	}
	fmt.Fprintf(w, "spyder %s — mapeando %s\n", version, cfg.URL)
	fmt.Fprintf(w, "  escopo: %s | %d página(s) máx | profundidade %d | %d aba(s)\n",
		cfg.Scope, cfg.MaxPages, cfg.MaxDepth, cfg.Concurrency)
	fmt.Fprintf(w, "  %s\n", modoRobots)
	if len(cfg.Headers) > 0 {
		fmt.Fprintf(w, "  %d header(s) extra em toda request\n", len(cfg.Headers))
	}
	if cfg.Forever {
		fmt.Fprintln(w, "  modo: descoberta CONTÍNUA por wordlist (nunca para — Ctrl+C encerra)")
	}
	fmt.Fprintf(w, "  banco: %s\n", outPath)
	fmt.Fprintf(w, "  >> dashboard ao vivo: %s\n", url)
}

// abrirNavegador tenta abrir a URL no navegador padrão (best-effort).
func abrirNavegador(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Start()
}

const uso = `spyder - mapeia as rotas de um site e mostra ao vivo

uso:
  spyder --url=https://site.com [opções]

obrigatória:
  --url            ponto de partida (seed), com http:// ou https://

opções:
  --out=PATH       arquivo SQLite (padrão: spyder-<ts>.db)
  --dashboard=ADDR endereço da área web ao vivo (padrão: 127.0.0.1:8750)
  --header "K: V"  header extra em toda request (repetível)
  --max-pages=N    teto de páginas renderizadas (padrão: 300)
  --max-depth=N    profundidade máxima a partir do seed (padrão: 4)
  --concurrency=N  abas headless em paralelo (padrão: 2)
  --delay=DUR      espera entre requests por polidez (padrão: 300ms)
  --scope=MODE     same-host (padrão) | subdomains
  --ignore-robots  ignora robots.txt (só no que é SEU/autorizado)
  --chrome=PATH    caminho do Chrome (senão auto-detecta)
  --open           abre o dashboard no navegador ao iniciar

  descoberta contínua (nunca para, até Ctrl+C):
  --forever        varre wordlists em loop, recursivo
  --fetch          baixa sozinho o catálogo de wordlists, cacheado (padrão: on)
  --wordlist=PATH  wordlist local (repetível)
  --wordlist-url=U wordlist remota pra baixar (repetível)
`
