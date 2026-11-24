package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "certspy:", err)
		os.Exit(1)
	}
}

// run é o corpo de main sem os.Exit: lê a config, coleta das fontes, mescla,
// resolve, grava as saídas locais e só então (se pedido) publica.
func run(args []string, stdout, stderr io.Writer) error {
	cfg, err := parseConfig(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, uso)
			return nil
		}
		return err
	}

	agora := time.Now()
	cfg = resolverSaidas(cfg, agora)
	apexes := cfg.apexes()
	banner(stdout, cfg, apexes)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	con := &console{w: stdout, aviso: stderr, quiet: cfg.Quiet}
	timeout := time.Duration(cfg.TimeoutS) * time.Second

	var achados []achado
	if cfg.usaCT() {
		achados = append(achados, coletarCT(ctx, cfg, timeout, con)...)
	}
	if cfg.usaLive() {
		achados = append(achados, coletarLive(ctx, cfg, timeout, con)...)
	}

	doms := mesclar(achados)
	marcarApex(doms, apexes)
	if cfg.resolverDNS() {
		resolverTodos(ctx, doms, cfg.Concurrency, timeout, con.dominioResolvido)
	}

	if err := gravarSaidas(cfg, doms, apexes, agora); err != nil {
		return err
	}
	resumoFinal(stdout, cfg, doms)

	if cfg.PublishURL == "" {
		return nil
	}
	p := montarPayload(apexes, doms, cfg.PublishAll, agora)
	fmt.Fprintf(stdout, "\npublicando %d domínios em %s ...\n", len(p.Domains), cfg.PublishURL)
	if err := publicar(ctx, cfg, p); err != nil {
		return fmt.Errorf("%w (as saídas locais já foram gravadas)", err)
	}
	fmt.Fprintln(stdout, "publicado.")
	return nil
}

// resolverSaidas preenche os caminhos que o usuário não deu com nome
// automático baseado no horário.
func resolverSaidas(cfg Config, agora time.Time) Config {
	ts := agora.Format("20060102-150405")
	if cfg.OutTXT == "" {
		cfg.OutTXT = "certspy-" + ts + ".txt"
	}
	if cfg.OutJSONL == "" {
		cfg.OutJSONL = "certspy-" + ts + ".jsonl"
	}
	if cfg.OutMD == "" {
		cfg.OutMD = "certspy-" + ts + ".md"
	}
	return cfg
}

// coletarCT consulta o crt.sh para cada domínio, sequencialmente: o serviço
// é de terceiros, lento, e rajada em paralelo só piora as respostas sob
// carga. Falha de uma consulta vira aviso e a execução segue com o resto.
func coletarCT(ctx context.Context, cfg Config, timeout time.Duration, con *console) []achado {
	cli := &http.Client{Timeout: timeout}
	var out []achado
	for _, d := range cfg.Domains {
		d = normalizarNome(d)
		// "%.d" traz os subdomínios; "d" sozinho traz o apex.
		for _, consulta := range []string{"%." + d, d} {
			if ctx.Err() != nil {
				return out
			}
			entradas, err := consultarCT(ctx, cli, cfg.UserAgent, consulta)
			if err != nil {
				con.avisar("%v", err)
				continue
			}
			achados := achadosDeCT(entradas)
			con.info("ct   %-30s %d certificados, %d nomes", consulta, len(entradas), len(achados))
			out = append(out, achados...)
		}
	}
	return out
}

// coletarLive faz os handshakes em paralelo (cada host é um servidor
// diferente, então não há o problema do serviço único do CT).
func coletarLive(ctx context.Context, cfg Config, timeout time.Duration, con *console) []achado {
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		out []achado
		sem = make(chan struct{}, cfg.Concurrency)
	)
	for _, h := range cfg.Hosts {
		sem <- struct{}{}
		wg.Add(1)
		go func(h string) {
			defer func() { <-sem; wg.Done() }()
			host, porta := hostPorta(h, cfg.Port)
			nomes, err := sansAoVivo(ctx, host, porta, timeout, cfg.Insecure)
			if err != nil {
				con.avisar("%v", err)
				return
			}
			con.info("live %-30s %d nomes no certificado", fmt.Sprintf("%s:%d", host, porta), len(nomes))
			mu.Lock()
			defer mu.Unlock()
			for _, n := range nomes {
				out = append(out, achado{Nome: n, Fonte: fonteLive})
			}
		}(h)
	}
	wg.Wait()
	return out
}

func gravarSaidas(cfg Config, doms []dominio, apexes []string, agora time.Time) error {
	if err := gravarTXT(cfg.OutTXT, doms); err != nil {
		return err
	}
	if err := gravarJSONL(cfg.OutJSONL, doms); err != nil {
		return err
	}
	if cfg.NoMD {
		return nil
	}
	return gravarMD(cfg.OutMD, doms, apexes, fontesAtivas(cfg), agora)
}

