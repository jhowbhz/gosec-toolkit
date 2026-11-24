package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "download-dump:", err)
		os.Exit(1)
	}
}

// run é o corpo de main sem os.Exit: lê a config, prepara saída e manifesto,
// monta o crawler e roda até terminar ou receber Ctrl+C.
func run(args []string, stdout, stderr io.Writer) error {
	cfg, err := parseConfig(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, uso)
			return nil
		}
		return err
	}

	cfg.Out = resolverSaida(cfg)
	logPath, logFile, err := abrirManifesto(cfg)
	if err != nil {
		return err
	}
	defer logFile.Close()

	rec := newRecorder(logFile, stdout, cfg.Quiet)
	c, err := newCrawler(cfg, rec)
	if err != nil {
		return err
	}

	banner(stdout, cfg, logPath)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := c.run(ctx); err != nil {
		return err
	}
	return resumoFinal(stdout, cfg, rec)
}

// resolverSaida devolve a pasta de saída pedida ou um nome automático
// baseado no host da semente e no horário.
func resolverSaida(cfg Config) string {
	if cfg.Out != "" {
		return cfg.Out
	}
	seed, _ := cfg.seedURL() // já validado em parseConfig
	host := sanitizarSegmento(strings.ReplaceAll(seed.Host, ":", "_"))
	return fmt.Sprintf("dump-%s-%s", host, time.Now().Format("20060102-150405"))
}

// abrirManifesto cria o arquivo JSONL (caminho pedido ou automático).
func abrirManifesto(cfg Config) (string, *os.File, error) {
	path := cfg.LogFile
	if path == "" {
		path = fmt.Sprintf("download-dump-%s.jsonl", time.Now().Format("20060102-150405"))
	}
	f, err := os.Create(path)
	if err != nil {
		return "", nil, fmt.Errorf("não consegui criar o manifesto %s: %w", path, err)
	}
	return path, f, nil
}

func banner(w io.Writer, cfg Config, logPath string) {
	robotsMode := "respeitando robots.txt"
	if cfg.IgnoreRobots {
		robotsMode = "IGNORANDO robots.txt (--ignore-robots)"
	}
	fmt.Fprintf(w, "download-dump: %s\n", cfg.URL)
	fmt.Fprintf(w, "  escopo: %s   profundidade: %s\n", cfg.Scope, profundidadeLabel(cfg.Depth))
	fmt.Fprintf(w, "  saída (espelho): %s/\n", cfg.Out)
	fmt.Fprintf(w, "  manifesto: %s\n", logPath)
	fmt.Fprintf(w, "  %s\n", robotsMode)
	if cfg.ProbeSensitive {
		fmt.Fprintln(w, "  sonda ativa de sensíveis: LIGADA (só confere existência, não baixa)")
	}
	if cfg.Insecure {
		fmt.Fprintln(w, "  TLS do destino NÃO verificado (--insecure)")
	}
	fmt.Fprintln(w, "  Ctrl+C encerra graciosamente")
	fmt.Fprintln(w)
}

// resumoFinal imprime os contadores e, principalmente, os arquivos sensíveis
// encontrados — que NÃO foram baixados, só sinalizados.
func resumoFinal(w io.Writer, cfg Config, rec *recorder) error {
	baixados, pulados, erros, sensiveis := rec.resumo()
	fmt.Fprintf(w, "\nconcluído: %d baixados, %d pulados, %d erros, %d sensíveis sinalizados\n",
		baixados, pulados, erros, sensiveis)

	sens := rec.relatorioSensiveis()
	if len(sens) == 0 {
		return nil
	}
	fmt.Fprintln(w, "\n!! ARQUIVOS SENSÍVEIS ENCONTRADOS (não baixados — confira manualmente):")
	for _, s := range sens {
		fmt.Fprintf(w, "   [%d] %s  <%s>\n", s.Status, s.URL, s.SensitiveNote)
	}
	return gravarAchadosSensiveis(cfg.Out, sens)
}

// gravarAchadosSensiveis deixa junto do espelho um texto com os sensíveis,
// para ficar com a evidência do teste sem precisar reabrir o manifesto.
func gravarAchadosSensiveis(outDir string, sens []registro) error {
	var b strings.Builder
	b.WriteString("Arquivos sensíveis encontrados pelo download-dump.\n")
	b.WriteString("Estes NÃO foram baixados — a ferramenta apenas confirmou que existem.\n\n")
	for _, s := range sens {
		fmt.Fprintf(&b, "[%d] %s\n      %s\n", s.Status, s.URL, s.SensitiveNote)
	}
	_, err := salvar(outDir, "_ACHADOS-SENSIVEIS.txt", []byte(b.String()))
	return err
}

func profundidadeLabel(d int) string {
	if d < 0 {
		return "ilimitada"
	}
	return fmt.Sprintf("%d", d)
}

const uso = `download-dump - espelha um site para navegação offline (raspagem autorizada)

uso:
  download-dump --url=<url> [opções]

exemplo:
  download-dump --url=https://meusite.com.br/ --header="Cookie: sid=..." --probe-sensitive

obrigatória:
  --url             URL semente, com scheme (http:// ou https://)

opções:
  --out=DIR         pasta de saída do espelho (padrão: dump-<host>-<ts>)
  --header="N: V"   header de request, repetível (User-Agent, Cookie, Authorization...)
  --user-agent=UA   User-Agent do crawler
  --scope=MODO      até onde segue links: host (padrão) | domain | prefix
  --depth=N         profundidade máxima de links; -1 = ilimitado (padrão)
  --concurrency=N   workers paralelos (padrão: 4)
  --delay=MS        pausa mínima por host entre requests (robots Crawl-delay vence)
  --max-body=KB     pula arquivos maiores que isso; 0 = sem limite (padrão)
  --include=REGEX   só segue/salva caminhos que casam
  --exclude=REGEX   nunca segue/salva caminhos que casam
  --probe-sensitive sonda caminhos sensíveis comuns (.env, .git/config...) — não baixa, só avisa
  --ignore-robots   ignora robots.txt (use SÓ em alvo autorizado)
  --insecure        não valida o cert TLS do destino
  --log-file=PATH   caminho do manifesto JSONL (padrão: download-dump-<ts>.jsonl)
  --quiet           não imprime o resumo por recurso no console
`
