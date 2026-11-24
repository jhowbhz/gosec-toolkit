package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "headeraudit:", err)
		os.Exit(1)
	}
}

// run é o corpo de main sem os.Exit: lê a config, carrega as URLs, abre o
// JSONL e audita até terminar ou receber Ctrl+C.
func run(args []string, stdout, stderr io.Writer) error {
	cfg, err := parseConfig(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, uso)
			return nil
		}
		return err
	}

	urls, err := carregarURLs(cfg)
	if err != nil {
		return err
	}
	logPath, logFile, err := abrirRelatorio(cfg)
	if err != nil {
		return err
	}
	defer logFile.Close()

	rec := newRecorder(logFile, stdout, cfg.Quiet)
	a, err := newAuditor(cfg, rec)
	if err != nil {
		return err
	}

	banner(stdout, cfg, len(urls), logPath)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a.run(ctx, urls)
	resumoFinal(stdout, rec)
	return nil
}

// abrirRelatorio cria o arquivo JSONL (caminho pedido ou automático).
func abrirRelatorio(cfg Config) (string, *os.File, error) {
	path := cfg.LogFile
	if path == "" {
		path = fmt.Sprintf("headeraudit-%s.jsonl", time.Now().Format("20060102-150405"))
	}
	f, err := os.Create(path)
	if err != nil {
		return "", nil, fmt.Errorf("não consegui criar o relatório %s: %w", path, err)
	}
	return path, f, nil
}

func banner(w io.Writer, cfg Config, total int, logPath string) {
	fmt.Fprintf(w, "headeraudit: %d URL(s) a auditar\n", total)
	fmt.Fprintf(w, "  origin de teste (CORS): %s\n", cfg.Origin)
	fmt.Fprintf(w, "  concorrência: %d   delay por host: %dms   timeout: %ds\n",
		cfg.Concurrency, cfg.DelayMS, cfg.TimeoutS)
	fmt.Fprintf(w, "  relatório: %s\n", logPath)
	if cfg.Insecure {
		fmt.Fprintln(w, "  TLS do destino NÃO verificado (--insecure)")
	}
	fmt.Fprintln(w, "  Ctrl+C encerra graciosamente")
	fmt.Fprintln(w)
}

// resumoFinal imprime a distribuição de notas, da melhor para a pior.
func resumoFinal(w io.Writer, rec *recorder) {
	notas, erros := rec.resumo()
	fmt.Fprintf(w, "\nconcluído: %d erro(s) de rede\n", erros)
	fmt.Fprintln(w, "distribuição de notas:")
	for _, n := range notaPorPeso {
		fmt.Fprintf(w, "  %s: %d\n", n, notas[n])
	}
}

const uso = `headeraudit - audita a postura HTTP (headers, cookies, CORS) de URLs suas/autorizadas

uso:
  headeraudit --url=<url> [--url=<url> ...] [opções]
  headeraudit --urls-file=urls.txt [opções]
  headeraudit --jsonl=download-dump-<ts>.jsonl [opções]

exemplo:
  headeraudit --url=https://meusite.com.br/ --header="Cookie: sid=..." --delay=200

fonte de URLs (ao menos uma):
  --url=URL         URL a auditar, com scheme; repetível
  --urls-file=PATH  arquivo com uma URL por linha (# comenta)
  --jsonl=PATH      extrai URLs únicas de um manifesto do download-dump

opções:
  --header="N: V"   header extra em toda request, repetível (Cookie de área logada...)
  --origin=ORIGIN   Origin usado no teste de CORS (padrão: https://attacker.example)
  --concurrency=N   requests em paralelo (padrão: 4)
  --delay=MS        pausa mínima entre requests ao mesmo host (padrão: 0)
  --timeout=S       timeout por request em segundos (padrão: 15)
  --insecure        não valida o cert TLS do destino
  --log-file=PATH   JSONL de saída (padrão: headeraudit-<ts>.jsonl)
  --quiet           não imprime a linha por URL no console
`