func fontesAtivas(cfg Config) []string {
	var f []string
	if cfg.usaCT() {
		f = append(f, "ct (crt.sh)")
	}
	if cfg.usaLive() {
		f = append(f, "live (handshake TLS)")
	}
	return f
}

func banner(w io.Writer, cfg Config, apexes []string) {
	fmt.Fprintf(w, "certspy: alvo %s\n", strings.Join(apexes, ", "))
	fmt.Fprintf(w, "  fontes: %s\n", strings.Join(fontesAtivas(cfg), " + "))
	if cfg.usaCT() {
		fmt.Fprintln(w, "  ct: consulta o crt.sh (serviço de terceiros; dado público)")
	}
	if cfg.resolverDNS() {
		fmt.Fprintln(w, "  resolução DNS: ligada")
	} else {
		fmt.Fprintln(w, "  resolução DNS: desligada (--no-resolve)")
	}
	if cfg.Insecure {
		fmt.Fprintln(w, "  TLS NÃO verificado (--insecure)")
	}
	if cfg.PublishURL != "" {
		escopo := "só nomes sob o apex do alvo"
		if cfg.PublishAll {
			escopo = "TODOS os nomes, inclusive fora do apex (--publish-all)"
		}
		fmt.Fprintf(w, "  !! PUBLICAÇÃO ATIVA: vai enviar por POST para %s (%s)\n", cfg.PublishURL, escopo)
	}
	fmt.Fprintln(w, "  Ctrl+C encerra graciosamente")
	fmt.Fprintln(w)
}

func resumoFinal(w io.Writer, cfg Config, doms []dominio) {
	vivos, sobApex := 0, 0
	for _, d := range doms {
		if d.Vivo {
			vivos++
		}
		if d.SobApex {
			sobApex++
		}
	}
	fmt.Fprintf(w, "\nconcluído: %d domínios (%d sob o apex), %d vivos\n", len(doms), sobApex, vivos)
	fmt.Fprintf(w, "  txt:   %s\n  jsonl: %s\n", cfg.OutTXT, cfg.OutJSONL)
	if !cfg.NoMD {
		fmt.Fprintf(w, "  md:    %s\n", cfg.OutMD)
	}
}

// console serializa o progresso (vem de várias goroutines) e separa o que é
// aviso (stderr) do que é informação (stdout).
type console struct {
	mu    sync.Mutex
	w     io.Writer
	aviso io.Writer
	quiet bool
}

func (c *console) info(format string, a ...any) {
	if c.quiet {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	fmt.Fprintf(c.w, format+"\n", a...)
}

// avisar não respeita --quiet: falha de fonte precisa ser vista.
func (c *console) avisar(format string, a ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	fmt.Fprintf(c.aviso, "aviso: "+format+"\n", a...)
}

func (c *console) dominioResolvido(d dominio) {
	if d.Vivo {
		c.info("vivo %-40s %s", d.Nome, strings.Join(d.IPs, ","))
	}
}

const uso = `certspy - descobre domínios e subdomínios a partir de certificados SSL públicos

uso:
  certspy --domain=<dominio> [--host=<host[:porta]>] [opções]

exemplo:
  certspy --domain=meusite.com.br --host=www.meusite.com.br

fontes (pelo menos uma entrada):
  --domain=DOM        domínio-alvo para consultar no CT via crt.sh (repetível)
  --host=HOST[:PORT]  host para extrair SANs do certificado ao vivo (repetível)
  --no-ct             não consulta o crt.sh
  --no-live           não faz handshake ao vivo

opções:
  --resolve           resolve DNS e marca os vivos (padrão: ligado)
  --no-resolve        desliga a resolução DNS
  --port=N            porta do handshake quando o host não traz porta (padrão: 443)
  --concurrency=N     resoluções/handshakes em paralelo, 1..64 (padrão: 8)
  --timeout=S         timeout por request/conexão em segundos (padrão: 20)
  --out-txt=PATH      lista, um domínio por linha (padrão: certspy-<ts>.txt)
  --out-jsonl=PATH    metadados por domínio (padrão: certspy-<ts>.jsonl)
  --out-md=PATH       relatório markdown (padrão: certspy-<ts>.md)
  --no-md             não gera o markdown
  --insecure          não valida TLS no handshake ao vivo e no publish
  --user-agent=UA     User-Agent das requests HTTP
  --quiet             não imprime o progresso por domínio

publicação (desligada por padrão):
  --publish-url=URL    envia o resultado por POST (JSON) para o SEU endpoint
  --publish-token=STR  valor do header Authorization
  --publish-all        inclui nomes fora do apex do alvo (padrão: só sob o apex)
`
