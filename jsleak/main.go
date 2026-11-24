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
		fmt.Fprintln(os.Stderr, "jsleak:", err)
		os.Exit(1)
	}
}

// run é o corpo de main sem os.Exit: lê a config, prepara o manifesto,
// monta o varredor e roda até terminar ou receber Ctrl+C.
func run(args []string, stdout, stderr io.Writer) error {
	cfg, err := parseConfig(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, uso)
			return nil
		}
		return err
	}

	logPath, logFile, err := abrirManifesto(cfg)
	if err != nil {
		return err
	}
	defer logFile.Close()

	rec := newRecorder(logFile, stdout, cfg.Quiet)
	v, err := newVarredor(cfg, rec)
	if err != nil {
		return err
	}

	banner(stdout, cfg, logPath)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	v.run(ctx)
	resumoFinal(stdout, rec)
	return nil
}

// abrirManifesto cria o arquivo JSONL (caminho pedido ou automático).
func abrirManifesto(cfg Config) (string, *os.File, error) {
	path := cfg.LogFile
	if path == "" {
		path = fmt.Sprintf("jsleak-%s.jsonl", time.Now().Format("20060102-150405"))
	}
	f, err := os.Create(path)
	if err != nil {
		return "", nil, fmt.Errorf("não consegui criar o manifesto %s: %w", path, err)
	}
	return path, f, nil
}

func banner(w io.Writer, cfg Config, logPath string) {
	fmt.Fprintf(w, "jsleak: %d diretório(s), %d arquivo(s), %d URL(s)\n",
		len(cfg.Dirs), len(cfg.Files), len(cfg.URLs))
	fmt.Fprintf(w, "  manifesto: %s\n", logPath)
	if cfg.Redact {
		fmt.Fprintln(w, "  redação: LIGADA (segredos aparecem só pelo prefixo)")
	} else {
		fmt.Fprintln(w, "  redação: DESLIGADA (--no-redact) — o manifesto terá segredos em texto puro")
	}
	if cfg.Insecure {
		fmt.Fprintln(w, "  TLS do destino NÃO verificado (--insecure)")
	}
	fmt.Fprintln(w, "  Ctrl+C encerra graciosamente")
	fmt.Fprintln(w)
}

// resumoFinal imprime o total por severidade, da mais grave à menos grave.
func resumoFinal(w io.Writer, rec *recorder) {
	porSev, fontes, erros := rec.resumo()
	total := 0
	for _, n := range porSev {
		total += n
	}
	fmt.Fprintf(w, "\nconcluído: %d fonte(s) varridas, %d achado(s), %d erro(s)\n", fontes, total, erros)
	for _, sev := range ordemSeveridades {
		fmt.Fprintf(w, "  %-8s %d\n", sev, porSev[sev])
	}
}

const uso = `jsleak - procura segredos e endpoints vazados em arquivos locais e URLs (auditoria autorizada)

uso:
  jsleak [--dir=PATH | --file=PATH | --url=URL]... [opções]

exemplo:
  jsleak --dir=dump-meusite.com.br-20260101-120000 --url=https://meusite.com.br/app.js

fontes (pelo menos uma):
  --dir=PATH        varre um diretório recursivamente (repetível)
  --file=PATH       varre um arquivo (repetível)
  --url=URL         baixa e varre uma URL via GET passivo, com scheme http/https (repetível)

opções:
  --header="N: V"   header das requests de --url, repetível (Cookie, Authorization...)
  --ext=LIST        extensões varridas em --dir, csv (padrão: js,html,json,txt,css,map,env,xml,yml,yaml,ts)
  --max-body=KB     ignora arquivo/resposta maior que isso; 0 = sem limite (padrão)
  --context=N       chars de contexto em volta do match (padrão: 40)
  --redact          redige o segredo na saída (padrão: ligado)
  --no-redact       mostra o match inteiro (só no SEU alvo)
  --timeout=S       timeout por request de --url, em segundos (padrão: 15)
  --insecure        não valida o cert TLS do destino nas URLs
  --log-file=PATH   caminho do manifesto JSONL (padrão: jsleak-<ts>.jsonl)
  --quiet           não imprime cada achado no console
`
