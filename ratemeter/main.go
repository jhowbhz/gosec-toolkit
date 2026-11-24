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
		fmt.Fprintln(os.Stderr, "ratemeter:", err)
		os.Exit(1)
	}
}

// run é o corpo de main sem os.Exit: lê a config, abre o JSONL, monta o
// medidor e roda até terminar ou receber Ctrl+C.
func run(args []string, stdout, stderr io.Writer) error {
	cfg, err := parseConfig(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, uso)
			return nil
		}
		return err
	}

	logPath, logFile, err := abrirLog(cfg)
	if err != nil {
		return err
	}
	defer logFile.Close()

	rec := newRecorder(logFile, stdout, cfg.Quiet)
	m, err := newMeter(cfg, rec)
	if err != nil {
		return err
	}

	banner(stdout, cfg, logPath)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	inicio := time.Now()
	motivo := m.run(ctx)
	escreverResumo(stdout, rec.registros(), motivo, time.Since(inicio))
	return nil
}

// abrirLog cria o arquivo JSONL (caminho pedido ou automático).
func abrirLog(cfg Config) (string, *os.File, error) {
	path := cfg.LogFile
	if path == "" {
		path = fmt.Sprintf("ratemeter-%s.jsonl", time.Now().Format("20060102-150405"))
	}
	f, err := os.Create(path)
	if err != nil {
		return "", nil, fmt.Errorf("não consegui criar o log %s: %w", path, err)
	}
	return path, f, nil
}

func banner(w io.Writer, cfg Config, logPath string) {
	fmt.Fprintf(w, "ratemeter: %s %s\n", cfg.Method, cfg.URL)
	fmt.Fprintln(w, "  ATENÇÃO: faz requisições ATIVAS contra o alvo; use só no que é seu/autorizado")
	if cfg.StopOn429 {
		fmt.Fprintln(w, "  parando no primeiro 429")
	} else {
		fmt.Fprintln(w, "  --continue-on-429: NÃO vai parar no 429 (só no seu alvo, com cautela)")
	}
	rampa := ""
	if cfg.Ramp {
		rampa = fmt.Sprintf(" em rampa de %d degraus", cfg.RampSteps)
	}
	fmt.Fprintf(w, "  até %d req/s%s   total: %d   concorrência: %d\n", cfg.Rate, rampa, cfg.Total, cfg.Concurrency)
	if cfg.Duration > 0 {
		fmt.Fprintf(w, "  duração máxima: %ds\n", cfg.Duration)
	}
	fmt.Fprintf(w, "  log: %s\n", logPath)
	if cfg.Insecure {
		fmt.Fprintln(w, "  TLS do destino NÃO verificado (--insecure)")
	}
	fmt.Fprintln(w, "  Ctrl+C encerra graciosamente")
	fmt.Fprintln(w)
}

const uso = `ratemeter - mede o comportamento de rate-limit/quota do SEU endpoint (teste autorizado)

faz requisições ATIVAS; é lento por padrão e para no primeiro 429. Não é ferramenta de carga.

uso:
  ratemeter --url=<url> [opções]

exemplo:
  ratemeter --url=https://api.meusite.com.br/v1/consulta --header="Authorization: Bearer ..." --rate=5 --ramp

obrigatória:
  --url              endpoint medido, com scheme (http:// ou https://)

opções:
  --method=MÉTODO    método HTTP (padrão: GET)
  --header="N: V"    header de request, repetível
  --body=STR         corpo do request (POST/PUT)
  --body-file=PATH   corpo lido de arquivo (exclusivo com --body)
  --rate=N           requests por segundo alvo (padrão: 1) — TETO RÍGIDO 50
  --ramp             sobe de 1/s até --rate em degraus
  --ramp-steps=N     nº de degraus da rampa (padrão: 5)
  --total=N          total de requests (padrão: 60) — TETO RÍGIDO 5000
  --duration=S       para após S segundos; 0 = só --total manda (o teto do total vale sempre)
  --concurrency=N    requests em voo em paralelo (padrão: 1) — TETO RÍGIDO 20
  --stop-on-429      para ao receber 429 (PADRÃO LIGADO)
  --continue-on-429  desliga o stop-on-429 (só no SEU alvo, com cautela)
  --timeout=S        timeout por request (padrão: 15)
  --insecure         não valida o cert TLS do destino
  --log-file=PATH    JSONL por request (padrão: ratemeter-<ts>.jsonl)
  --quiet            não imprime uma linha por request
`
