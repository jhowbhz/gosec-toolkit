package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "tlsscan:", err)
		os.Exit(1)
	}
}

// run é o corpo de main sem os.Exit: lê a config, junta os alvos, abre o
// relatório e escaneia um a um.
func run(args []string, stdout, stderr io.Writer) error {
	cfg, err := parseConfig(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, uso)
			return nil
		}
		return err
	}

	alvos, err := coletarAlvos(cfg)
	if err != nil {
		return err
	}
	if len(alvos) == 0 {
		return errors.New("nenhum alvo encontrado em --host/--hosts-file")
	}

	logPath := cfg.LogFile
	if logPath == "" {
		logPath = fmt.Sprintf("tlsscan-%s.jsonl", time.Now().Format("20060102-150405"))
	}
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("não consegui criar o relatório %s: %w", logPath, err)
	}
	defer logFile.Close()

	fmt.Fprintf(stdout, "tlsscan: %d alvo(s)   relatório: %s\n\n", len(alvos), logPath)

	rec := newRecorder(logFile, stdout, cfg.Quiet)
	esc := escaneador{cfg: cfg, agora: time.Now}
	for _, a := range alvos {
		rec.record(esc.escanear(a))
	}

	hosts, falhas, criticas, altas := rec.resumo()
	fmt.Fprintf(stdout, "\nconcluído: %d hosts, %d falhas, %d issues critical, %d high\n",
		hosts, falhas, criticas, altas)
	return nil
}

// coletarAlvos junta --host e as linhas do --hosts-file. Linhas em branco e
// comentários (#) são ignorados para o arquivo poder ser anotado à mão.
func coletarAlvos(cfg Config) ([]string, error) {
	alvos := append([]string{}, cfg.Hosts...)
	if cfg.HostsFile == "" {
		return alvos, nil
	}
	f, err := os.Open(cfg.HostsFile)
	if err != nil {
		return nil, fmt.Errorf("não consegui abrir --hosts-file: %w", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		linha := strings.TrimSpace(sc.Text())
		if linha == "" || strings.HasPrefix(linha, "#") {
			continue
		}
		alvos = append(alvos, linha)
	}
	return alvos, sc.Err()
}

const uso = `tlsscan - inspeciona a config TLS e o certificado de um host (passivo, alvo autorizado)

uso:
  tlsscan --host=<host[:porta]> [opções]

exemplo:
  tlsscan --host=meusite.com.br --full-chain

obrigatória (uma das duas):
  --host=HOST[:PORT]  host a escanear, repetível
  --hosts-file=PATH   arquivo com um host por linha (# comenta)

opções:
  --port=N            porta quando o host não traz porta (padrão: 443)
  --servername=SNI    força o SNI (padrão: o próprio host)
  --timeout=S         timeout por conexão em segundos (padrão: 15)
  --full-chain        mostra todos os certs da cadeia (padrão: só a folha)
  --no-hsts           não faz o GET de checagem de HSTS (só handshake)
  --log-file=PATH     relatório JSONL (padrão: tlsscan-<ts>.jsonl)
  --quiet             não imprime o resumo por host no console
`
