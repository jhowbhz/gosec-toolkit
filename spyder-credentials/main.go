package main

import (
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func main() {
	code, err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "spyder-credentials:", err)
		os.Exit(1)
	}
	os.Exit(code)
}

// run é o corpo de main sem os.Exit: coleta as fontes, varre, consolida e
// reporta. Devolve o exit code (2 se houver segredo, 0 caso contrário).
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	cfg, err := parseConfig(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, uso)
			return 0, nil
		}
		return 1, err
	}
	headers, _ := cfg.parsedHeaders()

	fontes, err := coletarFontes(cfg, stdin, headers, stderr)
	if err != nil {
		return 1, err
	}

	dets := detectoresPadrao()
	var todos []achado
	for _, f := range fontes {
		todos = append(todos, varrer(string(f.dados), f.nome, dets, cfg.Entropy)...)
	}
	achados := consolidar(todos, cfg.ShowFull)
	if cfg.OnlySec {
		achados = soSecretos(achados)
	}

	logPath := cfg.LogFile
	if logPath == "" {
		logPath = fmt.Sprintf("spyder-credentials-%s.jsonl", time.Now().Format("20060102-150405"))
	}
	if lf, err := os.Create(logPath); err == nil {
		escreverJSONL(lf, achados)
		lf.Close()
	}

	banner(stdout, cfg, len(fontes), logPath)
	if !cfg.Quiet {
		if len(achados) == 0 {
			fmt.Fprintln(stdout, "\nnenhuma credencial exposta encontrada.")
		} else {
			fmt.Fprintln(stdout)
			escreverConsole(stdout, achados)
		}
	}

	if len(achados) > 0 {
		outPath := cfg.Out
		if outPath == "" {
			outPath = "_CREDENCIAIS-ENCONTRADAS.txt"
		}
		os.WriteFile(outPath, []byte(textoResumo(achados)), 0o644)
	}

	return exitCode(achados), nil
}

// coletarFontes junta arquivos/pastas, stdin e URLs numa lista de fontes.
// Falha ao buscar uma URL vira aviso, não erro fatal (as outras seguem).
func coletarFontes(cfg Config, stdin io.Reader, headers http.Header, stderr io.Writer) ([]fonte, error) {
	var fontes []fonte
	if len(cfg.Paths) > 0 {
		fs, err := lerArquivosDe(cfg.Paths)
		if err != nil {
			return nil, err
		}
		fontes = append(fontes, fs...)
	}
	if cfg.Stdin {
		f, err := lerStdin(stdin)
		if err != nil {
			return nil, err
		}
		fontes = append(fontes, f)
	}
	if len(cfg.URLs) > 0 {
		client := clienteHTTP(cfg.Insecure)
		for _, u := range cfg.URLs {
			f, err := fetchURL(client, u, headers, cfg.UserAgent)
			if err != nil {
				fmt.Fprintf(stderr, "aviso: falha ao buscar %s: %v\n", u, err)
				continue
			}
			fontes = append(fontes, f)
		}
	}
	return fontes, nil
}

// soSecretos filtra apenas os achados da classe secreto.
func soSecretos(achados []achado) []achado {
	var out []achado
	for _, a := range achados {
		if a.Classe == classeSecreto {
			out = append(out, a)
		}
	}
	return out
}

// clienteHTTP monta o cliente usado para buscar URLs.
func clienteHTTP(insecure bool) *http.Client {
	tr := &http.Transport{}
	if insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &http.Client{Transport: tr, Timeout: 30 * time.Second}
}

func banner(w io.Writer, cfg Config, nFontes int, logPath string) {
	fmt.Fprintf(w, "spyder-credentials: %d fonte(s) varrida(s)\n", nFontes)
	fmt.Fprintf(w, "  manifesto: %s\n", logPath)
	if cfg.ShowFull {
		fmt.Fprintln(w, "  ATENÇÃO: --show-full liga a exibição dos segredos em claro")
	}
	if cfg.Entropy {
		fmt.Fprintln(w, "  detector de alta entropia: LIGADO (pode gerar ruído)")
	}
}

const uso = `spyder-credentials - varre conteúdo e reporta credenciais expostas (uso autorizado)

uso:
  spyder-credentials [opções] <caminho...>
  spyder-credentials --url=<url> [--url=...] [opções]
  cat arquivo | spyder-credentials --stdin

exemplos:
  download-dump --url=https://meusite.com/ --out=dump && spyder-credentials dump/
  spyder-credentials --url=https://meusite.com/app.js --header="Cookie: sid=..."

entrada (ao menos uma):
  <caminho...>       arquivos/pastas a varrer (ex.: o espelho do download-dump)
  --url=URL          URL avulsa a buscar e varrer (repetível; NÃO rastreia o site)
  --stdin            lê o conteúdo da entrada padrão

opções:
  --header="N: V"    header de request para as URLs (Authorization, Cookie...) (repetível)
  --user-agent=UA    User-Agent das buscas
  --insecure         não valida o cert TLS ao buscar URLs
  --entropy          liga o detector genérico de alta entropia (ruidoso)
  --show-full        mostra o valor inteiro dos segredos (sem mascarar)
  --only-secrets     reporta só a classe 'secreto'
  --out=PATH         arquivo-resumo (padrão: _CREDENCIAIS-ENCONTRADAS.txt)
  --log-file=PATH    manifesto JSONL (padrão: spyder-credentials-<ts>.jsonl)
  --quiet            não imprime o relatório no console

saída: classes 'secreto' (nunca deveria vazar), 'publicavel' (público por desenho)
e 'suspeito' (conferir à mão). Exit code 2 se houver algum 'secreto'.
`
