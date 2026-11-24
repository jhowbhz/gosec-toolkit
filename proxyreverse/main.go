package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// version identifica a build; vai no header que marca o tráfego pro destino.
const version = "0.1.0"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "proxyreverse:", err)
		os.Exit(1)
	}
}

// run é o corpo de main sem os.Exit: lê a config, abre o log, monta o
// handler e sobe os servidores até um sinal de parada.
func run(args []string, stdout, stderr io.Writer) error {
	cfg, err := parseConfig(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, uso)
			return nil
		}
		return err
	}

	if err := checarLoopDireto(cfg); err != nil {
		return err
	}

	logFile, logPath, err := abrirLog(cfg)
	if err != nil {
		return err
	}
	defer logFile.Close()

	rec := newRecorder(logFile, stdout, cfg.Quiet)
	handler, err := newHandler(cfg, rec)
	if err != nil {
		return err
	}

	servidores, err := montarServidores(cfg, handler)
	if err != nil {
		return err
	}

	banner(stdout, cfg, logPath, servidores)
	return servir(servidores, stdout)
}

// servidor junta um http.Server ao seu listener já aberto (bind feito
// de forma síncrona pra que porta ocupada vire erro claro no startup).
type servidor struct {
	s     *http.Server
	ln    net.Listener
	label string
}

// montarServidores abre os listeners (HTTP e/ou HTTPS). O bind acontece
// aqui — é a verdade fresca que a validação de flag não podia saber:
// porta ocupada ou falta de permissão pra 80/443 aparece agora.
func montarServidores(cfg Config, handler http.Handler) ([]servidor, error) {
	httpAddr, httpsAddr := cfg.listenAddrs()
	var out []servidor

	if httpAddr != "" {
		ln, err := net.Listen("tcp", httpAddr)
		if err != nil {
			return nil, fmt.Errorf("não consegui escutar HTTP em %s: %w (porta ocupada ou precisa de admin pra :80)", httpAddr, err)
		}
		out = append(out, servidor{s: &http.Server{Handler: handler}, ln: ln, label: "HTTP  " + httpAddr})
	}

	if httpsAddr != "" {
		cert, err := loadCert(cfg)
		if err != nil {
			fecharTodos(out)
			return nil, fmt.Errorf("não consegui preparar o cert HTTPS: %w", err)
		}
		raw, err := net.Listen("tcp", httpsAddr)
		if err != nil {
			fecharTodos(out)
			return nil, fmt.Errorf("não consegui escutar HTTPS em %s: %w (porta ocupada ou precisa de admin pra :443)", httpsAddr, err)
		}
		ln := tls.NewListener(raw, &tls.Config{Certificates: []tls.Certificate{cert}})
		out = append(out, servidor{s: &http.Server{Handler: handler}, ln: ln, label: "HTTPS " + httpsAddr})
	}

	if len(out) == 0 {
		return nil, errors.New("nada pra escutar: confira --listen")
	}
	return out, nil
}

// servir sobe cada servidor e bloqueia até Ctrl+C / SIGTERM ou até um
// servidor cair sozinho, então encerra todos graciosamente.
func servir(servidores []servidor, stdout io.Writer) error {
	erros := make(chan error, len(servidores))
	for _, sv := range servidores {
		go func(sv servidor) {
			if err := sv.s.Serve(sv.ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				erros <- err
			}
		}(sv)
	}

	parar := make(chan os.Signal, 1)
	signal.Notify(parar, os.Interrupt, syscall.SIGTERM)

	var runErr error
	select {
	case <-parar:
		fmt.Fprintln(stdout, "\nencerrando...")
	case runErr = <-erros:
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, sv := range servidores {
		sv.s.Shutdown(ctx)
	}
	return runErr
}

func fecharTodos(servidores []servidor) {
	for _, sv := range servidores {
		sv.ln.Close()
	}
}

// abrirLog resolve o arquivo JSONL: o caminho pedido ou um nome automático
// com timestamp, criado no diretório atual.
func abrirLog(cfg Config) (*os.File, string, error) {
	path := cfg.LogFile
	if path == "" {
		path = fmt.Sprintf("proxyreverse-%s.jsonl", time.Now().Format("20060102-150405"))
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, "", fmt.Errorf("não consegui criar o log %s: %w", path, err)
	}
	return f, path, nil
}

func banner(w io.Writer, cfg Config, logPath string, servidores []servidor) {
	hostMode := "Host reescrito pro destino"
	if cfg.PreserveHost {
		hostMode = "Host original preservado (--preserve-host)"
	}
	fmt.Fprintf(w, "proxyreverse: %s  ->  %s\n", cfg.From, cfg.To)
	for _, sv := range servidores {
		fmt.Fprintf(w, "  escutando %s\n", sv.label)
	}
	if cfg.UpstreamAddr != "" {
		fmt.Fprintf(w, "  discando direto: %s (Host/SNI mantido = %s)\n", cfg.UpstreamAddr, cfg.upstreamHostLabel())
	}
	fmt.Fprintf(w, "  identificando no destino: %s: proxyreverse/%s\n", identHeader, version)
	if len(cfg.Headers) > 0 {
		fmt.Fprintf(w, "  + %d header(s) extra pro destino\n", len(cfg.Headers))
	}
	for _, u := range cfg.acessoURLs() {
		fmt.Fprintf(w, "  >> abra no navegador: %s  (o --from, NÃO o --to)\n", u)
	}
	fmt.Fprintf(w, "     garanta no hosts: 127.0.0.1  %s\n", cfg.fromHost())
	fmt.Fprintf(w, "  %s\n", hostMode)
	if cfg.Insecure {
		fmt.Fprintln(w, "  TLS do upstream NÃO verificado (--insecure)")
	}
	fmt.Fprintf(w, "  log: %s\n", logPath)
	fmt.Fprintln(w, "  Ctrl+C pra encerrar")
}

const uso = `proxyreverse - proxy reverso de interceptação pra teste

uso:
  proxyreverse --from=<host> --to=<url> [opções]

exemplo:
  proxyreverse --from=meusite.com.br --to=https://203.0.113.5 --preserve-host

obrigatórias:
  --from            host que o proxy escuta (aponte no hosts pra 127.0.0.1)
  --to              destino real, com scheme (http:// ou https://)

opções:
  --http=ADDR       escuta HTTP (ex.: 127.0.0.1:8080 ou :80)
  --https=ADDR      escuta HTTPS (ex.: 127.0.0.1:8443 ou :443)
                    sem --http nem --https: sobe os dois em :80 e :443
  --upstream-addr=HOST:PORTA  disca esse IP/porta real, mantendo o Host/SNI
                    do --to (use quando o nome aponta pra 127.0.0.1 no hosts)
  --header "K: V"   header extra pro destino (repetível). Um header
                    X-Proxyreverse já vai sempre, identificando a ferramenta
  --preserve-host   mantém o Host original no request pro upstream
  --insecure        não valida o cert TLS do upstream (dev/self-signed)
  --cert / --key    cert e chave próprios pra escuta HTTPS (senão: auto-assinado)
  --log-file=PATH   caminho do log JSONL (padrão: proxyreverse-<ts>.jsonl)
  --max-body=KB     teto de corpo logado em KB (padrão: 256)
  --quiet           não imprime o resumo por request no console
`
